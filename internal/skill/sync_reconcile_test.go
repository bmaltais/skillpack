package skill_test

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// makeInstalledState builds a minimal State with one installed skill.
// installContent is written to a temp dir and its hash stored in state,
// so the skill starts as unmodified. Pass a different hash via overrideHash
// to simulate a locally modified skill.
func makeInstalledState(t *testing.T, addr, agentName, installContent, installedAtSHA, overrideHash string) (*state.State, string) {
	t.Helper()
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), installContent)

	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	storedHash := hash
	if overrideHash != "" {
		storedHash = overrideHash // force "modified" appearance
	}

	repoName := splitRepoName(addr)
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			repoName: {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: installedAtSHA,
					InstalledHash:  storedHash,
					LocalPath:      installDir,
				},
			},
		},
	}
	return st, hash
}

func splitRepoName(addr string) string {
	for i, c := range addr {
		if c == '/' {
			return addr[:i]
		}
	}
	return addr
}

// planByAddr returns the first plan item for addr+agent, or panics.
func planByAddr(plan []skill.SyncPlanItem, addr, agentName string) skill.SyncPlanItem {
	for _, p := range plan {
		if p.Addr == addr && p.AgentName == agentName {
			return p
		}
	}
	panic("no plan item for " + addr + "/" + agentName)
}

// ─── AlreadyCurrent ───────────────────────────────────────────────────────────

// TestReconcilePlan_AlreadyCurrent: installed SHA matches HEAD, no local edits.
func TestReconcilePlan_AlreadyCurrent(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-abc", "")
	repoHeads := map[string]string{"myrepo": "sha-abc"}

	plan := skill.ReconcilePlan(st, repoHeads)

	if len(plan) != 1 {
		t.Fatalf("want 1 plan item, got %d", len(plan))
	}
	if plan[0].Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent, got %q", plan[0].Action)
	}
}

// ─── NeedsUpdate ─────────────────────────────────────────────────────────────

// TestReconcilePlan_Update: HEAD advanced, no local edits → should update.
func TestReconcilePlan_Update(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-old", "")
	repoHeads := map[string]string{"myrepo": "sha-new"}

	plan := skill.ReconcilePlan(st, repoHeads)

	item := planByAddr(plan, "myrepo/coding/debugger", "copilot")
	if item.Action != skill.SyncUpdated {
		t.Errorf("want SyncUpdated, got %q", item.Action)
	}
}

// ─── NeedsPublish ─────────────────────────────────────────────────────────────

// TestReconcilePlan_Publish: local edits, no upstream change → should publish.
func TestReconcilePlan_Publish(t *testing.T) {
	// Store a wrong hash so IsModified returns true.
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-abc", "sha256:000badhash")
	repoHeads := map[string]string{"myrepo": "sha-abc"} // HEAD unchanged

	plan := skill.ReconcilePlan(st, repoHeads)

	item := planByAddr(plan, "myrepo/coding/debugger", "copilot")
	if item.Action != skill.SyncPublished {
		t.Errorf("want SyncPublished, got %q", item.Action)
	}
}

// ─── Conflict ────────────────────────────────────────────────────────────────

// TestReconcilePlan_Conflict: local edits AND upstream change → conflict.
func TestReconcilePlan_Conflict(t *testing.T) {
	st, _ := makeInstalledState(t, "myrepo/coding/debugger", "copilot", "# Skill", "sha-old", "sha256:000badhash")
	repoHeads := map[string]string{"myrepo": "sha-new"} // HEAD advanced

	plan := skill.ReconcilePlan(st, repoHeads)

	item := planByAddr(plan, "myrepo/coding/debugger", "copilot")
	if item.Action != skill.SyncConflict {
		t.Errorf("want SyncConflict, got %q", item.Action)
	}
}

// ─── Multiple skills ─────────────────────────────────────────────────────────

// TestReconcilePlan_MultipleSkills verifies all four outcomes can appear in one plan.
func TestReconcilePlan_MultipleSkills(t *testing.T) {
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
			"repo/b": {"ag": {InstalledAtSHA: "sha-old", InstalledHash: hashB, LocalPath: installB}},   // needs update (HEAD advanced, no edits)
			"repo/c": {"ag": {InstalledAtSHA: "sha1", InstalledHash: "sha256:stale", LocalPath: installC}}, // needs publish (edits, HEAD same)
			"repo/d": {"ag": {InstalledAtSHA: "sha-old", InstalledHash: hashD + "x", LocalPath: installD}}, // conflict
		},
	}
	repoHeads := map[string]string{"repo": "sha1"}

	plan := skill.ReconcilePlan(st, repoHeads)
	if len(plan) != 4 {
		t.Fatalf("want 4 items, got %d", len(plan))
	}

	byAddr := make(map[string]skill.SyncAction)
	for _, p := range plan {
		byAddr[p.Addr] = p.Action
	}

	expect := map[string]skill.SyncAction{
		"repo/a": skill.SyncAlreadyCurrent,
		"repo/b": skill.SyncUpdated,
		"repo/c": skill.SyncPublished,
		"repo/d": skill.SyncConflict,
	}
	for addr, want := range expect {
		if got := byAddr[addr]; got != want {
			t.Errorf("addr %q: want %q, got %q", addr, want, got)
		}
	}
}

