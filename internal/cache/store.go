package cache

import (
	"net/http"
	"sync"
	"time"
)

// Entry is a cached HTTP response snapshot.
type Entry struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	StoredAt   time.Time
	ExpiresAt  time.Time
}

// Store is a concurrency-safe in-memory cache.
type Store struct {
	mu      sync.RWMutex
	entries map[string]Entry
	ttl     time.Duration
}

// NewStore creates a new cache store.
func NewStore(ttl time.Duration) *Store {
	return &Store{
		entries: make(map[string]Entry),
		ttl:     ttl,
	}
}

// Get returns a cache entry when present and not expired.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return Entry{}, false
	}

	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		s.mu.Lock()
		delete(s.entries, key)
		s.mu.Unlock()
		return Entry{}, false
	}

	return cloneEntry(entry), true
}

// Set stores an entry by key.
func (s *Store) Set(key string, entry Entry) {
	entry.StoredAt = time.Now()
	if s.ttl > 0 {
		entry.ExpiresAt = entry.StoredAt.Add(s.ttl)
	}

	s.mu.Lock()
	s.entries[key] = cloneEntry(entry)
	s.mu.Unlock()
}

// Clear removes all cached entries.
func (s *Store) Clear() {
	s.mu.Lock()
	s.entries = make(map[string]Entry)
	s.mu.Unlock()
}

// Len returns number of active entries.
func (s *Store) Len() int {
	s.mu.RLock()
	n := len(s.entries)
	s.mu.RUnlock()
	return n
}

func cloneEntry(entry Entry) Entry {
	clonedHeader := make(http.Header, len(entry.Header))
	for k, vv := range entry.Header {
		copiedValues := make([]string, len(vv))
		copy(copiedValues, vv)
		clonedHeader[k] = copiedValues
	}

	body := make([]byte, len(entry.Body))
	copy(body, entry.Body)

	return Entry{
		StatusCode: entry.StatusCode,
		Header:     clonedHeader,
		Body:       body,
		StoredAt:   entry.StoredAt,
		ExpiresAt:  entry.ExpiresAt,
	}
}
