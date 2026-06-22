// Package alibaba implements EmbeddingExecutor for Alibaba Cloud Qwen text embeddings (DashScope).
//
// Uses OpenAI-compatible endpoint at DashScope:
//   POST https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings
//
// Models: text-embedding-v4, text-embedding-v3, text-embedding-v2, text-embedding-v1
// text-embedding-v4 supports custom dimensions via the standard dimensions field.
package alibaba

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/embedding"
)

func init() {
	embedding.RegisterEmbedding("alibaba", &QwenEmbeddingExecutor{})
}

// DefaultBaseURL is the DashScope OpenAI-compatible endpoint base.
const DefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

// QwenEmbeddingExecutor handles Alibaba Cloud Qwen text embeddings via DashScope
// OpenAI-compatible API.
type QwenEmbeddingExecutor struct {
	channel any
}

// Init initializes the executor with channel configuration.
func (e *QwenEmbeddingExecutor) Init(channel any) {
	e.channel = channel
}

// GetName returns the human-readable executor name.
func (e *QwenEmbeddingExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Qwen"
}

// getBaseURL returns the configured base URL or the default DashScope endpoint.
func (e *QwenEmbeddingExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return DefaultBaseURL
}

// getAPIKey returns the API key from the channel configuration.
func (e *QwenEmbeddingExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// Embed creates embeddings for the given input text(s).
// Input can be a single string or multiple strings.
// Returns vector embeddings and usage information.
// Uses the DashScope OpenAI-compatible endpoint which accepts the same JSON
// format as OpenAI /v1/embeddings.
func (e *QwenEmbeddingExecutor) Embed(req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	apiKey := e.getAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("qwen: API key not configured")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("qwen: marshal request: %w", err)
	}

	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	endpoint := baseURL + "/v1/embeddings"

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("qwen: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("qwen: http post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qwen: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qwen: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var result embedding.EmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("qwen: unmarshal response: %w", err)
	}

	return &result, nil
}
