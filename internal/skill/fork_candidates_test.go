package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// makeRepoCache creates a fake repo cache directory structure under a temp dir.
// skills is a map of skillName → slice of relPath subdirs to create under skillName.
// For each skill, a SKILL.md file is written.
func makeRepoCache(t *testing.T, skills []string) string {
	t.Helper()
	root := t.TempDir()
	for _, s := range skills {
		dir := filepath.Join(root, s)
		writeFile(t, filepath.Join(dir, "SKILL.md"), "# "+s)
	}
	return root
}

func TestDetectForkCandidates_NoRepos(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	st.InstalledSkills["my-skills/triage"] = map[string]state.InstalledSkillRecord{
		"copilot": {LocalPath: "/tmp/fake"},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates with no repos, got %v", candidates)
	}
}

func TestDetectForkCandidates_MatchInOtherRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Upstream repo has a skill named "triage"
	upstreamCache := makeRepoCache(t, []string{"triage"})

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills":       {CachePath: t.TempDir()},
			"upstream-skills": {CachePath: upstreamCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-skills/triage": {
				"copilot": {LocalPath: filepath.Join(t.TempDir(), "triage")},
			},
		},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	if candidates[0].Addr != "my-skills/triage" {
		t.Errorf("unexpected addr: %q", candidates[0].Addr)
	}
	if candidates[0].CandidateUpstream != "upstream-skills/triage" {
		t.Errorf("unexpected upstream: %q", candidates[0].CandidateUpstream)
	}
}

func TestDetectForkCandidates_AlreadyHasProvenance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	upstreamCache := makeRepoCache(t, []string{"triage"})

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills":       {CachePath: t.TempDir()},
			"upstream-skills": {CachePath: upstreamCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-skills/triage": {
				// already has upstream addr set — should not be a candidate
				"copilot": {
					LocalPath:    filepath.Join(t.TempDir(), "triage"),
					UpstreamAddr: "upstream-skills/triage",
					UpstreamSHA:  "abc123",
				},
			},
		},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates when provenance already set, got %v", candidates)
	}
}

func TestDetectForkCandidates_NoSKILLmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Upstream has a dir named "triage" but no SKILL.md
	upstreamCache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(upstreamCache, "triage"), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills":       {CachePath: t.TempDir()},
			"upstream-skills": {CachePath: upstreamCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-skills/triage": {
				"copilot": {LocalPath: filepath.Join(t.TempDir(), "triage")},
			},
		},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates when match dir has no SKILL.md, got %v", candidates)
	}
}

func TestDetectForkCandidates_NoMatchInOtherRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Upstream repo has a different skill — no basename match
	upstreamCache := makeRepoCache(t, []string{"debugger"})

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills":       {CachePath: t.TempDir()},
			"upstream-skills": {CachePath: upstreamCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-skills/triage": {
				"copilot": {LocalPath: filepath.Join(t.TempDir(), "triage")},
			},
		},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates when no basename match, got %v", candidates)
	}
}

func TestDetectForkCandidates_NestedSkillInUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Upstream has triage nested under a subdirectory
	upstreamCache := makeRepoCache(t, []string{"skills/engineering/triage"})

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills":       {CachePath: t.TempDir()},
			"upstream-skills": {CachePath: upstreamCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-skills/triage": {
				"copilot": {LocalPath: filepath.Join(t.TempDir(), "triage")},
			},
		},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate for nested skill, got %d", len(candidates))
	}
	if candidates[0].CandidateUpstream != "upstream-skills/skills/engineering/triage" {
		t.Errorf("unexpected upstream addr: %q", candidates[0].CandidateUpstream)
	}
}

func TestDetectForkCandidates_SkipsOwnRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Own repo also has a triage dir — should not be returned as a candidate
	ownCache := makeRepoCache(t, []string{"triage", "other-skill"})

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-skills": {CachePath: ownCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-skills/triage": {
				"copilot": {LocalPath: filepath.Join(t.TempDir(), "triage")},
			},
		},
	}

	candidates, err := skill.DetectForkCandidates(st)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates when only own repo has the skill, got %v", candidates)
	}
}


