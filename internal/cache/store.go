package cache

import (
	"container/list"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// State is the cache lookup result state.
type State string

const (
	StateMiss  State = "MISS"
	StateHit   State = "HIT"
	StateStale State = "STALE"
)

// Stats captures high-level cache statistics.
type Stats struct {
	Entries   int
	SizeBytes int64
	Hits      int64
	Misses    int64
	StaleHits int64
	Evictions int64
	HitRatio  float64
}

// Options configures cache storage behavior.
type Options struct {
	DefaultTTL           time.Duration
	StaleWhileRevalidate time.Duration
	StaleIfError         time.Duration
	MaxEntries           int
	MaxBytes             int64
	MaxEntryBytes        int64
}

// Entry is a cached HTTP response snapshot.
type Entry struct {
	StatusCode                int
	Header                    http.Header
	Body                      []byte
	StoredAt                  time.Time
	ExpiresAt                 time.Time
	StaleWhileRevalidateUntil time.Time
	StaleIfErrorUntil         time.Time
	ETag                      string
	LastModified              string
	Vary                      []string
	VaryValues                map[string]string
}

// IsFresh reports whether the entry is currently fresh.
func (e Entry) IsFresh(now time.Time) bool {
	if e.ExpiresAt.IsZero() {
		return true
	}
	return now.Before(e.ExpiresAt)
}

// CanServeStaleWhileRevalidate reports whether stale serving is allowed during background revalidation.
func (e Entry) CanServeStaleWhileRevalidate(now time.Time) bool {
	if e.ExpiresAt.IsZero() || now.Before(e.ExpiresAt) {
		return false
	}
	return !e.StaleWhileRevalidateUntil.IsZero() && now.Before(e.StaleWhileRevalidateUntil)
}

// CanServeStaleIfError reports whether stale serving is allowed when origin fails.
func (e Entry) CanServeStaleIfError(now time.Time) bool {
	if e.ExpiresAt.IsZero() || now.Before(e.ExpiresAt) {
		return false
	}
	return !e.StaleIfErrorUntil.IsZero() && now.Before(e.StaleIfErrorUntil)
}

type item struct {
	key     string
	baseKey string
	entry   Entry
	size    int64
}

// Store is a concurrency-safe in-memory LRU cache with stale serving support.
type Store struct {
	mu                sync.Mutex
	entries           map[string]*list.Element
	lru               *list.List
	variantsByBase    map[string]map[string]struct{}
	varyHeadersByBase map[string][]string
	opts              Options
	currentBytes      int64

	hits      int64
	misses    int64
	staleHits int64
	evictions int64
}

// NewStore creates a cache store with legacy-compatible TTL-only behavior.
func NewStore(ttl time.Duration) *Store {
	return NewStoreWithOptions(Options{
		DefaultTTL:    ttl,
		MaxEntries:    5000,
		MaxEntryBytes: 1024 * 1024,
		MaxBytes:      512 * 1024 * 1024,
	})
}

// NewStoreWithOptions creates a new configurable cache store.
func NewStoreWithOptions(opts Options) *Store {
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 5000
	}
	if opts.MaxEntryBytes <= 0 {
		opts.MaxEntryBytes = 1024 * 1024
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = int64(opts.MaxEntries) * opts.MaxEntryBytes
	}

	return &Store{
		entries:           make(map[string]*list.Element),
		lru:               list.New(),
		variantsByBase:    make(map[string]map[string]struct{}),
		varyHeadersByBase: make(map[string][]string),
		opts:              opts,
	}
}

// Get returns a fresh cache entry for an exact key.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ele, ok := s.entries[key]
	if !ok {
		s.misses++
		return Entry{}, false
	}

	item := ele.Value.(*item)
	now := time.Now()
	if !item.entry.IsFresh(now) {
		s.misses++
		s.removeElement(ele)
		return Entry{}, false
	}

	s.hits++
	s.lru.MoveToFront(ele)
	return cloneEntry(item.entry), true
}

// Lookup finds a cache variant using base key plus request headers.
func (s *Store) Lookup(baseKey string, reqHeaders http.Header) (Entry, State, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	variantKey := s.variantKeyForLookup(baseKey, reqHeaders)
	ele, ok := s.entries[variantKey]
	if !ok {
		s.misses++
		return Entry{}, StateMiss, ""
	}

	item := ele.Value.(*item)
	now := time.Now()

	if item.entry.IsFresh(now) {
		s.hits++
		s.lru.MoveToFront(ele)
		return cloneEntry(item.entry), StateHit, variantKey
	}

	if item.entry.CanServeStaleWhileRevalidate(now) || item.entry.CanServeStaleIfError(now) {
		s.staleHits++
		s.lru.MoveToFront(ele)
		return cloneEntry(item.entry), StateStale, variantKey
	}

	s.misses++
	s.removeElement(ele)
	return Entry{}, StateMiss, ""
}

