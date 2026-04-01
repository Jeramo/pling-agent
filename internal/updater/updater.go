package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
)

var updateMu sync.Mutex

// CheckAndUpdate checks the backend for the latest version and self-updates if outdated.
// Called after each heartbeat. If an update is applied, the process restarts via exec.
func CheckAndUpdate(client *api.Client, currentVersion string) {
	if !updateMu.TryLock() {
		return // another update is already in progress
	}
	defer updateMu.Unlock()
	latest, err := fetchLatestVersion(client)
	if err != nil {
		log.Printf("[updater] failed to check version: %v", err)
		return
	}

	if latest == "" || latest == currentVersion {
		return
	}

	log.Printf("[updater] update available: %s → %s", currentVersion, latest)

	if err := downloadAndReplace(latest); err != nil {
		log.Printf("[updater] failed to update: %v", err)
		return
	}

	log.Printf("[updater] updated to %s, restarting...", latest)
	restart()
}

func fetchLatestVersion(client *api.Client) (string, error) {
	body, status, err := client.Get("/api/agent/version")
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("status %d", status)
	}

	var resp struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.Version, nil
}

func downloadAndReplace(version string) error {
	// Determine download URL from GitHub releases
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	url := fmt.Sprintf(
		"https://github.com/Jeramo/pling-agent/releases/download/v%s/pling-agent-%s-%s",
		version, goos, goarch,
	)

	log.Printf("[updater] downloading %s", url)

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	// Write to temp file next to the current binary
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}

	// Follow symlinks to get the real path
	realExe, err := evalSymlinks(exe)
	if err != nil {
		realExe = exe
	}

	tmpPath := realExe + ".update"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	// Limit download size to 50 MB
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, 50*1024*1024))
	tmp.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write: %w", err)
	}

	// Reject suspiciously small downloads (truncated/empty)
	if n < 100*1024 {
		os.Remove(tmpPath)
		return fmt.Errorf("download too small (%d bytes), likely corrupted", n)
	}

	// Backup current binary before replacing
	backupPath := realExe + ".backup"
	os.Rename(realExe, backupPath) // best-effort, ignore error

	// Atomic replace: rename new over old
	if err := os.Rename(tmpPath, realExe); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, realExe)
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func restart() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("[updater] cannot find executable for restart: %v", err)
		os.Exit(1)
	}
	// Re-exec ourselves with the same args
	syscall.Exec(exe, os.Args, os.Environ())
	// If exec fails, just exit — the service manager (systemd/launchd) will restart us
	os.Exit(0)
}

func evalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
