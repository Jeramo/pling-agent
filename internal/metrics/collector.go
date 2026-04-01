package metrics

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type Snapshot struct {
	CPU       float64 `json:"cpu"`
	MemUsed   int     `json:"mem_used"`
	MemTotal  int     `json:"mem_total"`
	DiskUsed  int     `json:"disk_used"`
	DiskTotal int     `json:"disk_total"`
}

func Collect() (Snapshot, error) {
	var s Snapshot

	percents, err := cpu.Percent(time.Second, false)
	if err == nil && len(percents) > 0 {
		s.CPU = percents[0]
	}

	v, err := mem.VirtualMemory()
	if err == nil {
		s.MemUsed = int(v.Used / (1024 * 1024))
		s.MemTotal = int(v.Total / (1024 * 1024))
	}

	d, err := disk.Usage("/")
	if err == nil {
		s.DiskUsed = int(d.Used / (1024 * 1024 * 1024))
		s.DiskTotal = int(d.Total / (1024 * 1024 * 1024))
	}

	return s, nil
}
