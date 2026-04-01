package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
	"github.com/jeramo/pling-agent/internal/config"
	"github.com/jeramo/pling-agent/internal/heartbeat"
	"github.com/jeramo/pling-agent/internal/metrics"
	"github.com/jeramo/pling-agent/internal/schedule"
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

	go metrics.StartLoop(ctx, client, hostname, interval)
	go heartbeat.StartLoop(ctx, client, hostname, version)
	go schedule.StartLoop(ctx, client)

	<-ctx.Done()
	log.Println("shutting down")
}
