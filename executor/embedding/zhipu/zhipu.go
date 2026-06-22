// Package zhipu implements embedding executor for Zhipu (智谱) text embeddings.
//
// Endpoint: POST /v1/embeddings
// Models: embedding-3 (supports custom dimensions 256-2048), embedding-2
// Auth: Authorization: Bearer
//
// API format is identical to OpenAI /v1/embeddings.
// Base URL: https://open.bigmodel.cn/api/paas/v4
package zhipu

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
	embedding.RegisterEmbedding("zhipu", &ZhipuEmbeddingExecutor{})
}

// ZhipuEmbeddingExecutor handles Zhipu text embeddings via OpenAI-compatible format.
type ZhipuEmbeddingExecutor struct {
	channel any
}

func (e *ZhipuEmbeddingExecutor) Init(channel any) {
	e.channel = channel
}

func (e *ZhipuEmbeddingExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "ZhipuEmbedding"
}

func (e *ZhipuEmbeddingExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://open.bigmodel.cn/api/paas/v4"
}

func (e *ZhipuEmbeddingExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// Embed sends a text embedding request to Zhipu API.
// Request and response use the same JSON format as OpenAI /v1/embeddings.
func (e *ZhipuEmbeddingExecutor) Embed(req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("zhipu: embed marshal: %w", err)
	}

	apiKey := e.getAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("zhipu: embed: API key not configured")
	}

	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	url := baseURL + "/v1/embeddings"

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("zhipu: embed create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("zhipu: embed do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("zhipu: embed read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhipu: embed status %d: %s", resp.StatusCode, string(respBody))
	}

	var result embedding.EmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("zhipu: embed unmarshal: %w", err)
	}

	return &result, nil
}
