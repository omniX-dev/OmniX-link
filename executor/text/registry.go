package executor

import (
	"sync"

	"github.com/just4zeroq/Omni-link/translator"
)

var (
	registry   = make(map[string]Executor)
	registryMu sync.RWMutex
)

// Register registers an executor for a provider.
func Register(provider string, e Executor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[provider] = e
}

// GetByProvider returns the executor for the given provider, or nil.
func GetByProvider(provider string) Executor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[provider]
}

// GetByFormat returns an executor that natively supports the given format.
// Returns the first match; if multiple, prefers the one with matching relay mode.
func GetByFormat(fmt translator.Format) Executor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, e := range registry {
		for _, cap := range e.NativeEndpoints() {
			if cap.Format == fmt {
				return e
			}
		}
	}
	return nil
}

// All returns a copy of all registered executors.
func All() map[string]Executor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	result := make(map[string]Executor, len(registry))
	for k, v := range registry {
		result[k] = v
	}
	return result
}
