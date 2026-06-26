package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// TestSuggestReplacements finds skills in registered repos whose basename
// matches the stale address and excludes the stale address itself.
func TestSuggestReplacements(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// repoA has the stale skill's original (now-deleted) location omitted; repoB
	// and repoC each carry a skill with the same basename "debugger".
	repoB := t.TempDir()
	writeFile(t, filepath.Join(repoB, "skills", "engineering", "debugger", "SKILL.md"), "# Debugger")
	repoC := t.TempDir()
	writeFile(t, filepath.Join(repoC, "debugger", "SKILL.md"), "# Debugger")
	// A non-matching skill should be ignored.
	writeFile(t, filepath.Join(repoC, "triage", "SKILL.md"), "# Triage")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-b": {CachePath: repoB},
			"repo-c": {CachePath: repoC},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	got := skill.SuggestReplacements("old-repo/coding/debugger", st)
	want := []string{
		"repo-b/skills/engineering/debugger",
		"repo-c/debugger",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSuggestReplacements_ExcludesSelf ensures the stale address is not
// suggested as its own replacement when it still exists in a repo cache.
func TestSuggestReplacements_ExcludesSelf(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	repoA := t.TempDir()
	writeFile(t, filepath.Join(repoA, "coding", "debugger", "SKILL.md"), "# Debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {CachePath: repoA},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	got := skill.SuggestReplacements("repo-a/coding/debugger", st)
	if len(got) != 0 {
		t.Fatalf("expected no suggestions (only match is self), got %v", got)
	}
}

// TestSuggestReplacements_NoMatch returns an empty slice when no candidate
// shares the stale address basename.
func TestSuggestReplacements_NoMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	repoA := t.TempDir()
	writeFile(t, filepath.Join(repoA, "triage", "SKILL.md"), "# Triage")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {CachePath: repoA},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	got := skill.SuggestReplacements("old-repo/coding/debugger", st)
	if got == nil {
		t.Fatal("expected empty (non-nil) slice, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected no suggestions, got %v", got)
	}
}

// TestRelink re-points a stale installed skill to a valid replacement address
// and verifies (AC #3) that a subsequent ReconcilePlan no longer reports it as
// a stale mapping.
func TestRelink(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldAddr := "old-repo/coding/debugger"
	newAddr := "new-repo/skills/debugger"
	agentName := "copilot"

	// Old repo cache is a git clone whose skill path no longer exists (stale).
	oldCache := t.TempDir()
	writeFile(t, filepath.Join(oldCache, "README.md"), "old repo")
	initGitRepo(t, oldCache)

	// New repo cache is a git clone that carries the replacement skill.
	newCache := t.TempDir()
	writeFile(t, filepath.Join(newCache, "skills", "debugger", "SKILL.md"), "# Debugger v2")
	writeFile(t, filepath.Join(newCache, "skills", "debugger", "extra.md"), "extra")
	initGitRepo(t, newCache)

	// Installed copy holds the OLD skill's content.
	installDir := filepath.Join(t.TempDir(), "debugger")
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Debugger v1")
	oldHash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"old-repo": {CachePath: oldCache},
			"new-repo": {CachePath: newCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			oldAddr: {
				agentName: {
					InstalledAtSHA: "sha-old",
					InstalledHash:  oldHash,
					LocalPath:      installDir,
				},
			},
		},
	}

	if err := skill.Relink(oldAddr, newAddr, agentName, false, st); err != nil {
		t.Fatalf("Relink: %v", err)
	}

	// State record moved from oldAddr to newAddr.
	if _, ok := st.InstalledSkills[oldAddr]; ok {
		t.Errorf("old address %q should no longer be installed", oldAddr)
	}
	rec, ok := st.InstalledSkills[newAddr][agentName]
	if !ok {
		t.Fatalf("new address %q not recorded for agent %q", newAddr, agentName)
	}

	// Installed content refreshed to the replacement skill.
	if data, _ := os.ReadFile(filepath.Join(installDir, "SKILL.md")); string(data) != "# Debugger v2" {
		t.Errorf("SKILL.md not refreshed: got %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(installDir, "extra.md")); err != nil {
		t.Errorf("replacement extra file not copied: %v", err)
	}

	// Install snapshot updated: hash matches refreshed content, SHA matches new repo HEAD.
	newContentHash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatalf("ComputeHash after relink: %v", err)
	}
	if rec.InstalledHash != newContentHash {
		t.Errorf("InstalledHash not refreshed: got %q want %q", rec.InstalledHash, newContentHash)
	}
	if rec.InstalledAtSHA == "sha-old" || rec.InstalledAtSHA == "" {
		t.Errorf("InstalledAtSHA not updated to new repo HEAD: got %q", rec.InstalledAtSHA)
	}

	// AC #3: a subsequent sync no longer reports a stale mapping.
	heads, err := skill.CollectRepoHeads(st)
	if err != nil {
		t.Fatalf("CollectRepoHeads: %v", err)
	}
	plan := skill.ReconcilePlan(st, heads)
	item := planByAddr(plan, newAddr, agentName)
	if item.Err != nil {
		t.Fatalf("unexpected plan error: %v", item.Err)
	}
	if item.Action == skill.SyncStaleAddress {
		t.Errorf("relinked skill should not be stale, got %q", item.Action)
	}
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("relinked skill should be already-current, got %q", item.Action)
	}
}

