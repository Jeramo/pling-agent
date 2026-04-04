package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jeramo/pling-agent/internal/api"
)

var updateMu sync.Mutex

// resolvedExePath is captured at startup before any update replaces the binary.
var resolvedExePath string

func init() {
	exe, err := os.Executable()
	if err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			resolvedExePath = real
		} else {
			resolvedExePath = exe
		}
	}
}

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

	// Install/update the system tray on supported platforms
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		installTray(latest)
	}

	log.Printf("[updater] updated to %s, restarting...", latest)
	restart()
}

func installTray(version string) {
	trayPath := filepath.Join(filepath.Dir(resolvedExePath), "pling-tray")
	url := fmt.Sprintf(
		"https://github.com/Jeramo/pling-agent/releases/download/v%s/pling-tray-%s-%s",
		version, runtime.GOOS, runtime.GOARCH,
	)
	log.Printf("[updater] downloading tray from %s", url)

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Get(url)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		log.Printf("[updater] tray download failed (non-critical)")
		return
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(filepath.Dir(trayPath), "pling-tray-*.tmp")
	if err != nil {
		return
	}
	io.Copy(tmp, io.LimitReader(resp.Body, 50*1024*1024))
	tmp.Close()
	os.Chmod(tmp.Name(), 0755)
	os.Rename(tmp.Name(), trayPath)

	// Sign ad-hoc on macOS so it doesn't get killed
	exec.Command("codesign", "-s", "-", "-f", trayPath).Run()
	log.Printf("[updater] tray installed at %s", trayPath)
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

	// Use the executable path captured at startup
	realExe := resolvedExePath
	if realExe == "" {
		return fmt.Errorf("executable path not resolved at startup")
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

	// Verify SHA-256 checksum from sidecar file
	checksumURL := url + ".sha256"
	log.Printf("[updater] downloading checksum %s", checksumURL)
	csResp, err := (&http.Client{Timeout: 15 * time.Second}).Get(checksumURL)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum download: %w", err)
	}
	defer csResp.Body.Close()

	if csResp.StatusCode != 200 {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum download returned %d", csResp.StatusCode)
	}

	csBody, err := io.ReadAll(io.LimitReader(csResp.Body, 1024))
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum read: %w", err)
	}

	// Parse expected checksum (first hex field, handles "hash  filename" format)
	fields := strings.Fields(string(csBody))
	if len(fields) == 0 {
		os.Remove(tmpPath)
		return fmt.Errorf("empty checksum file")
	}
	expectedHash := strings.TrimSpace(fields[0])
	if len(expectedHash) != 64 {
		os.Remove(tmpPath)
		return fmt.Errorf("invalid checksum format: %q", string(csBody))
	}

	// Compute actual SHA-256 of downloaded binary
	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("reopen temp for checksum: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, tmpFile); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("hash compute: %w", err)
	}
	tmpFile.Close()

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	log.Printf("[updater] checksum verified: %s", actualHash)

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
	// Use the executable path captured at startup (before update replaced binary)
	exe := resolvedExePath
	if exe == "" {
		log.Printf("[updater] cannot find executable for restart")
		os.Exit(1)
	}
	restartExec(exe)
}
