package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"relay/internal/cache"
	"relay/internal/errorhandler"
	"relay/internal/logging"
	"relay/internal/metrics"
	"relay/internal/middleware"
)

func TestFullRequestLifecycleWithMetrics(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte("ok"))
	}))
	defer origin.Close()

	originURL, _ := url.Parse(origin.URL)
	store := cache.NewStoreWithOptions(cache.Options{DefaultTTL: time.Minute, MaxEntries: 100, MaxBytes: 2 * 1024 * 1024, MaxEntryBytes: 64 * 1024})
	logger := logging.NewWithWriter(io.Discard, "debug", true)
	h, err := NewHandlerWithOptions(HandlerOptions{
		Origin:       originURL,
		Cache:        store,
		Logger:       logger,
		ErrorHandler: errorhandler.New(logger, true),
		CacheMethods: []string{http.MethodGet},
		PolicyDefaults: cache.PolicyDefaults{
			TTL:                  time.Minute,
			StaleWhileRevalidate: 30 * time.Second,
			StaleIfError:         time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	reg := metrics.New()
	mux := http.NewServeMux()
	mux.Handle("/_relay/metrics", reg.Handler())
	mux.Handle("/", h)
	relay := httptest.NewServer(middleware.Chain(mux, middleware.Metrics(reg)))
	defer relay.Close()

	res1, _ := mustGet(t, relay.URL+"/products")
	if got := res1.Header.Get("X-Cache"); got != cacheMiss {
		t.Fatalf("first response X-Cache = %q, want MISS", got)
	}
	res2, _ := mustGet(t, relay.URL+"/products")
	if got := res2.Header.Get("X-Cache"); got != cacheHit {
		t.Fatalf("second response X-Cache = %q, want HIT", got)
	}

	metricsRes, metricsBody := mustGet(t, relay.URL+"/_relay/metrics")
	if metricsRes.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", metricsRes.StatusCode)
	}
	if !strings.Contains(metricsBody, "relay_requests_total") {
		t.Fatal("metrics output missing relay_requests_total")
	}
}
