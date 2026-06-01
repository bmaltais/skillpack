package skill_test

import (
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
	hash, _ := skill.ComputeHash(installDir)

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
