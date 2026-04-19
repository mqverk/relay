package middleware

import (
	"net/http"
	"time"

	"relay/internal/metrics"
)

// Metrics records request counters and latency histograms.
func Metrics(reg *metrics.Registry) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			reg.RecordRequest(r.Method, rw.status, rw.Header().Get("X-Cache"), rw.Header().Get("X-Cache-Detail"), time.Since(start))
		})
	}
}
