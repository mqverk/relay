package cache

import (
	"net/http"
	"testing"
	"time"
)

func TestPolicyFromResponseHeadersHonorsCacheControl(t *testing.T) {
	now := time.Now()
	header := http.Header{}
	header.Set("Cache-Control", "public, max-age=60, stale-while-revalidate=30, stale-if-error=120")

	policy := PolicyFromResponseHeaders(header, now, PolicyDefaults{TTL: 10 * time.Second})
	if !policy.Cacheable {
		t.Fatal("expected response to be cacheable")
	}
	if got := policy.ExpiresAt.Sub(now).Round(time.Second); got != 60*time.Second {
		t.Fatalf("ttl = %s, want 60s", got)
	}
	if got := policy.StaleWhileRevalidateUntil.Sub(policy.ExpiresAt).Round(time.Second); got != 30*time.Second {
		t.Fatalf("stale-while-revalidate = %s, want 30s", got)
	}
	if got := policy.StaleIfErrorUntil.Sub(policy.ExpiresAt).Round(time.Second); got != 120*time.Second {
		t.Fatalf("stale-if-error = %s, want 120s", got)
	}
}

func TestPolicyFromResponseHeadersNoStore(t *testing.T) {
	policy := PolicyFromResponseHeaders(http.Header{"Cache-Control": {"no-store"}}, time.Now(), PolicyDefaults{TTL: time.Minute})
	if policy.Cacheable {
		t.Fatal("expected no-store to disable caching")
	}
}

func TestPolicyFromResponseHeadersParsesVary(t *testing.T) {
	header := http.Header{}
	header.Set("Vary", "Accept-Encoding, Accept-Language")
	policy := PolicyFromResponseHeaders(header, time.Now(), PolicyDefaults{TTL: time.Minute})

	if len(policy.Vary) != 2 {
		t.Fatalf("vary count = %d, want 2", len(policy.Vary))
	}
	if policy.Vary[0] != "Accept-Encoding" {
		t.Fatalf("vary[0] = %s", policy.Vary[0])
	}
	if policy.Vary[1] != "Accept-Language" {
		t.Fatalf("vary[1] = %s", policy.Vary[1])
	}
}

func TestPolicyFromResponseHeadersNoCacheDisablesStaleWindows(t *testing.T) {
	now := time.Now()
	header := http.Header{}
	header.Set("Cache-Control", "no-cache, stale-while-revalidate=30, stale-if-error=120")

	policy := PolicyFromResponseHeaders(header, now, PolicyDefaults{TTL: time.Minute})
	if !policy.ExpiresAt.Equal(now) {
		t.Fatalf("expires_at should equal now for no-cache")
	}
	if !policy.StaleWhileRevalidateUntil.IsZero() {
		t.Fatal("stale-while-revalidate should be disabled for no-cache")
	}
	if !policy.StaleIfErrorUntil.IsZero() {
		t.Fatal("stale-if-error should be disabled for no-cache")
	}
}

func TestPolicyFromResponseHeadersUsesExpiresHeader(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Minute).Format(http.TimeFormat)
	header := http.Header{}
	header.Set("Expires", expiresAt)

	policy := PolicyFromResponseHeaders(header, now, PolicyDefaults{TTL: 10 * time.Second})
	if !policy.Cacheable {
		t.Fatal("expected policy to be cacheable")
	}
	if got := policy.ExpiresAt.Sub(now); got < 119*time.Second || got > 121*time.Second {
		t.Fatalf("expires ttl = %s, want approximately 2m", got)
	}
}
