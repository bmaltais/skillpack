package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

func TestComputeHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# My Skill\nDoes things.")
	writeFile(t, filepath.Join(dir, "references", "api.md"), "API details.")

	h1, err := skill.ComputeHash(dir)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	h2, err := skill.ComputeHash(dir)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hash is not deterministic: %q vs %q", h1, h2)
	}
}

func TestComputeHash_ChangesOnEdit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "original content")

	h1, _ := skill.ComputeHash(dir)

	writeFile(t, filepath.Join(dir, "SKILL.md"), "modified content")

	h2, _ := skill.ComputeHash(dir)

	if h1 == h2 {
		t.Error("hash should change when file content changes")
	}
}

func TestComputeHash_ChangesOnNewFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "content")

	h1, _ := skill.ComputeHash(dir)

	writeFile(t, filepath.Join(dir, "extra.md"), "new file")

	h2, _ := skill.ComputeHash(dir)

	if h1 == h2 {
		t.Error("hash should change when a file is added")
	}
}

func TestIsModified_NotModified(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "content")

	hash, _ := skill.ComputeHash(dir)
	rec := state.InstalledSkillRecord{
		InstalledHash: hash,
		LocalPath:     dir,
	}

	modified, err := skill.IsModified(rec)
	if err != nil {
		t.Fatalf("IsModified: %v", err)
	}
	if modified {
		t.Error("expected not modified")
	}
}

func TestIsModified_Modified(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "original")

	hash, _ := skill.ComputeHash(dir)

	// Simulate user editing the file after installation
	writeFile(t, filepath.Join(dir, "SKILL.md"), "edited by user")

	rec := state.InstalledSkillRecord{
		InstalledHash: hash,
		LocalPath:     dir,
	}

	modified, err := skill.IsModified(rec)
	if err != nil {
		t.Fatalf("IsModified: %v", err)
	}
	if !modified {
		t.Error("expected modified")
	}
}

func TestIsModified_MissingDir(t *testing.T) {
	rec := state.InstalledSkillRecord{
		InstalledHash: "sha256:anything",
		LocalPath:     "/nonexistent/path/to/skill",
	}
	modified, err := skill.IsModified(rec)
	if err != nil {
		t.Fatalf("IsModified on missing dir should not error: %v", err)
	}
	if modified {
		t.Error("missing dir should not report as modified")
	}
}

func TestComputeHash_SymlinkRoot(t *testing.T) {
	// Regression test: ComputeHash must not crash when dir is a symlink to a
	// directory (filepath.Walk treats a symlink root as a non-directory via
	// Lstat, causing os.ReadFile to fail with "is a directory").
	realDir := t.TempDir()
	writeFile(t, filepath.Join(realDir, "SKILL.md"), "content")

	linkDir := filepath.Join(t.TempDir(), "symlinked-skill")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	hashViaLink, err := skill.ComputeHash(linkDir)
	if err != nil {
		t.Fatalf("ComputeHash via symlink: %v", err)
	}

	hashViaReal, err := skill.ComputeHash(realDir)
	if err != nil {
		t.Fatalf("ComputeHash via real dir: %v", err)
	}

	if hashViaLink != hashViaReal {
		t.Errorf("hash mismatch: symlink path returned %q, real path returned %q", hashViaLink, hashViaReal)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
