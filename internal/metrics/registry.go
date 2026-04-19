package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type requestKey struct {
	Method string
	Status int
	Cache  string
}

// CacheSnapshot captures cache metrics for exposition.
type CacheSnapshot struct {
	Entries   int
	SizeBytes int64
	Hits      int64
	Misses    int64
	HitRatio  float64
}

// Registry stores counters and latency histograms for relay.
type Registry struct {
	mu sync.Mutex

	requests map[requestKey]int64

	latencyBuckets []float64
	latencyCounts  []int64
	latencyCount   int64
	latencySum     float64

	cache CacheSnapshot
}

// New creates a metrics registry with default histogram buckets.
func New() *Registry {
	buckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	return &Registry{
		requests:       make(map[requestKey]int64),
		latencyBuckets: buckets,
		latencyCounts:  make([]int64, len(buckets)),
	}
}

// RecordRequest updates request and latency metrics.
func (r *Registry) RecordRequest(method string, status int, cacheStatus string, duration time.Duration) {
	r.mu.Lock()
	r.requests[requestKey{Method: method, Status: status, Cache: cacheStatus}]++

	seconds := duration.Seconds()
	r.latencyCount++
	r.latencySum += seconds
	for i, bucket := range r.latencyBuckets {
		if seconds <= bucket {
			r.latencyCounts[i]++
		}
	}
	r.mu.Unlock()
}

// SetCacheSnapshot updates cache gauges/counters.
func (r *Registry) SetCacheSnapshot(snapshot CacheSnapshot) {
	r.mu.Lock()
	r.cache = snapshot
	r.mu.Unlock()
}

// Handler returns an HTTP handler exposing Prometheus-compatible metrics.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(r.RenderPrometheus()))
	})
}

// RenderPrometheus renders metrics in Prometheus text format.
func (r *Registry) RenderPrometheus() string {
	r.mu.Lock()
	requestCopy := make(map[requestKey]int64, len(r.requests))
	for k, v := range r.requests {
		requestCopy[k] = v
	}
	bucketCopy := append([]float64(nil), r.latencyBuckets...)
	countCopy := append([]int64(nil), r.latencyCounts...)
	latencyCount := r.latencyCount
	latencySum := r.latencySum
	cache := r.cache
	r.mu.Unlock()

	var b strings.Builder
	b.WriteString("# HELP relay_requests_total Total HTTP requests processed by relay.\n")
	b.WriteString("# TYPE relay_requests_total counter\n")

	keys := make([]requestKey, 0, len(requestCopy))
	for k := range requestCopy {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		if keys[i].Status != keys[j].Status {
			return keys[i].Status < keys[j].Status
		}
		return keys[i].Cache < keys[j].Cache
	})
	for _, k := range keys {
		v := requestCopy[k]
		b.WriteString(fmt.Sprintf("relay_requests_total{method=%q,status=%q,cache=%q} %d\n", k.Method, fmt.Sprintf("%d", k.Status), k.Cache, v))
	}

	b.WriteString("# HELP relay_request_duration_seconds Request latency histogram.\n")
	b.WriteString("# TYPE relay_request_duration_seconds histogram\n")
	for i, bucket := range bucketCopy {
		b.WriteString(fmt.Sprintf("relay_request_duration_seconds_bucket{le=%q} %d\n", fmt.Sprintf("%g", bucket), countCopy[i]))
	}
	b.WriteString(fmt.Sprintf("relay_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", latencyCount))
	b.WriteString(fmt.Sprintf("relay_request_duration_seconds_sum %g\n", latencySum))
	b.WriteString(fmt.Sprintf("relay_request_duration_seconds_count %d\n", latencyCount))

	b.WriteString("# HELP relay_cache_entries Number of entries currently in cache.\n")
	b.WriteString("# TYPE relay_cache_entries gauge\n")
	b.WriteString(fmt.Sprintf("relay_cache_entries %d\n", cache.Entries))

	b.WriteString("# HELP relay_cache_size_bytes Approximate cache memory usage in bytes.\n")
	b.WriteString("# TYPE relay_cache_size_bytes gauge\n")
	b.WriteString(fmt.Sprintf("relay_cache_size_bytes %d\n", cache.SizeBytes))

	b.WriteString("# HELP relay_cache_hits_total Total cache hits.\n")
	b.WriteString("# TYPE relay_cache_hits_total counter\n")
	b.WriteString(fmt.Sprintf("relay_cache_hits_total %d\n", cache.Hits))

	b.WriteString("# HELP relay_cache_misses_total Total cache misses.\n")
	b.WriteString("# TYPE relay_cache_misses_total counter\n")
	b.WriteString(fmt.Sprintf("relay_cache_misses_total %d\n", cache.Misses))

	b.WriteString("# HELP relay_cache_hit_ratio Cache hit ratio since process start.\n")
	b.WriteString("# TYPE relay_cache_hit_ratio gauge\n")
	b.WriteString(fmt.Sprintf("relay_cache_hit_ratio %g\n", cache.HitRatio))

	return b.String()
}
