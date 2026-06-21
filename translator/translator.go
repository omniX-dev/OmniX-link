// Package translator defines protocol format identifiers, shared data types,
// complete request/response structures, and universal format conversion.
//
// Supported formats:
//   - OpenAI Chat Completions (openai)
//   - Claude Messages (claude)
//   - OpenAI Responses API (openai_responses)
//   - Gemini (gemini)
//
// Convert() is the universal entry point for format conversion.
// Conversion functions live in conv.go, types are in protocol-specific files.
package translator

import (
	"encoding/json"
	"fmt"
)

// Format identifies an LLM API protocol format.
type Format string

const (
	FormatOpenAI          Format = "openai"
	FormatClaude          Format = "claude"
	FormatGemini          Format = "gemini"
	FormatOpenAIResponses Format = "openai_responses"
)

// EndpointPath returns the URL path for the given format.
func (f Format) EndpointPath() string {
	switch f {
	case FormatOpenAI:
		return "/v1/chat/completions"
	case FormatClaude:
		return "/v1/messages"
	case FormatOpenAIResponses:
		return "/v1/responses"
	default:
		return ""
	}
}

// DetectFormatFromPath returns the format from a URL path.
func DetectFormatFromPath(path string) (Format, bool) {
	switch path {
	case "/v1/chat/completions", "/chat/completions":
		return FormatOpenAI, true
	case "/v1/messages", "/messages":
		return FormatClaude, true
	case "/v1/responses", "/responses":
		return FormatOpenAIResponses, true
	}
	return "", false
}

// Detect detects format from path, falling back to body inspection.
// Path takes priority when non-empty. Body detection is used as fallback.
func Detect(body []byte, path string) Format {
	if path != "" {
		if f, ok := DetectFormatFromPath(path); ok {
			return f
		}
	}
	return DetectFormat(body)
}

// Convert transforms a request or response body between protocol formats.
// Supports direct conversion paths without intermediate format.
//
// Supported pairs (both request and response):
//
//	Claude ↔ OpenAI
//	OpenAI ↔ Responses API
//	Claude  ↔ Responses API
//	Gemini  ↔ OpenAI
//	Claude  ↔ Gemini
//	Responses ↔ Gemini
//
// Returns error for unsupported format pairs.
func Convert(body []byte, from, to Format) ([]byte, error) {
	if from == to {
		return body, nil
	}

	// Try hub fallback for unsupported direct pairs:
	// If the direct path isn't implemented, try via OpenAI intermediate.
	result, err := convertDirect(body, from, to)
	if err != nil {
		// Hub fallback: via OpenAI
		mid, hubErr := convertDirect(body, from, FormatOpenAI)
		if hubErr != nil {
			return nil, fmt.Errorf("translator: %s→%s: %w", from, to, err)
		}
		result, hubErr2 := convertDirect(mid, FormatOpenAI, to)
		if hubErr2 != nil {
			return nil, fmt.Errorf("translator: %s->%s: hub via openai: %w", from, to, hubErr2)
		}
		return result, nil
	}
	return result, nil
}

// convertDirect tries a direct format conversion without hub fallback.
func convertDirect(body []byte, from, to Format) ([]byte, error) {
	switch {
	case from == FormatClaude && to == FormatOpenAI:
		return claudeToOpenAI(body)
	case from == FormatOpenAI && to == FormatClaude:
		return openAIToClaude(body)
	case from == FormatOpenAIResponses && to == FormatOpenAI:
		return responsesToOpenAI(body)
	case from == FormatOpenAI && to == FormatOpenAIResponses:
		return openAIToResponses(body)
	case from == FormatClaude && to == FormatOpenAIResponses:
		return claudeToResponses(body)
	case from == FormatOpenAIResponses && to == FormatClaude:
		return responsesToClaude(body)
	case from == FormatOpenAI && to == FormatGemini:
		return openAIToGemini(body)
	case from == FormatGemini && to == FormatOpenAI:
		return geminiToOpenAI(body)
	case from == FormatClaude && to == FormatGemini:
		return claudeToGemini(body)
	case from == FormatGemini && to == FormatClaude:
		return geminiToClaude(body)
	case from == FormatOpenAIResponses && to == FormatGemini:
		return responsesToGemini(body)
	case from == FormatGemini && to == FormatOpenAIResponses:
		return geminiToResponses(body)
	default:
		return nil, fmt.Errorf("unsupported direct conversion %s → %s", from, to)
	}
}

// DetectFormat inspects body to determine which protocol format it uses.
// Accepts raw JSON bytes, unmarshals internally.
func DetectFormat(body []byte) Format {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return FormatOpenAI // default on parse error
	}

	// OpenAI Responses Request: has "input" and no "messages"
	if input, ok := raw["input"]; ok && input != nil {
		switch input.(type) {
		case string, []any:
			if _, hasMsgs := raw["messages"]; !hasMsgs {
				return FormatOpenAIResponses
			}
		}
	}

	// Messages-based formats
	if _, ok := raw["messages"]; ok {
		if _, hasSystem := raw["system"]; hasSystem {
			return FormatClaude
		}
		if _, hasT := raw["temperature"]; hasT {
			return FormatOpenAI
		}
		if model, ok := raw["model"].(string); ok {
			for _, prefix := range []string{"claude-", "anthropic."} {
				if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
					return FormatClaude
				}
			}
		}
		// Claude requires max_tokens; OpenAI doesn't.
		// No temperature + max_tokens present → likely Claude.
		if _, hasT := raw["temperature"]; !hasT {
			if _, hasMT := raw["max_tokens"]; hasMT {
				return FormatClaude
			}
		}
		return FormatOpenAI
	}

	// Gemini request: has "contents" (array) and no "messages"/"input"
	if _, ok := raw["contents"]; ok {
		if _, hasMsgs := raw["messages"]; !hasMsgs {
			if _, hasInput := raw["input"]; !hasInput {
				return FormatGemini
			}
		}
	}

	// Response format detections
	if _, ok := raw["choices"]; ok {
		return FormatOpenAI // OpenAI Chat response
	}
	if _, ok := raw["candidates"]; ok {
		return FormatGemini // Gemini response
	}
	if t, _ := raw["type"].(string); t == "message" {
		if _, ok := raw["content"]; ok {
			return FormatClaude // Claude response
		}
	}
	if obj, _ := raw["object"].(string); obj == "response" {
		return FormatOpenAIResponses // Responses API response
	}

	return FormatOpenAI // default fallback
}

// --- Shared Types ---

// Usage captures basic token usage. Protocol-specific usage fields
// are defined in each protocol's response type.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// MessageRole constants.
const (
	RoleSystem    = "system"
	RoleDeveloper = "developer"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// FinishReason constants.
const (
	FinishReasonStop          = "stop"
	FinishReasonLength        = "length"
	FinishReasonToolCalls     = "tool_calls"
	FinishReasonContentFilter = "content_filter"

	ClaudeFinishEndTurn   = "end_turn"
	ClaudeFinishMaxTokens = "max_tokens"
	ClaudeFinishStopSeq   = "stop_sequence"
	ClaudeFinishToolUse   = "tool_use"
)
