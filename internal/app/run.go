package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"relay/internal/cache"
	"relay/internal/config"
	"relay/internal/proxy"
	"relay/internal/server"
)

// Run executes the relay application and returns a process exit code.
func Run(args []string) int {
	cfg, err := config.Parse(args)
	if err != nil {
		if errors.Is(err, config.ErrHelpRequested) {
			fmt.Print(config.Usage())
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, config.Usage())
		return 2
	}

	store := cache.DefaultStore()

	if cfg.ClearCache {
		cache.ClearDefault()
		log.Println("cache cleared")
		return 0
	}

	if cfg.CacheStats {
		stats := store.Stats()
		fmt.Printf("entries=%d bytes=%d hits=%d misses=%d hit_ratio=%.4f\n", stats.Entries, stats.SizeBytes, stats.Hits, stats.Misses, stats.HitRatio)
		return 0
	}

	h, err := proxy.NewHandler(cfg.Origin, store, log.Default())
	if err != nil {
		log.Printf("create proxy handler: %v", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, cfg.Port, h, log.Default()); err != nil {
		log.Printf("server failed: %v", err)
		return 1
	}

	return 0
}
