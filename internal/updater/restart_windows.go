//go:build windows

package updater

import (
	"os"
	"os/exec"
	"syscall"
)

func restartExec(exe string) {
	// Windows doesn't support syscall.Exec — start a detached process and exit.
	// CREATE_NEW_PROCESS_GROUP ensures the new process survives the parent's exit
	// and isn't killed when the service manager terminates this process.
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Start()
	os.Exit(0)
}
