package middleware

import (
	"net/http"
	"time"
)

// HookSet allows custom request/response hooks.
type HookSet struct {
	BeforeRequest []func(*http.Request)
	AfterResponse []func(*http.Request, int, time.Duration)
}

// Hooks runs the provided hook callbacks around request processing.
func Hooks(hooks HookSet) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, hook := range hooks.BeforeRequest {
				hook(r)
			}
			start := time.Now()
			rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			latency := time.Since(start)
			for _, hook := range hooks.AfterResponse {
				hook(r, rw.status, latency)
			}
		})
	}
}
