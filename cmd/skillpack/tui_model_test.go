package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

// makeGitClone creates a minimal directory that passes the .git sentinel check.
func makeGitClone(t *testing.T, base string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(base, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return base
}

// makeSkillDir creates a skill directory with a SKILL.md file.
func makeSkillDir(t *testing.T, root, relPath string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Test Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- computeSkillProblems tests ---

// TestComputeSkillProblems_HealthySkill verifies a skill that still exists in the
// repo cache is classified as problemNone.
func TestComputeSkillProblems_HealthySkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "repo-a"))
	makeSkillDir(t, cacheDir, "coding/debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {URL: "https://example.com/repo-a.git", CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo-a/coding/debugger": {
				"copilot": {InstalledAtSHA: "abc", InstalledHash: "def"},
			},
		},
	}

	discovered := map[string]bool{"repo-a/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if p, ok := got["repo-a/coding/debugger"]; ok && p != problemNone {
		t.Errorf("expected problemNone for healthy skill, got %v", p)
	}
}

// TestComputeSkillProblems_StaleSkill verifies a skill absent from the discovered
// set is classified as problemStale.
func TestComputeSkillProblems_StaleSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "repo-a"))
	// Note: the skill directory is NOT created — it's gone from the cache.

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {URL: "https://example.com/repo-a.git", CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo-a/coding/debugger": {
				"copilot": {InstalledAtSHA: "abc", InstalledHash: "def"},
			},
		},
	}

	// The skill is NOT in the discovered set (simulates path gone from cache).
	discovered := map[string]bool{}
	got := computeSkillProblems(st, discovered)

	if got["repo-a/coding/debugger"] != problemStale {
		t.Errorf("expected problemStale, got %v", got["repo-a/coding/debugger"])
	}
}

// TestComputeSkillProblems_BrokenUpstream verifies a fork whose upstream path
// no longer exists is classified as problemBrokenUpstream.
func TestComputeSkillProblems_BrokenUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Fork's own repo cache (with the fork skill still present).
	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger")

	// Upstream repo cache exists but the upstream skill path is gone.
	upstreamCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "upstream-repo"))
	// upstream skill NOT created — it's been deleted upstream.

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo":       {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			"upstream-repo": {URL: "https://example.com/upstream-repo.git", CachePath: upstreamCacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	// Fork itself IS discoverable; its UpstreamAddr is broken.
	discovered := map[string]bool{"my-repo/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if got["my-repo/coding/debugger"] != problemBrokenUpstream {
		t.Errorf("expected problemBrokenUpstream, got %v", got["my-repo/coding/debugger"])
	}
}

// TestComputeSkillProblems_UpstreamRepoNotRegistered verifies that a fork whose
// upstream repo is NOT registered is NOT flagged as broken (unknown state, not broken).
func TestComputeSkillProblems_UpstreamRepoNotRegistered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			// "upstream-repo" is NOT registered.
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	discovered := map[string]bool{"my-repo/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if p := got["my-repo/coding/debugger"]; p != problemNone {
		t.Errorf("expected problemNone when upstream repo not registered, got %v", p)
	}
}

// TestComputeSkillProblems_HealthyFork verifies a fork whose upstream skill still
// exists is classified as problemNone.
func TestComputeSkillProblems_HealthyFork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger")

	upstreamCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "upstream-repo"))
	makeSkillDir(t, upstreamCacheDir, "coding/debugger") // upstream skill still exists

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo":       {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			"upstream-repo": {URL: "https://example.com/upstream-repo.git", CachePath: upstreamCacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	discovered := map[string]bool{"my-repo/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if p := got["my-repo/coding/debugger"]; p != problemNone {
		t.Errorf("expected problemNone for healthy fork, got %v", p)
	}
}

// --- refreshSkills tests ---

// TestRefreshSkills_StaleAddressAppearsInRows verifies that a stale installed skill
// (not discoverable in any repo cache) is added to the row list with problemStale.
func TestRefreshSkills_StaleAddressAppearsInRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "repo-a"))
	// The skill path is gone from the cache — do NOT create it.

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {URL: "https://example.com/repo-a.git", CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo-a/coding/debugger": {
				"copilot": {InstalledAtSHA: "abc", InstalledHash: "def"},
			},
		},
	}

	m := initialModel(cfg, st)

	var found *tuiRow
	for i := range m.rows {
		if m.rows[i].kind == skillRow && m.rows[i].addr == "repo-a/coding/debugger" {
			found = &m.rows[i]
			break
		}
	}

	if found == nil {
		t.Fatal("stale skill was not added to TUI rows")
	}
	if found.problem != problemStale {
		t.Errorf("expected problemStale, got %v", found.problem)
	}
}

// TestRefreshSkills_BrokenUpstreamMarked verifies a fork with a missing upstream
// path is marked as problemBrokenUpstream in the TUI rows.
func TestRefreshSkills_BrokenUpstreamMarked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger") // fork itself is fine

	upstreamCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "upstream-repo"))
	// upstream skill path is gone — NOT created

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo":       {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			"upstream-repo": {URL: "https://example.com/upstream-repo.git", CachePath: upstreamCacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	m := initialModel(cfg, st)

	var found *tuiRow
	for i := range m.rows {
		if m.rows[i].kind == skillRow && m.rows[i].addr == "my-repo/coding/debugger" {
			found = &m.rows[i]
			break
		}
	}

	if found == nil {
		t.Fatal("fork skill was not found in TUI rows")
	}
	if found.problem != problemBrokenUpstream {
		t.Errorf("expected problemBrokenUpstream, got %v", found.problem)
	}
}
