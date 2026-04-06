package metrics

import (
	"errors"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"
)

type Snapshot struct {
	CPU       float64 `json:"cpu"`
	MemUsed   int     `json:"mem_used"`
	MemTotal  int     `json:"mem_total"`
	DiskUsed  int     `json:"disk_used"`
	DiskTotal int     `json:"disk_total"`
	// New extended metrics (optional — zero means unavailable)
	Uptime       int     `json:"uptime"`        // seconds
	SwapUsed     int     `json:"swap_used"`      // MB
	SwapTotal    int     `json:"swap_total"`     // MB
	NetBytesSent uint64  `json:"net_bytes_sent"` // total bytes sent (cumulative)
	NetBytesRecv uint64  `json:"net_bytes_recv"` // total bytes received (cumulative)
	ProcessCount int     `json:"process_count"`
	Load1        float64 `json:"load_1"`         // 1-minute load average
	Load5        float64 `json:"load_5"`         // 5-minute load average
	Load15       float64 `json:"load_15"`        // 15-minute load average
	Temperature  float64 `json:"temperature"`    // CPU temp in °C (0 = unavailable)
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

	// Extended metrics — best-effort, never fail the collection
	if uptime, err := host.Uptime(); err == nil {
		s.Uptime = int(uptime)
	}

	if sw, err := mem.SwapMemory(); err == nil {
		s.SwapUsed = int(sw.Used / (1024 * 1024))
		s.SwapTotal = int(sw.Total / (1024 * 1024))
	}

	if counters, err := net.IOCounters(false); err == nil && len(counters) > 0 {
		s.NetBytesSent = counters[0].BytesSent
		s.NetBytesRecv = counters[0].BytesRecv
	}

	if pids, err := process.Pids(); err == nil {
		s.ProcessCount = len(pids)
	}

	if avg, err := load.Avg(); err == nil {
		s.Load1 = avg.Load1
		s.Load5 = avg.Load5
		s.Load15 = avg.Load15
	}

	if temps, err := sensors.TemperaturesWithContext(nil); err == nil {
		// Use the first CPU/SoC temperature found
		for _, t := range temps {
			if t.Temperature > 0 {
				s.Temperature = t.Temperature
				break
			}
		}
	}

	return s, nil
}
