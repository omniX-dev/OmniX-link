package deepseek

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
	executor.Register("deepseek", &DeepSeekExecutor{})
}

// DeepSeekExecutor handles DeepSeek API with OpenAI and Claude endpoints.
type DeepSeekExecutor struct {
	channel any
}

func (e *DeepSeekExecutor) Init(channel any) {
	e.channel = channel
}

func (e *DeepSeekExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "DeepSeek"
}

func (e *DeepSeekExecutor) NativeEndpoints() []executor.Endpoint {
	return []executor.Endpoint{
		{Format: translator.FormatOpenAI, PathSuffix: "/v1/chat/completions"},
		{Format: translator.FormatClaude, PathSuffix: "/anthropic/v1/messages"},
	}
}

func (e *DeepSeekExecutor) GetRequestURL(info *executor.RequestInfo) (string, error) {
	baseURL := info.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	suffix := executor.GetPathSuffix(e, info.UpstreamFormat)
	return baseURL + suffix, nil
}

func (e *DeepSeekExecutor) SetupRequestHeader(header http.Header, info *executor.RequestInfo) error {
	switch info.UpstreamFormat {
	case translator.FormatClaude:
		header.Set("x-api-key", info.ApiKey)
		header.Set("anthropic-version", "2023-06-01")
	default:
		header.Set("Authorization", "Bearer "+info.ApiKey)
	}
	header.Set("Content-Type", "application/json")
	if info.IsStream {
		header.Set("Accept", "text/event-stream")
	} else {
		header.Set("Accept", "application/json")
	}
	return nil
}

func (e *DeepSeekExecutor) ConvertRequest(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *DeepSeekExecutor) ConvertResponse(body []byte, from, to translator.Format) ([]byte, error) {
	if from == to {
		return body, nil
	}
	return translator.Convert(body, from, to)
}

func (e *DeepSeekExecutor) RequestCustomize(body []byte, info *executor.RequestInfo) []byte {
	if info.ActualModelName != "" {
		body = executor.ReplaceModelField(body, info.ActualModelName)
	}
	if info.IsStream && info.UpstreamFormat == translator.FormatOpenAI {
		body = executor.InjectStreamOptionsOpenAI(body)
	}
	body = dsInjectThinking(body, info)
	return body
}

func (e *DeepSeekExecutor) ResponseCustomize(body []byte, info *executor.RequestInfo) []byte {
	return body
}

func (e *DeepSeekExecutor) NewResponseStream(from, to translator.Format) (executor.ResponseStream, error) {
	if from == to {
		return nil, nil
	}

	switch {
	case from == translator.FormatClaude && to == translator.FormatOpenAI:
		return executor.NewClaudeToOpenAIStream(), nil
	case from == translator.FormatOpenAI && to == translator.FormatClaude:
		return executor.NewOpenAIToClaudeStream(), nil
	default:
		return nil, fmt.Errorf("deepseek: streaming conversion %s→%s not implemented", from, to)
	}
}

func (e *DeepSeekExecutor) DoRequest(info *executor.RequestInfo, body io.Reader) (*http.Response, error) {
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
// DeepSeek-specific customizations
// ========================================================================

// dsInjectThinking injects DeepSeek's thinking/reasoning configuration.
func dsInjectThinking(body []byte, info *executor.RequestInfo) []byte {
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
			if info.ReasoningEffort != "" {
				raw["reasoning_effort"], _ = json.Marshal(dsMapEffort(info.ReasoningEffort))
			}
		}
	}

	result, _ := json.Marshal(raw)
	return result
}

// dsMapEffort maps standard reasoning effort to DeepSeek's effort levels.
func dsMapEffort(effort string) string {
	switch effort {
	case "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh", "max":
		return "max"
	default:
		return "high"
	}
}
