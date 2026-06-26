package skill_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// makeInstalledState builds a minimal State with one installed skill.
// makeSkillDir creates a temp install directory containing SKILL.md with the
// given content, computes its hash, and returns (dir, realHash, storedHash).
// If overrideHash is non-empty the storedHash differs from realHash, simulating
// a locally modified skill.
func makeSkillDir(t *testing.T, content, overrideHash string) (dir, realHash, storedHash string) {
	t.Helper()
	dir = t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), content)
	var err error
	realHash, err = skill.ComputeHash(dir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	storedHash = realHash
	if overrideHash != "" {
		storedHash = overrideHash
	}
	return dir, realHash, storedHash
}

// installContent is written to a temp dir and its hash stored in state,
// so the skill starts as unmodified. Pass a different hash via overrideHash
// to simulate a locally modified skill.
func makeInstalledState(t *testing.T, addr, agentName, installContent, installedAtSHA, overrideHash string) (*state.State, string) {
	t.Helper()
	installDir, hash, storedHash := makeSkillDir(t, installContent, overrideHash)

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

// ─── Fork skills ─────────────────────────────────────────────────────────────

// makeForkState builds a State with one forked skill.
// upstreamAddr is the full skill address in the upstream origin repo.
// installedAtSHA is from the fork's own repo (irrelevant for upstream detection).
// upstreamSHA is the stored upstream HEAD at fork time.
// Pass overrideHash to simulate a locally modified installed copy.
func makeForkState(t *testing.T, addr, agentName, upstreamAddr, installedAtSHA, upstreamSHA, overrideHash string) *state.State {
	t.Helper()
	installDir, _, storedHash := makeSkillDir(t, "# Forked skill", overrideHash)
	forkRepoName := splitRepoName(addr)
	upstreamRepoName := splitRepoName(upstreamAddr)
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			forkRepoName:     {CachePath: t.TempDir()},
			upstreamRepoName: {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: installedAtSHA,
					InstalledHash:  storedHash,
					LocalPath:      installDir,
					UpstreamAddr:   upstreamAddr,
					UpstreamSHA:    upstreamSHA,
				},
			},
		},
	}
	return st
}

// TestReconcilePlan_Fork_AlreadyCurrent: forked skill, upstream_sha == upstream
// HEAD, no local edits → up-to-date.
// Regression guard: installed_at_sha is a SHA from the fork's own repo and must
// NOT be compared against the upstream HEAD.
func TestReconcilePlan_Fork_AlreadyCurrent(t *testing.T) {
	st := makeForkState(t,
		"my-skills/debugger", "copilot",
		"upstream-skills/tools/debugger",
		"fork-repo-sha-abc",    // installed_at_sha: from my-skills repo — irrelevant
		"upstream-sha-current", // upstream_sha == upstream HEAD
		"",
	)
	repoHeads := map[string]string{
		"my-skills":        "fork-repo-sha-abc",
		"upstream-skills":  "upstream-sha-current", // upstream unchanged
	}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, "my-skills/debugger", "copilot")
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent, got %q (regression: installed_at_sha must not be compared against upstream HEAD)", item.Action)
	}
}

// TestReconcilePlan_Fork_Update: forked skill, upstream_sha != upstream HEAD,
// no local edits → should update.
func TestReconcilePlan_Fork_Update(t *testing.T) {
	st := makeForkState(t,
		"my-skills/debugger", "copilot",
		"upstream-skills/tools/debugger",
		"fork-repo-sha-abc",
		"upstream-sha-old", // upstream_sha is behind
		"",
	)
	repoHeads := map[string]string{
		"my-skills":       "fork-repo-sha-abc",
		"upstream-skills": "upstream-sha-new", // upstream advanced
	}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, "my-skills/debugger", "copilot")
	if item.Action != skill.SyncUpdated {
		t.Errorf("want SyncUpdated, got %q", item.Action)
	}
}

