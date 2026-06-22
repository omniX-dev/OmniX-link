package embedding

// EmbeddingRequest is the standard input for text embedding.
// Compatible with OpenAI /v1/embeddings.
type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          any      `json:"input"`           // string or []string
	Dimensions     int      `json:"dimensions,omitempty"`     // optional: truncate to N dimensions
	EncodingFormat string   `json:"encoding_format,omitempty"` // "float" or "base64"
	User           string   `json:"user,omitempty"`
}

// EmbeddingResponse is the standard output for text embedding.
// Compatible with OpenAI /v1/embeddings response.
type EmbeddingResponse struct {
	Object string          `json:"object"` // "list"
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  Usage           `json:"usage"`
}

// EmbeddingData holds a single embedding vector.
type EmbeddingData struct {
	Object    string          `json:"object"`              // "embedding"
	Index     int             `json:"index"`
	Embedding []float64       `json:"embedding"`           // float vector
	EmbeddingB64 string       `json:"embedding_b64,omitempty"` // base64-encoded when encoding_format=base64
}

// Usage contains token usage information.
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
