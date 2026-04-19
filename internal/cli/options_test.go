package cli

import (
	"errors"
	"testing"
)

func TestParseValidStartConfig(t *testing.T) {
	cfg, err := Parse([]string{"--port", "3000", "--origin", "http://dummyjson.com"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Port != 3000 {
		t.Fatalf("port = %d, want 3000", cfg.Port)
	}
	if cfg.Origin == nil || cfg.Origin.String() != "http://dummyjson.com" {
		t.Fatalf("origin = %v, want http://dummyjson.com", cfg.Origin)
	}
	if cfg.ClearCache {
		t.Fatal("clear-cache should be false")
	}
}

func TestParseClearCacheConfig(t *testing.T) {
	cfg, err := Parse([]string{"--clear-cache"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.ClearCache {
		t.Fatal("clear-cache should be true")
	}
}

func TestParseRequiresOriginAndPort(t *testing.T) {
	_, err := Parse([]string{"--port", "3000"})
	if err == nil {
		t.Fatal("expected parse error for missing origin")
	}

	_, err = Parse([]string{"--origin", "http://dummyjson.com"})
	if err == nil {
		t.Fatal("expected parse error for missing port")
	}
}

func TestParseHelpRequested(t *testing.T) {
	_, err := Parse([]string{"--help"})
	if !errors.Is(err, ErrHelpRequested) {
		t.Fatalf("expected ErrHelpRequested, got %v", err)
	}
}
