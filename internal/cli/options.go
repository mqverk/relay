package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
)

// ErrHelpRequested indicates that the user requested help output.
var ErrHelpRequested = errors.New("help requested")

// Config holds validated command-line configuration.
type Config struct {
	Port       int
	Origin     *url.URL
	ClearCache bool
}

// Usage returns the command help text.
func Usage() string {
	return `relay - HTTP caching proxy

Usage:
	relay --port <number> --origin <url>
	relay --clear-cache

Options:
	--port         Port to listen on (required unless --clear-cache)
	--origin       Base URL of origin server (required unless --clear-cache)
	--clear-cache  Clear in-memory cache and exit
`
}

// Parse validates command-line arguments and returns a typed configuration.
func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("relay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var origin string
	cfg := Config{}

	fs.IntVar(&cfg.Port, "port", 0, "port to listen on")
	fs.StringVar(&origin, "origin", "", "origin base URL")
	fs.BoolVar(&cfg.ClearCache, "clear-cache", false, "clear cache and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelpRequested
		}
		return Config{}, fmt.Errorf("parse flags: %w", err)
	}

	if cfg.ClearCache {
		if origin != "" || cfg.Port != 0 {
			return Config{}, errors.New("--clear-cache cannot be combined with --origin or --port")
		}
		return cfg, nil
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		return Config{}, errors.New("--port must be between 1 and 65535")
	}

	if origin == "" {
		return Config{}, errors.New("--origin is required")
	}

	u, err := url.Parse(origin)
	if err != nil {
		return Config{}, fmt.Errorf("parse --origin: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Config{}, errors.New("--origin must use http or https")
	}
	if u.Host == "" {
		return Config{}, errors.New("--origin must include a host")
	}

	cfg.Origin = u
	return cfg, nil
}
