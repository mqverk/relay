package errors

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestAppErrorHTTPStatusByCategory(t *testing.T) {
	tests := []struct {
		name     string
		category Category
		want     int
	}{
		{name: "config", category: CategoryConfig, want: http.StatusBadRequest},
		{name: "rate", category: CategoryRate, want: http.StatusTooManyRequests},
		{name: "timeout", category: CategoryTimeout, want: http.StatusGatewayTimeout},
		{name: "network", category: CategoryNetwork, want: http.StatusBadGateway},
		{name: "cache", category: CategoryCache, want: http.StatusServiceUnavailable},
		{name: "internal", category: CategoryInternal, want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.category, "code", "msg")
			if got := err.HTTPStatus(); got != tt.want {
				t.Fatalf("HTTPStatus = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizePreservesAppError(t *testing.T) {
	source := WithMeta(New(CategoryConfig, "invalid_config", "bad config"), "field", "origin")
	normalized := Normalize(source)
	if normalized != source {
		t.Fatal("expected Normalize to return original AppError instance")
	}
}

func TestNormalizeFromDeadlineExceeded(t *testing.T) {
	normalized := Normalize(context.DeadlineExceeded)
	if normalized.Category != CategoryTimeout {
		t.Fatalf("category = %s, want %s", normalized.Category, CategoryTimeout)
	}
	if normalized.Code != "origin_timeout" {
		t.Fatalf("code = %s, want origin_timeout", normalized.Code)
	}
}

func TestNormalizeFromDNSError(t *testing.T) {
	normalized := Normalize(&net.DNSError{Err: "no such host", Name: "example.invalid"})
	if normalized.Category != CategoryNetwork {
		t.Fatalf("category = %s, want %s", normalized.Category, CategoryNetwork)
	}
	if normalized.Code != "dns_failure" {
		t.Fatalf("code = %s, want dns_failure", normalized.Code)
	}
}

func TestNormalizeFromGenericError(t *testing.T) {
	source := errors.New("boom")
	normalized := Normalize(source)
	if normalized.Category != CategoryNetwork {
		t.Fatalf("category = %s, want %s", normalized.Category, CategoryNetwork)
	}
	if normalized.Code != "origin_network_error" {
		t.Fatalf("code = %s, want origin_network_error", normalized.Code)
	}
	if !errors.Is(normalized, source) {
		t.Fatal("expected normalized error to wrap source")
	}
}