// Set stores an exact-key entry using default store TTL settings.
func (s *Store) Set(key string, entry Entry) {
	now := time.Now()
	policy := Policy{
		Cacheable: true,
	}
	if s.opts.DefaultTTL > 0 {
		policy.ExpiresAt = now.Add(s.opts.DefaultTTL)
	}
	if s.opts.StaleWhileRevalidate > 0 {
		policy.StaleWhileRevalidateUntil = policy.ExpiresAt.Add(s.opts.StaleWhileRevalidate)
	}
	if s.opts.StaleIfError > 0 {
		policy.StaleIfErrorUntil = policy.ExpiresAt.Add(s.opts.StaleIfError)
	}
	_, _ = s.SetWithRequest(key, nil, entry, policy)
}

// SetWithRequest stores a request variant using response policy and returns the variant key.
func (s *Store) SetWithRequest(baseKey string, reqHeaders http.Header, entry Entry, policy Policy) (string, bool) {
	if !policy.Cacheable {
		return "", false
	}
	if len(entry.Body) > int(s.opts.MaxEntryBytes) {
		return "", false
	}

	now := time.Now()
	stored := cloneEntry(entry)
	stored.StoredAt = now
	stored.ExpiresAt = policy.ExpiresAt
	if stored.ExpiresAt.IsZero() {
		if s.opts.DefaultTTL > 0 {
			stored.ExpiresAt = now.Add(s.opts.DefaultTTL)
		}
	}

	stored.StaleWhileRevalidateUntil = policy.StaleWhileRevalidateUntil
	if stored.StaleWhileRevalidateUntil.IsZero() && s.opts.StaleWhileRevalidate > 0 {
		stored.StaleWhileRevalidateUntil = stored.ExpiresAt.Add(s.opts.StaleWhileRevalidate)
	}

	stored.StaleIfErrorUntil = policy.StaleIfErrorUntil
	if stored.StaleIfErrorUntil.IsZero() && s.opts.StaleIfError > 0 {
		stored.StaleIfErrorUntil = stored.ExpiresAt.Add(s.opts.StaleIfError)
	}

	stored.ETag = strings.TrimSpace(stored.Header.Get("ETag"))
	stored.LastModified = strings.TrimSpace(stored.Header.Get("Last-Modified"))

	varyHeaders := normalizeHeaders(policy.Vary)
	stored.Vary = copyStringSlice(varyHeaders)
	stored.VaryValues = make(map[string]string, len(varyHeaders))
	for _, header := range varyHeaders {
		stored.VaryValues[header] = strings.TrimSpace(reqHeaders.Get(header))
	}

	variantKey := composeVariantKey(baseKey, stored.VaryValues)
	entrySize := estimateEntrySize(stored)
	if s.opts.MaxBytes > 0 && entrySize > s.opts.MaxBytes {
		return "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.entries[variantKey]; ok {
		s.removeElement(old)
	}

	elem := s.lru.PushFront(&item{key: variantKey, baseKey: baseKey, entry: stored, size: entrySize})
	s.entries[variantKey] = elem
	s.currentBytes += entrySize

	if _, ok := s.variantsByBase[baseKey]; !ok {
		s.variantsByBase[baseKey] = make(map[string]struct{})
	}
	s.variantsByBase[baseKey][variantKey] = struct{}{}
	if len(varyHeaders) > 0 {
		s.varyHeadersByBase[baseKey] = copyStringSlice(varyHeaders)
	} else {
		delete(s.varyHeadersByBase, baseKey)
	}

	s.enforceLimits()
	return variantKey, true
}

// Refresh updates an existing variant key using a new policy while preserving key mapping.
func (s *Store) Refresh(variantKey string, entry Entry, policy Policy) bool {
	s.mu.Lock()
	ele, ok := s.entries[variantKey]
	s.mu.Unlock()
	if !ok {
		return false
	}
	baseKey := ele.Value.(*item).baseKey
	_, saved := s.SetWithRequest(baseKey, headerFromVaryValues(entry.VaryValues), entry, policy)
	return saved
}

// Clear removes all cached entries.
func (s *Store) Clear() {
	s.mu.Lock()
	s.entries = make(map[string]*list.Element)
	s.variantsByBase = make(map[string]map[string]struct{})
	s.varyHeadersByBase = make(map[string][]string)
	s.lru.Init()
	s.currentBytes = 0
	s.mu.Unlock()
}

