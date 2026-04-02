package heartbeat

import (
	"context"
	"log"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
	"github.com/jeramo/pling-agent/internal/updater"
)

func StartLoop(ctx context.Context, client *api.Client, hostname, version string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	beat(client, hostname, version)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beat(client, hostname, version)
		}
	}
}

func beat(client *api.Client, hostname, version string) {
	payload := map[string]string{
		"hostname": hostname,
		"version":  version,
	}
	_, status, err := client.Post("/api/agent/heartbeat", payload)
	if err != nil {
		log.Printf("[heartbeat] failed: %v", err)
		return
	}
	if status != 200 {
		log.Printf("[heartbeat] returned status %d", status)
	}

	// Check for updates after each successful heartbeat (non-blocking)
	if version != "dev" {
		go updater.CheckAndUpdate(client, version)
	}
}