// TestReconcilePlan_Fork_Publish: forked skill, upstream unchanged, local edits
// → should publish.
func TestReconcilePlan_Fork_Publish(t *testing.T) {
	st := makeForkState(t,
		"my-skills/debugger", "copilot",
		"upstream-skills/tools/debugger",
		"fork-repo-sha-abc",
		"upstream-sha-current",
		"sha256:000badhash", // simulate local modification
	)
	repoHeads := map[string]string{
		"my-skills":       "fork-repo-sha-abc",
		"upstream-skills": "upstream-sha-current", // upstream unchanged
	}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, "my-skills/debugger", "copilot")
	if item.Action != skill.SyncPublished {
		t.Errorf("want SyncPublished, got %q", item.Action)
	}
}

// TestReconcilePlan_Fork_Conflict: forked skill, upstream changed AND local
// edits → conflict.
func TestReconcilePlan_Fork_Conflict(t *testing.T) {
	st := makeForkState(t,
		"my-skills/debugger", "copilot",
		"upstream-skills/tools/debugger",
		"fork-repo-sha-abc",
		"upstream-sha-old",
		"sha256:000badhash", // simulate local modification
	)
	repoHeads := map[string]string{
		"my-skills":       "fork-repo-sha-abc",
		"upstream-skills": "upstream-sha-new",
	}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, "my-skills/debugger", "copilot")
	if item.Action != skill.SyncConflict {
		t.Errorf("want SyncConflict, got %q", item.Action)
	}
}

// ─── ApplySync ──────────────────────────────────────────────────────────────

// TestApplySync_AlreadyCurrent: plan items with SyncAlreadyCurrent are returned
// in results without modification.
func TestApplySync_AlreadyCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, _ := makeInstalledState(t, "repo/skill", "copilot", "# S", "sha", "")

	plan := []skill.SyncPlanItem{
		{Addr: "repo/skill", AgentName: "copilot", Action: skill.SyncAlreadyCurrent},
	}
	results, conflicts, err := skill.ApplySync(plan, nil, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("want 0 conflicts, got %d", len(conflicts))
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent, got %q", results[0].Action)
	}
	if results[0].Err != nil {
		t.Errorf("want no error, got %v", results[0].Err)
	}
}

// TestApplySync_CollectsConflicts: SyncConflict plan items go to the conflicts
// slice and do not appear in results.
func TestApplySync_CollectsConflicts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, _ := makeInstalledState(t, "repo/skill", "copilot", "# S", "sha", "")

	plan := []skill.SyncPlanItem{
		{Addr: "repo/skill", AgentName: "copilot", Action: skill.SyncConflict},
	}
	results, conflicts, err := skill.ApplySync(plan, nil, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results, got %d", len(results))
	}
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Action != skill.SyncConflict {
		t.Errorf("want SyncConflict, got %q", conflicts[0].Action)
	}
	if conflicts[0].Addr != "repo/skill" || conflicts[0].AgentName != "copilot" {
		t.Errorf("unexpected conflict identity: %+v", conflicts[0])
	}
}

// TestApplySync_PlanErrorPassthrough: plan items whose Err field is set are
// forwarded as error results without attempting to apply the action.
func TestApplySync_PlanErrorPassthrough(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	planErr := fmt.Errorf("repo not found in cache")
	plan := []skill.SyncPlanItem{
		{Addr: "repo/skill", AgentName: "copilot", Action: skill.SyncAlreadyCurrent, Err: planErr},
	}
	results, conflicts, err := skill.ApplySync(plan, nil, st)
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("want 0 conflicts, got %d", len(conflicts))
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("want Err propagated in result, got nil")
	}
}

