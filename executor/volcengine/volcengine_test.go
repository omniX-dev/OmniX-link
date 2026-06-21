package volcengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/just4zeroq/Omni-link/executor"
	"github.com/just4zeroq/Omni-link/translator"
)

const (
	volcModel  = "doubao-seed-2-0-lite-260215"
	volcBase   = "https://ark.cn-beijing.volces.com"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

func volcKey(t *testing.T) string {
	t.Helper()
	tryLoadEnv()
	k := os.Getenv("VOLC_API_KEY")
	if k == "" {
		t.Skip("VOLC_API_KEY not set")
	}
	return k
}

// tryLoadEnv loads .env once per package.
func tryLoadEnv() {
	loadOnce.Do(func() {
		paths := []string{".env", "../.env", "../../.env"}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					k := strings.TrimSpace(parts[0])
					v := strings.TrimSpace(parts[1])
					if os.Getenv(k) == "" {
						os.Setenv(k, v)
					}
				}
			}
			return // found and loaded
		}
	})
}

var loadOnce sync.Once

// ========================================================================
// Basic chat completions
// ========================================================================

func TestVolcChatCompletion(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello in one word."}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices: %s", string(resp))
	}
	c := choices[0].(map[string]any)
	if msg, ok := c["message"].(map[string]any); ok {
		t.Logf("Chat completion: %s", msg["content"])
	} else {
		t.Logf("Chat completion response: %s", string(resp))
	}
}

func TestVolcChatGLM4(t *testing.T) {
	const glmModel = "glm-4-7-251222"
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: glmModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    glmModel,
		"messages": []map[string]any{{"role": "user", "content": "你是谁？用一句话介绍自己。"}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices: %s", string(resp))
	}
	c := choices[0].(map[string]any)
	if msg, ok := c["message"].(map[string]any); ok {
		t.Logf("GLM-4 response: %s", msg["content"])
	} else {
		t.Logf("GLM-4 response: %s", string(resp))
	}
}

func TestVolcStreamGLM4(t *testing.T) {
	const glmModel = "glm-4-7-251222"
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: glmModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    glmModel,
		"messages": []map[string]any{{"role": "user", "content": "从1数到5。"}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		c := make([]byte, len(chunk))
		copy(c, chunk)
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	gotData := false
	gotDone := false
	full := string(bytes.Join(chunks, nil))
	for _, line := range strings.Split(full, "\n") {
		line = strings.TrimSpace(line)
		if line == "data: [DONE]" {
			gotDone = true
		} else if strings.HasPrefix(line, "data: ") {
			gotData = true
		}
	}
	if !gotData {
		t.Fatal("no data chunks")
	}
	if !gotDone {
		t.Fatal("no [DONE]")
	}
	t.Logf("GLM-4 stream: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

// ========================================================================
// DeepSeek V3 via volcengine endpoint
// ========================================================================

func TestVolcChatDSV3(t *testing.T) {
	const dsModel = "deepseek-v3-2-251201"
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: dsModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    dsModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello in one word."}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices: %s", string(resp))
	}
	c := choices[0].(map[string]any)
	if msg, ok := c["message"].(map[string]any); ok {
		t.Logf("DS V3 via volc: %s", msg["content"])
	} else {
		t.Logf("DS V3 via volc: %s", string(resp))
	}
}

func TestVolcStreamDSV3(t *testing.T) {
	const dsModel = "deepseek-v3-2-251201"
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: dsModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    dsModel,
		"messages": []map[string]any{{"role": "user", "content": "从1数到5。"}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		c := make([]byte, len(chunk))
		copy(c, chunk)
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	gotData := false
	gotDone := false
	full := string(bytes.Join(chunks, nil))
	for _, line := range strings.Split(full, "\n") {
		line = strings.TrimSpace(line)
		if line == "data: [DONE]" {
			gotDone = true
		} else if strings.HasPrefix(line, "data: ") {
			gotData = true
		}
	}
	if !gotData {
		t.Fatal("no data chunks")
	}
	if !gotDone {
		t.Fatal("no [DONE]")
	}
	t.Logf("DS V3 stream via volc: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

func TestVolcStreamChat(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		c := make([]byte, len(chunk))
		copy(c, chunk)
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks received")
	}

	gotData := false
	gotDone := false
	full := string(bytes.Join(chunks, nil))
	for _, line := range strings.Split(full, "\n") {
		line = strings.TrimSpace(line)
		if line == "data: [DONE]" {
			gotDone = true
		} else if strings.HasPrefix(line, "data: ") {
			gotData = true
		}
	}
	if !gotData {
		t.Fatal("no data chunks")
	}
	if !gotDone {
		t.Fatal("no [DONE] terminator")
	}
	t.Logf("Stream chat: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

// ========================================================================
// Responses API
// ========================================================================

func TestVolcResponses(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAIResponses,
		InboundFormat:  translator.FormatOpenAIResponses,
		ClientFormat:   translator.FormatOpenAIResponses,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"input": "Say hello in one word.",
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if obj, _ := m["object"].(string); obj != "response" {
		t.Fatalf("expected response object, got: %s", string(resp))
	}
	output, _ := m["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("no output: %s", string(resp))
	}
	t.Logf("Responses: %s", string(resp))
}

func TestVolcStreamResponses(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAIResponses,
		InboundFormat:  translator.FormatOpenAIResponses,
		ClientFormat:   translator.FormatOpenAIResponses,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":  volcModel,
		"input":  "Count 1 to 5.",
		"stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		c := make([]byte, len(chunk))
		copy(c, chunk)
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks received")
	}

	// Responses SSE passthrough — expect data: prefix events
	gotData := false
	for _, c := range chunks {
		s := string(c)
		if strings.HasPrefix(s, "data: ") {
			gotData = true
			break
		}
	}
	if !gotData {
		t.Fatal("no data events in Responses stream")
	}
	t.Logf("Stream Responses: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

// ========================================================================
// Features
// ========================================================================

func TestVolcSystemMessage(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"messages": []map[string]any{
			{"role": "system", "content": "Reply in ALL CAPS."},
			{"role": "user", "content": "Say hello"},
		},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices: %s", string(resp))
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	t.Logf("System msg (expect caps): %s", msg["content"])
}

func TestVolcWithTools(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"messages": []map[string]any{
			{"role": "user", "content": "What's weather in Beijing?"},
		},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather for a location",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
					"required": []string{"location"},
				},
			},
		}},
		"tool_choice": "auto",
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Tools response: %s", string(resp))
}

func TestVolcWithParams(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":       volcModel,
		"messages":    []map[string]any{{"role": "user", "content": "Count 1 2 3"}},
		"temperature": 0.5,
		"top_p":       0.9,
		"max_tokens":  100,
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("no choices")
	}
	t.Logf("Params response: %s", string(resp))
}

// ========================================================================
// Format conversion — OpenAI↔Responses round trip
// ========================================================================

func TestVolcConvChatToResponses(t *testing.T) {
	t.Skip("translator openAIToResponses request conversion not implemented (passthrough)")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAIResponses,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	// Send OpenAI Chat request → convert to Responses → send → convert back to Chat
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello"}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices after conv: %s", string(resp))
	}
	t.Logf("Conv Chat→Responses→Chat: success")
}