// TestRelink_InvalidNewAddr errors when the replacement address does not exist
// in any registered repo cache, leaving state untouched.
func TestRelink_InvalidNewAddr(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldAddr := "old-repo/coding/debugger"
	agentName := "copilot"

	newCache := t.TempDir()
	writeFile(t, filepath.Join(newCache, "README.md"), "new repo")
	initGitRepo(t, newCache)

	installDir := filepath.Join(t.TempDir(), "debugger")
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"new-repo": {CachePath: newCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			oldAddr: {
				agentName: {LocalPath: installDir},
			},
		},
	}

	err := skill.Relink(oldAddr, "new-repo/skills/missing", agentName, false, st)
	if err == nil {
		t.Fatal("expected error for missing replacement skill, got nil")
	}
	if _, ok := st.InstalledSkills[oldAddr]; !ok {
		t.Error("old address should remain installed after a failed relink")
	}
}

// TestRelink_NotInstalled errors when the stale address is not installed.
func TestRelink_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	if err := skill.Relink("old-repo/x", "new-repo/y", "copilot", false, st); err == nil {
		t.Fatal("expected error for not-installed skill, got nil")
	}
}

// TestRelink_SameAddr rejects relinking an address to itself.
func TestRelink_SameAddr(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	addr := "repo/coding/debugger"
	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {"copilot": {}},
		},
	}

	if err := skill.Relink(addr, addr, "copilot", false, st); err == nil {
		t.Fatal("expected error for identical addresses, got nil")
	}
}

// TestRelink_ModifiedWithoutForce refuses to relink a locally-modified skill
// unless force is set, and succeeds (refreshing content) when force is true.
func TestRelink_ModifiedWithoutForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldAddr := "old-repo/coding/debugger"
	newAddr := "new-repo/skills/debugger"
	agentName := "copilot"

	newCache := t.TempDir()
	writeFile(t, filepath.Join(newCache, "skills", "debugger", "SKILL.md"), "# Debugger v2")
	initGitRepo(t, newCache)

	// Installed copy was edited after install: on-disk hash != recorded hash.
	installDir := filepath.Join(t.TempDir(), "debugger")
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# locally edited")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"new-repo": {CachePath: newCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			oldAddr: {
				agentName: {
					InstalledAtSHA: "sha-old",
					InstalledHash:  "sha256:stale", // does not match on-disk content
					LocalPath:      installDir,
				},
			},
		},
	}

	if err := skill.Relink(oldAddr, newAddr, agentName, false, st); err == nil {
		t.Fatal("expected error relinking a modified skill without --force")
	}
	if _, ok := st.InstalledSkills[oldAddr]; !ok {
		t.Error("old address should remain installed after a refused relink")
	}

	if err := skill.Relink(oldAddr, newAddr, agentName, true, st); err != nil {
		t.Fatalf("Relink with force: %v", err)
	}
	if _, ok := st.InstalledSkills[newAddr][agentName]; !ok {
		t.Errorf("forced relink should move record to %q", newAddr)
	}
	if data, _ := os.ReadFile(filepath.Join(installDir, "SKILL.md")); string(data) != "# Debugger v2" {
		t.Errorf("forced relink should refresh content, got %q", string(data))
	}
}

