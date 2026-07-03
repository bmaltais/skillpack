package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// ─── defaultPackPath ──────────────────────────────────────────────────────────

func TestDefaultPackPath(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"go-dev", "packs/go-dev"},
		{"Go Development", "packs/go-development"},
		{"MY PACK", "packs/my-pack"},
		{"simple", "packs/simple"},
	}
	for _, tc := range cases {
		got := defaultPackPath(tc.name)
		if got != tc.want {
			t.Errorf("defaultPackPath(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ─── ValidatePackCreate ───────────────────────────────────────────────────────

func TestValidatePackCreate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := ValidatePackCreate("my-pack", 2, 1); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("empty name", func(t *testing.T) {
		err := ValidatePackCreate("", 2, 1)
		if err == nil || !strings.Contains(err.Error(), "name") {
			t.Errorf("expected name error, got %v", err)
		}
	})
	t.Run("whitespace name", func(t *testing.T) {
		err := ValidatePackCreate("   ", 2, 1)
		if err == nil || !strings.Contains(err.Error(), "name") {
			t.Errorf("expected name error, got %v", err)
		}
	})
	t.Run("no skills", func(t *testing.T) {
		err := ValidatePackCreate("my-pack", 0, 1)
		if err == nil || !strings.Contains(err.Error(), "skill") {
			t.Errorf("expected skill error, got %v", err)
		}
	})
	t.Run("no repos", func(t *testing.T) {
		err := ValidatePackCreate("my-pack", 1, 0)
		if err == nil || !strings.Contains(err.Error(), "repo") {
			t.Errorf("expected repo error, got %v", err)
		}
	})
}

// ─── buildPackFromWizard ──────────────────────────────────────────────────────

func TestBuildPackFromWizard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {URL: "https://github.com/example/my-repo", CachePath: "/tmp/my-repo"},
		},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	cfg := &config.Config{Agents: make(map[string]config.AgentConfig)}

	skills := []repo.SkillInfo{
		{Address: "my-repo/coding/debugger", RepoName: "my-repo"},
		{Address: "my-repo/coding/tester", RepoName: "my-repo"},
		{Address: "other-repo/utils/lint", RepoName: "other-repo"},
	}

	m := packCreateModel{
		nameInput:    "Go Dev",
		descInput:    "Go development pack",
		allSkills:    skills,
		skillSel:     map[int]bool{0: true, 1: true}, // select first two skills
		cfg:          cfg,
		st:           st,
	}

	p, err := m.buildPackFromWizard()
	if err != nil {
		t.Fatalf("buildPackFromWizard() error: %v", err)
	}

	if p.Name != "Go Dev" {
		t.Errorf("Name = %q, want %q", p.Name, "Go Dev")
	}
	if p.Description != "Go development pack" {
		t.Errorf("Description = %q, want %q", p.Description, "Go development pack")
	}
	if len(p.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(p.Skills))
	}
	if len(p.Repos) != 1 || p.Repos[0].Name != "my-repo" {
		t.Errorf("Repos = %+v, want [{my-repo ...}]", p.Repos)
	}
}

func TestBuildPackFromWizard_Errors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{Agents: make(map[string]config.AgentConfig)}

	t.Run("empty name", func(t *testing.T) {
		m := packCreateModel{
			nameInput: "",
			allSkills: []repo.SkillInfo{{Address: "r/s", RepoName: "r"}},
			skillSel:  map[int]bool{0: true},
			cfg:       cfg,
			st: &state.State{
				Repos:           make(map[string]state.RepoRecord),
				InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
			},
		}
		_, err := m.buildPackFromWizard()
		if err == nil || !strings.Contains(err.Error(), "name") {
			t.Errorf("expected name error, got %v", err)
		}
	})

	t.Run("no skills selected", func(t *testing.T) {
		m := packCreateModel{
			nameInput: "my-pack",
			allSkills: []repo.SkillInfo{{Address: "r/s", RepoName: "r"}},
			skillSel:  map[int]bool{0: false},
			cfg:       cfg,
			st: &state.State{
				Repos:           make(map[string]state.RepoRecord),
				InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
			},
		}
		_, err := m.buildPackFromWizard()
		if err == nil || !strings.Contains(err.Error(), "skill") {
			t.Errorf("expected skill error, got %v", err)
		}
	})
}

