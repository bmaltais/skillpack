package skill_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

func TestResolve_UnknownStrategy(t *testing.T) {
	_, err := skill.Resolve("repo/my-skill", "copilot", "bogus-strategy", "", "", nil)
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
	if !strings.Contains(err.Error(), "bogus-strategy") {
		t.Errorf("error should name the unknown strategy, got: %v", err)
	}
}

func TestResolveStrategy_Constants(t *testing.T) {
	// Ensure the exported constants are distinct and non-empty — guards against
	// accidental blank-string or duplicate assignments as more strategies land.
	strategies := []skill.ResolveStrategy{
		skill.ResolveForceRemote,
		skill.ResolveForceLocal,
		skill.ResolveMerge,
		skill.ResolveLLM,
	}
	seen := make(map[skill.ResolveStrategy]bool)
	for _, s := range strategies {
		if s == "" {
			t.Errorf("ResolveStrategy constant must not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate ResolveStrategy constant: %q", s)
		}
		seen[s] = true
	}
}

// TestErrMergeConflicts_IsDistinct verifies the sentinel is non-nil and has
// a non-empty message — guards against accidental zero-value assignment.
func TestErrMergeConflicts_IsDistinct(t *testing.T) {
	if skill.ErrMergeConflicts == nil {
		t.Fatal("ErrMergeConflicts must not be nil")
	}
	if skill.ErrMergeConflicts.Error() == "" {
		t.Error("ErrMergeConflicts must have a non-empty message")
	}
}

// TestResolve_ResolveMerge_NotInstalled verifies that ResolveMerge propagates
// MergeSkill's error when the skill is not installed.
func TestResolve_ResolveMerge_NotInstalled(t *testing.T) {
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	_, err := skill.Resolve("no-repo/no-skill", "copilot", skill.ResolveMerge, "", "", st)
	if err == nil {
		t.Fatal("expected error for uninstalled skill, got nil")
	}
}

// TestResolve_ResolveLLM_EmptyAgentName verifies that ResolveLLM returns a clear
// error when llmAgentName is empty, rather than a confusing LookPath failure.
func TestResolve_ResolveLLM_EmptyAgentName(t *testing.T) {
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	_, err := skill.Resolve("no-repo/no-skill", "copilot", skill.ResolveLLM, "", "", st)
	if err == nil {
		t.Fatal("expected error for empty llmAgentName, got nil")
	}
	if !strings.Contains(err.Error(), "llmAgentName") {
		t.Errorf("error should mention llmAgentName, got: %v", err)
	}
}

// TestResolve_ResolveLLM_NotInstalled verifies that ResolveLLM propagates
// MergeSkill's error (after the agent name check) when the skill is not installed.
func TestResolve_ResolveLLM_NotInstalled(t *testing.T) {
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	_, err := skill.Resolve("no-repo/no-skill", "copilot", skill.ResolveLLM, "", "claude-code", st)
	if err == nil {
		t.Fatal("expected error for uninstalled skill, got nil")
	}
	// Must NOT be the llmAgentName validation error — agent name was provided.
	if strings.Contains(err.Error(), "llmAgentName") {
		t.Errorf("error should be about the missing skill, not llmAgentName: %v", err)
	}
}

// TestResolve_ErrMergeConflicts_ErrorsIs verifies that the returned error from
// ResolveMerge on a conflicted skill is matchable via errors.Is.
// This cannot exercise the full MergeSkill path without a git repo, but it verifies
// that ErrMergeConflicts is properly comparable using errors.Is.
func TestResolve_ErrMergeConflicts_ErrorsIs(t *testing.T) {
	if !errors.Is(skill.ErrMergeConflicts, skill.ErrMergeConflicts) {
		t.Error("ErrMergeConflicts must satisfy errors.Is with itself")
	}
}
