package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"relay/internal/cache"
	"relay/internal/metrics"
)

// HealthHandler builds a health endpoint handler with uptime details.
func HealthHandler(startedAt time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"uptime_seconds": int64(time.Since(startedAt).Seconds()),
		})
	})
}

// ReadinessHandler builds a readiness endpoint handler.
func ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ready"})
	})
}

// CacheAdminHandler builds the cache admin endpoint handler.
func CacheAdminHandler(store *cache.Store, metricsReg *metrics.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			stats := store.Stats()
			metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{Entries: stats.Entries, SizeBytes: stats.SizeBytes, Hits: stats.Hits, Misses: stats.Misses, HitRatio: stats.HitRatio})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"entries":    stats.Entries,
				"size_bytes": stats.SizeBytes,
				"hits":       stats.Hits,
				"misses":     stats.Misses,
				"stale_hits": stats.StaleHits,
				"evictions":  stats.Evictions,
				"hit_ratio":  stats.HitRatio,
			})
		case http.MethodDelete:
			w.Header().Set("Content-Type", "application/json")
			baseKey := strings.TrimSpace(r.URL.Query().Get("base_key"))
			if baseKey == "" {
				store.Clear()
				metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{})
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "cache_cleared", "scope": "all"})
				return
			}

			removed := store.DeleteBaseKey(baseKey)
			stats := store.Stats()
			metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{Entries: stats.Entries, SizeBytes: stats.SizeBytes, Hits: stats.Hits, Misses: stats.Misses, HitRatio: stats.HitRatio})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":         "cache_cleared",
				"scope":          "base_key",
				"base_key":       baseKey,
				"removed_entries": removed,
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
