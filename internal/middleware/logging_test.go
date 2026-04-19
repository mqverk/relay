package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"relay/internal/logging"
)

func TestLoggingMiddlewareEmitsResponseBytesAndCacheFields(t *testing.T) {
	var logs bytes.Buffer
	logger := logging.NewWithWriter(&logs, "info", true)

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("X-Cache-Detail", "STALE")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/items?limit=1", nil)
	req.Header.Set("X-Request-Id", "req-123")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("failed to parse log record: %v", err)
	}

	fields, ok := record["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected fields object in log record")
	}
	if got := fields["status"]; got != float64(http.StatusCreated) {
		t.Fatalf("status = %v, want %d", got, http.StatusCreated)
	}
	if got := fields["response_bytes"]; got != float64(5) {
		t.Fatalf("response_bytes = %v, want 5", got)
	}
	if got := fields["cache"]; got != "HIT" {
		t.Fatalf("cache = %v, want HIT", got)
	}
	if got := fields["cache_detail"]; got != "STALE" {
		t.Fatalf("cache_detail = %v, want STALE", got)
	}
	if got := fields["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
}

func TestLoggingMiddlewareDefaultsStatusTo200(t *testing.T) {
	var logs bytes.Buffer
	logger := logging.NewWithWriter(&logs, "info", false)

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("failed to parse log record: %v", err)
	}
	fields := record["fields"].(map[string]any)
	if got := fields["status"]; got != float64(http.StatusOK) {
		t.Fatalf("status = %v, want 200", got)
	}
	if got := fields["response_bytes"]; got != float64(2) {
		t.Fatalf("response_bytes = %v, want 2", got)
	}
}
