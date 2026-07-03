package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

// initTestGitRepo initialises a bare git repo at dir and commits all existing files.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init %s: %v", dir, err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("."); err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ─── isPackPartial ────────────────────────────────────────────────────────────

func TestIsPackPartial_Complete(t *testing.T) {
	rec := state.InstalledPackRecord{
		Skills: map[string]map[string]state.PackSkillStatus{
			"repo/skill-a": {"claude-code": {Installed: true}},
			"repo/skill-b": {"claude-code": {Installed: true}},
		},
	}
	if isPackPartial(rec) {
		t.Error("expected complete pack to not be partial")
	}
}

func TestIsPackPartial_Partial(t *testing.T) {
	rec := state.InstalledPackRecord{
		Skills: map[string]map[string]state.PackSkillStatus{
			"repo/skill-a": {"claude-code": {Installed: true}},
			"repo/skill-b": {"claude-code": {Installed: false, Error: "auth failed"}},
		},
	}
	if !isPackPartial(rec) {
		t.Error("expected pack with a failed skill to be partial")
	}
}

func TestIsPackPartial_Empty(t *testing.T) {
	rec := state.InstalledPackRecord{Skills: map[string]map[string]state.PackSkillStatus{}}
	if isPackPartial(rec) {
		t.Error("empty pack should not be partial")
	}
}

// ─── removeStrings ────────────────────────────────────────────────────────────

func TestRemoveStrings_RemovesSome(t *testing.T) {
	got := removeStrings([]string{"a", "b", "c"}, []string{"b"})
	want := []string{"a", "c"}
	sort.Strings(got)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("removeStrings = %v, want %v", got, want)
	}
}

func TestRemoveStrings_RemovesAll(t *testing.T) {
	got := removeStrings([]string{"a", "b"}, []string{"a", "b"})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestRemoveStrings_NothingToRemove(t *testing.T) {
	got := removeStrings([]string{"a", "b"}, []string{"c"})
	if len(got) != 2 {
		t.Errorf("expected 2 elements, got %v", got)
	}
}

// ─── skillsInPack ─────────────────────────────────────────────────────────────

func TestSkillsInPack_SortedOutput(t *testing.T) {
	rec := state.InstalledPackRecord{
		Skills: map[string]map[string]state.PackSkillStatus{
			"repo/skill-b": {"agent": {Installed: true}},
			"repo/skill-a": {"agent": {Installed: true}},
			"repo/skill-c": {"agent": {Installed: true}},
		},
	}
	got := skillsInPack(rec)
	want := []string{"repo/skill-a", "repo/skill-b", "repo/skill-c"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("skillsInPack = %v, want %v", got, want)
	}
}

// ─── isLocalPath ─────────────────────────────────────────────────────────────

func TestIsLocalPath_AbsolutePath(t *testing.T) {
	if !isLocalPath("/home/user/packs/go-dev") {
		t.Error("absolute path should be local")
	}
}

func TestIsLocalPath_RelativePath(t *testing.T) {
	if !isLocalPath("./packs/go-dev") {
		t.Error("relative path should be local")
	}
}

func TestIsLocalPath_HomePath(t *testing.T) {
	if !isLocalPath("~/packs/go-dev") {
		t.Error("~/... path should be local")
	}
}

func TestIsLocalPath_RegisteredAddr(t *testing.T) {
	if isLocalPath("my-repo/packs/go-dev") {
		t.Error("registered address should not be local")
	}
}

func TestIsLocalPath_HTTPS(t *testing.T) {
	if isLocalPath("https://example.com/pack.yaml") {
		t.Error("HTTPS URL should not be local")
	}
}

// ─── packAddrFromName ─────────────────────────────────────────────────────────

func TestPackAddrFromName_NormalisesName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Go Dev Pack", "go-dev-pack"},
		{"my-pack", "my-pack"},
		{"Mixed Case", "mixed-case"},
	}
	for _, tc := range cases {
		got := packAddrFromName(tc.in)
		if got != tc.want {
			t.Errorf("packAddrFromName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ─── loadPackDefinition from filepath ────────────────────────────────────────

func TestLoadPackDefinition_FromFilepath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Create a pack.yaml on disk.
	packDir := t.TempDir()
	content := `
name: test-pack
description: A test pack
repos:
  - name: my-repo
    url: https://github.com/example/my-repo.git
skills:
  - my-repo/coding/debugger
`
	if err := os.WriteFile(filepath.Join(packDir, "pack.yaml"), []byte(content), 0600); err != nil {
		t.Fatalf("writing pack.yaml: %v", err)
	}

	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}
	cfg := &config.Config{Agents: make(map[string]config.AgentConfig)}

	pk, addr, err := loadPackDefinition(packDir, cfg, st)
	if err != nil {
		t.Fatalf("loadPackDefinition: %v", err)
	}
	if pk.Name != "test-pack" {
		t.Errorf("Pack.Name = %q, want test-pack", pk.Name)
	}
	if addr != "test-pack" {
		t.Errorf("addr = %q, want test-pack", addr)
	}
	if len(pk.Skills) != 1 || pk.Skills[0] != "my-repo/coding/debugger" {
		t.Errorf("Skills = %v", pk.Skills)
	}
}

// ─── selectAgentsForPack (non-interactive) ────────────────────────────────────

func TestSelectAgentsForPack_DefaultAgent(t *testing.T) {
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: "~/.claude/skills"},
			"copilot":     {SkillDir: "~/.copilot/skills"},
		},
	}
	// Non-interactive (no --agent or --all-agents) → default agent.
	agents, err := selectAgentsForPack("", false, cfg)
	if err != nil {
		t.Fatalf("selectAgentsForPack: %v", err)
	}
	if len(agents) != 1 || agents[0] != "claude-code" {
		t.Errorf("agents = %v, want [claude-code]", agents)
	}
}

