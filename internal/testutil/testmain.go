// Package testutil provides shared test helpers for skillpack test packages.
package testutil

import (
	"os"
	"testing"
)

// RunWithTempHome sets HOME to a fresh /tmp directory, runs m, cleans up, and
// returns the exit code. Use it as the sole body of every TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RunWithTempHome(m)) }
//
// This guarantees no test in the package can write to the developer's real
// ~/.skillpack, even if the test forgets to call t.Setenv("HOME", ...).
func RunWithTempHome(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "skillpack-test-*")
	if err != nil {
		panic("failed to create test home dir: " + err.Error())
	}
	defer os.RemoveAll(tmp) // runs on normal return; os.Exit is called by caller

	if err := os.Setenv("HOME", tmp); err != nil {
		panic("failed to set HOME: " + err.Error())
	}
	return m.Run()
}