// DeleteBaseKey removes all variants under a base key and returns removed entry count.
func (s *Store) DeleteBaseKey(baseKey string) int {
	if strings.TrimSpace(baseKey) == "" {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	variants := s.variantsByBase[baseKey]
	if len(variants) == 0 {
		return 0
	}

	keys := make([]string, 0, len(variants))
	for key := range variants {
		keys = append(keys, key)
	}

	removed := 0
	for _, key := range keys {
		ele, ok := s.entries[key]
		if !ok {
			continue
		}
		s.removeElement(ele)
		removed++
	}
	return removed
}

// Len returns number of active entries.
func (s *Store) Len() int {
	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()
	return n
}

// Stats returns a snapshot of cache statistics.
func (s *Store) Stats() Stats {
	s.mu.Lock()
	hits := s.hits
	misses := s.misses
	staleHits := s.staleHits
	ratio := 0.0
	total := hits + misses + staleHits
	if total > 0 {
		ratio = float64(hits+staleHits) / float64(total)
	}
	stats := Stats{
		Entries:   len(s.entries),
		SizeBytes: s.currentBytes,
		Hits:      hits,
		Misses:    misses,
		StaleHits: staleHits,
		Evictions: s.evictions,
		HitRatio:  ratio,
	}
	s.mu.Unlock()
	return stats
}

func (s *Store) variantKeyForLookup(baseKey string, reqHeaders http.Header) string {
	varyHeaders := s.varyHeadersByBase[baseKey]
	if len(varyHeaders) == 0 {
		return baseKey
	}
	varyValues := make(map[string]string, len(varyHeaders))
	for _, header := range varyHeaders {
		varyValues[header] = strings.TrimSpace(reqHeaders.Get(header))
	}
	return composeVariantKey(baseKey, varyValues)
}

func (s *Store) enforceLimits() {
	for s.lru.Len() > s.opts.MaxEntries || (s.opts.MaxBytes > 0 && s.currentBytes > s.opts.MaxBytes) {
		back := s.lru.Back()
		if back == nil {
			return
		}
		s.removeElement(back)
		s.evictions++
	}
}

func (s *Store) removeElement(ele *list.Element) {
	if ele == nil {
		return
	}
	it := ele.Value.(*item)
	delete(s.entries, it.key)
	s.currentBytes -= it.size
	s.lru.Remove(ele)

	variants := s.variantsByBase[it.baseKey]
	delete(variants, it.key)
	if len(variants) == 0 {
		delete(s.variantsByBase, it.baseKey)
		delete(s.varyHeadersByBase, it.baseKey)
	}
}

func composeVariantKey(baseKey string, varyValues map[string]string) string {
	if len(varyValues) == 0 {
		return baseKey
	}
	keys := make([]string, 0, len(varyValues))
	for k := range varyValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b := strings.Builder{}
	b.WriteString(baseKey)
	b.WriteString("|vary")
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(url.QueryEscape(varyValues[k]))
	}
	return b.String()
}

func estimateEntrySize(entry Entry) int64 {
	total := int64(len(entry.Body))
	for k, values := range entry.Header {
		total += int64(len(k))
		for _, v := range values {
			total += int64(len(v))
		}
	}
	for k, v := range entry.VaryValues {
		total += int64(len(k) + len(v))
	}
	return total
}

func headerFromVaryValues(varyValues map[string]string) http.Header {
	h := make(http.Header, len(varyValues))
	for k, v := range varyValues {
		h.Set(k, v)
	}
	return h
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

	vary := copyStringSlice(entry.Vary)
	varyValues := make(map[string]string, len(entry.VaryValues))
	for k, v := range entry.VaryValues {
		varyValues[k] = v
	}

	return Entry{
		StatusCode:                entry.StatusCode,
		Header:                    clonedHeader,
		Body:                      body,
		StoredAt:                  entry.StoredAt,
		ExpiresAt:                 entry.ExpiresAt,
		StaleWhileRevalidateUntil: entry.StaleWhileRevalidateUntil,
		StaleIfErrorUntil:         entry.StaleIfErrorUntil,
		ETag:                      entry.ETag,
		LastModified:              entry.LastModified,
		Vary:                      vary,
		VaryValues:                varyValues,
	}
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func normalizeHeaders(headers []string) []string {
	if len(headers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(headers))
	normalized := make([]string, 0, len(headers))
	for _, header := range headers {
		h := http.CanonicalHeaderKey(strings.TrimSpace(header))
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		normalized = append(normalized, h)
	}
	sort.Strings(normalized)
	return normalized
}

// BuildBaseKey builds a normalized cache base key from method, path, and query.
func BuildBaseKey(method string, requestURL *url.URL) string {
	if requestURL == nil {
		return strings.ToUpper(strings.TrimSpace(method)) + " /"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path := requestURL.EscapedPath()
	if path == "" {
		path = "/"
	}
	queryValues := requestURL.Query()
	query := normalizeQuery(queryValues)
	if query == "" {
		return fmt.Sprintf("%s %s", method, path)
	}
	return fmt.Sprintf("%s %s?%s", method, path, query)
}

func normalizeQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	b := strings.Builder{}
	first := true
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, v := range vals {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(url.QueryEscape(key))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}
