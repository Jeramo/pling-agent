package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
	"github.com/jeramo/pling-agent/internal/config"
	"github.com/jeramo/pling-agent/internal/heartbeat"
	"github.com/jeramo/pling-agent/internal/metrics"
	"github.com/jeramo/pling-agent/internal/schedule"
	"github.com/jeramo/pling-agent/internal/share"
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

	hostname := cfg.Hostname()
	interval := time.Duration(cfg.MetricsInterval) * time.Second
	client := api.New(cfg.APIURL, cfg.Token)

	log.Printf("pling-agent %s starting (host=%s, interval=%s)", version, hostname, interval)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(4)
	go func() { defer wg.Done(); metrics.StartLoop(ctx, client, hostname, interval) }()
	go func() { defer wg.Done(); heartbeat.StartLoop(ctx, client, hostname, version) }()
	go func() { defer wg.Done(); schedule.StartLoop(ctx, client) }()
	go func() { defer wg.Done(); share.StartLoop(ctx, client) }()

	<-ctx.Done()
	log.Println("shutting down, waiting for in-flight operations...")

	// Wait for goroutines with a timeout so we don't hang forever
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("graceful shutdown complete")
	case <-time.After(10 * time.Second):
		log.Println("shutdown timed out, exiting")
	}
}
