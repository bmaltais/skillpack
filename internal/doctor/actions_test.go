package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bmaltais/skillpack/internal/doctor"
	"github.com/bmaltais/skillpack/internal/state"
)

// gitRepoWithRemote turns dir into a committed repo whose origin is a local
// bare clone, so CommitAndPush can really push. Returns the remote path.
func gitRepoWithRemote(t *testing.T, dir string) string {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := r.Worktree()
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "t", Email: "t@t", When: time.Now()}
	if _, err := wt.Commit("init", &gogit.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatal(err)
	}
	remote := t.TempDir()
	if _, err := gogit.PlainClone(remote, true, &gogit.CloneOptions{URL: "file://" + dir}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := r.Config()
	cfg.Remotes["origin"] = &gitconfig.RemoteConfig{Name: "origin", URLs: []string{"file://" + remote},
		Fetch: []gitconfig.RefSpec{"refs/heads/*:refs/remotes/origin/*"}}
	if err := r.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return remote
}

// pairState builds repos a and b (both writable via a token) holding a/t and b/t.
func pairState(t *testing.T, bodyA, bodyB string) *state.State {
	t.Helper()
	st := &state.State{Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
		InstalledPacks:  map[string]state.InstalledPackRecord{}}
	for name, body := range map[string]string{"a": bodyA, "b": bodyB} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "t", "SKILL.md"), body)
		remote := gitRepoWithRemote(t, dir)
		st.Repos[name] = state.RepoRecord{CachePath: dir, URL: "file://" + remote}
	}
	return st
}

func TestDeleteCopy_IdenticalIsRemovedAndPushed(t *testing.T) {
	st := pairState(t, "# same", "# same")
	if err := doctor.DeleteCopy(st, "b/t", "a/t", "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Repos["b"].CachePath, "t")); !os.IsNotExist(err) {
		t.Fatal("b/t should be gone from the cache")
	}
	if _, err := os.Stat(filepath.Join(st.Repos["a"].CachePath, "t", "SKILL.md")); err != nil {
		t.Fatal("the kept copy must be untouched")
	}
	// The deletion reached the remote.
	remote, _ := gogit.PlainOpen(strings.TrimPrefix(st.Repos["b"].URL, "file://"))
	ref, _ := remote.Head()
	c, _ := remote.CommitObject(ref.Hash())
	if !strings.Contains(c.Message, "remove duplicate b/t") {
		t.Fatalf("remote head is %q", c.Message)
	}
}

func TestDeleteCopy_Refusals(t *testing.T) {
	st := pairState(t, "# one", "# two")
	if err := doctor.DeleteCopy(st, "b/t", "a/t", "tok"); err == nil || !strings.Contains(err.Error(), "differ") {
		t.Fatalf("diverged copies must not be deleted, got %v", err)
	}
	st = pairState(t, "# same", "# same")
	st.InstalledSkills["b/t"] = map[string]state.InstalledSkillRecord{"claude": {}}
	if err := doctor.DeleteCopy(st, "b/t", "a/t", "tok"); err == nil || !strings.Contains(err.Error(), "installed") {
		t.Fatalf("installed copy must not be deleted, got %v", err)
	}
	st = pairState(t, "# same", "# same")
	if err := doctor.DeleteCopy(st, "b/t", "a/t", ""); err == nil || !strings.Contains(err.Error(), "write access") {
		t.Fatalf("no token and no SSH must refuse, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.Repos["b"].CachePath, "t", "SKILL.md")); err != nil {
		t.Fatal("a refused delete must leave the files alone")
	}
}

func TestDeleteCopy_PushFailureRestoresFiles(t *testing.T) {
	st := pairState(t, "# same", "# same")
	rec := st.Repos["b"]
	r, _ := gogit.PlainOpen(rec.CachePath)
	cfg, _ := r.Config()
	cfg.Remotes["origin"].URLs = []string{"file:///nonexistent/remote"} // pushes use the repo's own origin
	if err := r.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := doctor.DeleteCopy(st, "b/t", "a/t", "tok"); err == nil {
		t.Fatal("push to a missing remote must fail")
	}
	if _, err := os.Stat(filepath.Join(rec.CachePath, "t", "SKILL.md")); err != nil {
		t.Fatalf("failed push must restore the skill files: %v", err)
	}
}

func TestOverwriteCopy_TargetMatchesSourceKeepsOwnProvenance(t *testing.T) {
	st := pairState(t, "# source", "# target")
	write(t, filepath.Join(st.Repos["a"].CachePath, "t", "extra.md"), "extra")
	write(t, filepath.Join(st.Repos["b"].CachePath, "t", ".skillpack-fork"), "mine")
	write(t, filepath.Join(st.Repos["b"].CachePath, "t", "stale.md"), "old")
	// commit the setup so the overwrite is the only change
	if err := doctor.OverwriteCopy(st, "b/t", "a/t", "tok"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(st.Repos["b"].CachePath, "t")
	if b, _ := os.ReadFile(filepath.Join(dir, "SKILL.md")); string(b) != "# source" {
		t.Fatalf("SKILL.md = %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "extra.md")); err != nil {
		t.Fatal("files from the source must be copied")
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.md")); !os.IsNotExist(err) {
		t.Fatal("files only in the target must be removed")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".skillpack-fork")); string(b) != "mine" {
		t.Fatalf("target's provenance must be kept, got %q", b)
	}
}
