package errors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// Category groups errors by failure domain.
type Category string

const (
	CategoryNetwork  Category = "network"
	CategoryTimeout  Category = "timeout"
	CategoryCache    Category = "cache"
	CategoryConfig   Category = "config"
	CategoryRate     Category = "rate_limit"
	CategoryInternal Category = "internal"
)

// AppError is the normalized error type used by relay.
type AppError struct {
	Category Category
	Code     string
	Message  string
	Cause    error
	Meta     map[string]string
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

// HTTPStatus maps error category to HTTP status code.
func (e *AppError) HTTPStatus() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	switch e.Category {
	case CategoryConfig:
		return http.StatusBadRequest
	case CategoryRate:
		return http.StatusTooManyRequests
	case CategoryTimeout:
		return http.StatusGatewayTimeout
	case CategoryNetwork:
		return http.StatusBadGateway
	case CategoryCache:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Unwrap exposes the wrapped cause.
func (e *AppError) Unwrap() error { return e.Cause }

// New creates an AppError without an underlying cause.
func New(category Category, code, message string) *AppError {
	return &AppError{Category: category, Code: code, Message: message}
}

// Wrap creates an AppError with a wrapped cause.
func Wrap(category Category, code, message string, cause error) *AppError {
	return &AppError{Category: category, Code: code, Message: message, Cause: cause}
}

// WithMeta attaches metadata to an AppError.
func WithMeta(err *AppError, key, value string) *AppError {
	if err == nil {
		return nil
	}
	if err.Meta == nil {
		err.Meta = map[string]string{}
	}
	err.Meta[key] = value
	return err
}

// AsAppError extracts an AppError from an arbitrary error.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// Normalize converts arbitrary errors into AppError for consistent handling.
func Normalize(err error) *AppError {
	if err == nil {
		return New(CategoryInternal, "internal_error", "internal server error")
	}
	if appErr, ok := AsAppError(err); ok {
		return appErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Wrap(CategoryTimeout, "origin_timeout", "origin request timed out", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Wrap(CategoryTimeout, "origin_timeout", "origin request timed out", err)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return Wrap(CategoryNetwork, "dns_failure", "dns resolution failed for origin", err)
	}
	return Wrap(CategoryNetwork, "origin_network_error", "origin request failed", err)
}
