package main

import (
	"fmt"
	"log"
	"os"

	"relay/internal/cli"
)

func main() {
	cfg, err := cli.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("invalid arguments: %v", err)
	}

	if cfg.ClearCache {
		fmt.Println("cache cleared")
		return
	}

	log.Printf("relay configured: port=%d origin=%s", cfg.Port, cfg.Origin.String())
}
