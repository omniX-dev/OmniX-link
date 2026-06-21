package audio

import (
	"maps"
	"sync"
)

var (
	registry   = make(map[string]AudioExecutor)
	registryMu sync.RWMutex
)

// RegisterAudio registers an audio executor for a provider.
func RegisterAudio(provider string, e AudioExecutor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[provider] = e
}

// GetAudio returns the audio executor for the given provider, or nil.
func GetAudio(provider string) AudioExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[provider]
}

// AllAudioExecutors returns a copy of all registered audio executors.
func AllAudioExecutors() map[string]AudioExecutor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return maps.Clone(registry)
}
