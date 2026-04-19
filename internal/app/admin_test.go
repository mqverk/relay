package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"relay/internal/cache"
	"relay/internal/metrics"
)

func TestHealthHandler(t *testing.T) {
	h := HealthHandler(time.Now().Add(-5 * time.Second))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_relay/health", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["status"]; got != "ok" {
		t.Fatalf("status = %v, want ok", got)
	}
}

func TestReadinessHandler(t *testing.T) {
	h := ReadinessHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_relay/ready", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["status"]; got != "ready" {
		t.Fatalf("status = %v, want ready", got)
	}
}

func TestCacheAdminHandlerDeleteBaseKey(t *testing.T) {
	store := cache.NewStoreWithOptions(cache.Options{DefaultTTL: time.Minute, MaxEntries: 10, MaxBytes: 1024 * 1024, MaxEntryBytes: 1024})
	reg := metrics.New()
	h := CacheAdminHandler(store, reg)

	baseKey := "GET /products"
	policy := cache.Policy{Cacheable: true, ExpiresAt: time.Now().Add(time.Minute), Vary: []string{"Accept-Language"}}
	_, _ = store.SetWithRequest(baseKey, http.Header{"Accept-Language": {"en-US"}}, cache.Entry{StatusCode: 200, Body: []byte("en")}, policy)
	_, _ = store.SetWithRequest(baseKey, http.Header{"Accept-Language": {"fr-FR"}}, cache.Entry{StatusCode: 200, Body: []byte("fr")}, policy)

	req := httptest.NewRequest(http.MethodDelete, "/_relay/cache?base_key=GET%20%2Fproducts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["scope"]; got != "base_key" {
		t.Fatalf("scope = %v, want base_key", got)
	}
	if got := payload["removed_entries"]; got != float64(2) {
		t.Fatalf("removed_entries = %v, want 2", got)
	}
}

func TestCacheAdminHandlerDeleteAll(t *testing.T) {
	store := cache.NewStoreWithOptions(cache.Options{DefaultTTL: time.Minute, MaxEntries: 10, MaxBytes: 1024 * 1024, MaxEntryBytes: 1024})
	reg := metrics.New()
	h := CacheAdminHandler(store, reg)

	store.Set("GET /products", cache.Entry{StatusCode: 200, Body: []byte("ok")})

	req := httptest.NewRequest(http.MethodDelete, "/_relay/cache", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if store.Len() != 0 {
		t.Fatalf("cache length = %d, want 0", store.Len())
	}
}
