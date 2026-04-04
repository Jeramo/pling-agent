//go:build windows

package updater

import (
	"os"
	"os/exec"
)

func restartExec(exe string) {
	// Windows doesn't support syscall.Exec — start a new process and exit
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	os.Exit(0)
}
