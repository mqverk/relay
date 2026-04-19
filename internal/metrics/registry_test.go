package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestRegistryRenderPrometheus(t *testing.T) {
	reg := New()
	reg.RecordRequest("GET", 200, "HIT", "STALE", 25*time.Millisecond)
	reg.RecordRequest("GET", 200, "MISS", "", 45*time.Millisecond)
	reg.SetCacheSnapshot(CacheSnapshot{Entries: 10, SizeBytes: 4096, Hits: 8, Misses: 2, HitRatio: 0.8})

	output := reg.RenderPrometheus()
	expected := []string{
		"relay_requests_total",
		"relay_request_duration_seconds_bucket",
		"relay_cache_decisions_total{state=\"HIT\"} 1",
		"relay_cache_decisions_total{state=\"MISS\"} 1",
		"relay_cache_decision_details_total{state=\"HIT\",detail=\"STALE\"} 1",
		"relay_cache_entries 10",
		"relay_cache_size_bytes 4096",
		"relay_cache_hit_ratio 0.8",
	}
	for _, token := range expected {
		if !strings.Contains(output, token) {
			t.Fatalf("expected metrics output to contain %q", token)
		}
	}
}
