package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"relay/internal/cache"
	"relay/internal/errorhandler"
	"relay/internal/logging"
)

func TestHandlerRevalidatesStaleEntryWithNotModified(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.Header().Set("Cache-Control", "max-age=0")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "max-age=0")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("origin-body"))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	res1, _ := mustGet(t, srv.URL+"/products")
	if got := res1.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("first request X-Cache = %q, want %q", got, cacheMiss)
	}

	res2, body := mustGet(t, srv.URL+"/products")
	if got := res2.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("second request X-Cache = %q, want %q", got, cacheHit)
	}
	if got := res2.Header.Get("X-Cache-Detail"); got != "REVALIDATED" {
		t.Fatalf("second request X-Cache-Detail = %q, want REVALIDATED", got)
	}
	if body != "origin-body" {
		t.Fatalf("unexpected body: %s", body)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("origin calls = %d, want 2", got)
	}
}

func TestHandlerServesStaleOnOriginError(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call >= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream error"))
			return
		}
		w.Header().Set("Cache-Control", "max-age=0, stale-if-error=60")
		w.Header().Set("ETag", `"v2"`)
		_, _ = w.Write([]byte("stale-body"))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	_, _ = mustGet(t, srv.URL+"/products")
	res2, body := mustGet(t, srv.URL+"/products")

	if got := res2.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("stale fallback X-Cache = %q, want %q", got, cacheHit)
	}
	if got := res2.Header.Get("X-Cache-Detail"); got != "STALE_IF_ERROR" {
		t.Fatalf("stale fallback detail = %q, want STALE_IF_ERROR", got)
	}
	if body != "stale-body" {
		t.Fatalf("stale fallback body = %q, want stale-body", body)
	}
}

func TestHandlerBypassesCacheForConfiguredHeader(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer origin.Close()

	originURL, _ := url.Parse(origin.URL)
	store := cache.NewStoreWithOptions(cache.Options{DefaultTTL: time.Minute, MaxEntries: 100, MaxBytes: 1024 * 1024, MaxEntryBytes: 64 * 1024})
	logger := logging.NewWithWriter(io.Discard, "debug", true)
	h, err := NewHandlerWithOptions(HandlerOptions{
		Origin:             originURL,
		Cache:              store,
		Logger:             logger,
		ErrorHandler:       errorhandler.New(logger, true),
		CacheMethods:       []string{http.MethodGet},
		CacheBypassHeaders: []string{"Authorization"},
		PolicyDefaults: cache.PolicyDefaults{
			TTL:                  time.Minute,
			StaleWhileRevalidate: time.Second,
			StaleIfError:         time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	req1, _ := http.NewRequest(http.MethodGet, srv.URL+"/items", nil)
	req1.Header.Set("Authorization", "Bearer token")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("request 1 failed: %v", err)
	}
	res1.Body.Close()

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/items", nil)
	req2.Header.Set("Authorization", "Bearer token")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request 2 failed: %v", err)
	}
	res2.Body.Close()

	if got := res1.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("request 1 X-Cache = %q, want MISS", got)
	}
	if got := res2.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("request 2 X-Cache = %q, want MISS", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("origin calls = %d, want 2", got)
	}
}

func mustNewAdvancedHandler(t *testing.T, originRaw string) *Handler {
	t.Helper()
	originURL, err := url.Parse(originRaw)
	if err != nil {
		t.Fatalf("parse origin URL: %v", err)
	}
	store := cache.NewStoreWithOptions(cache.Options{DefaultTTL: time.Minute, MaxEntries: 100, MaxBytes: 1024 * 1024, MaxEntryBytes: 64 * 1024})
	logger := logging.NewWithWriter(io.Discard, "debug", true)
	h, err := NewHandlerWithOptions(HandlerOptions{
		Origin:       originURL,
		Cache:        store,
		Logger:       logger,
		ErrorHandler: errorhandler.New(logger, true),
		CacheMethods: []string{http.MethodGet},
		PolicyDefaults: cache.PolicyDefaults{
			TTL:                  0,
			StaleWhileRevalidate: 0,
			StaleIfError:         time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	return h
}