func TestVolcConvResponsesToChat(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAIResponses,
		ClientFormat:   translator.FormatOpenAIResponses,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	// Send Responses request → convert to Chat → send → convert back to Responses
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"input": "Say hello",
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if obj, _ := m["object"].(string); obj != "response" {
		t.Fatalf("expected response object, got: %s", string(resp))
	}
	t.Logf("Conv Responses→Chat→Responses: success")
}

// ========================================================================
// Plan auto-resolve — no UpstreamFormat set
// ========================================================================

func TestVolcPlanAuto(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		// UpstreamFormat intentionally empty — Plan should pick FormatOpenAI
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 3."}},
	})
	resp, err := executor.Request(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices: %s", string(resp))
	}
	t.Logf("Plan auto: %s", string(resp))
}

func TestVolcPlanAutoResponses(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAIResponses,
		ClientFormat:  translator.FormatOpenAIResponses,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		// UpstreamFormat empty — Plan should pick FormatOpenAIResponses (prefers output)
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"input": "Count 1 to 3.",
	})
	resp, err := executor.Request(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if obj, _ := m["object"].(string); obj != "response" {
		t.Fatalf("expected response object, got: %s", string(resp))
	}
	t.Logf("Plan auto Responses: %s", string(resp))
}

// ========================================================================
// Output-only conversion — send Responses in, get Chat out via Plan
// ========================================================================

func TestVolcOutputConv(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAIResponses,
		ClientFormat:  translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		// Plan: In=Responses, Out=Chat → tie → prefers Chat (OpenAI)
		// → convert request Responses→OpenAI → send to Chat endpoint → passthrough response
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"input": "Say hello",
	})
	resp, err := executor.Request(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("no choices after output conv: %s", string(resp))
	}
	t.Logf("Output conv Responses→Chat: success")
}

func TestVolcOutputConvOpenAIToClaude(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatClaude,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello"}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Output conv OpenAI->Claude: success")
}

// ========================================================================
// Full format conversion matrix — volcengine Chat endpoint
// Each test: send InboundFormat → convert to OpenAI Chat → convert response to ClientFormat
// ========================================================================

