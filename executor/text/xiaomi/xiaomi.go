package xiaomi

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
	executor.Register("xiaomi", &XiaomiExecutor{})
}

// XiaomiExecutor handles Xiaomi MiMo OpenAI-compatible endpoints.
type XiaomiExecutor struct {
	channel any
}

func (e *XiaomiExecutor) Init(channel any) {
	e.channel = channel
}

func (e *XiaomiExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "XiaomiMiMo"
}

func (e *XiaomiExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatOpenAI, PathSuffix: "/v1/chat/completions"},
	}
}

func (e *XiaomiExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://api.xiaomimimo.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	suffix := executor.GetPathSuffix(e, info.UpstreamFormat)
	return baseURL + suffix, nil
}

func (e *XiaomiExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	} else {
		header.Set("Accept", "application/json")
	}
	return nil
}

func (e *XiaomiExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *XiaomiExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *XiaomiExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	if info.ActualModelName != "" {
		body = executor.ReplaceModelField(body, info.ActualModelName)
	}
	if info.IsStream {
		body = executor.InjectStreamOptionsOpenAI(body)
	}
	body = xiaomiInjectThinking(body, info)
	return body
}

func (e *XiaomiExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *XiaomiExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}

	switch {
	case from == translator.FormatOpenAI && to == translator.FormatClaude:
		return executor.NewOpenAIToClaudeStream(), nil
	case from == translator.FormatOpenAI && to == translator.FormatOpenAIResponses:
		return nil, nil
	default:
		return nil, fmt.Errorf("xiaomi: streaming conversion %s->%s not implemented", from, to)
	}
}

func (e *XiaomiExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
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
// Xiaomi MiMo-specific customizations
// ========================================================================

// xiaomiInjectThinking injects MiMo's thinking/reasoning configuration.
// MiMo supports enable_thinking: true|false.
// Multi-turn tool calls must preserve reasoning_content in assistant messages.
func xiaomiInjectThinking(body []byte, info *executor.RequestInfo) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}

	if info.ThinkingDisabled {
		raw["enable_thinking"] = json.RawMessage(`false`)
	} else if info.ThinkingEnabled {
		if _, exists := raw["enable_thinking"]; !exists {
			raw["enable_thinking"] = json.RawMessage(`true`)
		}
	}

	result, _ := json.Marshal(raw)
	return result
}
