package cache

import "sync"

var (
	defaultStoreOnce sync.Once
	defaultStore     *Store
)

// DefaultStore returns the process-wide cache store used by the relay runtime.
func DefaultStore() *Store {
	defaultStoreOnce.Do(func() {
		defaultStore = NewStore(0)
	})
	return defaultStore
}

// ClearDefault clears the process-wide runtime cache.
func ClearDefault() {
	DefaultStore().Clear()
}