// TestApplySync_NilTokenFor: passing nil tokenFor does not panic.
func TestApplySync_NilTokenFor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, _ := makeInstalledState(t, "repo/skill", "copilot", "# S", "sha", "")

	plan := []skill.SyncPlanItem{
		{Addr: "repo/skill", AgentName: "copilot", Action: skill.SyncAlreadyCurrent},
	}
	_, _, err := skill.ApplySync(plan, nil, st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── Phantom conflict ────────────────────────────────────────────────────────

// TestReconcilePlan_PhantomConflict: installed files are byte-identical to the
// upstream cache but InstalledHash is stale and InstalledAtSHA lags behind HEAD.
// ReconcilePlan must return SyncAlreadyCurrent, not SyncConflict.
func TestReconcilePlan_PhantomConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const skillContent = "# Skill content X"
	const addr = "myrepo/coding/debugger"
	const agentName = "copilot"

	// Build installed dir with content X; stale hash simulates a prior
	// --force-remote reset that left InstalledHash out of date.
	st, _ := makeInstalledState(t, addr, agentName, skillContent, "sha-old", "sha256:stale-hash")
	installDir := st.InstalledSkills[addr][agentName].LocalPath

	// Build upstream cache dir at <cachePath>/coding/debugger with the same content X.
	cacheRoot := st.Repos["myrepo"].CachePath
	upstreamDir := filepath.Join(cacheRoot, "coding", "debugger")
	writeFile(t, filepath.Join(upstreamDir, "SKILL.md"), skillContent)

	// HEAD has advanced past InstalledAtSHA, so hasUpstream = true.
	repoHeads := map[string]string{"myrepo": "sha-new"}

	// Sanity: installed and upstream dirs must have identical content hashes.
	installedHash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash(installed): %v", err)
	}
	upstreamHash, err := skill.ComputeHash(upstreamDir)
	if err != nil {
		t.Fatalf("ComputeHash(upstream): %v", err)
	}
	if installedHash != upstreamHash {
		t.Fatalf("test setup error: hashes differ (installed=%s, upstream=%s)", installedHash, upstreamHash)
	}

	plan := skill.ReconcilePlan(st, repoHeads)

	item := planByAddr(plan, addr, agentName)
	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent for phantom conflict, got %q", item.Action)
	}
}

// ─── Spurious-update regression (issue #91) ──────────────────────────────────

// TestReconcilePlan_SpuriousUpdate_UnrelatedCommit: repo HEAD advances due to
// an unrelated commit, but this specific skill's directory content is unchanged.
// ReconcilePlan must return SyncAlreadyCurrent, not SyncUpdated.
func TestReconcilePlan_SpuriousUpdate_UnrelatedCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const skillContent = "# My Skill\nSome content."
	const addr = "my-repo/coding/my-skill"
	const agentName = "copilot"

	// Install dir and upstream cache dir have identical content.
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), skillContent)

	cacheRoot := t.TempDir()
	skillCacheDir := filepath.Join(cacheRoot, "coding", "my-skill")
	writeFile(t, filepath.Join(skillCacheDir, "SKILL.md"), skillContent)

	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: cacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: "sha-old", // repo HEAD has since advanced
					InstalledHash:  hash,
					LocalPath:      installDir,
				},
			},
		},
	}
	// Repo HEAD advanced (unrelated commit elsewhere in the repo).
	repoHeads := map[string]string{"my-repo": "sha-new"}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, addr, agentName)
	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent (skill content unchanged), got %q", item.Action)
	}
}

// TestReconcilePlan_Update_SkillContentChanged: repo HEAD advances AND the
// skill directory content changed in the upstream cache → SyncUpdated.
func TestReconcilePlan_Update_SkillContentChanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const addr = "my-repo/coding/my-skill"
	const agentName = "copilot"

	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Old content")

	cacheRoot := t.TempDir()
	skillCacheDir := filepath.Join(cacheRoot, "coding", "my-skill")
	writeFile(t, filepath.Join(skillCacheDir, "SKILL.md"), "# New content") // changed upstream

	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: cacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: "sha-old",
					InstalledHash:  hash,
					LocalPath:      installDir,
				},
			},
		},
	}
	repoHeads := map[string]string{"my-repo": "sha-new"}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, addr, agentName)
	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action != skill.SyncUpdated {
		t.Errorf("want SyncUpdated (skill content changed), got %q", item.Action)
	}
}

