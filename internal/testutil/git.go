package testutil

import (
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var testAuthor = &object.Signature{Name: "test", Email: "test@test.com"}

func commit(t testing.TB, w *gogit.Worktree, msg string) {
	t.Helper()
	sig := *testAuthor
	sig.When = time.Now()
	if _, err := w.Commit(msg, &gogit.CommitOptions{Author: &sig}); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

// InitGitRepo runs git init in dir and commits every file already in it.
func InitGitRepo(t testing.TB, dir string) {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init %s: %v", dir, err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("."); err != nil {
		t.Fatal(err)
	}
	commit(t, w, "initial")
}

// CommitFile stages relPath in the repo at dir and commits it.
func CommitFile(t testing.TB, dir, relPath string) {
	t.Helper()
	r, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add(relPath); err != nil {
		t.Fatal(err)
	}
	commit(t, w, "change "+relPath)
}
