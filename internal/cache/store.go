package cache

import (
	"net/http"
	"sync"
	"time"
)

// Stats captures high-level cache statistics.
type Stats struct {
	Entries   int
	SizeBytes int64
	Hits      int64
	Misses    int64
	HitRatio  float64
}

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
	hits    int64
	misses  int64
	bytes   int64
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
		s.mu.Lock()
		s.misses++
		s.mu.Unlock()
		return Entry{}, false
	}

	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		s.mu.Lock()
		s.misses++
		s.bytes -= int64(len(entry.Body))
		delete(s.entries, key)
		s.mu.Unlock()
		return Entry{}, false
	}

	s.mu.Lock()
	s.hits++
	s.mu.Unlock()

	return cloneEntry(entry), true
}

// Set stores an entry by key.
func (s *Store) Set(key string, entry Entry) {
	entry.StoredAt = time.Now()
	if s.ttl > 0 {
		entry.ExpiresAt = entry.StoredAt.Add(s.ttl)
	}

	s.mu.Lock()
	if old, ok := s.entries[key]; ok {
		s.bytes -= int64(len(old.Body))
	}
	s.entries[key] = cloneEntry(entry)
	s.bytes += int64(len(entry.Body))
	s.mu.Unlock()
}

// Clear removes all cached entries.
func (s *Store) Clear() {
	s.mu.Lock()
	s.entries = make(map[string]Entry)
	s.bytes = 0
	s.mu.Unlock()
}

// Len returns number of active entries.
func (s *Store) Len() int {
	s.mu.RLock()
	n := len(s.entries)
	s.mu.RUnlock()
	return n
}

// Stats returns a snapshot of cache statistics.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	hits := s.hits
	misses := s.misses
	ratio := 0.0
	if hits+misses > 0 {
		ratio = float64(hits) / float64(hits+misses)
	}
	stats := Stats{
		Entries:   len(s.entries),
		SizeBytes: s.bytes,
		Hits:      hits,
		Misses:    misses,
		HitRatio:  ratio,
	}
	s.mu.RUnlock()
	return stats
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
