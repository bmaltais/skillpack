// Package layering holds tests that enforce the Layering rules in CODING_STANDARDS.md.
package layering

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// repoRoot walks up from the test's working directory to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// violations returns "path:line: text" for every line under root/sub in a
// non-test .go file that matches re, skipping files in allow (slash paths
// relative to root).
func violations(t *testing.T, sub string, re *regexp.Regexp, allow map[string]bool) []string {
	t.Helper()
	root := repoRoot(t)
	var found []string
	err := filepath.WalkDir(filepath.Join(root, sub), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if allow[filepath.ToSlash(rel)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				found = append(found, filepath.ToSlash(rel)+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// cmd never saves state: internal operations own persistence.
func TestCmdNeverCallsStateSave(t *testing.T) {
	re := regexp.MustCompile(`(\bstate\.Save|\bst\.Save|\.St\.Save)\(`)
	for _, v := range violations(t, "cmd", re, nil) {
		t.Errorf("cmd must not call state.Save (internal operations own persistence): %s", v)
	}
}

// internalPrintDebt lists files that still print from internal/*. Do not add to it;
// remove an entry when its prints become part of a returned result.
var internalPrintDebt = map[string]bool{
	"internal/skill/skill.go":  true, // "skipping ... (already installed)"
	"internal/skill/update.go": true, // "Nothing to commit"
}

// internal/* never prints: operations return a structured result instead.
func TestInternalNeverPrints(t *testing.T) {
	re := regexp.MustCompile(`\bfmt\.Print|\bprintln\(`)
	for _, v := range violations(t, "internal", re, internalPrintDebt) {
		t.Errorf("internal/* must not print (return a structured result): %s", v)
	}
}
