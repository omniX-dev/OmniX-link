package deepseek

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

	"github.com/just4zeroq/Omni-link/executor/text"
	"github.com/just4zeroq/Omni-link/translator"
)

const (
	deepSeekOpenAI = "https://api.deepseek.com"
	deepSeekClaude = "https://api.deepseek.com/anthropic"
	deepSeekModel  = "deepseek-chat"
)

func deepSeekKey(t *testing.T) string {
	t.Helper()
	tryLoadEnv()
	k := os.Getenv("DEEPSEEK_API_KEY")
	if k == "" {
		t.Skip("DEEPSEEK_API_KEY not set")
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

var httpClient = &http.Client{}

func TestDeepSeekOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	body := map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello in one word."}},
		"stream":   false,
	}
	status, resp := doPost(t, deepSeekOpenAI+"/v1/chat/completions", body, "Bearer "+key)
	if status != 200 {
		t.Fatalf("status %d: %s", status, string(resp))
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	c, _ := m["choices"].([]any)
	if len(c) == 0 {
		t.Fatal("no choices")
	}
	t.Logf("OpenAI: %s", c[0].(map[string]any)["message"].(map[string]any)["content"])
}

func TestDeepSeekClaude(t *testing.T) {
	key := deepSeekKey(t)
	body := map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Say hello in one word."}},
		"max_tokens": 4096,
		"stream":     false,
	}
	status, resp := doPost(t, deepSeekClaude+"/v1/messages", body, key)
	if status != 200 {
		t.Fatalf("status %d: %s", status, string(resp))
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	c, _ := m["content"].([]any)
	if len(c) == 0 {
		t.Fatal("no content")
	}
	t.Logf("Claude: %s", c[0].(map[string]any)["text"])
}

func TestDeepSeekOpenAIDirect(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "test-oa", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 3."}},
		"stream": false,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["choices"]; !ok {
		t.Fatalf("no choices: %s", string(resp))
	}
	t.Logf("OA direct: %s", string(resp))
}

func TestDeepSeekClaudeDirect(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "test-cl", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 3."}},
		"max_tokens": 4096, "stream": false,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content: %s", string(resp))
	}
	t.Logf("Claude direct: %s", string(resp))
}

func TestDeepSeekConvOpenAIToClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "test-conv", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Hi"}},
		"stream": false,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["choices"]; !ok {
		t.Fatalf("no choices: %s", string(resp))
	}
	t.Logf("Conv: %s", string(resp))
}

// ========================================================================
// Streaming tests
// ========================================================================

func TestDeepSeekStreamOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "stream-oa", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	// Verify SSE structure — "data: " prefix chunks
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
		t.Fatal("no data chunks received")
	}
	if !gotDone {
		t.Fatal("no [DONE] terminator")
	}
	t.Logf("Stream OpenAI: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

func TestDeepSeekStreamClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "stream-cl", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"max_tokens": 4096, "stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	// Verify Claude SSE structure — events with "event: " prefix
	gotContentDelta := false
	gotStop := false
	for _, c := range chunks {
		s := string(c)
		if strings.Contains(s, "event: content_block_delta") {
			gotContentDelta = true
		}
		if strings.Contains(s, "event: message_stop") {
			gotStop = true
		}
	}
	if !gotContentDelta {
		t.Fatal("no content_block_delta event")
	}
	if !gotStop {
		t.Fatal("no message_stop event")
	}
	t.Logf("Stream Claude: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

func TestDeepSeekStreamConvOpenAIToClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "stream-conv", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	// After conversion Claude→OpenAI, output should be OpenAI-format SSE.
	gotData := false
	gotDone := false
	for _, c := range chunks {
		s := string(c)
		if strings.HasPrefix(s, "data: ") {
			if strings.TrimSpace(s) == "data: [DONE]" {
				gotDone = true
			} else {
				gotData = true
			}
		}
	}
	if !gotData {
		t.Fatal("no converted data chunks")
	}
	if !gotDone {
		t.Fatal("no [DONE] after conversion")
	}
	t.Logf("Stream Conv: %d chunks, %.2f KB", len(chunks), float64(len(bytes.Join(chunks, nil)))/1024)
}

// ========================================================================
// Reverse conversion — Claude→OpenAI→Claude
// ========================================================================

func TestDeepSeekConvClaudeToOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "conv-co", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Hi"}},
		"max_tokens": 4096,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Conv Claude→OpenAI→Claude: success")
}

// ========================================================================
// Plan auto-resolve — test executor.Request() end-to-end without UpstreamFormat
// ========================================================================

func TestDeepSeekPlanAutoOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatOpenAI,
		Model: deepSeekModel,
		ApiKey: key,
		BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 3."}},
	})
	resp, err := executor.Request(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["choices"]; !ok {
		t.Fatalf("no choices (expected OpenAI format): %s", string(resp))
	}
	t.Logf("Plan auto OpenAI: %s", string(resp))
}

func TestDeepSeekPlanAutoClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatClaude,
		ClientFormat:  translator.FormatClaude,
		Model: deepSeekModel,
		ApiKey: key,
		BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Count 1 to 3."}},
		"max_tokens": 4096,
	})
	resp, err := executor.Request(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Plan auto Claude: %s", string(resp))
}

func TestDeepSeekPlanAutoConv(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatClaude,
		Model: deepSeekModel,
		ApiKey: key,
		BaseURL: deepSeekOpenAI,
	}
	// Plan: In=OpenAI, Out=Claude → candidates [OpenAI, Claude] → score 1 each → tie → prefers Claude
	// → sends via Claude endpoint → response converted to Claude format
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Hi"}},
	})
	resp, err := executor.Request(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format): %s", string(resp))
	}
	t.Logf("Plan auto conv OpenAI→Claude: success")
}

// ========================================================================
// Plan auto-resolve via ExecuteStream
// ========================================================================

func TestDeepSeekPlanAutoStreamOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatOpenAI,
		Model: deepSeekModel,
		ApiKey: key,
		BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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
	t.Logf("Plan auto stream OpenAI: %d chunks", len(chunks))
}

func TestDeepSeekPlanAutoStreamClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		InboundFormat: translator.FormatClaude,
		ClientFormat:  translator.FormatClaude,
		Model: deepSeekModel,
		ApiKey: key,
		BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"max_tokens": 4096, "stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	gotContent := false
	gotStop := false
	for _, c := range chunks {
		s := string(c)
		if strings.Contains(s, "event: content_block_delta") {
			gotContent = true
		}
		if strings.Contains(s, "event: message_stop") {
			gotStop = true
		}
	}
	if !gotContent {
		t.Fatal("no content_block_delta")
	}
	if !gotStop {
		t.Fatal("no message_stop")
	}
	t.Logf("Plan auto stream Claude: %d chunks", len(chunks))
}

// ========================================================================
// Output-only conversion — response format differs from upstream
// ========================================================================

func TestDeepSeekOutputConvOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "out-oa", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	// Send OpenAI request → OpenAI endpoint → convert response to Claude format
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Say hello"}},
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["content"]; !ok {
		t.Fatalf("no content (expected Claude format after conv): %s", string(resp))
	}
	t.Logf("OutputConv OpenAI→Claude: success")
}

func TestDeepSeekOutputConvClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "out-cl", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	// Send Claude request → Claude endpoint → convert response to OpenAI format
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Say hello"}},
		"max_tokens": 4096,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	if _, ok := m["choices"]; !ok {
		t.Fatalf("no choices (expected OpenAI format after conv): %s", string(resp))
	}
	t.Logf("OutputConv Claude→OpenAI: success")
}

// ========================================================================
// Features — system message, tools, params
// ========================================================================

func TestDeepSeekSystemMessage(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "sysmsg", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
		"messages": []map[string]any{
			{"role": "system", "content": "Reply in ALL CAPS."},
			{"role": "user", "content": "Say hello"},
		},
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("no choices")
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	content, _ := msg["content"].(string)
	t.Logf("System msg response (expect caps): %s", content)
}

func TestDeepSeekWithTools(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "tools", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model": deepSeekModel,
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
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	t.Logf("Tools response: %s", string(resp))
}

func TestDeepSeekWithParams(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "params", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	stop := []string{"."}
	b, _ := json.Marshal(map[string]any{
		"model":       deepSeekModel,
		"messages":    []map[string]any{{"role": "user", "content": "Count 1 2 3"}},
		"temperature": 0.5,
		"top_p":       0.9,
		"stop":        stop,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
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
// Error handling
// ========================================================================

func TestDeepSeekErrorBadKey(t *testing.T) {
	_ = deepSeekKey(t) // skip if no key configured, but use bad key for actual test
	info := &executor.RequestInfo{
		RequestID: "err-key", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: "sk-bad-key-that-will-fail",
		BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})

	e := executor.GetByProvider("deepseek")
	status, _, err := execReqRaw(e, info, b)
	if err != nil {
		t.Fatalf("execReqRaw err: %v", err)
	}
	if status == 200 {
		t.Fatal("expected non-200 status for bad key")
	}
	t.Logf("Bad key status: %d (expected 401/403)", status)
}

func TestDeepSeekErrorBadModel(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "err-model", UpstreamFormat: translator.FormatOpenAI,
		Model: "nonexistent-model-xyz", ActualModelName: "nonexistent-model-xyz",
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    "nonexistent-model-xyz",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})

	e := executor.GetByProvider("deepseek")
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
// Streaming — reverse conversion + output-only
// ========================================================================

func TestDeepSeekStreamConvClaudeToOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "str-co", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"max_tokens": 4096, "stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	// Claude→OpenAI→Claude streaming → Claude SSE events
	gotContent := false
	gotStop := false
	for _, c := range chunks {
		s := string(c)
		if strings.Contains(s, "event: content_block_delta") {
			gotContent = true
		}
		if strings.Contains(s, "event: message_stop") {
			gotStop = true
		}
	}
	if !gotContent {
		t.Fatal("no content_block_delta")
	}
	if !gotStop {
		t.Fatal("no message_stop")
	}
	t.Logf("Stream ConvClaude→OpenAI→Claude: %d chunks", len(chunks))
}

