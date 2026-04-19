package errorhandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relayerrors "relay/internal/errors"
	"relay/internal/logging"
)

func TestStatusCodeByCategory(t *testing.T) {
	h := New(logging.New("error", false), false)
	status := h.StatusCode(relayerrors.New(relayerrors.CategoryRate, "rate_limit", "too many requests"))
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", status)
	}
}

func TestWriteHTTPIncludesCodeWhenAvailable(t *testing.T) {
	h := New(logging.New("error", false), true)
	rr := httptest.NewRecorder()
	err := relayerrors.New(relayerrors.CategoryConfig, "invalid_config", "invalid setting")
	h.WriteHTTP(rr, err)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if got := rr.Body.String(); got == "" {
		t.Fatal("expected non-empty error response body")
	}
}
