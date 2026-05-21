package skill_test

import (
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
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
