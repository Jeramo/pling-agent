package schedule

import (
	"bytes"
	"context"
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
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return Result{
		ExitCode: exitCode,
		Output:   string(output),
	}
}
