package metrics

import (
	"errors"
	"log"
	"os"
	"runtime"
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
	var failCount int

	percents, err := cpu.Percent(time.Second, false)
	if err != nil {
		log.Printf("[metrics] cpu collection failed: %v", err)
		failCount++
	} else if len(percents) > 0 {
		s.CPU = percents[0]
	}

	v, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("[metrics] memory collection failed: %v", err)
		failCount++
	} else {
		s.MemUsed = int(v.Used / (1024 * 1024))
		s.MemTotal = int(v.Total / (1024 * 1024))
	}

	diskRoot := "/"
	if runtime.GOOS == "darwin" {
		// On macOS, "/" is a read-only APFS snapshot. The writable data volume
		// is at /System/Volumes/Data — use it for accurate user-visible usage.
		if _, err := os.Stat("/System/Volumes/Data"); err == nil {
			diskRoot = "/System/Volumes/Data"
		}
	} else if runtime.GOOS == "windows" {
		diskRoot = os.Getenv("SystemDrive")
		if diskRoot == "" {
			diskRoot = "C:"
		}
		diskRoot += "\\"
	}
	d, err := disk.Usage(diskRoot)
	if err != nil {
		log.Printf("[metrics] disk collection failed: %v", err)
		failCount++
	} else {
		s.DiskUsed = int(d.Used / (1024 * 1024 * 1024))
		s.DiskTotal = int(d.Total / (1024 * 1024 * 1024))
	}

	if failCount == 3 {
		return s, errors.New("all metric collectors failed")
	}

	return s, nil
}
