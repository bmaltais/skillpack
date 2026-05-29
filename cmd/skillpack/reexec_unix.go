//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// reexecSelf replaces the current process with the (newly updated) binary,
// preserving all command-line arguments. Uses execve — no new PID.
func reexecSelf() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	return syscall.Exec(execPath, os.Args, os.Environ())
}