// ─── StaleAddress ─────────────────────────────────────────────────────────────

// TestReconcilePlan_StaleAddress_HeadChanged: HEAD advanced but the skill path
// no longer exists in the repo cache → should classify as SyncStaleAddress.
func TestReconcilePlan_StaleAddress_HeadChanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	addr := "myrepo/coding/gone-skill"
	agentName := "copilot"

	// Install dir exists with content.
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Gone Skill")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Cache root is a simulated git clone (has .git dir) but the skill path is absent.
	cacheRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cacheRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"myrepo": {CachePath: cacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: "sha-old",
					InstalledHash:  hash,
					LocalPath:      installDir,
				},
			},
		},
	}
	repoHeads := map[string]string{"myrepo": "sha-new"}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, addr, agentName)

	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action != skill.SyncStaleAddress {
		t.Errorf("want SyncStaleAddress, got %q", item.Action)
	}
}

// TestReconcilePlan_StaleAddress_HeadUnchanged: when HEAD hasn't changed but
// the skill directory is missing, the skill is reported as stale.
func TestReconcilePlan_StaleAddress_HeadUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	addr := "myrepo/coding/gone-skill"
	agentName := "copilot"

	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Gone Skill")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Simulated git clone without the skill path.
	cacheRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cacheRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"myrepo": {CachePath: cacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: "sha-abc",
					InstalledHash:  hash,
					LocalPath:      installDir,
				},
			},
		},
	}
	// HEAD unchanged — same SHA as InstalledAtSHA.
	repoHeads := map[string]string{"myrepo": "sha-abc"}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, addr, agentName)

	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action != skill.SyncStaleAddress {
		t.Errorf("want SyncStaleAddress, got %q", item.Action)
	}
}

// TestReconcilePlan_StaleAddress_SkillMdMissing: the skill directory exists
// in the cache but has no SKILL.md → treated as stale, not as an update.
func TestReconcilePlan_StaleAddress_SkillMdMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	addr := "myrepo/coding/partial-skill"
	agentName := "copilot"

	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Partial Skill")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Simulated git clone: skill directory exists but has no SKILL.md.
	cacheRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cacheRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheRoot, "coding", "partial-skill"), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"myrepo": {CachePath: cacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				agentName: {
					InstalledAtSHA: "sha-old",
					InstalledHash:  hash,
					LocalPath:      installDir,
				},
			},
		},
	}
	repoHeads := map[string]string{"myrepo": "sha-new"}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, addr, agentName)

	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action != skill.SyncStaleAddress {
		t.Errorf("want SyncStaleAddress, got %q", item.Action)
	}
}

// ─── Fork stale upstream path ─────────────────────────────────────────────────

