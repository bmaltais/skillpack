package repo_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

func TestNameFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		// Standard HTTPS GitHub URLs
		{"https://github.com/owner/awesome-skills.git", "owner-awesome-skills"},
		{"https://github.com/owner/awesome-skills", "owner-awesome-skills"},
		{"https://github.com/owner/repo/", "owner-repo"},
		// SSH syntax
		{"git@github.com:owner/awesome-skills.git", "owner-awesome-skills"},
		{"git@gitlab.com:org/repo.git", "org-repo"},
		// No owner segment (host/repo only)
		{"https://internal.company.com/myrepo.git", "myrepo"},
		// Trailing slash after .git-stripped path
		{"https://github.com/owner/repo.git/", "owner-repo"},
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

func TestDiscoverSkills_SkipsGitDir(t *testing.T) {
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
		t.Errorf("expected 1 skill (.git skipped), got %d: %v", len(skills), skillAddrs(skills))
	}
}

func TestDiscoverSkills_HiddenNonGitDirs(t *testing.T) {
	// Skills nested under hidden directories other than .git (e.g. .agents/skills/<name>)
	// must be discovered. This mirrors the warpdotdev/common-skills layout.
	root := t.TempDir()
	mkSkill(t, root, filepath.Join(".agents", "skills", "debugger"))
	mkSkill(t, root, filepath.Join(".agents", "skills", "blogger"))
	mkSkill(t, root, "top-level-skill")

	st := &state.State{
		Repos:           map[string]state.RepoRecord{"r": {CachePath: root}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	skills, err := repo.DiscoverSkills("r", st)
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
	for _, want := range []string{"r/.agents/skills/debugger", "r/.agents/skills/blogger", "r/top-level-skill"} {
		if !addrSet[want] {
			t.Errorf("missing skill %q; got %v", want, skillAddrs(skills))
		}
	}
}

func TestDiscoverSkills_PrunesNestedTraversalOrderIndependently(t *testing.T) {
	root := t.TempDir()
	mkSkill(t, root, "parent-skill")
	// "Examples" sorts before "SKILL.md" and would be walked first if pruning
	// occurred only when visiting SKILL.md as a file.
	mkSkill(t, root, filepath.Join("parent-skill", "Examples", "nested"))

	st := &state.State{
		Repos:           map[string]state.RepoRecord{"r": {CachePath: root}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	skills, err := repo.DiscoverSkills("r", st)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}

	addrSet := make(map[string]bool)
	for _, s := range skills {
		addrSet[s.Address] = true
	}
	if len(skills) != 1 {
		t.Fatalf("expected only parent skill due to pruning, got %d: %v", len(skills), skillAddrs(skills))
	}
	if !addrSet["r/parent-skill"] {
		t.Fatalf("missing parent skill; got %v", skillAddrs(skills))
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

// TestUpdate_DivergedCache seeds a local cache clone that has an extra commit
// not present on the remote, then calls Update() and asserts:
//  1. Update() returns no error (previously it returned non-fast-forward).
//  2. The cache HEAD matches the remote HEAD (hard reset succeeded).
func TestUpdate_DivergedCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}

	// --- set up "remote" (plain, non-bare) repo with one commit ---
	remoteDir := t.TempDir()
	remote, err := gogit.PlainInit(remoteDir, false)
	if err != nil {
		t.Fatalf("remote init: %v", err)
	}
	rw, err := remote.Worktree()
	if err != nil {
		t.Fatalf("remote worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, "README.md"), []byte("# remote"), 0600); err != nil {
		t.Fatalf("remote write: %v", err)
	}
	if _, err := rw.Add("README.md"); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	remoteCommit, err := rw.Commit("initial commit", &gogit.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("remote commit: %v", err)
	}

	// --- clone into cacheDir ---
	cacheDir := t.TempDir()
	cache, err := gogit.PlainClone(cacheDir, false, &gogit.CloneOptions{URL: remoteDir})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	// --- add a diverging commit to the cache (not pushed to remote) ---
	cw, err := cache.Worktree()
	if err != nil {
		t.Fatalf("cache worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "diverged.md"), []byte("diverged"), 0600); err != nil {
		t.Fatalf("diverged write: %v", err)
	}
	if _, err := cw.Add("diverged.md"); err != nil {
		t.Fatalf("diverged add: %v", err)
	}
	if _, err := cw.Commit("diverged commit", &gogit.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("diverged commit: %v", err)
	}

	// sanity check: cache HEAD ≠ remote HEAD
	cacheHead, _ := cache.Head()
	if cacheHead.Hash() == remoteCommit {
		t.Fatal("precondition failed: cache HEAD should differ from remote HEAD")
	}

	// --- run Update() ---
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"test-repo": {URL: remoteDir, CachePath: cacheDir},
		},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	if _, err := repo.Update("test-repo", "", st); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// --- assert cache HEAD == remote HEAD ---
	cacheHead, err = cache.Head()
	if err != nil {
		t.Fatalf("cache head after update: %v", err)
	}
	if cacheHead.Hash() != remoteCommit {
		t.Errorf("cache HEAD = %s, want %s (remote HEAD)", cacheHead.Hash(), remoteCommit)
	}

	// diverged.md must no longer exist
	if _, err := os.Stat(filepath.Join(cacheDir, "diverged.md")); err == nil {
		t.Error("diverged.md should have been removed by hard reset")
	}
}

// TestAdd_RecoverFromLeftOverCache verifies that re-adding a repo after
// repo remove does not fail when the old cache directory still exists on disk.
// It should re-register the existing clone rather than returning an error.
func TestAdd_RecoverFromLeftOverCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}

	// --- set up a bare-ish "remote" repo with one commit ---
	remoteDir := t.TempDir()
	remote, err := gogit.PlainInit(remoteDir, false)
	if err != nil {
		t.Fatalf("remote init: %v", err)
	}
	rw, err := remote.Worktree()
	if err != nil {
		t.Fatalf("remote worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, "README.md"), []byte("# remote"), 0600); err != nil {
		t.Fatalf("remote write: %v", err)
	}
	if _, err := rw.Add("README.md"); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	if _, err := rw.Commit("initial", &gogit.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("remote commit: %v", err)
	}

	// --- pre-populate the cache dir as if a prior clone was left behind ---
	reposDir, err := config.ReposDir()
	if err != nil {
		t.Fatalf("ReposDir: %v", err)
	}
	if err := os.MkdirAll(reposDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cacheDir := filepath.Join(reposDir, "test-repo")
	if _, err := gogit.PlainClone(cacheDir, false, &gogit.CloneOptions{URL: remoteDir}); err != nil {
		t.Fatalf("pre-populate clone: %v", err)
	}

	// --- state without the repo registered (simulates repo remove) ---
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	// --- re-add the repo: should recover, not fail ---
	recovered, err := repo.Add("test-repo", remoteDir, "", st)
	if err != nil {
		t.Fatalf("Add after remove: unexpected error: %v", err)
	}
	if !recovered {
		t.Error("Add should report recovered=true when reusing existing cache")
	}

	// repo must be registered in state
	rec, ok := st.Repos["test-repo"]
	if !ok {
		t.Fatal("repo not registered in state after recovery")
	}
	if rec.CachePath != cacheDir {
		t.Errorf("CachePath = %q, want %q", rec.CachePath, cacheDir)
	}
	if rec.URL != remoteDir {
		t.Errorf("URL = %q, want %q", rec.URL, remoteDir)
	}
}
