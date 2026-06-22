// Package embedding defines the interface and types for text embedding providers.
//
// Standard format: OpenAI /v1/embeddings compatible.
// All providers adapt to/from this standard via their executor.
//
// EmbeddingRequest → HTTP POST /v1/embeddings → EmbeddingResponse
package embedding

// EmbeddingExecutor is the interface for text embedding providers.
type EmbeddingExecutor interface {
	// Init initializes the executor with channel configuration.
	Init(channel any) // *model.Channel

	// GetName returns the human-readable executor name.
	GetName() string

	// Embed creates embeddings for the given input text(s).
	// Input can be a single string or multiple strings.
	// Returns vector embeddings and usage information.
	Embed(req *EmbeddingRequest) (*EmbeddingResponse, error)
}
