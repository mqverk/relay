package cache

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestStoreSetGetClonesValues(t *testing.T) {
	store := NewStore(0)
	store.Set("k", Entry{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       []byte("ok"),
	})

	entry, ok := store.Get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}

	entry.Header.Set("Content-Type", "text/plain")
	entry.Body[0] = 'n'

	second, ok := store.Get("k")
	if !ok {
		t.Fatal("expected cache hit on second read")
	}
	if got := second.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected header value: %s", got)
	}
	if got := string(second.Body); got != "ok" {
		t.Fatalf("unexpected body value: %s", got)
	}
}

func TestStoreClear(t *testing.T) {
	store := NewStore(0)
	store.Set("k", Entry{StatusCode: 200})
	store.Clear()

	if _, ok := store.Get("k"); ok {
		t.Fatal("expected cache miss after clear")
	}
	if n := store.Len(); n != 0 {
		t.Fatalf("expected empty cache, got %d", n)
	}
}

func TestStoreTTLExpiration(t *testing.T) {
	store := NewStore(10 * time.Millisecond)
	store.Set("k", Entry{StatusCode: 200})
	time.Sleep(25 * time.Millisecond)

	if _, ok := store.Get("k"); ok {
		t.Fatal("expected cache entry to expire")
	}
}

func TestStoreEvictsLeastRecentlyUsed(t *testing.T) {
	store := NewStoreWithOptions(Options{DefaultTTL: time.Minute, MaxEntries: 2, MaxBytes: 1024 * 1024, MaxEntryBytes: 1024})
	store.Set("a", Entry{StatusCode: 200, Body: []byte("a")})
	store.Set("b", Entry{StatusCode: 200, Body: []byte("b")})

	if _, ok := store.Get("a"); !ok {
		t.Fatal("expected to read key a")
	}

	store.Set("c", Entry{StatusCode: 200, Body: []byte("c")})

	if _, ok := store.Get("b"); ok {
		t.Fatal("expected key b to be evicted as LRU")
	}
	if _, ok := store.Get("a"); !ok {
		t.Fatal("expected key a to remain in cache")
	}
	if _, ok := store.Get("c"); !ok {
		t.Fatal("expected key c to be in cache")
	}
}

func TestStoreEvictsWhenByteLimitExceeded(t *testing.T) {
	store := NewStoreWithOptions(Options{DefaultTTL: time.Minute, MaxEntries: 10, MaxBytes: 40, MaxEntryBytes: 1024})
	store.Set("a", Entry{StatusCode: 200, Body: []byte("aaaaaaaaaaaaaaaaaaaa")})
	store.Set("b", Entry{StatusCode: 200, Body: []byte("bbbbbbbbbbbbbbbbbbbb")})

	if _, ok := store.Get("a"); ok {
		t.Fatal("expected key a to be evicted when byte limit exceeded")
	}
	if _, ok := store.Get("b"); !ok {
		t.Fatal("expected key b to remain in cache")
	}
}

func TestLookupSupportsVaryHeaders(t *testing.T) {
	store := NewStoreWithOptions(Options{DefaultTTL: time.Minute, MaxEntries: 10, MaxBytes: 1024 * 1024, MaxEntryBytes: 1024})
	policy := Policy{Cacheable: true, ExpiresAt: time.Now().Add(time.Minute), Vary: []string{"Accept-Language"}}

	requestHeaders := http.Header{"Accept-Language": {"en-US"}}
	if _, ok := store.SetWithRequest("GET /products", requestHeaders, Entry{StatusCode: 200, Body: []byte("en")}, policy); !ok {
		t.Fatal("expected SetWithRequest to store entry")
	}

	entry, state, _ := store.Lookup("GET /products", http.Header{"Accept-Language": {"en-US"}})
	if state != StateHit {
		t.Fatalf("state = %s, want %s", state, StateHit)
	}
	if got := string(entry.Body); got != "en" {
		t.Fatalf("body = %s, want en", got)
	}

	_, state, _ = store.Lookup("GET /products", http.Header{"Accept-Language": {"fr-FR"}})
	if state != StateMiss {
		t.Fatalf("state = %s, want %s", state, StateMiss)
	}
}

func TestBuildBaseKeyNormalizesQuery(t *testing.T) {
	u, err := url.Parse("http://example.com/items?b=2&a=3&a=1")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	key := BuildBaseKey("get", u)
	if key != "GET /items?a=1&a=3&b=2" {
		t.Fatalf("normalized key = %s", key)
	}
}
