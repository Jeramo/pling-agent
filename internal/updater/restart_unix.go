//go:build !windows

package updater

import (
	"os"
	"syscall"
)

func restartExec(exe string) {
	syscall.Exec(exe, os.Args, os.Environ())
	os.Exit(0)
}
