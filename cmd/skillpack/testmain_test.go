package main

import (
	"os"
	"testing"
)

// TestMain redirects HOME to a temporary directory under /tmp before any test
// in this package runs. This guarantees that no test can accidentally write to
// the developer's real ~/.skillpack.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "skillpack-test-*")
	if err != nil {
		panic("failed to create test home dir: " + err.Error())
	}
	os.Setenv("HOME", tmp)
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