// TestRelinkUpstream_SetUpstream updates UpstreamAddr and UpstreamSHA in state
// without touching installed files or the fork's own address.
func TestRelinkUpstream_SetUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	forkAddr := "my-skills/debugger"
	newUpstream := "upstream-repo/debugger"
	agentName := "copilot"

	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "debugger", "SKILL.md"), "# Upstream Debugger")
	initGitRepo(t, upstreamCache)

	installDir := filepath.Join(t.TempDir(), "debugger")
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# My forked debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"upstream-repo": {CachePath: upstreamCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			forkAddr: {
				agentName: {
					InstalledAtSHA: "sha-fork",
					InstalledHash:  "hash-fork",
					LocalPath:      installDir,
					UpstreamAddr:   "old-upstream/debugger",
					UpstreamSHA:    "sha-old-upstream",
				},
			},
		},
	}

	if err := skill.RelinkUpstream(forkAddr, newUpstream, agentName, st); err != nil {
		t.Fatalf("RelinkUpstream: %v", err)
	}

	rec := st.InstalledSkills[forkAddr][agentName]

	// Upstream pointer updated.
	if rec.UpstreamAddr != newUpstream {
		t.Errorf("UpstreamAddr = %q, want %q", rec.UpstreamAddr, newUpstream)
	}
	if rec.UpstreamSHA == "" || rec.UpstreamSHA == "sha-old-upstream" {
		t.Errorf("UpstreamSHA should be updated to new repo HEAD, got %q", rec.UpstreamSHA)
	}

	// Fork's own address unchanged.
	if _, ok := st.InstalledSkills[forkAddr]; !ok {
		t.Errorf("fork address %q should remain unchanged", forkAddr)
	}

	// Installed files untouched (byte-identical).
	data, _ := os.ReadFile(filepath.Join(installDir, "SKILL.md"))
	if string(data) != "# My forked debugger" {
		t.Errorf("installed file should be untouched, got %q", string(data))
	}

	// InstalledHash unchanged.
	if rec.InstalledHash != "hash-fork" {
		t.Errorf("InstalledHash should be unchanged, got %q", rec.InstalledHash)
	}
}

// TestRelinkUpstream_SetUpstream_InvalidAddr rejects an address that does not
// exist in any registered repo without writing any state change.
func TestRelinkUpstream_SetUpstream_InvalidAddr(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	forkAddr := "my-skills/debugger"
	agentName := "copilot"

	cache := t.TempDir()
	writeFile(t, filepath.Join(cache, "README.md"), "empty repo")
	initGitRepo(t, cache)

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"upstream-repo": {CachePath: cache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			forkAddr: {
				agentName: {
					UpstreamAddr: "old-upstream/debugger",
					UpstreamSHA:  "sha-original",
				},
			},
		},
	}

	err := skill.RelinkUpstream(forkAddr, "upstream-repo/nonexistent", agentName, st)
	if err == nil {
		t.Fatal("expected error for invalid upstream addr, got nil")
	}

	// No state change.
	rec := st.InstalledSkills[forkAddr][agentName]
	if rec.UpstreamAddr != "old-upstream/debugger" {
		t.Errorf("UpstreamAddr should be unchanged after error, got %q", rec.UpstreamAddr)
	}
	if rec.UpstreamSHA != "sha-original" {
		t.Errorf("UpstreamSHA should be unchanged after error, got %q", rec.UpstreamSHA)
	}
}

// TestRelinkUpstream_ClearUpstream clears both UpstreamAddr and UpstreamSHA,
// so isFork returns false for the record afterward.
func TestRelinkUpstream_ClearUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	forkAddr := "my-skills/debugger"
	agentName := "copilot"

	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			forkAddr: {
				agentName: {
					InstalledAtSHA: "sha-fork",
					InstalledHash:  "hash-fork",
					UpstreamAddr:   "upstream-repo/debugger",
					UpstreamSHA:    "sha-upstream",
				},
			},
		},
	}

	// Pass empty string to clear upstream.
	if err := skill.RelinkUpstream(forkAddr, "", agentName, st); err != nil {
		t.Fatalf("RelinkUpstream (clear): %v", err)
	}

	rec := st.InstalledSkills[forkAddr][agentName]

	// isFork contract: a record with no UpstreamAddr is not a fork.
	if rec.UpstreamAddr != "" {
		t.Errorf("UpstreamAddr should be cleared, got %q", rec.UpstreamAddr)
	}
	if rec.UpstreamSHA != "" {
		t.Errorf("UpstreamSHA should be cleared, got %q", rec.UpstreamSHA)
	}
}

// TestRelinkUpstream_NotInstalled errors when the skill is not installed.
func TestRelinkUpstream_NotInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	if err := skill.RelinkUpstream("nonexistent/skill", "upstream/skill", "copilot", st); err == nil {
		t.Fatal("expected error for not-installed skill, got nil")
	}
}
