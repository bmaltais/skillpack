package skill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// ─── PlanUpdate: fork with missing upstream repo ─────────────────────────────

func TestPlanUpdate_ForkMissingUpstream_FallsBackToOwnRepo(t *testing.T) {
	// Set up a skill installed from "bmaltais-skills/grill-me" that was forked
	// from "mattpocock-skills/grill-me". The upstream repo is NOT registered.
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Grill Me")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"bmaltais-skills": {CachePath: t.TempDir()},
			// "mattpocock-skills" is NOT registered
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"bmaltais-skills/grill-me": {
				"pi": {
					InstalledAtSHA: "sha-own",
					InstalledHash:  hash,
					LocalPath:      installDir,
					UpstreamAddr:   "mattpocock-skills/grill-me",
					UpstreamSHA:    "sha-upstream-old",
				},
			},
		},
	}
	// Own repo HEAD matches InstalledAtSHA — should be "up-to-date"
	repoHeads := map[string]string{"bmaltais-skills": "sha-own"}

	plan := skill.PlanUpdate(st, repoHeads)

	if len(plan) != 1 {
		t.Fatalf("want 1 plan item, got %d", len(plan))
	}
	item := plan[0]
	if item.Err != nil {
		t.Fatalf("expected no error, got: %v", item.Err)
	}
	if item.Action != skill.UpdateAlreadyCurrent {
		t.Errorf("want UpdateAlreadyCurrent, got %q", item.Action)
	}
	if item.Warning == "" {
		t.Error("expected a warning about missing upstream repo")
	}
	if !strings.Contains(item.Warning, "mattpocock-skills") {
		t.Errorf("warning should mention the missing upstream repo, got: %q", item.Warning)
	}
}

func TestPlanUpdate_ForkMissingUpstream_DetectsOwnRepoUpdate(t *testing.T) {
	// Own repo HEAD has advanced beyond InstalledAtSHA — should show "update available"
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Grill Me")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"bmaltais-skills": {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"bmaltais-skills/grill-me": {
				"pi": {
					InstalledAtSHA: "sha-old",
					InstalledHash:  hash,
					LocalPath:      installDir,
					UpstreamAddr:   "mattpocock-skills/grill-me",
					UpstreamSHA:    "sha-upstream-old",
				},
			},
		},
	}
	// Own repo HEAD has advanced
	repoHeads := map[string]string{"bmaltais-skills": "sha-new"}

	plan := skill.PlanUpdate(st, repoHeads)

	item := plan[0]
	if item.Err != nil {
		t.Fatalf("expected no error, got: %v", item.Err)
	}
	if item.Action != skill.UpdateAvailable {
		t.Errorf("want UpdateAvailable, got %q", item.Action)
	}
	if item.Warning == "" {
		t.Error("expected a warning about missing upstream repo")
	}
}

func TestPlanUpdate_ForkMissingUpstream_BothReposMissing_Errors(t *testing.T) {
	// When BOTH upstream and own repo are missing, should error
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Grill Me")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"bmaltais-skills/grill-me": {
				"pi": {
					InstalledAtSHA: "sha-own",
					InstalledHash:  hash,
					LocalPath:      installDir,
					UpstreamAddr:   "mattpocock-skills/grill-me",
					UpstreamSHA:    "sha-upstream-old",
				},
			},
		},
	}
	repoHeads := map[string]string{}

	plan := skill.PlanUpdate(st, repoHeads)

	item := plan[0]
	if item.Err == nil {
		t.Fatal("expected error when both repos are missing")
	}
	if !strings.Contains(item.Err.Error(), "bmaltais-skills") {
		t.Errorf("error should reference own repo name, got: %q", item.Err.Error())
	}
}

// ─── ReconcilePlan: fork with missing upstream repo ──────────────────────────

func TestReconcilePlan_ForkMissingUpstream_FallsBackToOwnRepo(t *testing.T) {
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Grill Me")
	hash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatal(err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"bmaltais-skills": {CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"bmaltais-skills/grill-me": {
				"pi": {
					InstalledAtSHA: "sha-own",
					InstalledHash:  hash,
					LocalPath:      installDir,
					UpstreamAddr:   "mattpocock-skills/grill-me",
					UpstreamSHA:    "sha-upstream-old",
				},
			},
		},
	}
	repoHeads := map[string]string{"bmaltais-skills": "sha-own"}

	plan := skill.ReconcilePlan(st, repoHeads)

	if len(plan) != 1 {
		t.Fatalf("want 1 plan item, got %d", len(plan))
	}
	item := plan[0]
	if item.Err != nil {
		t.Fatalf("expected no error, got: %v", item.Err)
	}
	if item.Action != skill.SyncAlreadyCurrent {
		t.Errorf("want SyncAlreadyCurrent, got %q", item.Action)
	}
	if item.Warning == "" {
		t.Error("expected a warning about missing upstream repo")
	}
	if !strings.Contains(item.Warning, "mattpocock-skills") {
		t.Errorf("warning should mention the missing upstream repo, got: %q", item.Warning)
	}
}

// ─── ApplySync integration: fork with missing upstream ───────────────────────

func TestApplySync_ForkMissingUpstream_UpdatesFromOwnRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Set up the own-repo cache with an updated skill directory.
	ownRepoCache := t.TempDir()
	skillCacheDir := filepath.Join(ownRepoCache, "grill-me")
	writeFile(t, filepath.Join(skillCacheDir, "SKILL.md"), "# Grill Me v2 — updated content")

	// Set up the installed skill directory with older content.
	installDir := t.TempDir()
	writeFile(t, filepath.Join(installDir, "SKILL.md"), "# Grill Me v1")
	oldHash, err := skill.ComputeHash(installDir)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize a real git repo at ownRepoCache so HeadSHA works.
	initGitRepo(t, ownRepoCache)

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"bmaltais-skills": {CachePath: ownRepoCache},
			// "mattpocock-skills" is NOT registered
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"bmaltais-skills/grill-me": {
				"pi": {
					InstalledAtSHA: "sha-old",
					InstalledHash:  oldHash,
					LocalPath:      installDir,
					UpstreamAddr:   "mattpocock-skills/grill-me",
					UpstreamSHA:    "sha-upstream-old",
				},
			},
		},
	}
	// Own repo HEAD has advanced.
	repoHeads := map[string]string{"bmaltais-skills": "sha-new"}

	plan := skill.ReconcilePlan(st, repoHeads)
	if len(plan) != 1 {
		t.Fatalf("want 1 plan item, got %d", len(plan))
	}
	if plan[0].Action != skill.SyncUpdated {
		t.Fatalf("want SyncUpdated, got %q (err: %v)", plan[0].Action, plan[0].Err)
	}
	if !plan[0].UpstreamDisabled {
		t.Fatal("expected UpstreamDisabled to be true")
	}

	// Execute the plan — should NOT error despite missing upstream repo.
	results, conflicts, err := skill.ApplySync(plan, nil, st)
	if err != nil {
		t.Fatalf("ApplySync returned error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Err != nil {
		t.Fatalf("expected no error in result, got: %v", r.Err)
	}
	if r.Action != skill.SyncUpdated {
		t.Errorf("want SyncUpdated result, got %q", r.Action)
	}
	if r.Warning == "" {
		t.Error("expected warning to be propagated to result")
	}

	// Verify the installed directory was updated with new content.
	content, err := os.ReadFile(filepath.Join(installDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "v2") {
		t.Errorf("installed content should be updated to v2, got: %s", string(content))
	}
}
