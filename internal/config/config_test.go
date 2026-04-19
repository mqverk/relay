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

func TestParseReadsMaxResponseHeaderBytesFromFlag(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3001", "--origin", "http://example.com", "--max-response-header-bytes", "2048"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.MaxResponseHeaderBytes != 2048 {
		t.Fatalf("max response header bytes = %d, want 2048", cfg.MaxResponseHeaderBytes)
	}
}
