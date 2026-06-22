// Package openai implements OpenAI-compatible text embeddings via /v1/embeddings.
package openai

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
	embedding.RegisterEmbedding("openai", &OpenAIEmbeddingExecutor{})
}

// OpenAIEmbeddingExecutor handles OpenAI-compatible text embedding endpoints.
type OpenAIEmbeddingExecutor struct {
	channel any
}

// Init stores the channel configuration.
func (e *OpenAIEmbeddingExecutor) Init(channel any) {
	e.channel = channel
}

// GetName returns the executor name from channel or a default.
func (e *OpenAIEmbeddingExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "OpenAIEmbedding"
}

// getBaseURL extracts the base URL from the channel or returns the default.
func (e *OpenAIEmbeddingExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return strings.TrimRight(url, "/")
		}
	}
	return "https://api.openai.com"
}

// getAPIKey extracts the API key from the channel.
func (e *OpenAIEmbeddingExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// Embed sends a text embedding request to the OpenAI-compatible /v1/embeddings endpoint.
func (e *OpenAIEmbeddingExecutor) Embed(req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai: embedding marshal: %w", err)
	}

	baseURL := e.getBaseURL()
	apiKey := e.getAPIKey()

	url := baseURL + "/v1/embeddings"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: embedding new request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: embedding request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: embedding read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: embedding %s: %s", resp.Status, string(respBody))
	}

	var embedResp embedding.EmbeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("openai: embedding unmarshal: %w", err)
	}

	return &embedResp, nil
}