func TestSelectAgentsForPack_ExplicitAgent(t *testing.T) {
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: "~/.claude/skills"},
			"copilot":     {SkillDir: "~/.copilot/skills"},
		},
	}
	agents, err := selectAgentsForPack("copilot", false, cfg)
	if err != nil {
		t.Fatalf("selectAgentsForPack: %v", err)
	}
	if len(agents) != 1 || agents[0] != "copilot" {
		t.Errorf("agents = %v, want [copilot]", agents)
	}
}

func TestSelectAgentsForPack_AllAgents(t *testing.T) {
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: "~/.claude/skills"},
			"copilot":     {SkillDir: "~/.copilot/skills"},
		},
	}
	agents, err := selectAgentsForPack("", true, cfg)
	if err != nil {
		t.Fatalf("selectAgentsForPack: %v", err)
	}
	sort.Strings(agents)
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %v", agents)
	}
}

// ─── packListInstalled ────────────────────────────────────────────────────────

func TestPackListInstalled_Empty(t *testing.T) {
	st := &state.State{InstalledPacks: make(map[string]state.InstalledPackRecord)}
	// Should not error on empty.
	if err := packListInstalled(st); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPackListInstalled_ShowsStatus(t *testing.T) {
	st := &state.State{
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/go-dev": {
				PackAddress: "my-repo/packs/go-dev",
				InstalledAt: time.Now(),
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/coding/debugger": {
						"claude-code": {Installed: true},
					},
				},
			},
		},
	}
	// Should not error.
	if err := packListInstalled(st); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── runPackInstall (integration) ─────────────────────────────────────────────

func TestRunPackInstall_FromFilepath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Set up a fake agent skill dir.
	agentSkillDir := t.TempDir()

	// Set up a fake repo cache containing the skill.
	repoCache := t.TempDir()
	skillInRepo := filepath.Join(repoCache, "coding", "debugger")
	if err := os.MkdirAll(skillInRepo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillInRepo, "SKILL.md"), []byte("# Debugger"), 0600); err != nil {
		t.Fatal(err)
	}
	// Initialise a git repo so HeadSHA works.
	initTestGitRepo(t, repoCache)

	// Build state + config.
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: agentSkillDir},
		},
	}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {URL: "fake://my-repo", CachePath: repoCache},
		},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}

	// Write a pack.yaml on disk.
	packDir := t.TempDir()
	packYAML := fmt.Sprintf(`
name: my-test-pack
repos:
  - name: my-repo
    url: fake://my-repo
skills:
  - my-repo/coding/debugger
`)
	if err := os.WriteFile(filepath.Join(packDir, "pack.yaml"), []byte(packYAML), 0600); err != nil {
		t.Fatal(err)
	}

	app := &App{Cfg: cfg, St: st}
	if err := runPackInstall(packDir, "", false, app); err != nil {
		t.Fatalf("runPackInstall: %v", err)
	}

	// Verify the pack record was saved.
	packAddr := packAddrFromName("my-test-pack")
	rec, ok := st.InstalledPacks[packAddr]
	if !ok {
		t.Fatalf("pack record not saved; InstalledPacks = %v", st.InstalledPacks)
	}
	if len(rec.Agents) != 1 || rec.Agents[0] != "claude-code" {
		t.Errorf("Agents = %v", rec.Agents)
	}
	// The skill should be installed successfully.
	skillStatus := rec.Skills["my-repo/coding/debugger"]["claude-code"]
	if !skillStatus.Installed {
		t.Errorf("skill should be installed; Error = %q", skillStatus.Error)
	}
	// The skill record in InstalledSkills should also exist.
	if _, ok := st.InstalledSkills["my-repo/coding/debugger"]["claude-code"]; !ok {
		t.Error("skill should be recorded in InstalledSkills")
	}
}

