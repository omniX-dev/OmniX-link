package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ResolveUpstreamFormat fills in info.UpstreamFormat from InboundFormat + ClientFormat
// using Plan() with the executor's NativeEndpoints. No-op if UpstreamFormat already set.
func ResolveUpstreamFormat(e Executor, info *RequestInfo) {
	if info.UpstreamFormat != "" {
		return
	}
	info.UpstreamFormat = Plan(info.InboundFormat, info.ClientFormat, e.NativeEndpoints())
}

// Request sends a non-streaming request through the full pipeline.
// UpstreamFormat auto-resolved from InboundFormat + ClientFormat via Plan.
func Request(e Executor, info *RequestInfo, body []byte) ([]byte, error) {
	ResolveUpstreamFormat(e, info)

	up := info.UpstreamFormat
	conv, err := e.ConvertRequest(body, info.InboundFormat, up)
	if err != nil {
		return nil, fmt.Errorf("convert request: %w", err)
	}
	conv = e.RequestCustomize(conv, info)

	resp, err := e.DoRequest(info, bytes.NewReader(conv))
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	respBody = e.ResponseCustomize(respBody, info)
	return e.ConvertResponse(respBody, up, info.ClientFormat)
}

// ExecuteStream runs full streaming pipeline:
// convert → customize → HTTP → read SSE → convert stream → callback.
// UpstreamFormat auto-resolved from InboundFormat + ClientFormat via Plan.
func ExecuteStream(ctx context.Context, e Executor, info *RequestInfo, body []byte, callback func([]byte) error) error {
	if !info.IsStream {
		return fmt.Errorf("ExecuteStream requires IsStream=true")
	}

	ResolveUpstreamFormat(e, info)
	up := info.UpstreamFormat

	// 1. Convert request body.
	conv, err := e.ConvertRequest(body, info.InboundFormat, up)
	if err != nil {
		return fmt.Errorf("convert request: %w", err)
	}

	// 2. Vendor customizations.
	conv = e.RequestCustomize(conv, info)

	// 3. Build upstream URL.
	reqURL, err := e.GetRequestURL(info)
	if err != nil {
		return fmt.Errorf("get url: %w", err)
	}

	// 4. Create HTTP request with context.
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(conv))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if err := e.SetupRequestHeader(httpReq.Header, info); err != nil {
		return fmt.Errorf("setup header: %w", err)
	}

	// 5. Execute.
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upstream error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	// 6. Get stream converter.
	stream, err := e.NewResponseStream(up, info.ClientFormat)
	if err != nil {
		return fmt.Errorf("new response stream: %w", err)
	}

	// 7. Read SSE response body, feed through converter.
	readBuf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(readBuf)
		if n > 0 {
			if stream != nil {
				converted, feedErr := stream.Feed(readBuf[:n])
				if feedErr != nil {
					continue
				}
				if len(converted) > 0 {
					if cbErr := callback(converted); cbErr != nil {
						return cbErr
					}
				}
			} else {
				if cbErr := callback(readBuf[:n]); cbErr != nil {
					return cbErr
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	// 8. Flush trailing data from stream converter.
	if stream != nil {
		trailing, endErr := stream.End()
		if endErr == nil && len(trailing) > 0 {
			callback(trailing)
		}
	}

	return nil
}
