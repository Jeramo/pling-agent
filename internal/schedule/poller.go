package schedule

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
)

type Schedule struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	Interval string `json:"interval"`
}

type ScheduleResponse struct {
	OK        bool       `json:"ok"`
	Schedules []Schedule `json:"schedules"`
}

func StartLoop(ctx context.Context, client *api.Client) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	var schedules []Schedule
	nextRun := map[string]time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, status, err := client.Get("/api/schedules?agent=true")
			if err != nil || status != 200 {
				continue
			}
			var resp ScheduleResponse
			if json.Unmarshal(data, &resp) != nil || !resp.OK {
				continue
			}
			schedules = resp.Schedules

			// Clean up stale entries for deleted schedules
			activeIDs := make(map[string]bool, len(schedules))
			for _, s := range schedules {
				activeIDs[s.ID] = true
			}
			for id := range nextRun {
				if !activeIDs[id] {
					delete(nextRun, id)
				}
			}

			now := time.Now()
			for _, s := range schedules {
				due, exists := nextRun[s.ID]
				if !exists {
					nextRun[s.ID] = now.Add(parseInterval(s.Interval))
					continue
				}
				if now.Before(due) {
					continue
				}

				cmdPreview := s.Command
				if len(cmdPreview) > 80 {
					cmdPreview = cmdPreview[:80] + "..."
				}
				log.Printf("[schedule] running %s: %s", s.ID, cmdPreview)
				result := RunCommand(ctx, s.Command, 5*time.Minute)
				result.ScheduleID = s.ID

				_, _, _ = client.Post("/api/schedule-results", result)
				nextRun[s.ID] = now.Add(parseInterval(s.Interval))
			}
		}
	}
}

func parseInterval(s string) time.Duration {
	switch s {
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "daily":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}
