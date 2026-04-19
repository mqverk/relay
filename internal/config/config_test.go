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
