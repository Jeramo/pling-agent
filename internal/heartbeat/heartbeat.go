package heartbeat

import (
	"context"
	"log"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
	"github.com/jeramo/pling-agent/internal/updater"
)

var updateCh = make(chan struct{}, 1)

func StartLoop(ctx context.Context, client *api.Client, hostname, version string, aliases []string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Single goroutine for update checks — prevents unbounded goroutine spawning
	if version != "dev" {
		go func() {
			for range updateCh {
				updater.CheckAndUpdate(client, version)
			}
		}()
	}

	beat(client, hostname, version, aliases)

	for {
		select {
		case <-ctx.Done():
			close(updateCh)
			return
		case <-ticker.C:
			beat(client, hostname, version, aliases)
		}
	}
}

func beat(client *api.Client, hostname, version string, aliases []string) {
	payload := map[string]interface{}{
		"hostname": hostname,
		"version":  version,
		"aliases":  aliases,
	}
	_, status, err := client.Post("/api/agent/heartbeat", payload)
	if err != nil {
		log.Printf("[heartbeat] failed: %v", err)
		return
	}
	if status != 200 {
		log.Printf("[heartbeat] returned status %d", status)
	}

	// Check for updates after each successful heartbeat (non-blocking, single goroutine)
	if version != "dev" {
		select {
		case updateCh <- struct{}{}:
		default:
			// Update check already in progress, skip
		}
	}
}
