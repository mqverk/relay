package errorhandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"relay/internal/erroradvisor"
	relayerrors "relay/internal/errors"
	"relay/internal/logging"
)

// Handler centralizes error-to-response mapping and logging.
type Handler struct {
	logger *logging.Logger
	debug  bool
}

// New creates a new error handler.
func New(logger *logging.Logger, debug bool) *Handler {
	return &Handler{logger: logger, debug: debug}
}

// StatusCode maps internal errors to HTTP status codes.
func (h *Handler) StatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	if appErr, ok := relayerrors.AsAppError(err); ok {
		switch appErr.Category {
		case relayerrors.CategoryConfig:
			return http.StatusBadRequest
		case relayerrors.CategoryRate:
			return http.StatusTooManyRequests
		case relayerrors.CategoryTimeout:
			return http.StatusGatewayTimeout
		case relayerrors.CategoryNetwork:
			return http.StatusBadGateway
		case relayerrors.CategoryCache:
			return http.StatusServiceUnavailable
		default:
			return http.StatusInternalServerError
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}

	return http.StatusBadGateway
}

// WriteHTTP writes a consistent JSON error response.
func (h *Handler) WriteHTTP(w http.ResponseWriter, err error) {
	status := h.StatusCode(err)
	suggestion := erroradvisor.Suggest(err)

	payload := map[string]any{
		"error":       http.StatusText(status),
		"status_code": status,
	}
	if appErr, ok := relayerrors.AsAppError(err); ok {
		payload["code"] = appErr.Code
		payload["message"] = appErr.Message
	}
	if h.debug {
		payload["suggestion"] = suggestion
		payload["details"] = err.Error()
	}

	if h.logger != nil {
		h.logger.Error("request failed", map[string]any{"status": status, "error": err.Error(), "suggestion": suggestion})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
