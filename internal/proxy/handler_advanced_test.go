package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
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

func TestHandlerDoesNotCacheStatusCreatedByDefault(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	res1, _ := mustGet(t, srv.URL+"/products")
	res2, _ := mustGet(t, srv.URL+"/products")
	if got := res1.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("first response X-Cache = %q, want MISS", got)
	}
	if got := res2.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("second response X-Cache = %q, want MISS", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("origin calls = %d, want 2", got)
	}
}

func TestHandlerReturns304FromCacheOnIfNoneMatch(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("body"))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	_, _ = mustGet(t, srv.URL+"/products")
	res, _ := mustGetWithHeaders(t, srv.URL+"/products", map[string]string{"If-None-Match": `"v1"`})

	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", res.StatusCode)
	}
	if got := res.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("origin calls = %d, want 1", got)
	}
}

func TestHandlerReturns304FromCacheOnIfModifiedSince(t *testing.T) {
	var calls int32
	lastModified := time.Now().UTC().Add(-time.Minute).Format(http.TimeFormat)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("Last-Modified", lastModified)
		_, _ = w.Write([]byte("body"))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	_, _ = mustGet(t, srv.URL+"/products")
	ifModifiedSince := time.Now().UTC().Format(http.TimeFormat)
	res, _ := mustGetWithHeaders(t, srv.URL+"/products", map[string]string{"If-Modified-Since": ifModifiedSince})

	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", res.StatusCode)
	}
	if got := res.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("origin calls = %d, want 1", got)
	}
}

func TestHandlerCoalescesConcurrentIdenticalRequests(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(75 * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte("ok"))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, body, err := doGetWithHeaders(srv.URL+"/products", nil)
			if err != nil {
				errs <- err
				return
			}
			if res.StatusCode != http.StatusOK || body != "ok" {
				errs <- errUnexpectedResponse
				return
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent request failed: %v", err)
		}
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("origin calls = %d, want 1", got)
	}
}

func TestHandlerSeparatesCoalescingForDifferentHeaders(t *testing.T) {
	var calls int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(75 * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("Vary", "Accept-Language")
		_, _ = w.Write([]byte(r.Header.Get("Accept-Language")))
	}))
	defer origin.Close()

	h := mustNewAdvancedHandler(t, origin.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	var wg sync.WaitGroup
	bodies := make(chan string, 2)
	errs := make(chan error, 2)
	langs := []string{"en-US", "fr-FR"}
	for _, lang := range langs {
		lang := lang
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, body, err := doGetWithHeaders(srv.URL+"/products", map[string]string{"Accept-Language": lang})
			if err != nil {
				errs <- err
				return
			}
			if res.StatusCode != http.StatusOK {
				errs <- errUnexpectedResponse
				return
			}
			bodies <- body
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent request failed: %v", err)
		}
	}

	close(bodies)
	seen := map[string]bool{}
	for body := range bodies {
		seen[body] = true
	}
	if !seen["en-US"] || !seen["fr-FR"] {
		t.Fatalf("expected both language variants, got %#v", seen)
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

func mustGetWithHeaders(t *testing.T, target string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
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

var errUnexpectedResponse = &url.Error{Op: "GET", URL: "unexpected", Err: io.ErrUnexpectedEOF}

func doGetWithHeaders(target string, headers map[string]string) (*http.Response, string, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return res, string(body), nil
}
