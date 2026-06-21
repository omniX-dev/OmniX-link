package image

import (
	"maps"
	"sync"
)

var (
	registry   = make(map[string]ImageExecutor)
	registryMu sync.RWMutex
)

// RegisterImage registers an image executor for a provider.
func RegisterImage(provider string, e ImageExecutor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[provider] = e
}

// GetImage returns the image executor for the given provider, or nil.
func GetImage(provider string) ImageExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[provider]
}

// AllImageExecutors returns a copy of all registered image executors.
func AllImageExecutors() map[string]ImageExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return maps.Clone(registry)
}