// ─── renderYAML ───────────────────────────────────────────────────────────────

func TestRenderYAML(t *testing.T) {
	p := &packYAML{
		Name:        "go-dev",
		Description: "Go development pack",
		Repos:       []packRepoRef{{Name: "my-repo", URL: "https://github.com/example/my-repo"}},
		Skills:      []string{"my-repo/coding/debugger", "my-repo/coding/tester"},
	}

	yml, err := renderYAML(p)
	if err != nil {
		t.Fatalf("renderYAML() error: %v", err)
	}
	if !strings.Contains(yml, "name: go-dev") {
		t.Errorf("YAML missing name field:\n%s", yml)
	}
	if !strings.Contains(yml, "description: Go development pack") {
		t.Errorf("YAML missing description field:\n%s", yml)
	}
	if !strings.Contains(yml, "my-repo/coding/debugger") {
		t.Errorf("YAML missing skill:\n%s", yml)
	}
}

func TestRenderYAML_NoDescription(t *testing.T) {
	p := &packYAML{
		Name:    "minimal",
		Repos:   []packRepoRef{{Name: "r", URL: "https://example.com/r"}},
		Skills:  []string{"r/a"},
	}
	yml, err := renderYAML(p)
	if err != nil {
		t.Fatalf("renderYAML() error: %v", err)
	}
	if strings.Contains(yml, "description") {
		t.Errorf("YAML should omit description when empty:\n%s", yml)
	}
}

// ─── rebuildVisible (skill filter) ───────────────────────────────────────────

func TestRebuildVisible(t *testing.T) {
	m := packCreateModel{
		allSkills: []repo.SkillInfo{
			{Address: "my-repo/coding/debugger"},
			{Address: "my-repo/coding/tester"},
			{Address: "my-repo/writing/blogger"},
		},
		skillSel:      make(map[int]bool),
		visibleSkills: make([]int, 0),
	}

	// No filter — all skills visible.
	m.rebuildVisible()
	if len(m.visibleSkills) != 3 {
		t.Errorf("no filter: visible=%d, want 3", len(m.visibleSkills))
	}

	// Filter "coding" — only first two.
	m.skillFilter = "coding"
	m.rebuildVisible()
	if len(m.visibleSkills) != 2 {
		t.Errorf("filter 'coding': visible=%d, want 2", len(m.visibleSkills))
	}

	// Filter "blogger".
	m.skillFilter = "blogger"
	m.rebuildVisible()
	if len(m.visibleSkills) != 1 {
		t.Errorf("filter 'blogger': visible=%d, want 1", len(m.visibleSkills))
	}

	// Filter no match.
	m.skillFilter = "zzzzz"
	m.rebuildVisible()
	if len(m.visibleSkills) != 0 {
		t.Errorf("filter 'zzzzz': visible=%d, want 0", len(m.visibleSkills))
	}
}

// ─── sanitizePackPath ────────────────────────────────────────────────────────

func TestSanitizePackPath(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"packs/go-dev", "packs/go-dev", false},
		{"  packs/go-dev  ", "packs/go-dev", false},
		{"packs//go-dev", "packs/go-dev", false}, // double slash cleaned
		{"/etc/passwd", "", true},                // absolute path rejected
		{"../escape", "", true},                  // traversal rejected
		{"packs/../../../etc", "", true},          // traversal via clean rejected
		{"", "", true},                           // empty rejected
		{".", "", true},                           // dot-only rejected
	}
	for _, tc := range cases {
		got, err := sanitizePackPath(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("sanitizePackPath(%q) error=%v wantErr=%v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("sanitizePackPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── cmdCommitAndPush filesystem write (smoke test only) ─────────────────────

// TestPackYAMLWriteAndRead verifies that pack YAML content can be written to
// and read back from disk without corruption. It does not test git operations
// (which require a real repo and remote) — that coverage lives in gitops tests.
func TestPackYAMLWriteAndRead(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	packDir := filepath.Join(t.TempDir(), "packs", "test-pack")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "name: test-pack\nrepos:\n  - name: r\n    url: https://example.com\nskills:\n  - r/s\n"
	packFile := filepath.Join(packDir, "pack.yaml")
	if err := os.WriteFile(packFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(packFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("pack.yaml round-trip mismatch:\ngot:  %q\nwant: %q", string(data), content)
	}
}