func TestDeepSeekStreamOutputConvOpenAI(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "str-outoa", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	// OpenAI→OpenAI stream → convert to Claude SSE events
	gotContent := false
	gotStop := false
	for _, c := range chunks {
		s := string(c)
		if strings.Contains(s, "event: content_block_delta") {
			gotContent = true
		}
		if strings.Contains(s, "event: message_stop") {
			gotStop = true
		}
	}
	if !gotContent {
		t.Fatal("no content_block_delta (expected Claude SSE)")
	}
	if !gotStop {
		t.Fatal("no message_stop")
	}
	t.Logf("Stream OutputConv OpenAI→Claude: %d chunks", len(chunks))
}

func TestDeepSeekStreamOutputConvClaude(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "str-outcl", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Count 1 to 5."}},
		"max_tokens": 4096, "stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	// Claude→Claude stream → convert to OpenAI SSE
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
		t.Fatal("no data chunks (expected OpenAI SSE)")
	}
	if !gotDone {
		t.Fatal("no [DONE]")
	}
	t.Logf("Stream OutputConv Claude→OpenAI: %d chunks", len(chunks))
}

// ========================================================================
// Context cancellation
// ========================================================================

func TestDeepSeekStreamCancel(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "cancel", UpstreamFormat: translator.FormatOpenAI,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatOpenAI, ClientFormat: translator.FormatOpenAI,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true,
	}
	b, _ := json.Marshal(map[string]any{
		"model":    deepSeekModel,
		"messages": []map[string]any{{"role": "user", "content": "Write a long essay about AI."}},
		"stream":   true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var count int
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
		count++
		return nil
	})
	if err != nil && !strings.Contains(err.Error(), "context canceled") &&
		!strings.Contains(err.Error(), "context deadline exceeded") &&
		!strings.Contains(err.Error(), "Client.Timeout") {
		t.Logf("Cancel err (non-fatal): %v", err)
	}
	if count > 0 {
		t.Logf("Stream cancel: got %d chunks before timeout (expected partial read)", count)
	} else {
		t.Log("Stream cancel: no chunks before timeout (also acceptable)")
	}
}

// ========================================================================
// Thinking/Reasoning (try with DeepSeek)
// ========================================================================

func TestDeepSeekThinking(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "think", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		ReasoningEffort: "high",
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Solve: 23 × 47"}},
		"max_tokens": 4096,
	})
	resp, err := execReq(executor.GetByProvider("deepseek"), info, b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	mustUnmarshal(t, resp, &m)
	// May or may not have thinking field
	if thinking, ok := m["thinking"]; ok {
		t.Logf("Thinking enabled: %v", thinking)
	} else {
		t.Log("No thinking in response (model may not support)")
	}
	t.Logf("Thinking response: %s", string(resp))
}

func TestDeepSeekStreamWithThinking(t *testing.T) {
	key := deepSeekKey(t)
	info := &executor.RequestInfo{
		RequestID: "str-think", UpstreamFormat: translator.FormatClaude,
		Model: deepSeekModel, ActualModelName: deepSeekModel,
		InboundFormat: translator.FormatClaude, ClientFormat: translator.FormatClaude,
		ApiKey: key, BaseURL: deepSeekOpenAI,
		IsStream: true, ReasoningEffort: "high",
	}
	b, _ := json.Marshal(map[string]any{
		"model":      deepSeekModel,
		"messages":   []map[string]any{{"role": "user", "content": "Solve: 23 × 47"}},
		"max_tokens": 4096, "stream": true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks [][]byte
	err := executor.ExecuteStream(ctx, executor.GetByProvider("deepseek"), info, b, func(chunk []byte) error {
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

	gotDelta := false
	for _, c := range chunks {
		if strings.Contains(string(c), "event: content_block_delta") {
			gotDelta = true
			break
		}
	}
	if !gotDelta {
		t.Fatal("no content_block_delta")
	}
	t.Logf("Stream thinking: %d chunks", len(chunks))
}

// ─── helpers ───

func doPost(t *testing.T, url string, body any, auth string) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("Authorization", auth)
	} else {
		req.Header.Set("x-api-key", auth)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

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
