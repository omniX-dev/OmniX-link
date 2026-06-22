package embedding

import (
	"maps"
	"sync"
)

var (
	registry   = make(map[string]EmbeddingExecutor)
	registryMu sync.RWMutex
)

// RegisterEmbedding registers an embedding executor for a provider.
func RegisterEmbedding(provider string, e EmbeddingExecutor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[provider] = e
}

// GetEmbedding returns the embedding executor for the given provider, or nil.
func GetEmbedding(provider string) EmbeddingExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[provider]
}

// AllEmbeddingExecutors returns a copy of all registered embedding executors.
func AllEmbeddingExecutors() map[string]EmbeddingExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return maps.Clone(registry)
}
