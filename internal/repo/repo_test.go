package repo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bernard/skillpack/internal/repo"
	"github.com/bernard/skillpack/internal/state"
)

func TestNameFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/awesome-skills.git", "awesome-skills"},
		{"https://github.com/owner/awesome-skills", "awesome-skills"},
		{"git@github.com:owner/awesome-skills.git", "awesome-skills"},
		{"https://github.com/owner/repo/", "repo"},
	}
	for _, tc := range cases {
		got := repo.NameFromURL(tc.url)
		if got != tc.want {
			t.Errorf("NameFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestDiscoverSkills_Flat(t *testing.T) {
	// Build a fake repo cache on disk
	root := t.TempDir()
	mkSkill(t, root, "debugger")
	mkSkill(t, root, "blogger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: root},
		},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	skills, err := repo.DiscoverSkills("my-repo", st)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestDiscoverSkills_Categorised(t *testing.T) {
	root := t.TempDir()
	mkSkill(t, root, filepath.Join("coding", "debugger"))
	mkSkill(t, root, filepath.Join("writing", "blogger"))
	mkSkill(t, root, "misc")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {CachePath: root},
		},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	skills, err := repo.DiscoverSkills("my-repo", st)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(skills) != 3 {
		t.Errorf("expected 3 skills, got %d: %v", len(skills), skillAddrs(skills))
	}

	addrSet := make(map[string]bool)
	for _, s := range skills {
		addrSet[s.Address] = true
	}
	for _, want := range []string{"my-repo/coding/debugger", "my-repo/writing/blogger", "my-repo/misc"} {
		if !addrSet[want] {
			t.Errorf("missing skill %q; got %v", want, skillAddrs(skills))
		}
	}
}

func TestDiscoverSkills_SkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	mkSkill(t, root, "valid-skill")
	// A SKILL.md inside .git should not be discovered
	mkSkill(t, root, filepath.Join(".git", "hooks", "sneaky"))

	st := &state.State{
		Repos:           map[string]state.RepoRecord{"r": {CachePath: root}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	skills, err := repo.DiscoverSkills("r", st)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected 1 skill (hidden dir skipped), got %d: %v", len(skills), skillAddrs(skills))
	}
}

func TestFindSkill_NotFound(t *testing.T) {
	root := t.TempDir()
	st := &state.State{
		Repos:           map[string]state.RepoRecord{"r": {CachePath: root}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	_, err := repo.FindSkill("r/nonexistent", st)
	if err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestFindSkill_InvalidAddress(t *testing.T) {
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	_, err := repo.FindSkill("noslash", st)
	if err == nil {
		t.Error("expected error for address without slash")
	}
}

func mkSkill(t *testing.T, root, relPath string) {
	t.Helper()
	dir := filepath.Join(root, relPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# skill"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func skillAddrs(skills []repo.SkillInfo) []string {
	out := make([]string, len(skills))
	for i, s := range skills {
		out[i] = s.Address
	}
	return out
}
