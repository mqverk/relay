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

	res2, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request 2 failed: %v", err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status 2 = %d, want 429", res2.StatusCode)
	}
}
