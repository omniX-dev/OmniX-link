// Package jina implements EmbeddingExecutor for Jina AI text embeddings.
//
// API: https://api.jina.ai/v1/embeddings (OpenAI-compatible)
// Models: jina-embeddings-v3, jina-embeddings-v2-base-zh, jina-embeddings-v2-base-en,
//
//	jina-embeddings-v2-base-code, jina-embeddings-v4, jina-clip-v2
//
// Supports Matryoshka dimensions: 32, 128, 256, 512, 768, 1024, 2048
package jina

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
	embedding.RegisterEmbedding("jina", &JinaEmbeddingExecutor{})
}

// JinaEmbeddingExecutor handles Jina AI text embeddings via OpenAI-compatible API.
type JinaEmbeddingExecutor struct {
	channel any
}

func (e *JinaEmbeddingExecutor) Init(channel any) {
	e.channel = channel
}

func (e *JinaEmbeddingExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "JinaEmbedding"
}

func (e *JinaEmbeddingExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return strings.TrimSuffix(url, "/")
		}
	}
	return "https://api.jina.ai/v1"
}

func (e *JinaEmbeddingExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// Embed sends a text embedding request to Jina AI and returns the response.
func (e *JinaEmbeddingExecutor) Embed(req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("jina: marshal: %w", err)
	}

	resp, err := e.doRequest("/embeddings", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jina: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jina: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result embedding.EmbeddingResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("jina: unmarshal: %w", err)
	}
	return &result, nil
}

func (e *JinaEmbeddingExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("jina: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}
