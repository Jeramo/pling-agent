package metrics

import (
	"context"
	"log"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
)

type ReportPayload struct {
	HostID    string  `json:"host_id"`
	CPU       float64 `json:"cpu"`
	MemUsed   int     `json:"mem_used"`
	MemTotal  int     `json:"mem_total"`
	DiskUsed  int     `json:"disk_used"`
	DiskTotal int     `json:"disk_total"`
}

func StartLoop(ctx context.Context, client *api.Client, hostname string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	report(client, hostname)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report(client, hostname)
		}
	}
}

func report(client *api.Client, hostname string) {
	snap, err := Collect()
	if err != nil {
		log.Printf("[metrics] collect failed: %v", err)
		return
	}

	payload := ReportPayload{
		HostID:    hostname,
		CPU:       snap.CPU,
		MemUsed:   snap.MemUsed,
		MemTotal:  snap.MemTotal,
		DiskUsed:  snap.DiskUsed,
		DiskTotal: snap.DiskTotal,
	}

	_, status, err := client.Post("/api/metrics", payload)
	if err != nil {
		log.Printf("[metrics] report failed: %v", err)
		return
	}
	if status != 200 {
		log.Printf("[metrics] report returned status %d", status)
	}
}
