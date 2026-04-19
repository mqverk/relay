package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"relay/internal/errorhandler"
	"relay/internal/logging"
)

func TestRecoverConvertsPanicTo500(t *testing.T) {
	logger := logging.NewWithWriter(io.Discard, "error", true)
	errHandler := errorhandler.New(logger, false)
	h := Recover(errHandler, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	if body := rr.Body.String(); body == "" {
		t.Fatal("expected JSON error body")
	}
}

func TestRecoverDoesNotOverrideWrittenResponse(t *testing.T) {
	logger := logging.NewWithWriter(io.Discard, "error", true)
	errHandler := errorhandler.New(logger, false)
	h := Recover(errHandler, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		panic("late panic")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic-late", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
}
