package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/config"
)

func TestExpandPath_Home(t *testing.T) {
	home, _ := os.UserHomeDir()
	got, err := config.ExpandPath("~/.claude/skills")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, ".claude/skills")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandPath_Absolute(t *testing.T) {
	got, err := config.ExpandPath("/absolute/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/absolute/path" {
		t.Errorf("got %q, want %q", got, "/absolute/path")
	}
}

func TestExpandPath_NoTilde(t *testing.T) {
	got, err := config.ExpandPath("relative/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "relative/path" {
		t.Errorf("got %q, want %q", got, "relative/path")
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Use a temp dir so the test doesn't touch ~/.skillpack
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: "~/.claude/skills"},
		},
	}

	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DefaultAgent != "claude-code" {
		t.Errorf("DefaultAgent: got %q, want %q", loaded.DefaultAgent, "claude-code")
	}
	if _, ok := loaded.Agents["claude-code"]; !ok {
		t.Error("expected claude-code in agents")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	if cfg.DefaultAgent != "" {
		t.Errorf("expected empty DefaultAgent, got %q", cfg.DefaultAgent)
	}
}

func TestDetectAgents_AddsNewAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create a directory that matches a known agent's DetectDir
	piDir := filepath.Join(tmp, ".pi", "agent")
	if err := os.MkdirAll(piDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Agents: make(map[string]config.AgentConfig)}
	modified := config.DetectAgents(cfg)

	if !modified {
		t.Error("expected DetectAgents to report modified=true")
	}
	if _, ok := cfg.Agents["pi"]; !ok {
		t.Error("expected pi agent to be detected")
	}
}

func TestDetectAgents_SkipsExisting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create the detect dir
	piDir := filepath.Join(tmp, ".pi", "agent")
	if err := os.MkdirAll(piDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-configure pi with a custom path
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"pi": {SkillDir: "~/custom/pi/skills"},
		},
	}
	modified := config.DetectAgents(cfg)

	if modified {
		t.Error("expected DetectAgents not to modify config when agent already exists")
	}
	// Should preserve custom path
	if cfg.Agents["pi"].SkillDir != "~/custom/pi/skills" {
		t.Errorf("expected custom SkillDir preserved, got %q", cfg.Agents["pi"].SkillDir)
	}
}

func TestDetectAgents_IgnoresFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create a FILE (not directory) at the detect path
	piPath := filepath.Join(tmp, ".pi", "agent")
	if err := os.MkdirAll(filepath.Join(tmp, ".pi"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(piPath, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Agents: make(map[string]config.AgentConfig)}
	modified := config.DetectAgents(cfg)

	if modified {
		t.Error("expected DetectAgents not to detect a file as an agent")
	}
	if _, ok := cfg.Agents["pi"]; ok {
		t.Error("expected pi NOT to be detected when path is a file")
	}
}

func TestDetectAgents_FirstRunWithDetectDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create pi's detect dir (but NOT the skills dir)
	piDir := filepath.Join(tmp, ".pi", "agent")
	if err := os.MkdirAll(piDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Load with no config file — should auto-detect
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Agents["pi"]; !ok {
		t.Error("expected pi to be auto-detected on first-run Load")
	}
}
