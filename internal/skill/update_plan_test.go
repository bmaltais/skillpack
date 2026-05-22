package skill_test

import (
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// planItem returns the first plan item for addr+agent, or panics.
func planItem(plan []skill.UpdatePlanItem, addr, agentName string) skill.UpdatePlanItem {
	for _, p := range plan {
		if p.Addr == addr && p.AgentName == agentName {
			return p
		}
	}
	panic("no plan item for " + addr + "/" + agentName)
}

// ─── AlreadyCurrent ───────────────────────────────────────────────────────────

func TestPlanUpdate_AlreadyCurrent(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-abc", "")
	repoHeads := map[string]string{"myrepo": "sha-abc"}

	plan := skill.PlanUpdate(st, repoHeads)

	if len(plan) != 1 {
		t.Fatalf("want 1 plan item, got %d", len(plan))
	}
	if plan[0].Action != skill.UpdateAlreadyCurrent {
		t.Errorf("want UpdateAlreadyCurrent, got %q", plan[0].Action)
	}
}

// ─── UpdateAvailable ─────────────────────────────────────────────────────────

func TestPlanUpdate_UpdateAvailable(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-old", "")
	repoHeads := map[string]string{"myrepo": "sha-new"}

	plan := skill.PlanUpdate(st, repoHeads)

	item := planItem(plan, "myrepo/coding/debugger", "copilot")
	if item.Action != skill.UpdateAvailable {
		t.Errorf("want UpdateAvailable, got %q", item.Action)
	}
}

// ─── LocallyModified ─────────────────────────────────────────────────────────

func TestPlanUpdate_LocallyModified(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-abc", "sha256:000badhash")
	repoHeads := map[string]string{"myrepo": "sha-abc"}

	plan := skill.PlanUpdate(st, repoHeads)

	item := planItem(plan, "myrepo/coding/debugger", "copilot")
	if item.Action != skill.UpdateLocallyModified {
		t.Errorf("want UpdateLocallyModified, got %q", item.Action)
	}
}

// ─── Conflict ────────────────────────────────────────────────────────────────

func TestPlanUpdate_Conflict(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-old", "sha256:000badhash")
	repoHeads := map[string]string{"myrepo": "sha-new"}

	plan := skill.PlanUpdate(st, repoHeads)

	item := planItem(plan, "myrepo/coding/debugger", "copilot")
	if item.Action != skill.UpdateConflict {
		t.Errorf("want UpdateConflict, got %q", item.Action)
	}
}

// ─── Multiple skills ─────────────────────────────────────────────────────────

func TestPlanUpdate_MultipleSkills(t *testing.T) {
	installA := t.TempDir()
	installB := t.TempDir()
	installC := t.TempDir()
	installD := t.TempDir()

	writeFile(t, filepath.Join(installA, "SKILL.md"), "A")
	writeFile(t, filepath.Join(installB, "SKILL.md"), "B")
	writeFile(t, filepath.Join(installC, "SKILL.md"), "C")
	writeFile(t, filepath.Join(installD, "SKILL.md"), "D")

	hashA, _ := skill.ComputeHash(installA)
	hashB, _ := skill.ComputeHash(installB)
	_, _ = skill.ComputeHash(installC) // hash not needed; stale value used directly below
	hashD, _ := skill.ComputeHash(installD)

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo": {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo/a": {"ag": {InstalledAtSHA: "sha1", InstalledHash: hashA, LocalPath: installA}},       // already current
			"repo/b": {"ag": {InstalledAtSHA: "sha-old", InstalledHash: hashB, LocalPath: installB}},    // needs update
			"repo/c": {"ag": {InstalledAtSHA: "sha1", InstalledHash: "sha256:stale", LocalPath: installC}}, // needs publish
			"repo/d": {"ag": {InstalledAtSHA: "sha-old", InstalledHash: hashD + "x", LocalPath: installD}}, // conflict
		},
	}
	repoHeads := map[string]string{"repo": "sha1"}

	plan := skill.PlanUpdate(st, repoHeads)
	if len(plan) != 4 {
		t.Fatalf("want 4 items, got %d", len(plan))
	}

	byAddr := make(map[string]skill.UpdateAction)
	for _, p := range plan {
		byAddr[p.Addr] = p.Action
	}

	expect := map[string]skill.UpdateAction{
		"repo/a": skill.UpdateAlreadyCurrent,
		"repo/b": skill.UpdateAvailable,
		"repo/c": skill.UpdateLocallyModified,
		"repo/d": skill.UpdateConflict,
	}
	for addr, want := range expect {
		if got := byAddr[addr]; got != want {
			t.Errorf("addr %q: want %q, got %q", addr, want, got)
		}
	}
}

// ─── Missing repo in repoHeads ───────────────────────────────────────────────

func TestPlanUpdate_MissingRepo(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-abc", "")
	// repoHeads is empty — myrepo not present
	repoHeads := map[string]string{}

	plan := skill.PlanUpdate(st, repoHeads)

	if len(plan) != 1 {
		t.Fatalf("want 1 plan item, got %d", len(plan))
	}
	if plan[0].Err == nil {
		t.Error("expected error for missing repo")
	}
}

// ─── Empty state ─────────────────────────────────────────────────────────────

func TestPlanUpdate_EmptyState(t *testing.T) {
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	repoHeads := map[string]string{}

	plan := skill.PlanUpdate(st, repoHeads)
	if len(plan) != 0 {
		t.Errorf("want 0 items for empty state, got %d", len(plan))
	}
}
