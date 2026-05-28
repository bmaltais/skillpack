//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// reexecSelf starts a new process with the updated binary and exits the current
// one. Windows does not support execve (in-place process replacement).
func reexecSelf() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not restart: %w", err)
	}
	os.Exit(0)
}
