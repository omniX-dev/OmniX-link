package executor

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/just4zeroq/Omni-link/translator"
)

// ========================================================================
// SSE formatting helpers
// ========================================================================

func formatOpenAIChunk(data map[string]any) []byte {
	b, _ := json.Marshal(data)
	return []byte("data: " + string(b) + "\n\n")
}

func formatClaudeEvent(event string, data map[string]any) []byte {
	b, _ := json.Marshal(data)
	return []byte("event: " + event + "\ndata: " + string(b) + "\n\n")
}

func parseSSEEventType(event []byte) string {
	for _, line := range bytes.Split(event, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("event: ")) {
			return string(bytes.TrimPrefix(line, []byte("event: ")))
		}
	}
	return ""
}

func parseSSEDataField(event []byte) []byte {
	for _, line := range bytes.Split(event, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data: ")) {
			return bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))
		}
	}
	return nil
}

func mapStreamFinishReason(finish string) string {
	switch finish {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return finish
	}
}

func mapFinishReasonToClaude(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}

func randHex(n int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hexChars[int(streamRandState%16)]
		streamRandState = streamRandState*6364136223846793005 + 1442695040888963407
	}
	return string(b)
}

var streamRandState uint64 = 1442695040888963407

// ========================================================================
// ClaudeToOpenAIStream — Claude SSE → OpenAI Chat SSE
// ========================================================================

// ClaudeToOpenAIStream converts Claude SSE events to OpenAI Chat format SSE.
type ClaudeToOpenAIStream struct {
	buf       bytes.Buffer
	blockIdx  int
	msgID     string
	model     string
	usage     *translator.Usage
	finished  bool
}

// NewClaudeToOpenAIStream creates a new Claude→OpenAI stream converter.
func NewClaudeToOpenAIStream() *ClaudeToOpenAIStream {
	return &ClaudeToOpenAIStream{blockIdx: -1}
}

func (s *ClaudeToOpenAIStream) Feed(chunk []byte) ([]byte, error) {
	s.buf.Write(chunk)
	if !bytes.Contains(s.buf.Bytes(), []byte("\n\n")) {
		return nil, nil
	}

	data := s.buf.Bytes()
	s.buf.Reset()

	var out []byte
	for {
		idx := bytes.Index(data, []byte("\n\n"))
		if idx < 0 {
			s.buf.Write(data)
			break
		}

		event := data[:idx]
		data = data[idx+2:]

		converted, err := s.convertEvent(event)
		if err != nil {
			continue
		}
		if len(converted) > 0 {
			out = append(out, converted...)
		}
	}

	return out, nil
}

func (s *ClaudeToOpenAIStream) End() ([]byte, error) {
	if s.finished {
		return nil, nil
	}
	s.finished = true

	if s.buf.Len() > 0 {
		converted, _ := s.convertEvent(s.buf.Bytes())
		s.buf.Reset()
		if len(converted) > 0 {
			return append(converted, []byte("data: [DONE]\n\n")...), nil
		}
	}
	return []byte("data: [DONE]\n\n"), nil
}

func (s *ClaudeToOpenAIStream) Usage() *translator.Usage {
	return s.usage
}

