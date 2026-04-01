package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
)

// CheckAndUpdate checks the backend for the latest version and self-updates if outdated.
// Called after each heartbeat. If an update is applied, the process restarts via exec.
func CheckAndUpdate(client *api.Client, currentVersion string) {
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
	_, err = io.Copy(tmp, io.LimitReader(resp.Body, 50*1024*1024))
	tmp.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write: %w", err)
	}

	// Atomic replace: rename new over old
	if err := os.Rename(tmpPath, realExe); err != nil {
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
	// Resolve symlinks manually since filepath.EvalSymlinks may not be available
	info, err := os.Lstat(path)
	if err != nil {
		return path, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := os.Readlink(path)
		if err != nil {
			return path, err
		}
		if !strings.HasPrefix(resolved, "/") {
			// Relative symlink
			dir := path[:strings.LastIndex(path, "/")+1]
			resolved = dir + resolved
		}
		return resolved, nil
	}
	return path, nil
}
