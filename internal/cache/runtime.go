package cache

import "sync"

var (
	runtimeMu    sync.Mutex
	defaultStore *Store
)

// DefaultStore returns the process-wide cache store used by the relay runtime.
func DefaultStore() *Store {
	runtimeMu.Lock()
	if defaultStore == nil {
		defaultStore = NewStore(0)
	}
	runtimeMu.Unlock()
	return defaultStore
}

// ConfigureDefaultStore initializes or returns the process-wide cache store with explicit options.
func ConfigureDefaultStore(opts Options) *Store {
	runtimeMu.Lock()
	if defaultStore == nil {
		defaultStore = NewStoreWithOptions(opts)
	}
	runtimeMu.Unlock()
	return defaultStore
}

// ClearDefault clears the process-wide runtime cache.
func ClearDefault() {
	DefaultStore().Clear()
}