func TestRunPackInstall_PartialOnMissingRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	agentSkillDir := t.TempDir()
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: agentSkillDir},
		},
	}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}

	// Pack references a repo that cannot be added (fake URL).
	packDir := t.TempDir()
	packYAML := `
name: partial-pack
repos:
  - name: unreachable-repo
    url: https://unreachable.invalid/repo.git
skills:
  - unreachable-repo/coding/debugger
`
	if err := os.WriteFile(filepath.Join(packDir, "pack.yaml"), []byte(packYAML), 0600); err != nil {
		t.Fatal(err)
	}

	app := &App{Cfg: cfg, St: st}
	// Install should NOT return an error — it installs what it can and marks the rest partial.
	if err := runPackInstall(packDir, "", false, app); err != nil {
		t.Fatalf("runPackInstall: %v", err)
	}

	packAddr := packAddrFromName("partial-pack")
	rec, ok := st.InstalledPacks[packAddr]
	if !ok {
		t.Fatal("pack record should exist even on partial install")
	}
	if !isPackPartial(rec) {
		t.Error("pack should be partial when a repo could not be registered")
	}
	skillStatus := rec.Skills["unreachable-repo/coding/debugger"]["claude-code"]
	if skillStatus.Installed {
		t.Error("skill from unreachable repo should be marked not installed")
	}
	if !strings.Contains(skillStatus.Error, "repo unavailable") {
		t.Errorf("Error = %q, want 'repo unavailable'", skillStatus.Error)
	}
}

// ─── direct remove marks pack partial ─────────────────────────────────────────

func TestDirectRemoveMarksPackPartial(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Set up state with a skill installed for a pack.
	skillDir := t.TempDir()
	skillInstallPath := filepath.Join(skillDir, "debugger")
	if err := os.MkdirAll(skillInstallPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillInstallPath, "SKILL.md"), []byte("# Debugger"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:fake"

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {URL: "fake://my-repo", CachePath: t.TempDir()},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"claude-code": {
					InstalledAtSHA: "abc123",
					InstalledHash:  hash,
					LocalPath:      skillInstallPath,
				},
			},
		},
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/go-dev": {
				PackAddress: "my-repo/packs/go-dev",
				InstalledAt: time.Now(),
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/coding/debugger": {
						"claude-code": {Installed: true},
					},
				},
			},
		},
	}

	// Verify owning packs are found before remove.
	owning := st.FindPacksOwningSkill("my-repo/coding/debugger")
	if len(owning) != 1 {
		t.Fatalf("expected 1 owning pack, got %v", owning)
	}

	// Simulate what the remove command does: mark pack partial after removing skill.
	st.MarkPackSkillMissing("my-repo/packs/go-dev", "my-repo/coding/debugger", "claude-code", "directly removed by user")

	rec := st.InstalledPacks["my-repo/packs/go-dev"]
	if !isPackPartial(rec) {
		t.Error("pack should be partial after skill removal")
	}
	s := rec.Skills["my-repo/coding/debugger"]["claude-code"]
	if s.Installed {
		t.Error("skill should be marked not installed")
	}
	if s.Error != "directly removed by user" {
		t.Errorf("Error = %q", s.Error)
	}
}

// ─── resolvePackAgents ────────────────────────────────────────────────────────

func TestResolvePackAgents_AllAgents(t *testing.T) {
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: "~/.claude/skills"},
		},
	}
	agents, err := resolvePackAgents("", true, []string{"claude-code", "copilot"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %v", agents)
	}
}

func TestResolvePackAgents_EmptyPackAgents(t *testing.T) {
	cfg := &config.Config{DefaultAgent: "claude-code", Agents: map[string]config.AgentConfig{}}
	_, err := resolvePackAgents("", true, []string{}, cfg)
	if err == nil {
		t.Error("expected error for empty pack agents with --all-agents")
	}
}
