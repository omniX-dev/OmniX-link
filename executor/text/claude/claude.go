package claude

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/text"
	"github.com/just4zeroq/Omni-link/translator"
)

func init() {
	executor.Register("claude", &ClaudeExecutor{})
}

// ClaudeExecutor handles Anthropic Claude endpoints.
type ClaudeExecutor struct {
	channel any
}

func (e *ClaudeExecutor) Init(channel any) {
	e.channel = channel
}

func (e *ClaudeExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Claude"
}

func (e *ClaudeExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatClaude, PathSuffix: "/v1/messages"},
	}
}

func (e *ClaudeExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	suffix := executor.GetPathSuffix(e, info.UpstreamFormat)
	return baseURL + suffix, nil
}

func (e *ClaudeExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	header.Set("x-api-key", info.ApiKey)
	header.Set("anthropic-version", "2023-06-01")
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	}
	return nil
}

func (e *ClaudeExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *ClaudeExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *ClaudeExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	if info.ActualModelName != "" {
		body = executor.ReplaceModelField(body, info.ActualModelName)
	}
	return body
}

func (e *ClaudeExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *ClaudeExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}

	switch {
	case from == translator.FormatClaude && to == translator.FormatOpenAI:
		return executor.NewClaudeToOpenAIStream(), nil
	case from == translator.FormatClaude && to == translator.FormatOpenAIResponses:
		return nil, nil
	case from == translator.FormatOpenAI && to == translator.FormatClaude:
		return executor.NewOpenAIToClaudeStream(), nil
	default:
		return nil, fmt.Errorf("claude: streaming conversion %s→%s not implemented", from, to)
	}
}

func (e *ClaudeExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
	reqURL, err := e.GetRequestURL(info)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if err := e.SetupRequestHeader(httpReq.Header, info); err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}
