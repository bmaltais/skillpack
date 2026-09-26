package testutil_test

import (
	"os"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/testutil"
)

func TestMain(m *testing.M) {
	// Simulate the environment git gives a pre-commit hook.
	os.Setenv("GIT_DIR", "/nonexistent/.git")
	os.Setenv("GIT_INDEX_FILE", "/nonexistent/index")
	os.Exit(testutil.RunWithTempHome(m))
}

func TestRunWithTempHome_ClearsGitEnv(t *testing.T) {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "GIT_") {
			t.Errorf("%s still set; git tests could act on the real repository", name)
		}
	}
}

func TestRunWithTempHome_IsolatesHome(t *testing.T) {
	if !strings.Contains(os.Getenv("HOME"), "skillpack-test-") {
		t.Errorf("HOME = %q, want a skillpack-test temp dir", os.Getenv("HOME"))
	}
}
