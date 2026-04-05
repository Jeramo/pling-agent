package schedule

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

const maxOutputBytes = 10 * 1024

type Result struct {
	ScheduleID string `json:"schedule_id"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
}

func RunCommand(ctx context.Context, command string, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	output := out.Bytes()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			exitCode = -2 // timeout
			// Truncate first, then append marker so it's never cut off
			timeoutMsg := []byte("\n[pling-agent: command timed out]")
			if len(output) > maxOutputBytes-len(timeoutMsg) {
				output = output[:maxOutputBytes-len(timeoutMsg)]
			}
			output = append(output, timeoutMsg...)
		} else {
			exitCode = -1
		}
	}

	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
	}

	return Result{
		ExitCode: exitCode,
		Output:   string(output),
	}
}
