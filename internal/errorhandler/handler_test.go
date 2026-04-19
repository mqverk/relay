package errorhandler

import (
	"encoding/json"
	"errors"
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

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if got := payload["code"]; got != "invalid_config" {
		t.Fatalf("code = %v, want invalid_config", got)
	}
	if got := payload["message"]; got != "invalid setting" {
		t.Fatalf("message = %v, want invalid setting", got)
	}
}

func TestWriteHTTPNormalizesGenericError(t *testing.T) {
	h := New(logging.New("error", false), false)
	rr := httptest.NewRecorder()
	h.WriteHTTP(rr, errors.New("origin unreachable"))

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if got := payload["code"]; got != "origin_network_error" {
		t.Fatalf("code = %v, want origin_network_error", got)
	}
}

func TestWriteHTTPNilErrorFallsBackToInternalError(t *testing.T) {
	h := New(logging.New("error", false), false)
	rr := httptest.NewRecorder()
	h.WriteHTTP(rr, nil)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}
