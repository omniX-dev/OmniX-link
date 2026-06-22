package minimax

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
	executor.Register("minimax", &MinimaxExecutor{})
}

// MinimaxExecutor handles MiniMax-compatible endpoints.
type MinimaxExecutor struct {
	channel any
}

func (e *MinimaxExecutor) Init(channel any) {
	e.channel = channel
}

func (e *MinimaxExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "MiniMax"
}

func (e *MinimaxExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatOpenAI, PathSuffix: "/v1/chat/completions"},
	}
}

func (e *MinimaxExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://api.minimaxi.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	suffix := executor.GetPathSuffix(e, info.UpstreamFormat)
	return baseURL + suffix, nil
}

func (e *MinimaxExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	} else {
		header.Set("Accept", "application/json")
	}
	return nil
}

func (e *MinimaxExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *MinimaxExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *MinimaxExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	if info.ActualModelName != "" {
		body = executor.ReplaceModelField(body, info.ActualModelName)
	}
	if info.IsStream {
		body = executor.InjectStreamOptionsOpenAI(body)
	}
	body = minimaxInjectThinking(body, info)
	return body
}

func (e *MinimaxExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *MinimaxExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}

	switch {
	case from == translator.FormatOpenAI && to == translator.FormatClaude:
		return executor.NewOpenAIToClaudeStream(), nil
	case from == translator.FormatOpenAI && to == translator.FormatOpenAIResponses:
		return nil, nil
	default:
		return nil, fmt.Errorf("minimax: streaming conversion %s->%s not implemented", from, to)
	}
}

func (e *MinimaxExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
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
// MiniMax-specific customizations
// ========================================================================

// minimaxInjectThinking injects MiniMax's thinking/reasoning configuration.
// MiniMax-M3 supports thinking.type: "adaptive"|"disabled" and reasoning_split.
func minimaxInjectThinking(body []byte, info *executor.RequestInfo) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}

	if info.ThinkingDisabled {
		raw["thinking"] = json.RawMessage(`{"type":"disabled"}`)
	} else if info.ThinkingEnabled || info.ReasoningEffort != "" {
		if _, exists := raw["thinking"]; !exists {
			raw["thinking"] = json.RawMessage(`{"type":"adaptive"}`)
		}
		raw["reasoning_split"] = json.RawMessage(`true`)
	}

	result, _ := json.Marshal(raw)
	return result
}
