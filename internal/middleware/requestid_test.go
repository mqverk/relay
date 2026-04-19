package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDPreservesInboundHeader(t *testing.T) {
	var seen string
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(requestIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeader, "req-inbound")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if seen != "req-inbound" {
		t.Fatalf("seen request id = %q, want req-inbound", seen)
	}
	if got := rr.Header().Get(requestIDHeader); got != "req-inbound" {
		t.Fatalf("response request id = %q, want req-inbound", got)
	}
}

func TestRequestIDGeneratesWhenMissing(t *testing.T) {
	var seen string
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(requestIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if seen == "" {
		t.Fatal("expected generated request id in handler")
	}
	if got := rr.Header().Get(requestIDHeader); got == "" {
		t.Fatal("expected generated request id in response")
	} else if got != seen {
		t.Fatalf("response request id = %q, want %q", got, seen)
	}
}
