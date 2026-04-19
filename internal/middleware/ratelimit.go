package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter enforces per-client token-bucket limits.
type RateLimiter struct {
	mu              sync.Mutex
	rps             float64
	burst           float64
	trustProxy      bool
	clients         map[string]*bucket
	bucketTTL       time.Duration
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

// RateLimiterOptions configures a rate limiter.
type RateLimiterOptions struct {
	RPS        float64
	Burst      int
	TrustProxy bool
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return NewRateLimiterWithOptions(RateLimiterOptions{RPS: rps, Burst: burst})
}

// NewRateLimiterWithOptions creates a new rate limiter with extended options.
func NewRateLimiterWithOptions(opts RateLimiterOptions) *RateLimiter {
	if opts.RPS <= 0 {
		opts.RPS = 100
	}
	if opts.Burst <= 0 {
		opts.Burst = 200
	}
	return &RateLimiter{
		rps:        opts.RPS,
		burst:      float64(opts.Burst),
		trustProxy: opts.TrustProxy,
		clients:    make(map[string]*bucket),
		bucketTTL:  10 * time.Minute,
		cleanupInterval: 1 * time.Minute,
	}
}

// Middleware returns an HTTP middleware enforcing rate limits.
func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := rl.clientIDFromRequest(r)
			allowed, remaining, retryAfter := rl.allow(clientID, time.Now())
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(rl.burst)))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"Too Many Requests","status_code":429,"code":"rate_limited"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) clientIDFromRequest(r *http.Request) string {
	if rl.trustProxy {
		if forwarded := firstForwardedFor(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
		realIP := strings.TrimSpace(r.Header.Get("X-Real-Ip"))
		if net.ParseIP(realIP) != nil {
			return realIP
		}
	}
	return clientIP(r.RemoteAddr)
}

func firstForwardedFor(value string) string {
	for _, token := range strings.Split(value, ",") {
		candidate := strings.TrimSpace(token)
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return ""
}

func (rl *RateLimiter) allow(clientID string, now time.Time) (bool, int, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.maybeCleanup(now)

	b, ok := rl.clients[clientID]
	if !ok {
		rl.clients[clientID] = &bucket{tokens: rl.burst - 1, last: now}
		remaining := int(math.Floor(rl.burst - 1))
		if remaining < 0 {
			remaining = 0
		}
		return true, remaining, 0
	}

	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * rl.rps
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	if b.tokens < 1 {
		if rl.rps <= 0 {
			return false, 0, 1
		}
		retryAfter := int(math.Ceil((1 - b.tokens) / rl.rps))
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, 0, retryAfter
	}
	b.tokens -= 1
	remaining := int(math.Floor(b.tokens))
	if remaining < 0 {
		remaining = 0
	}
	return true, remaining, 0
}

func (rl *RateLimiter) maybeCleanup(now time.Time) {
	if rl.cleanupInterval <= 0 || rl.bucketTTL <= 0 {
		return
	}
	if !rl.lastCleanup.IsZero() && now.Sub(rl.lastCleanup) < rl.cleanupInterval {
		return
	}
	cutoff := now.Add(-rl.bucketTTL)
	for key, b := range rl.clients {
		if b.last.Before(cutoff) {
			delete(rl.clients, key)
		}
	}
	rl.lastCleanup = now
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
