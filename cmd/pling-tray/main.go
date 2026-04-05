package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/getlantern/systray"
)

const webUIURL = "http://127.0.0.1:9876"

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(boltIcon())
	systray.SetTooltip("Pling Agent")

	mStatus := systray.AddMenuItem("Checking...", "")
	mStatus.Disable()
	systray.AddSeparator()
	mSettings := systray.AddMenuItem("Settings", "Open the agent settings page")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit tray", "Remove the menu bar icon")

	// Poll agent status every 15s
	go func() {
		for {
			if agentRunning() {
				mStatus.SetTitle("Agent: running")
			} else {
				mStatus.SetTitle("Agent: offline")
			}
			time.Sleep(15 * time.Second)
		}
	}()

	go func() {
		for {
			select {
			case <-mSettings.ClickedCh:
				openBrowser(webUIURL)
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {}

func agentRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(webUIURL + "/api/config")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		fmt.Println("Open", url, "in your browser")
		return
	}
	_ = cmd.Run()
}

// boltIcon generates a 22x22 PNG with a lightning bolt — used as the menu bar icon.
func boltIcon() []byte {
	const s = 22
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	white := color.RGBA{255, 255, 255, 255}

	// Lightning bolt coordinates (hand-tuned for 22x22)
	bolt := [][2]int{
		// Top triangle
		{10, 2}, {11, 2}, {12, 2},
		{9, 3}, {10, 3}, {11, 3},
		{8, 4}, {9, 4}, {10, 4},
		{7, 5}, {8, 5}, {9, 5},
		{6, 6}, {7, 6}, {8, 6},
		{5, 7}, {6, 7}, {7, 7},
		// Middle bar
		{5, 8}, {6, 8}, {7, 8}, {8, 8}, {9, 8}, {10, 8}, {11, 8}, {12, 8}, {13, 8},
		{6, 9}, {7, 9}, {8, 9}, {9, 9}, {10, 9}, {11, 9}, {12, 9}, {13, 9},
		// Bottom triangle
		{12, 10}, {13, 10}, {14, 10},
		{11, 11}, {12, 11}, {13, 11},
		{10, 12}, {11, 12}, {12, 12},
		{9, 13}, {10, 13}, {11, 13},
		{8, 14}, {9, 14}, {10, 14},
		{9, 15}, {10, 15},
		{10, 16},
	}

	for _, p := range bolt {
		if p[0] >= 0 && p[0] < s && p[1] >= 0 && p[1] < s {
			img.Set(p[0], p[1], white)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
