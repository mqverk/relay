package middleware

import (
	"net/http"
	"time"

	"relay/internal/logging"
)

// Logging logs request metadata and latency.
func Logging(logger *logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			logger.Info("request", map[string]any{
				"method":      r.Method,
				"path":        r.URL.Path,
				"query":       r.URL.RawQuery,
				"status":      rw.status,
				"remote_addr": r.RemoteAddr,
				"latency_ms":  float64(time.Since(start).Microseconds()) / 1000,
				"latency_us":  time.Since(start).Microseconds(),
				"cache":       rw.Header().Get("X-Cache"),
				"cache_detail": rw.Header().Get("X-Cache-Detail"),
				"request_id":  r.Header.Get("X-Request-Id"),
			})
		})
	}
}
