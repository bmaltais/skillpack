package skill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// ---------------------------------------------------------------------------
// LLM resolution tests
// ---------------------------------------------------------------------------

// TestLLMResolveConflicts_NoConflicts verifies that a skill with no conflict
// markers is left untouched.
func TestLLMResolveConflicts_NoConflicts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# Clean file\nNo conflicts here.")

	st := stateWithInstall("my-fork/debugger", "claude-code", dir, "sha1")

	resolver := func(prompt string) (string, error) {
		t.Error("resolver should not be called when there are no conflict markers")
		return "", nil
	}

	if err := skill.LLMResolveConflicts("my-fork/debugger", "claude-code", resolver, st); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should be unchanged
	data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(data) != "# Clean file\nNo conflicts here." {
		t.Error("file content should not change when there are no conflict markers")
	}
}

// TestLLMResolveConflicts_ResolvesMarkers verifies that conflicted files are
// sent to the resolver and the resolved content is written back.
func TestLLMResolveConflicts_ResolvesMarkers(t *testing.T) {
	dir := t.TempDir()
	conflictContent := "<<<<<<< ours (local)\nour version\n=======\ntheir version\n>>>>>>> theirs (upstream)\n"
	writeFile(t, filepath.Join(dir, "SKILL.md"), conflictContent)

	st := stateWithInstall("my-fork/debugger", "claude-code", dir, "sha1")

	resolvedContent := "# Resolved\nClean content.\n"
	resolver := func(prompt string) (string, error) {
		if !strings.Contains(prompt, "Skill: my-fork/debugger") {
			t.Errorf("prompt should contain skill addr; got: %s", prompt)
		}
		if !strings.Contains(prompt, "<<<<<<< ours") {
			t.Errorf("prompt should contain the conflict markers; got: %s", prompt)
		}
		return resolvedContent, nil
	}

	if err := skill.LLMResolveConflicts("my-fork/debugger", "claude-code", resolver, st); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(data) != resolvedContent {
		t.Errorf("expected resolved content %q; got %q", resolvedContent, string(data))
	}
}

// TestLLMResolveConflicts_ErrorOnRemainingMarkers verifies that the function
// returns an error if the LLM response still contains conflict markers.
func TestLLMResolveConflicts_ErrorOnRemainingMarkers(t *testing.T) {
	dir := t.TempDir()
	conflictContent := "<<<<<<< ours (local)\nour version\n=======\ntheir version\n>>>>>>> theirs (upstream)\n"
	writeFile(t, filepath.Join(dir, "SKILL.md"), conflictContent)

	original, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))

	st := stateWithInstall("my-fork/debugger", "claude-code", dir, "sha1")

	// Resolver returns content that still has conflict markers
	resolver := func(prompt string) (string, error) {
		return "<<<<<<< still broken\nfoo\n=======\nbar\n>>>>>>> end\n", nil
	}

	err := skill.LLMResolveConflicts("my-fork/debugger", "claude-code", resolver, st)
	if err == nil {
		t.Fatal("expected error when LLM response still has conflict markers")
	}
	if !strings.Contains(err.Error(), "conflict markers") {
		t.Errorf("error should mention conflict markers; got: %v", err)
	}

	// File must NOT be overwritten with the bad result
	data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if string(data) != string(original) {
		t.Error("file should not be overwritten when LLM result still has conflict markers")
	}
}

// TestLLMResolveConflicts_NotInstalled returns an error for unknown addr.
func TestLLMResolveConflicts_NotInstalled(t *testing.T) {
	st := emptyState()
	err := skill.LLMResolveConflicts("no-repo/no-skill", "claude-code", func(string) (string, error) { return "", nil }, st)
	if err == nil {
		t.Error("expected error for uninstalled skill")
	}
}

// ---------------------------------------------------------------------------
// Fork state transition tests (no git operations)
// ---------------------------------------------------------------------------

// TestForkedSkillRecord_UpstreamFields verifies that UpstreamAddr and UpstreamSHA
// round-trip correctly through the InstalledSkillRecord struct.
func TestForkedSkillRecord_Fields(t *testing.T) {
	rec := state.InstalledSkillRecord{
		InstalledAtSHA: "abc123",
		InstalledHash:  "sha256:deadbeef",
		LocalPath:      "/tmp/debugger",
		UpstreamAddr:   "matt-skills/debugger",
		UpstreamSHA:    "upstream-sha",
	}

	if rec.UpstreamAddr != "matt-skills/debugger" {
		t.Errorf("expected UpstreamAddr %q; got %q", "matt-skills/debugger", rec.UpstreamAddr)
	}
	if rec.UpstreamSHA != "upstream-sha" {
		t.Errorf("expected UpstreamSHA %q; got %q", "upstream-sha", rec.UpstreamSHA)
	}
}

// TestNonForkedSkillRecord_UpstreamFieldsEmpty verifies that non-forked records
// have empty UpstreamAddr and UpstreamSHA.
func TestNonForkedSkillRecord_UpstreamFieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "content")

	hash, _ := skill.ComputeHash(dir)
	rec := state.InstalledSkillRecord{
		InstalledAtSHA: "sha1",
		InstalledHash:  hash,
		LocalPath:      dir,
	}

	if rec.UpstreamAddr != "" {
		t.Errorf("non-forked record should have empty UpstreamAddr; got %q", rec.UpstreamAddr)
	}
	if rec.UpstreamSHA != "" {
		t.Errorf("non-forked record should have empty UpstreamSHA; got %q", rec.UpstreamSHA)
	}

	// IsModified should still work for non-forked records
	modified, err := skill.IsModified(rec)
	if err != nil {
		t.Fatalf("IsModified: %v", err)
	}
	if modified {
		t.Error("expected not modified")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func stateWithInstall(addr, agentName, localPath, sha string) *state.State {
	st := emptyState()
	st.InstalledSkills[addr] = map[string]state.InstalledSkillRecord{
		agentName: {
			InstalledAtSHA: sha,
			InstalledHash:  "sha256:placeholder",
			LocalPath:      localPath,
		},
	}
	return st
}
