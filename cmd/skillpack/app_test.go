package main

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

func TestAppFromCtx_NoApp(t *testing.T) {
	ctx := context.Background()
	if got := AppFromCtx(ctx); got != nil {
		t.Errorf("AppFromCtx(empty) = %v, want nil", got)
	}
}

func TestAppFromCtx_WithApp(t *testing.T) {
	cfg := &config.Config{
		DefaultAgent: "claude-code",
		Agents:       map[string]config.AgentConfig{"claude-code": {SkillDir: "/tmp/claude/skills"}},
	}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	// Use a fresh command so the test does not mutate the global rootCmd.
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.WithValue(context.Background(), appKey{}, &App{Cfg: cfg, St: st}))

	got := AppFromCtx(cmd.Context())
	if got == nil {
		t.Fatal("AppFromCtx returned nil")
	}
	if got.Cfg != cfg {
		t.Error("App.Cfg mismatch")
	}
	if got.St != st {
		t.Error("App.St mismatch")
	}
}
