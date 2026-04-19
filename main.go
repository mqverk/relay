package main

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

func main() {
	cfg, err := cli.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelpRequested) {
			fmt.Print(cli.Usage())
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, cli.Usage())
		os.Exit(2)
	}

	store := cache.DefaultStore()

	if cfg.ClearCache {
		cache.ClearDefault()
		log.Println("cache cleared")
		return
	}

	h, err := proxy.NewHandler(cfg.Origin, store, log.Default())
	if err != nil {
		log.Fatalf("create proxy handler: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, cfg.Port, h, log.Default()); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
