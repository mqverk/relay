package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"relay/internal/metrics"
)

func TestMetricsMiddlewareRecordsRequestAndCacheLabels(t *testing.T) {
	reg := metrics.New()
	h := Metrics(reg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("X-Cache-Detail", "STALE")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	h.ServeHTTP(rr, req)

	output := reg.RenderPrometheus()
	if !strings.Contains(output, `relay_requests_total{method="GET",status="200",cache="HIT"} 1`) {
		t.Fatalf("expected relay_requests_total sample, got:\n%s", output)
	}
	if !strings.Contains(output, `relay_cache_decisions_total{state="HIT"} 1`) {
		t.Fatalf("expected relay_cache_decisions_total sample, got:\n%s", output)
	}
	if !strings.Contains(output, `relay_cache_decision_details_total{state="HIT",detail="STALE"} 1`) {
		t.Fatalf("expected relay_cache_decision_details_total sample, got:\n%s", output)
	}
}
