package gemini

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/just4zeroq/Omni-link/executor"
	"github.com/just4zeroq/Omni-link/translator"
)

func init() {
	executor.Register("gemini", &GeminiExecutor{})
}

// GeminiExecutor handles Google Gemini API endpoints.
type GeminiExecutor struct {
	channel any
}

func (e *GeminiExecutor) Init(channel any) {
	e.channel = channel
}

func (e *GeminiExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Gemini"
}

func (e *GeminiExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatGemini, PathSuffix: "/v1beta/models/{model}:generateContent"},
	}
}

func (e *GeminiExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	model := info.ActualModelName
	if model == "" {
		model = info.Model
	}

	if info.IsStream {
		return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", baseURL, model), nil
	}
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, model), nil
}

func (e *GeminiExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	header.Set("x-goog-api-key", info.ApiKey)
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	} else {
		header.Set("Accept", "application/json")
	}
	return nil
}

func (e *GeminiExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *GeminiExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *GeminiExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *GeminiExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *GeminiExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}
	return nil, nil // streaming conversion not yet implemented for gemini
}

func (e *GeminiExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
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

	client := &http.Client{Timeout: 120 * time.Second}
	return client.Do(httpReq)
}
