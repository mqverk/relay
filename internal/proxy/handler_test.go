package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"relay/internal/cache"
)

func TestHandlerCachesGETResponses(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Origin", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"from-origin"}`))
	}))
	defer origin.Close()

	originURL, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatalf("parse origin URL: %v", err)
	}

	h, err := NewHandler(originURL, cache.NewStore(0), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	proxyServer := httptest.NewServer(h)
	defer proxyServer.Close()

	res1, body1 := mustGet(t, proxyServer.URL+"/products?limit=10")
	if got := res1.StatusCode; got != http.StatusCreated {
		t.Fatalf("first response status = %d, want %d", got, http.StatusCreated)
	}
	if got := res1.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("first response X-Cache = %q, want %q", got, cacheMiss)
	}
	if got := res1.Header.Get("X-Origin"); got != "ok" {
		t.Fatalf("first response missing origin header, got %q", got)
	}
	if got := body1; got != `{"status":"from-origin"}` {
		t.Fatalf("first response body mismatch: %s", got)
	}

	res2, body2 := mustGet(t, proxyServer.URL+"/products?limit=10")
	if got := res2.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("second response X-Cache = %q, want %q", got, cacheHit)
	}
	if got := body2; got != body1 {
		t.Fatalf("second response body mismatch: %s", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("origin calls = %d, want 1", got)
	}
}

func TestHandlerUsesMethodAndQueryInCacheKey(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(r.URL.RawQuery))
	}))
	defer origin.Close()

	originURL, _ := url.Parse(origin.URL)
	h, err := NewHandler(originURL, cache.NewStore(0), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	proxyServer := httptest.NewServer(h)
	defer proxyServer.Close()

	_, _ = mustGet(t, proxyServer.URL+"/items?a=1")
	_, _ = mustGet(t, proxyServer.URL+"/items?a=2")
	res3, _ := mustGet(t, proxyServer.URL+"/items?a=1")

	if got := res3.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("expected third request to be cache hit, got %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("origin calls = %d, want 2", got)
	}
}

func TestHandlerDoesNotCacheNonGETRequests(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("created"))
	}))
	defer origin.Close()

	originURL, _ := url.Parse(origin.URL)
	h, err := NewHandler(originURL, cache.NewStore(0), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	proxyServer := httptest.NewServer(h)
	defer proxyServer.Close()

	res1, _ := mustPost(t, proxyServer.URL+"/products")
	res2, _ := mustPost(t, proxyServer.URL+"/products")

	if got := res1.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("first POST X-Cache = %q, want %q", got, cacheMiss)
	}
	if got := res2.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("second POST X-Cache = %q, want %q", got, cacheMiss)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("origin calls = %d, want 2", got)
	}
}

func mustGet(t *testing.T, target string) (*http.Response, string) {
	t.Helper()
	res, err := http.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return res, string(body)
}

func mustPost(t *testing.T, target string) (*http.Response, string) {
	t.Helper()
	res, err := http.Post(target, "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return res, string(body)
}
