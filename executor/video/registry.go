package video

import (
	"maps"
	"sync"
)

var (
	registry   = make(map[string]VideoExecutor)
	registryMu sync.RWMutex
)

// RegisterVideo registers a video executor for a provider.
func RegisterVideo(provider string, e VideoExecutor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[provider] = e
}

// GetVideo returns the video executor for the given provider, or nil.
func GetVideo(provider string) VideoExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[provider]
}

// AllVideoExecutors returns a copy of all registered video executors.
func AllVideoExecutors() map[string]VideoExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return maps.Clone(registry)
}
