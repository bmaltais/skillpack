package pack_test

import (
	"testing"

	"github.com/bmaltais/skillpack/internal/pack"
)

func TestParse_HappyPath(t *testing.T) {
	yaml := `
name: go-dev
description: Go development skills
repos:
  - name: awesome-skills
    url: https://github.com/example/awesome-skills
skills:
  - awesome-skills/coding/debugger
  - awesome-skills/coding/test-writer
`
	p, err := pack.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "go-dev" {
		t.Errorf("Name = %q, want %q", p.Name, "go-dev")
	}
	if p.Description != "Go development skills" {
		t.Errorf("Description = %q, want %q", p.Description, "Go development skills")
	}
	if len(p.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(p.Repos))
	}
	if p.Repos[0].Name != "awesome-skills" {
		t.Errorf("Repos[0].Name = %q", p.Repos[0].Name)
	}
	if p.Repos[0].URL != "https://github.com/example/awesome-skills" {
		t.Errorf("Repos[0].URL = %q", p.Repos[0].URL)
	}
	if len(p.Skills) != 2 {
		t.Errorf("len(Skills) = %d, want 2", len(p.Skills))
	}
}

func TestParse_DescriptionOptional(t *testing.T) {
	yaml := `
name: minimal
repos:
  - name: r
    url: https://example.com/r
skills:
  - r/skill
`
	p, err := pack.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Description != "" {
		t.Errorf("Description should be empty, got %q", p.Description)
	}
}

func TestParse_MissingName(t *testing.T) {
	yaml := `
repos:
  - name: r
    url: https://example.com/r
skills:
  - r/skill
`
	_, err := pack.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestParse_MissingRepos(t *testing.T) {
	yaml := `
name: mypack
skills:
  - r/skill
`
	_, err := pack.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing repos, got nil")
	}
}

func TestParse_MissingSkills(t *testing.T) {
	yaml := `
name: mypack
repos:
  - name: r
    url: https://example.com/r
`
	_, err := pack.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing skills, got nil")
	}
}

func TestParse_RepoMissingName(t *testing.T) {
	yaml := `
name: mypack
repos:
  - url: https://example.com/r
skills:
  - r/skill
`
	_, err := pack.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for repo missing name, got nil")
	}
}

func TestParse_RepoMissingURL(t *testing.T) {
	yaml := `
name: mypack
repos:
  - name: r
skills:
  - r/skill
`
	_, err := pack.Parse([]byte(yaml))
	if err == nil {
		t.Error("expected error for repo missing url, got nil")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := pack.Parse([]byte(":\t:\n"))
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
