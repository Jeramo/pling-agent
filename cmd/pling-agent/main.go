package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeramo/pling-agent/internal/config"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("pling-agent", version)
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if cfg.Token == "" {
		log.Fatal("no API token configured — set token in config.toml or PLING_TOKEN env var")
	}

	log.Printf("pling-agent %s starting (host=%s, interval=%ds)", version, cfg.Hostname(), cfg.MetricsInterval)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Loops will be wired in later tasks
	_ = ctx

	<-ctx.Done()
	log.Println("shutting down")
}
