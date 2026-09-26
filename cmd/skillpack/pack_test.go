package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/state"
)

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
	if !pack.IsPartial(rec) {
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
