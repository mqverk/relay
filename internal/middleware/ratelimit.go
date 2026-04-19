package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter enforces per-client token-bucket limits.
type RateLimiter struct {
	mu      sync.Mutex
	rps     float64
	burst   float64
	clients map[string]*bucket
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	if rps <= 0 {
		rps = 100
	}
	if burst <= 0 {
		burst = 200
	}
	return &RateLimiter{
		rps:     rps,
		burst:   float64(burst),
		clients: make(map[string]*bucket),
	}
}

// Middleware returns an HTTP middleware enforcing rate limits.
func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := clientIP(r.RemoteAddr)
			if !rl.allow(clientID, time.Now()) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"Too Many Requests","status_code":429}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(clientID string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.clients[clientID]
	if !ok {
		rl.clients[clientID] = &bucket{tokens: rl.burst - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * rl.rps
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens -= 1
	return true
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