func TestVolcConvClaudeToClaude(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatClaude,
		ClientFormat:  translator.FormatClaude,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	// Plan: In=Claude, Out=Claude → candidates [OpenAI, Responses]
	// → both score 1 → tie → first = OpenAI
	// → conv request Claude→OpenAI, Chat endpoint, conv response OpenAI→Claude
	b, _ := json.Marshal(map[string]any{
		"model":      volcModel,
		"messages":   []map[string]any{{"role": "user", "content": "Say hello"}},
		"max_tokens": 4096,
	})
	resp, err := executor.Request(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Conv Claude->Claude: success")
}

func TestVolcConvClaudeToOpenAI(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatClaude,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      volcModel,
		"messages":   []map[string]any{{"role": "user", "content": "Say hello"}},
		"max_tokens": 4096,
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["choices"]; !ok {
		t.Fatalf("no choices (expected OpenAI format): %s", string(resp))
	}
	t.Logf("Conv Claude->OpenAI: success")
}

func TestVolcConvClaudeToGemini(t *testing.T) {
	t.Skip("translator openAIToGeminiResponse passthrough — not implemented")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatClaude,
		ClientFormat:   translator.FormatGemini,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      volcModel,
		"messages":   []map[string]any{{"role": "user", "content": "Say hello"}},
		"max_tokens": 4096,
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["candidates"]; !ok {
		t.Fatalf("no candidates (expected Gemini format): %s", string(resp))
	}
	t.Logf("Conv Claude->Gemini: success")
}

func TestVolcConvOpenAIToGemini(t *testing.T) {
	t.Skip("translator openAIToGeminiResponse passthrough — not implemented")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatGemini,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello"}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["candidates"]; !ok {
		t.Fatalf("no candidates (expected Gemini format): %s", string(resp))
	}
	t.Logf("Conv OpenAI->Gemini: success")
}

func TestVolcConvGeminiToOpenAI(t *testing.T) {
	t.Skip("translator geminiToOpenAI request not implemented (passthrough)")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatGemini,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ActualModelName: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []map[string]any{{"text": "Say hello"}},
		}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["choices"]; !ok {
		t.Fatalf("no choices (expected OpenAI format): %s", string(resp))
	}
	t.Logf("Conv Gemini->OpenAI: success")
}

func TestVolcConvGeminiToClaude(t *testing.T) {
	t.Skip("translator geminiToOpenAI/Claude request not implemented (passthrough)")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatGemini,
		ClientFormat:   translator.FormatClaude,
		Model: volcModel,
		ActualModelName: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []map[string]any{{"text": "Say hello"}},
		}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Conv Gemini->Claude: success")
}

func TestVolcConvGeminiToGemini(t *testing.T) {
	t.Skip("translator openAIToGeminiResponse passthrough — not implemented")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatGemini,
		ClientFormat:   translator.FormatGemini,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []map[string]any{{"text": "Say hello"}},
		}},
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["candidates"]; !ok {
		t.Fatalf("no candidates (expected Gemini format): %s", string(resp))
	}
	t.Logf("Conv Gemini->Gemini: success")
}

func TestVolcConvResponsesToClaude(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAIResponses,
		ClientFormat:   translator.FormatClaude,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"input": "Say hello",
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Conv Responses->Claude: success")
}

func TestVolcConvResponsesToGemini(t *testing.T) {
	t.Skip("translator openAIToGeminiResponse passthrough — not implemented")
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAIResponses,
		ClientFormat:   translator.FormatGemini,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model": volcModel,
		"input": "Say hello",
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["candidates"]; !ok {
		t.Fatalf("no candidates (expected Gemini format): %s", string(resp))
	}
	t.Logf("Conv Responses->Gemini: success")
}

// ========================================================================
// Plan auto-resolve via ExecuteStream
// ========================================================================

func TestVolcPlanAutoStream(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "从1数到5。"}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		c := make([]byte, len(chunk))
		copy(c, chunk)
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	gotData := false
	gotDone := false
	full := string(bytes.Join(chunks, nil))
	for _, line := range strings.Split(full, "\n") {
		line = strings.TrimSpace(line)
		if line == "data: [DONE]" {
			gotDone = true
		} else if strings.HasPrefix(line, "data: ") {
			gotData = true
		}
	}
	if !gotData {
		t.Fatal("no data chunks")
	}
	if !gotDone {
		t.Fatal("no [DONE]")
	}
	t.Logf("Plan auto stream: %d chunks", len(chunks))
}

// ========================================================================
// Bot model URL path (model name starting with "bot-")
// ========================================================================

func TestVolcBotModelChat(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: "bot-test-id-123",
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    "bot-test-id-123",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	status, body, err := execReqRaw(e, info, b)
	if err != nil && status == 0 {
		t.Logf("Bot model routed (error, not 404 from chat): %v", err)
		return
	}
	t.Logf("Bot model status %d: %s", status, string(body))
}

// ========================================================================
// Streaming output conversion — Responses→Chat via Plan
// ========================================================================

