package zhipu

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/text"
	"github.com/just4zeroq/Omni-link/translator"
)

func init() {
	executor.Register("zhipu", &ZhipuExecutor{})
}

// ZhipuExecutor handles GLM (Zhipu AI) OpenAI-compatible endpoints.
type ZhipuExecutor struct {
	channel any
}

func (e *ZhipuExecutor) Init(channel any) {
	e.channel = channel
}

func (e *ZhipuExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Zhipu"
}

func (e *ZhipuExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatOpenAI, PathSuffix: "/v1/chat/completions"},
	}
}

func (e *ZhipuExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	suffix := executor.GetPathSuffix(e, info.UpstreamFormat)
	return baseURL + suffix, nil
}

func (e *ZhipuExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	} else {
		header.Set("Accept", "application/json")
	}
	return nil
}

func (e *ZhipuExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *ZhipuExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *ZhipuExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	if info.ActualModelName != "" {
		body = executor.ReplaceModelField(body, info.ActualModelName)
	}
	if info.IsStream {
		body = executor.InjectStreamOptionsOpenAI(body)
	}
	body = zhipuInjectThinking(body, info)
	return body
}

func (e *ZhipuExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *ZhipuExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}

	switch {
	case from == translator.FormatOpenAI && to == translator.FormatClaude:
		return executor.NewOpenAIToClaudeStream(), nil
	case from == translator.FormatOpenAI && to == translator.FormatOpenAIResponses:
		return nil, nil
	default:
		return nil, fmt.Errorf("zhipu: streaming conversion %s→%s not implemented", from, to)
	}
}

func (e *ZhipuExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
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

// ========================================================================
// Zhipu-specific customizations
// ========================================================================

// zhipuInjectThinking injects GLM's thinking/reasoning configuration.
// GLM-4.5+ supports thinking.type: "enabled"|"disabled".
// GLM-5.2+ supports reasoning_effort.
func zhipuInjectThinking(body []byte, info *executor.RequestInfo) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}

	if info.ThinkingDisabled {
		raw["thinking"] = json.RawMessage(`{"type":"disabled"}`)
		delete(raw, "reasoning_effort")
	} else if info.ThinkingEnabled || info.ReasoningEffort != "" {
		if _, exists := raw["thinking"]; !exists {
			raw["thinking"] = json.RawMessage(`{"type":"enabled"}`)
		}
		if info.ReasoningEffort != "" {
			raw["reasoning_effort"], _ = json.Marshal(info.ReasoningEffort)
		}
	}

	result, _ := json.Marshal(raw)
	return result
}
