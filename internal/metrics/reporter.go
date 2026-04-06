package metrics

import (
	"context"
	"log"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
)

type ReportPayload struct {
	HostID       string  `json:"host_id"`
	CPU          float64 `json:"cpu"`
	MemUsed      int     `json:"mem_used"`
	MemTotal     int     `json:"mem_total"`
	DiskUsed     int     `json:"disk_used"`
	DiskTotal    int     `json:"disk_total"`
	Uptime       int     `json:"uptime"`
	SwapUsed     int     `json:"swap_used"`
	SwapTotal    int     `json:"swap_total"`
	NetBytesSent uint64  `json:"net_bytes_sent"`
	NetBytesRecv uint64  `json:"net_bytes_recv"`
	ProcessCount int     `json:"process_count"`
	Load1        float64 `json:"load_1"`
	Load5        float64 `json:"load_5"`
	Load15       float64 `json:"load_15"`
	Temperature  float64 `json:"temperature"`
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
		HostID:       hostname,
		CPU:          snap.CPU,
		MemUsed:      snap.MemUsed,
		MemTotal:     snap.MemTotal,
		DiskUsed:     snap.DiskUsed,
		DiskTotal:    snap.DiskTotal,
		Uptime:       snap.Uptime,
		SwapUsed:     snap.SwapUsed,
		SwapTotal:    snap.SwapTotal,
		NetBytesSent: snap.NetBytesSent,
		NetBytesRecv: snap.NetBytesRecv,
		ProcessCount: snap.ProcessCount,
		Load1:        snap.Load1,
		Load5:        snap.Load5,
		Load15:       snap.Load15,
		Temperature:  snap.Temperature,
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