func (s *ClaudeToOpenAIStream) convertEvent(event []byte) ([]byte, error) {
	eventType := parseSSEEventType(event)
	if eventType == "" {
		return nil, fmt.Errorf("no event type")
	}

	dataField := parseSSEDataField(event)
	if dataField == nil && eventType != "message_stop" {
		return nil, fmt.Errorf("no data field")
	}

	switch eventType {
	case "message_start":
		return s.onMessageStart(dataField)
	case "content_block_start":
		return s.onContentBlockStart(dataField)
	case "content_block_delta":
		return s.onContentBlockDelta(dataField)
	case "content_block_stop":
		return s.onContentBlockStop()
	case "message_delta":
		return s.onMessageDelta(dataField)
	case "message_stop":
		return s.onMessageStop()
	case "ping":
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *ClaudeToOpenAIStream) onMessageStart(data []byte) ([]byte, error) {
	var msg struct {
		Type    string `json:"type"`
		Message struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Role  string `json:"role"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	s.msgID = msg.Message.ID
	s.model = msg.Message.Model
	return nil, nil
}

func (s *ClaudeToOpenAIStream) onContentBlockStart(data []byte) ([]byte, error) {
	var block struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			Name string `json:"name,omitempty"`
			ID   string `json:"id,omitempty"`
		} `json:"content_block"`
	}
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, err
	}

	s.blockIdx = block.Index
	switch block.ContentBlock.Type {
	case "text":
		return formatOpenAIChunk(map[string]any{
			"choices": []map[string]any{{
				"index": block.Index,
				"delta": map[string]any{"role": "assistant", "content": ""},
			}},
		}), nil
	case "tool_use":
		return formatOpenAIChunk(map[string]any{
			"choices": []map[string]any{{
				"index": block.Index,
				"delta": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"index": block.Index,
						"id":    block.ContentBlock.ID,
						"type":  "function",
						"function": map[string]any{"name": block.ContentBlock.Name, "arguments": ""},
					}},
				},
			}},
		}), nil
	default:
		return nil, nil
	}
}

func (s *ClaudeToOpenAIStream) onContentBlockDelta(data []byte) ([]byte, error) {
	var delta struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type    string `json:"type"`
			Text    string `json:"text,omitempty"`
			Partial string `json:"partial_json,omitempty"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(data, &delta); err != nil {
		return nil, err
	}

	switch delta.Delta.Type {
	case "text_delta":
		return formatOpenAIChunk(map[string]any{
			"choices": []map[string]any{{
				"index": delta.Index,
				"delta": map[string]any{"content": delta.Delta.Text},
			}},
		}), nil
	case "input_json_delta":
		return formatOpenAIChunk(map[string]any{
			"choices": []map[string]any{{
				"index": delta.Index,
				"delta": map[string]any{
					"tool_calls": []map[string]any{{
						"index": delta.Index,
						"function": map[string]any{"arguments": delta.Delta.Partial},
					}},
				},
			}},
		}), nil
	default:
		return nil, nil
	}
}

func (s *ClaudeToOpenAIStream) onContentBlockStop() ([]byte, error) {
	return nil, nil
}

func (s *ClaudeToOpenAIStream) onMessageDelta(data []byte) ([]byte, error) {
	var msg struct {
		Type  string `json:"type"`
		Delta struct {
			StopReason   string `json:"stop_reason"`
			StopSequence string `json:"stop_sequence"`
		} `json:"delta"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	if msg.Usage.OutputTokens > 0 || msg.Usage.InputTokens > 0 {
		s.usage = &translator.Usage{
			PromptTokens:     msg.Usage.InputTokens,
			CompletionTokens: msg.Usage.OutputTokens,
		}
	}

	return formatOpenAIChunk(map[string]any{
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": mapStreamFinishReason(msg.Delta.StopReason),
		}},
	}), nil
}

func (s *ClaudeToOpenAIStream) onMessageStop() ([]byte, error) {
	return nil, nil
}

// ========================================================================
// OpenAIToClaudeStream — OpenAI Chat SSE → Claude SSE
// ========================================================================

// OpenAIToClaudeStream converts OpenAI Chat format SSE to Claude SSE events.
type OpenAIToClaudeStream struct {
	buf        bytes.Buffer
	blockIdx   int
	hasStarted bool
	hasContent bool
	usage      *translator.Usage
	finished   bool
}

// NewOpenAIToClaudeStream creates a new OpenAI→Claude stream converter.
func NewOpenAIToClaudeStream() *OpenAIToClaudeStream {
	return &OpenAIToClaudeStream{}
}

func (s *OpenAIToClaudeStream) Feed(chunk []byte) ([]byte, error) {
	s.buf.Write(chunk)
	if !bytes.Contains(s.buf.Bytes(), []byte("\n\n")) {
		return nil, nil
	}

	data := s.buf.Bytes()
	s.buf.Reset()

	var out []byte
	for {
		idx := bytes.Index(data, []byte("\n\n"))
		if idx < 0 {
			s.buf.Write(data)
			break
		}

		event := data[:idx]
		data = data[idx+2:]

		converted, err := s.convertChunk(event)
		if err != nil {
			continue
		}
		if len(converted) > 0 {
			out = append(out, converted...)
		}
	}

	return out, nil
}

func (s *OpenAIToClaudeStream) End() ([]byte, error) {
	if s.finished {
		return nil, nil
	}
	s.finished = true

	if s.buf.Len() > 0 {
		s.convertChunk(s.buf.Bytes())
		s.buf.Reset()
	}

	var out []byte
	if s.hasContent || s.hasStarted {
		out = append(out, formatClaudeEvent("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": s.blockIdx,
		})...)
	}

	out = append(out, formatClaudeEvent("message_stop", map[string]any{
		"type": "message_stop",
	})...)

	return out, nil
}

func (s *OpenAIToClaudeStream) Usage() *translator.Usage {
	return s.usage
}

func (s *OpenAIToClaudeStream) convertChunk(line []byte) ([]byte, error) {
	if !bytes.HasPrefix(line, []byte("data: ")) {
		return nil, nil
	}
	data := bytes.TrimPrefix(line, []byte("data: "))
	data = bytes.TrimSpace(data)

	if string(data) == "[DONE]" {
		return nil, nil
	}

	var chunk struct {
		Choices []struct {
			Index        int    `json:"index"`
			Delta        struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, err
	}

	if chunk.Usage != nil {
		s.usage = &translator.Usage{
			PromptTokens:     chunk.Usage.PromptTokens,
			CompletionTokens: chunk.Usage.CompletionTokens,
		}
	}

	if len(chunk.Choices) == 0 {
		return nil, nil
	}

	choice := chunk.Choices[0]
	var out []byte

	if !s.hasStarted && choice.Delta.Role == "assistant" {
		s.hasStarted = true
		out = append(out, formatClaudeEvent("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_" + randHex(8), "type": "message",
				"role": "assistant", "content": []any{}, "model": "",
			},
		})...)
	}

	if choice.Delta.Content != "" && !s.hasContent {
		s.hasContent = true
		s.blockIdx++
		out = append(out, formatClaudeEvent("content_block_start", map[string]any{
			"type": "content_block_start", "index": s.blockIdx,
			"content_block": map[string]any{"type": "text", "text": ""},
		})...)
		out = append(out, formatClaudeEvent("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": s.blockIdx,
			"delta": map[string]any{"type": "text_delta", "text": choice.Delta.Content},
		})...)
		return out, nil
	}

	if choice.Delta.Content != "" {
		out = append(out, formatClaudeEvent("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": s.blockIdx,
			"delta": map[string]any{"type": "text_delta", "text": choice.Delta.Content},
		})...)
	}

	for _, tc := range choice.Delta.ToolCalls {
		if tc.ID != "" {
			s.blockIdx++
			out = append(out, formatClaudeEvent("content_block_start", map[string]any{
				"type": "content_block_start", "index": s.blockIdx,
				"content_block": map[string]any{
					"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": map[string]any{},
				},
			})...)
		}
		if tc.Function.Arguments != "" {
			out = append(out, formatClaudeEvent("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": s.blockIdx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Function.Arguments},
			})...)
		}
	}

	if choice.FinishReason != "" {
		out = append(out, formatClaudeEvent("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": s.blockIdx,
		})...)
		out = append(out, formatClaudeEvent("message_delta", map[string]any{
			"type": "message_delta",
			"delta": map[string]any{"stop_reason": mapFinishReasonToClaude(choice.FinishReason), "stop_sequence": nil},
		})...)
	}

	return out, nil
}
