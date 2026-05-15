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
