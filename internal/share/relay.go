package share

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/jeramo/pling-agent/internal/api"
)

// Task represents a pending share relay task from the backend.
type Task struct {
	ID           string  `json:"id"`
	RoomID       string  `json:"room_id"`
	WsURL        string  `json:"ws_url"`
	PasswordHash *string `json:"password_hash"`
}

type taskListResponse struct {
	OK    bool   `json:"ok"`
	Tasks []Task `json:"tasks"`
}

// activeRelays tracks running relays so we don't double-start.
var (
	activeMu     sync.Mutex
	activeRelays = map[string]context.CancelFunc{}
)

// StartLoop polls the backend for share tasks and starts PTY relays.
func StartLoop(ctx context.Context, client *api.Client) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Stop all active relays
			activeMu.Lock()
			for id, cancel := range activeRelays {
				cancel()
				delete(activeRelays, id)
			}
			activeMu.Unlock()
			return
		case <-ticker.C:
			pollAndRelay(ctx, client)
		}
	}
}

func pollAndRelay(ctx context.Context, client *api.Client) {
	data, status, err := client.Get("/api/agent/share-tasks")
	if err != nil || status != 200 {
		return
	}

	var resp taskListResponse
	if json.Unmarshal(data, &resp) != nil || !resp.OK {
		return
	}

	for _, task := range resp.Tasks {
		activeMu.Lock()
		_, running := activeRelays[task.ID]
		activeMu.Unlock()

		if running {
			continue
		}

		// Acknowledge the task
		ackBody := map[string]string{"status": "active"}
		client.Post(fmt.Sprintf("/api/agent/share-tasks/%s/ack", task.ID), ackBody)

		// Start relay in background
		relayCtx, cancel := context.WithCancel(ctx)

		activeMu.Lock()
		activeRelays[task.ID] = cancel
		activeMu.Unlock()

		go func(t Task) {
			defer func() {
				activeMu.Lock()
				delete(activeRelays, t.ID)
				activeMu.Unlock()

				// Mark task as completed
				doneBody := map[string]string{"status": "completed"}
				client.Post(fmt.Sprintf("/api/agent/share-tasks/%s/ack", t.ID), doneBody)
			}()

			if err := runRelay(relayCtx, client, t); err != nil {
				log.Printf("[share] relay %s failed: %v", t.ID, err)
				failBody := map[string]string{"status": "failed"}
				client.Post(fmt.Sprintf("/api/agent/share-tasks/%s/ack", t.ID), failBody)
			}
		}(task)
	}
}

func runRelay(ctx context.Context, client *api.Client, task Task) error {
	log.Printf("[share] starting relay for room %s", task.RoomID)

	// Build WebSocket URL with host role and password hash
	wsURL := task.WsURL + "?role=host"
	if task.PasswordHash != nil && *task.PasswordHash != "" {
		wsURL += "&passwordHash=" + *task.PasswordHash
	}

	// Connect WebSocket to the Durable Object
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   32 * 1024,
		WriteBufferSize:  32 * 1024,
	}

	header := http.Header{}
	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer conn.Close()

	// Start a PTY with the user's default shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.CommandContext(ctx, shell, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("pty start: %w", err)
	}
	defer func() {
		ptmx.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Set initial terminal size
	pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	log.Printf("[share] relay active for room %s (shell=%s)", task.RoomID, shell)

	var wg sync.WaitGroup
	relayCtx, relayCancel := context.WithCancel(ctx)
	defer relayCancel()

	// PTY → WebSocket: read from PTY, send as binary to WS
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer relayCancel()
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-relayCtx.Done():
				return
			default:
			}
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("[share] pty read error: %v", err)
				}
				return
			}
			if n > 0 {
				if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					log.Printf("[share] ws write error: %v", err)
					return
				}
			}
		}
	}()

	// WebSocket → PTY: read from WS, write to PTY (guest keystrokes + control messages)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer relayCancel()
		for {
			select {
			case <-relayCtx.Done():
				return
			default:
			}
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("[share] ws read error: %v", err)
				}
				return
			}

			switch msgType {
			case websocket.TextMessage:
				// Could be control messages from guests (forwarded by DO)
				// For now, treat text as terminal input too
				ptmx.Write(data)
			case websocket.BinaryMessage:
				// Guest keystrokes
				ptmx.Write(data)
			}
		}
	}()

	// WebSocket keepalive ping
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-relayCtx.Done():
				return
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	wg.Wait()
	log.Printf("[share] relay ended for room %s", task.RoomID)
	return nil
}
