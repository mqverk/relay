package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterMiddleware(t *testing.T) {
	limiter := NewRateLimiter(1, 1)
	h := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	srv := httptest.NewServer(h)
	defer srv.Close()

	res1, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request 1 failed: %v", err)
	}
	res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("status 1 = %d, want 200", res1.StatusCode)
	}
	if got := res1.Header.Get("X-RateLimit-Limit"); got != "1" {
		t.Fatalf("X-RateLimit-Limit = %q, want 1", got)
	}
	if got := res1.Header.Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 0", got)
	}

	res2, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request 2 failed: %v", err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status 2 = %d, want 429", res2.StatusCode)
	}
	if got := res2.Header.Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header on rate-limited response")
	}
}

func TestRateLimiterUsesForwardedForWhenTrusted(t *testing.T) {
	limiter := NewRateLimiterWithOptions(RateLimiterOptions{RPS: 1, Burst: 1, TrustProxy: true})
	h := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	status1 := performRateLimitRequest(h, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "1.1.1.1"})
	status2 := performRateLimitRequest(h, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "2.2.2.2"})
	status3 := performRateLimitRequest(h, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "1.1.1.1"})

	if status1 != http.StatusOK {
		t.Fatalf("status1 = %d, want 200", status1)
	}
	if status2 != http.StatusOK {
		t.Fatalf("status2 = %d, want 200", status2)
	}
	if status3 != http.StatusTooManyRequests {
		t.Fatalf("status3 = %d, want 429", status3)
	}
}

func TestRateLimiterIgnoresForwardedForWhenNotTrusted(t *testing.T) {
	limiter := NewRateLimiterWithOptions(RateLimiterOptions{RPS: 1, Burst: 1, TrustProxy: false})
	h := limiter.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	status1 := performRateLimitRequest(h, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "1.1.1.1"})
	status2 := performRateLimitRequest(h, "127.0.0.1:12345", map[string]string{"X-Forwarded-For": "2.2.2.2"})

	if status1 != http.StatusOK {
		t.Fatalf("status1 = %d, want 200", status1)
	}
	if status2 != http.StatusTooManyRequests {
		t.Fatalf("status2 = %d, want 429", status2)
	}
}

func performRateLimitRequest(h http.Handler, remoteAddr string, headers map[string]string) int {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(rr, req)
	return rr.Code
}
