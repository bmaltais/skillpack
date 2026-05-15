package skill_test

import (
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// TestSync_SiblingUpdate_SinglePass is a regression test for issue #22.
//
// Scenario: two agents (copilot, claude-code) have the same skill installed.
// The copilot copy has local edits — it should be published. The claude-code
// copy is clean — it should be updated to the newly published version.
// Both operations must complete in a single Sync call, regardless of the
// (non-deterministic) order in which Go iterates the installed-skill map.
func TestSync_SiblingUpdate_SinglePass(t *testing.T) {
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	// --- Step 1: initialise the cache repo with an initial skill commit ---
	cacheDir := t.TempDir()
	cacheRepo, err := gogit.PlainInit(cacheDir, false)
	if err != nil {
		t.Fatalf("init cache: %v", err)
	}
	wt, err := cacheRepo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	skillRelPath := "coding/my-skill"
	skillCachePath := filepath.Join(cacheDir, skillRelPath)
	writeFile(t, filepath.Join(skillCachePath, "SKILL.md"), "# My Skill\nInitial content.")

	if _, err := wt.Add("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	initialHash, err := wt.Commit("initial commit", &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// --- Step 2: create a bare remote by cloning the cache, then retarget ---
	remoteDir := t.TempDir()
	if _, err := gogit.PlainClone(remoteDir, true, &gogit.CloneOptions{
		URL: "file://" + cacheDir,
	}); err != nil {
		t.Fatalf("clone to bare remote: %v", err)
	}
	cfg, err := cacheRepo.Config()
	if err != nil {
		t.Fatalf("read cache config: %v", err)
	}
	cfg.Remotes["origin"] = &gitconfig.RemoteConfig{
		Name:  "origin",
		URLs:  []string{"file://" + remoteDir},
		Fetch: []gitconfig.RefSpec{"refs/heads/*:refs/remotes/origin/*"},
	}
	if err := cacheRepo.SetConfig(cfg); err != nil {
		t.Fatalf("set cache config: %v", err)
	}

	// --- Step 3: install the skill for two agents ---
	copilotInstallDir := filepath.Join(t.TempDir(), "my-skill")
	claudeInstallDir := filepath.Join(t.TempDir(), "my-skill")

	writeFile(t, filepath.Join(copilotInstallDir, "SKILL.md"), "# My Skill\nInitial content.")
	writeFile(t, filepath.Join(claudeInstallDir, "SKILL.md"), "# My Skill\nInitial content.")

	copilotHash, _ := skill.ComputeHash(copilotInstallDir)
	claudeHash, _ := skill.ComputeHash(claudeInstallDir)

	addr := "my-repo/" + skillRelPath
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {
				URL:       "file://" + remoteDir,
				CachePath: cacheDir,
			},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"copilot": {
					InstalledAtSHA: initialHash.String(),
					InstalledHash:  copilotHash,
					LocalPath:      copilotInstallDir,
				},
				"claude-code": {
					InstalledAtSHA: initialHash.String(),
					InstalledHash:  claudeHash,
					LocalPath:      claudeInstallDir,
				},
			},
		},
	}

	// --- Step 4: simulate copilot editing its installed copy ---
	writeFile(t, filepath.Join(copilotInstallDir, "SKILL.md"), "# My Skill\nEdited by copilot.")

	// --- Step 5: single Sync call — must fully converge ---
	results, conflicts, err := skill.Sync(false, nil, st)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("unexpected conflicts: %v", conflicts)
	}

	// Build action map for easy assertion
	actions := make(map[string]skill.SyncAction)
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("error result for %s/%s: %v", r.Addr, r.AgentName, r.Err)
		}
		actions[r.AgentName] = r.Action
	}

	if actions["copilot"] != skill.SyncPublished {
		t.Errorf("copilot: expected SyncPublished, got %q", actions["copilot"])
	}
	// Before the fix this was intermittently SyncAlreadyCurrent, requiring a second sync.
	if actions["claude-code"] != skill.SyncUpdated {
		t.Errorf("claude-code: expected SyncUpdated, got %q (bug: required two syncs before fix)", actions["claude-code"])
	}
}