// ─── Second-pass pattern ─────────────────────────────────────────────────────

// TestReconcilePlan_SecondPass demonstrates that a second ReconcilePlan call on
// updated state surfaces the sibling-agent update that the first call missed.
//
// Scenario: two agents (copilot, claude-code) have the same skill installed.
// First pass: copilot is locally modified (→ SyncPublished), claude-code is
// already current. After simulating the publish (state updated with new SHA),
// a second ReconcilePlan call reveals that claude-code now needs updating.
func TestReconcilePlan_SecondPass(t *testing.T) {
	addr := "repo/coding/tool"

	installCopilot := t.TempDir()
	installClaude := t.TempDir()
	writeFile(t, filepath.Join(installCopilot, "SKILL.md"), "local edits")
	writeFile(t, filepath.Join(installClaude, "SKILL.md"), "original")

	hashCopilotModified := "sha256:bad" // simulates local modification
	hashClaude, _ := skill.ComputeHash(installClaude)

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo": {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"copilot": {InstalledAtSHA: "sha1", InstalledHash: hashCopilotModified, LocalPath: installCopilot},
				"claude":  {InstalledAtSHA: "sha1", InstalledHash: hashClaude, LocalPath: installClaude},
			},
		},
	}
	repoHeads := map[string]string{"repo": "sha1"}

	// First pass: copilot should publish, claude is already current.
	plan1 := skill.ReconcilePlan(st, repoHeads)
	sort.Slice(plan1, func(i, j int) bool { return plan1[i].AgentName < plan1[j].AgentName })

	if planByAddr(plan1, addr, "copilot").Action != skill.SyncPublished {
		t.Errorf("first pass copilot: want SyncPublished, got %q", planByAddr(plan1, addr, "copilot").Action)
	}
	if planByAddr(plan1, addr, "claude").Action != skill.SyncAlreadyCurrent {
		t.Errorf("first pass claude: want SyncAlreadyCurrent, got %q", planByAddr(plan1, addr, "claude").Action)
	}

	// Simulate the publish: cache HEAD advanced to sha2.
	repoHeads["repo"] = "sha2"
	// Copilot state updated (now at sha2).
	copilotRec := st.InstalledSkills[addr]["copilot"]
	copilotRec.InstalledAtSHA = "sha2"
	st.InstalledSkills[addr]["copilot"] = copilotRec

	// Second pass: claude should now need updating (HEAD sha2 != claude's sha1).
	plan2 := skill.ReconcilePlan(st, repoHeads)

	if planByAddr(plan2, addr, "claude").Action != skill.SyncUpdated {
		t.Errorf("second pass claude: want SyncUpdated, got %q", planByAddr(plan2, addr, "claude").Action)
	}
}

// ─── Error cases ────────────────────────────────────────────────────────────

// TestReconcilePlan_UnknownRepo: repo not in repoHeads → item has Err set.
func TestReconcilePlan_UnknownRepo(t *testing.T) {
	st, _ := makeInstalledState(t, "unknownrepo/skill", "copilot", "# Skill", "sha-abc", "")
	repoHeads := map[string]string{} // empty — repo not present

	plan := skill.ReconcilePlan(st, repoHeads)

	item := planByAddr(plan, "unknownrepo/skill", "copilot")
	if item.Err == nil {
		t.Error("want Err != nil for unknown repo, got nil")
	}
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent for unknown repo, got %q", item.Action)
	}
}

// TestReconcilePlan_IsModifiedError: LocalPath contains an invalid character (null byte)
// so os.Stat returns EINVAL (not ErrNotExist), causing IsModified to propagate the error.
func TestReconcilePlan_IsModifiedError(t *testing.T) {
	repoName := "myrepo"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			repoName: {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"myrepo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "sha-abc",
					InstalledHash:  "sha256:something",
					LocalPath:      "/invalid\x00path", // null byte → EINVAL, not ErrNotExist
				},
			},
		},
	}
	repoHeads := map[string]string{repoName: "sha-abc"} // same SHA so no upstream change

	plan := skill.ReconcilePlan(st, repoHeads)

	item := planByAddr(plan, "myrepo/coding/debugger", "copilot")
	if item.Err == nil {
		t.Error("want Err != nil when LocalPath is invalid, got nil")
	}
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent on error, got %q", item.Action)
	}
}
