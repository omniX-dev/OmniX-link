package volcengine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/just4zeroq/Omni-link/executor/text"
	"github.com/just4zeroq/Omni-link/translator"
)

func init() {
	executor.Register("volcengine", &VolcengineExecutor{})
}

// VolcengineExecutor handles 火山引擎 (ByteDance/Doubao) API endpoints.
type VolcengineExecutor struct {
	channel any
}

func (e *VolcengineExecutor) Init(channel any) {
	e.channel = channel
}

func (e *VolcengineExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Volcengine"
}

func (e *VolcengineExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatOpenAI, PathSuffix: "/api/v3/chat/completions"},
		{Format: translator.FormatOpenAIResponses, PathSuffix: "/api/v3/responses"},
	}
}

func (e *VolcengineExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	model := info.ActualModelName
	if model == "" {
		model = info.Model
	}

	if strings.HasPrefix(model, "bot-") {
		return baseURL + "/api/v3/bots/chat/completions", nil
	}
	suffix := executor.GetPathSuffix(e, info.UpstreamFormat)
	return baseURL + suffix, nil
}

func (e *VolcengineExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	} else {
		header.Set("Accept", "application/json")
	}
	return nil
}

func (e *VolcengineExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *VolcengineExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *VolcengineExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	if info.ActualModelName != "" {
		body = executor.ReplaceModelField(body, info.ActualModelName)
	} else if info.Model != "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err == nil {
			if _, exists := raw["model"]; !exists {
				raw["model"], _ = json.Marshal(info.Model)
				body, _ = json.Marshal(raw)
			}
		}
	}
	if info.IsStream && info.UpstreamFormat == translator.FormatOpenAI {
		body = executor.InjectStreamOptionsOpenAI(body)
	}
	return body
}

func (e *VolcengineExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *VolcengineExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}
	return nil, nil // passthrough — volcengine responses are already OpenAI-compatible
}

func (e *VolcengineExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
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
