package skill_test

// Tests for InstalledSkill.Open() — the constructor for the InstalledSkill handle.

import (
	"testing"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

func TestOpen_ErrorsWhenNotInstalled(t *testing.T) {
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
	cfg := &config.Config{}

	_, err := skill.Open("some-repo/some-skill", "claude-code", cfg, st)
	if err == nil {
		t.Fatal("expected error when skill is not installed, got nil")
	}
}

func TestOpen_ErrorsWhenAgentNotInstalled(t *testing.T) {
	addr := "my-repo/my-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: "/tmp/my-repo"},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"other-agent": {LocalPath: "/tmp/skills/my-skill"},
			},
		},
	}
	cfg := &config.Config{}

	_, err := skill.Open(addr, "claude-code", cfg, st)
	if err == nil {
		t.Fatal("expected error when skill is installed for a different agent only, got nil")
	}
}

func TestOpen_PopulatesCachePath(t *testing.T) {
	addr := "my-repo/my-skill"
	wantCachePath := "/tmp/my-repo-cache"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: wantCachePath},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					LocalPath:      "/tmp/skills/my-skill",
					InstalledHash:  "sha256:abc",
					InstalledAtSHA: "deadbeef",
				},
			},
		},
	}
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: "/tmp/skills"},
		},
	}

	is, err := skill.Open(addr, "claude-code", cfg, st)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if is.CachePath != wantCachePath {
		t.Errorf("CachePath = %q, want %q", is.CachePath, wantCachePath)
	}
	if is.Addr != addr {
		t.Errorf("Addr = %q, want %q", is.Addr, addr)
	}
	if is.AgentName != "claude-code" {
		t.Errorf("AgentName = %q, want %q", is.AgentName, "claude-code")
	}
	if is.Rec.InstalledHash != "sha256:abc" {
		t.Errorf("Rec.InstalledHash = %q, want %q", is.Rec.InstalledHash, "sha256:abc")
	}
}

func TestOpen_EmptyCachePathOnMissingRepo(t *testing.T) {
	addr := "missing-repo/some-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{}, // repo not registered
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {LocalPath: "/tmp/skills/some-skill"},
			},
		},
	}
	cfg := &config.Config{}

	is, err := skill.Open(addr, "claude-code", cfg, st)
	if err != nil {
		t.Fatalf("Open should succeed even when repo is not registered, got: %v", err)
	}
	if is.CachePath != "" {
		t.Errorf("CachePath should be empty when repo is not registered, got %q", is.CachePath)
	}
}

func TestOpen_IsFork_Delegation(t *testing.T) {
	addr := "my-repo/my-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: "/tmp/cache"},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					LocalPath:    "/tmp/skills/my-skill",
					UpstreamAddr: "upstream-repo/my-skill", // marks it as a fork
				},
			},
		},
	}
	cfg := &config.Config{}

	is, err := skill.Open(addr, "claude-code", cfg, st)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !is.IsFork() {
		t.Error("expected IsFork() to return true for skill with UpstreamAddr set")
	}
}

func TestInstalledSkill_Remove_SucceedsOnMissingPath(t *testing.T) {
	addr := "my-repo/my-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {LocalPath: "/tmp/nonexistent-path-xyz/my-skill"},
			},
		},
	}
	cfg := &config.Config{}

	is, err := skill.Open(addr, "claude-code", cfg, st)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Remove on a path that doesn't exist should still succeed (os.RemoveAll is idempotent)
	// but state should be cleaned up.
	if err := is.Remove(false); err != nil {
		t.Errorf("Remove on missing path should not error (os.RemoveAll is idempotent), got: %v", err)
	}
}

func TestInstalledSkill_IsModified_NotModified(t *testing.T) {
	// Open on a record with no local path — IsModified should report false.
	addr := "my-repo/my-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {LocalPath: "/tmp/nonexistent-path-for-ismod-test"},
			},
		},
	}
	cfg := &config.Config{}

	is, err := skill.Open(addr, "claude-code", cfg, st)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Non-existent path → isModified returns (false, nil) directly via the os.IsNotExist early-return
	modified, err := is.IsModified()
	if err != nil {
		t.Fatalf("IsModified: %v", err)
	}
	if modified {
		t.Error("expected IsModified to return false for non-existent path with empty installed hash")
	}
}

func TestInstalledSkill_Status_ErrorsWhenCachePathEmpty(t *testing.T) {
	// Status requires CachePath; if repo is not registered CachePath is empty.
	addr := "missing-repo/some-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {LocalPath: "/tmp/skills/some-skill"},
			},
		},
	}
	cfg := &config.Config{}

	is, err := skill.Open(addr, "claude-code", cfg, st)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = is.Status()
	if err == nil {
		t.Fatal("expected error from Status when CachePath is empty (repo not registered)")
	}
}
