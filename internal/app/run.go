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
	"relay/internal/cli"
	"relay/internal/proxy"
	"relay/internal/server"
)

// Run executes the relay application and returns a process exit code.
func Run(args []string) int {
	cfg, err := cli.Parse(args)
	if err != nil {
		if errors.Is(err, cli.ErrHelpRequested) {
			fmt.Print(cli.Usage())
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, cli.Usage())
		return 2
	}

	store := cache.DefaultStore()

	if cfg.ClearCache {
		cache.ClearDefault()
		log.Println("cache cleared")
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
