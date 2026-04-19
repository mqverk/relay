package errorhandler

import (
	"encoding/json"
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
	return relayerrors.Normalize(err).HTTPStatus()
}

// WriteHTTP writes a consistent JSON error response.
func (h *Handler) WriteHTTP(w http.ResponseWriter, err error) {
	normalized := relayerrors.Normalize(err)
	status := normalized.HTTPStatus()
	suggestion := erroradvisor.Suggest(normalized)

	payload := map[string]any{
		"error":       http.StatusText(status),
		"status_code": status,
		"code":        normalized.Code,
		"message":     normalized.Message,
	}
	if h.debug {
		payload["category"] = normalized.Category
		if len(normalized.Meta) > 0 {
			payload["meta"] = normalized.Meta
		}
		if suggestion != "" {
			payload["suggestion"] = suggestion
		}
		payload["details"] = normalized.Error()
	}

	if h.logger != nil {
		fields := map[string]any{
			"status":         status,
			"error_code":     normalized.Code,
			"error_category": normalized.Category,
			"error_message":  normalized.Message,
		}
		if suggestion != "" {
			fields["suggestion"] = suggestion
		}
		if h.debug {
			fields["details"] = normalized.Error()
		}
		h.logger.Error("request failed", fields)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