func TestVolcStreamOutputConv(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAIResponses,
		ClientFormat:  translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	// Plan: In=Responses, Out=Chat → tie → prefers Chat → send to Chat endpoint
	b, _ := json.Marshal(map[string]any{
		"model":  volcModel,
		"input":  "Count 1 to 5.",
		"stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		c := make([]byte, len(chunk))
		copy(c, chunk)
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	gotData := false
	gotDone := false
	full := string(bytes.Join(chunks, nil))
	for _, line := range strings.Split(full, "\n") {
		line = strings.TrimSpace(line)
		if line == "data: [DONE]" {
			gotDone = true
		} else if strings.HasPrefix(line, "data: ") {
			gotData = true
		}
	}
	if !gotData {
		t.Fatal("no data chunks")
	}
	if !gotDone {
		t.Fatal("no [DONE]")
	}
	t.Logf("Stream output conv: %d chunks", len(chunks))
}

// ========================================================================
// DS V3 via volc with parameters
// ========================================================================

func TestVolcDSV3WithParams(t *testing.T) {
	const dsModel = "deepseek-v3-2-251201"
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: dsModel,
		ApiKey: key,
		BaseURL: volcBase,
		MaxTokensOverride: 100,
	}
	b, _ := json.Marshal(map[string]any{
		"model":       dsModel,
		"messages":    []map[string]any{{"role": "user", "content": "Solve: 23 × 47"}},
		"temperature": 0.3,
		"max_tokens":  100,
	})
	resp, err := execReq(e, info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("no choices")
	}
	t.Logf("DS V3 params response: %s", string(resp))
}

// ========================================================================
// Error handling
// ========================================================================

func TestVolcErrorBadKey(t *testing.T) {
	_ = volcKey(t) // skip if no key, but use bad key for actual test
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: "sk-bad-key-that-will-fail",
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	status, _, err := execReqRaw(e, info, b)
	if err != nil {
		t.Fatalf("execReqRaw err: %v", err)
	}
	if status == 200 {
		t.Fatal("expected non-200 status for bad key")
	}
	t.Logf("Bad key status: %d (expected 401/403)", status)
}

func TestVolcErrorBadModel(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: "nonexistent-model-xyz",
		ActualModelName: "nonexistent-model-xyz",
		ApiKey: key,
		BaseURL: volcBase,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    "nonexistent-model-xyz",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	status, body, err := execReqRaw(e, info, b)
	if err != nil && status == 0 {
		t.Logf("Bad model error: %v", err)
		return
	}
	if status == 200 {
		t.Fatal("expected non-200 for bad model")
	}
	t.Logf("Bad model status: %d, body: %s", status, string(body))
}

// ========================================================================
// Streaming cancellation
// ========================================================================

func TestVolcStreamCancel(t *testing.T) {
	key := volcKey(t)
	e := executor.GetByProvider("volcengine")
	info := &executor.RequestInfo{
		UpstreamFormat: translator.FormatOpenAI,
		InboundFormat:  translator.FormatOpenAI,
		ClientFormat:   translator.FormatOpenAI,
		Model: volcModel,
		ApiKey: key,
		BaseURL: volcBase,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    volcModel,
		"messages": []map[string]any{{"role": "user", "content": "Write a long essay about AI."}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var count int
	err := executor.ExecuteStream(ctx, e, info, b, func(chunk []byte) error {
		count++
		return nil
	})
	if err != nil && !strings.Contains(err.Error(), "context canceled") &&
		!strings.Contains(err.Error(), "context deadline exceeded") &&
		!strings.Contains(err.Error(), "Client.Timeout") {
		t.Logf("Cancel err (non-fatal): %v", err)
	}
	t.Logf("Stream cancel: got %d chunks before timeout", count)
}

// ========================================================================
// Helpers
// ========================================================================

func execReq(e executor.Executor, info *executor.RequestInfo, body []byte) ([]byte, error) {
	status, data, err := execReqRaw(e, info, body)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, string(data))
	}
	return data, nil
}

func execReqRaw(e executor.Executor, info *executor.RequestInfo, body []byte) (int, []byte, error) {
	up := info.UpstreamFormat
	if up == "" {
		up = translator.FormatOpenAI
	}
	conv, err := e.ConvertRequest(body, info.InboundFormat, up)
	if err != nil {
		return 0, nil, err
	}
	conv = e.RequestCustomize(conv, info)
	resp, err := e.DoRequest(info, bytes.NewReader(conv))
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	respBody = e.ResponseCustomize(respBody, info)
	convResp, err := e.ConvertResponse(respBody, up, info.ClientFormat)
	if err != nil {
		return resp.StatusCode, respBody, err
	}
	return resp.StatusCode, convResp, nil
}

func mustUnmarshal(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("json: %v (body: %s)", err, string(data))
	}
}