// TestReconcilePlan_Fork_UpstreamPathMissing: forked skill whose upstream repo
// IS registered but whose upstream tracking path no longer exists in the
// upstream cache (e.g. upstream skill was renamed). The fork itself is valid.
// Expected: NOT SyncStaleAddress — falls back to own-repo evaluation with a
// warning and UpstreamDisabled set.
func TestReconcilePlan_Fork_UpstreamPathMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	forkAddr := "my-skills/diagnose"
	upstreamAddr := "upstream-skills/engineering/diagnose"
	agentName := "copilot"

	// Install dir with SKILL.md.
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Diagnose (fork)")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Fork's own repo cache — plain temp dir (no .git needed for own-repo stale check).
	forkCacheRoot := t.TempDir()

	// Upstream repo cache — simulated git clone (.git exists) but the upstream
	// skill path "engineering/diagnose" is absent (skill was renamed upstream).
	upstreamCacheRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(upstreamCacheRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills":        {CachePath: forkCacheRoot},
			"upstream-skills":  {CachePath: upstreamCacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			forkAddr: {
				agentName: {
					InstalledAtSHA: "fork-sha-abc",
					InstalledHash:  hash,
					LocalPath:      installDir,
					UpstreamAddr:   upstreamAddr,
					UpstreamSHA:    "upstream-sha-abc",
				},
			},
		},
	}
	// Fork's own repo HEAD is unchanged; upstream HEAD is also unchanged.
	repoHeads := map[string]string{
		"my-skills":       "fork-sha-abc",
		"upstream-skills": "upstream-sha-abc",
	}

	plan := skill.ReconcilePlan(st, repoHeads)
	item := planByAddr(plan, forkAddr, agentName)

	if item.Err != nil {
		t.Fatalf("unexpected error: %v", item.Err)
	}
	if item.Action == skill.SyncStaleAddress {
		t.Errorf("fork with missing upstream path must not be SyncStaleAddress; got %q", item.Action)
	}
	// The fork is current in its own repo — expect SyncAlreadyCurrent.
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent, got %q", item.Action)
	}
	if !item.UpstreamDisabled {
		t.Errorf("want UpstreamDisabled=true, got false")
	}
	if item.Warning == "" {
		t.Errorf("want non-empty Warning for broken upstream pointer, got empty")
	}
	for _, want := range []string{"upstream source no longer exists", "skillpack relink"} {
		if !strings.Contains(item.Warning, want) {
			t.Errorf("warning %q does not contain %q", item.Warning, want)
		}
	}
}

// TestReconcilePlan_StaleAddress_DoesNotBlockOtherSkills: a stale skill should
// not prevent other skills in the same plan from being processed correctly.
func TestReconcilePlan_StaleAddress_DoesNotBlockOtherSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	staleAddr := "myrepo/coding/gone-skill"
	goodAddr := "myrepo/coding/live-skill"
	agentName := "copilot"

	// Stale install dir.
	staleDir := t.TempDir()
	writeFile(t, filepath.Join(staleDir, "SKILL.md"), "# Gone")
	staleHash, _ := skill.ComputeHash(staleDir)

	// Good install dir with SKILL.md and matching cache.
	goodInstallDir := t.TempDir()
	writeFile(t, filepath.Join(goodInstallDir, "SKILL.md"), "# Live Skill")
	goodHash, _ := skill.ComputeHash(goodInstallDir)

	// Simulated git clone with only the live-skill path.
	cacheRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cacheRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	goodCacheDir := filepath.Join(cacheRoot, "coding", "live-skill")
	if err := os.MkdirAll(goodCacheDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(goodCacheDir, "SKILL.md"), "# Live Skill")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"myrepo": {CachePath: cacheRoot},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			staleAddr: {
				agentName: {
					InstalledAtSHA: "sha-old",
					InstalledHash:  staleHash,
					LocalPath:      staleDir,
				},
			},
			goodAddr: {
				agentName: {
					InstalledAtSHA: "sha-abc",
					InstalledHash:  goodHash,
					LocalPath:      goodInstallDir,
				},
			},
		},
	}
	repoHeads := map[string]string{"myrepo": "sha-abc"}

	plan := skill.ReconcilePlan(st, repoHeads)

	staleItem := planByAddr(plan, staleAddr, agentName)
	goodItem := planByAddr(plan, goodAddr, agentName)

	if staleItem.Action != skill.SyncStaleAddress {
		t.Errorf("stale skill: want SyncStaleAddress, got %q", staleItem.Action)
	}
	if goodItem.Err != nil {
		t.Errorf("good skill: unexpected error: %v", goodItem.Err)
	}
	if goodItem.Action != skill.SyncAlreadyCurrent {
		t.Errorf("good skill: want SyncAlreadyCurrent, got %q", goodItem.Action)
	}
}
