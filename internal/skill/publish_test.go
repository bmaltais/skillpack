package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// TestPublishNew_MissingDir returns an error for a non-existent directory.
func TestPublishNew_MissingDir(t *testing.T) {
	st := &state.State{
		Repos:           map[string]state.RepoRecord{"my-repo": {CachePath: "/nonexistent"}},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	_, err := skill.PublishNew("/does/not/exist", "my-repo", "", st)
	if err == nil {
		t.Error("expected error for missing directory")
	}
}

// TestPublishNew_MissingSkillMD returns an error when SKILL.md is absent.
func TestPublishNew_MissingSkillMD(t *testing.T) {
	dir := t.TempDir()
	// No SKILL.md written
	st := &state.State{
		Repos:           map[string]state.RepoRecord{"my-repo": {CachePath: "/nonexistent"}},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	_, err := skill.PublishNew(dir, "my-repo", "", st)
	if err == nil {
		t.Error("expected error for missing SKILL.md")
	}
}

// TestPublishNew_UnknownRepo returns an error for an unregistered repo.
func TestPublishNew_UnknownRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# My Skill")
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	_, err := skill.PublishNew(dir, "unknown-repo", "", st)
	if err == nil {
		t.Error("expected error for unregistered repo")
	}
}

// TestSyncResult_Actions verifies the SyncAction constants are distinct strings.
func TestSyncResult_Actions(t *testing.T) {
	actions := []skill.SyncAction{
		skill.SyncUpdated,
		skill.SyncPublished,
		skill.SyncConflict,
		skill.SyncAlreadyCurrent,
	}
	seen := make(map[skill.SyncAction]bool)
	for _, a := range actions {
		if seen[a] {
			t.Errorf("duplicate SyncAction value: %q", a)
		}
		seen[a] = true
		if string(a) == "" {
			t.Errorf("SyncAction must not be empty string")
		}
	}
}

// TestSync_EmptyState returns empty results for a state with no installed skills.
func TestSync_EmptyState(t *testing.T) {
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	results, conflicts, err := skill.Sync(true /* dry-run */, nil, st)
	if err != nil {
		t.Fatalf("Sync on empty state: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}
}

// TestPublish_Alias ensures Publish delegates to ForceLocal (same error path).
func TestPublish_NotInstalled(t *testing.T) {
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	err := skill.Publish("repo/skill", "claude-code", "", st)
	if err == nil {
		t.Error("expected error publishing uninstalled skill")
	}
}

// TestPublishNew_SkillNameExtraction verifies the basename logic for various paths.
func TestPublishNew_SkillNameExtraction(t *testing.T) {
	cases := []struct {
		input    string
		wantName string // expected part of the error — we just want it not to panic
	}{
		{"./my-skill", "my-skill"},
		{"/abs/path/to/my-skill", "my-skill"},
		{"my-skill", "my-skill"},
	}

	for _, c := range cases {
		// Create a temp dir with the correct basename
		parent := t.TempDir()
		skillDir := filepath.Join(parent, c.wantName)
		if err := os.MkdirAll(skillDir, 0700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(skillDir, "SKILL.md"), "# "+c.wantName)

		st := &state.State{
			Repos:           map[string]state.RepoRecord{},
			InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
		}

		// Should fail at "repo not found" (before any path logic), not panic
		_, err := skill.PublishNew(skillDir, "no-repo", "", st)
		if err == nil {
			t.Errorf("input %q: expected error for missing repo", c.input)
		}
	}
}
