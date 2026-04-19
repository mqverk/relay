package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParsePrecedenceCLIOverEnvOverFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relay.json")
	configJSON := `{
  "port": 7000,
  "origin": "http://file.example",
  "ttl": "10s",
  "log_level": "warn"
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("RELAY_PORT", "8000")
	t.Setenv("RELAY_ORIGIN", "http://env.example")
	t.Setenv("RELAY_TTL", "20s")

	cfg, err := Parse([]string{"--config", configPath, "--port", "9000", "--origin", "http://cli.example"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Port != 9000 {
		t.Fatalf("port = %d, want 9000", cfg.Port)
	}
	if cfg.Origin == nil || cfg.Origin.String() != "http://cli.example" {
		t.Fatalf("origin = %v, want http://cli.example", cfg.Origin)
	}
	if cfg.TTL != 20*time.Second {
		t.Fatalf("ttl = %s, want 20s", cfg.TTL)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("log level = %s, want warn", cfg.LogLevel)
	}
}

func TestParseYAMLConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relay.yaml")
	configYAML := `port: 3100
origin: http://yaml.example
ttl: 15s
log_level: debug
cache_methods:
  - GET
  - HEAD
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Parse([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Port != 3100 {
		t.Fatalf("port = %d, want 3100", cfg.Port)
	}
	if cfg.Origin == nil || cfg.Origin.String() != "http://yaml.example" {
		t.Fatalf("origin = %v, want http://yaml.example", cfg.Origin)
	}
	if cfg.TTL != 15*time.Second {
		t.Fatalf("ttl = %s, want 15s", cfg.TTL)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("log level = %s, want debug", cfg.LogLevel)
	}
}

func TestParseReadsCacheMaxBytesFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relay.json")
	configJSON := `{
  "port": 3200,
  "origin": "http://file.example",
  "cache_max_bytes": 8192
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Parse([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.CacheMaxBytes != 8192 {
		t.Fatalf("cache max bytes = %d, want 8192", cfg.CacheMaxBytes)
	}
}

func TestParseAllowsCacheStatsWithoutOrigin(t *testing.T) {
	cfg, err := Parse([]string{"--cache-stats"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.CacheStats {
		t.Fatal("expected cache-stats mode")
	}
}

func TestParseHelpRequested(t *testing.T) {
	_, err := Parse([]string{"--help"})
	if !errors.Is(err, ErrHelpRequested) {
		t.Fatalf("expected ErrHelpRequested, got %v", err)
	}
}

func TestParseRejectsUnsupportedConfigExtension(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relay.toml")
	if err := os.WriteFile(configPath, []byte("port=3000"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Parse([]string{"--config", configPath})
	if err == nil {
		t.Fatal("expected parse error for unsupported extension")
	}
}

func TestParseUsesEnvConfigPathWhenFlagMissing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relay.yml")
	configYAML := `
port: 4555
origin: http://env-config.example
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("RELAY_CONFIG", configPath)
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Port != 4555 {
		t.Fatalf("port = %d, want 4555", cfg.Port)
	}
	if cfg.Origin == nil || cfg.Origin.String() != "http://env-config.example" {
		t.Fatalf("origin = %v, want http://env-config.example", cfg.Origin)
	}
}

func TestParseReadsCacheMaxBytesFromEnv(t *testing.T) {
	t.Setenv("RELAY_PORT", "3001")
	t.Setenv("RELAY_ORIGIN", "http://env.example")
	t.Setenv("RELAY_CACHE_MAX_BYTES", "16384")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.CacheMaxBytes != 16384 {
		t.Fatalf("cache max bytes = %d, want 16384", cfg.CacheMaxBytes)
	}
}

func TestParseReadsMaxResponseHeaderBytesFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--max-response-header-bytes", "2048"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.MaxResponseHeaderBytes != 2048 {
		t.Fatalf("max response header bytes = %d, want 2048", cfg.MaxResponseHeaderBytes)
	}
}

func TestParseReadsMaxResponseBodyBytesFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--max-response-body-bytes", "4096"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.MaxResponseBodyBytes != 4096 {
		t.Fatalf("max response body bytes = %d, want 4096", cfg.MaxResponseBodyBytes)
	}
}

func TestParseReadsMaxResponseBodyBytesFromEnv(t *testing.T) {
	t.Setenv("RELAY_PORT", "3001")
	t.Setenv("RELAY_ORIGIN", "http://env.example")
	t.Setenv("RELAY_MAX_RESPONSE_BODY_BYTES", "8192")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.MaxResponseBodyBytes != 8192 {
		t.Fatalf("max response body bytes = %d, want 8192", cfg.MaxResponseBodyBytes)
	}
}

func TestParseReadsCacheMaxBytesFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--cache-max-bytes", "4096"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.CacheMaxBytes != 4096 {
		t.Fatalf("cache max bytes = %d, want 4096", cfg.CacheMaxBytes)
	}
}

func TestParseRejectsNonPositiveMaxResponseHeaderBytes(t *testing.T) {
	_, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--max-response-header-bytes", "0"})
	if err == nil {
		t.Fatal("expected parse error for non-positive max response header bytes")
	}
}

func TestParseRejectsNonPositiveMaxResponseBodyBytes(t *testing.T) {
	_, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--max-response-body-bytes", "0"})
	if err == nil {
		t.Fatal("expected parse error for non-positive max response body bytes")
	}
}

func TestParseRejectsNonPositiveCacheMaxBytes(t *testing.T) {
	_, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--cache-max-bytes", "0"})
	if err == nil {
		t.Fatal("expected parse error for non-positive cache max bytes")
	}
}

func TestParseReadsRateLimitTrustProxyFromEnv(t *testing.T) {
	t.Setenv("RELAY_PORT", "3001")
	t.Setenv("RELAY_ORIGIN", "http://env.example")
	t.Setenv("RELAY_RATE_LIMIT_TRUST_PROXY", "true")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.RateLimitTrustProxy {
		t.Fatal("rate limit trust proxy = false, want true")
	}
}

func TestParseReadsRateLimitTrustProxyFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--rate-limit-trust-proxy"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.RateLimitTrustProxy {
		t.Fatal("rate limit trust proxy = false, want true")
	}
}

func TestParseReadsHealthAndReadinessPathsFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--health-path", "/healthz", "--readiness-path", "/readyz"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.HealthPath != "/healthz" {
		t.Fatalf("health path = %q, want /healthz", cfg.HealthPath)
	}
	if cfg.ReadinessPath != "/readyz" {
		t.Fatalf("readiness path = %q, want /readyz", cfg.ReadinessPath)
	}
}

func TestParseReadsHealthAndReadinessPathsFromEnv(t *testing.T) {
	t.Setenv("RELAY_PORT", "3001")
	t.Setenv("RELAY_ORIGIN", "http://env.example")
	t.Setenv("RELAY_HEALTH_PATH", "/health-env")
	t.Setenv("RELAY_READINESS_PATH", "/ready-env")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.HealthPath != "/health-env" {
		t.Fatalf("health path = %q, want /health-env", cfg.HealthPath)
	}
	if cfg.ReadinessPath != "/ready-env" {
		t.Fatalf("readiness path = %q, want /ready-env", cfg.ReadinessPath)
	}
}

func TestParseReadsAdminTokenFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--admin-token", "secret-token"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.AdminToken != "secret-token" {
		t.Fatalf("admin token = %q, want secret-token", cfg.AdminToken)
	}
}

func TestParseReadsAdminTokenFromEnv(t *testing.T) {
	t.Setenv("RELAY_PORT", "3001")
	t.Setenv("RELAY_ORIGIN", "http://env.example")
	t.Setenv("RELAY_ADMIN_TOKEN", "env-secret")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.AdminToken != "env-secret" {
		t.Fatalf("admin token = %q, want env-secret", cfg.AdminToken)
	}
}
