package cache

import (
	"net/http"
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
