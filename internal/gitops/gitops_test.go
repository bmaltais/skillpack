package gitops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestIsSSHURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"git@github.com:user/repo.git", true},
		{"ssh://git@github.com/user/repo.git", true},
		{"https://github.com/user/repo.git", false},
		{"http://github.com/user/repo.git", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsSSHURL(tt.url); got != tt.want {
			t.Errorf("IsSSHURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestPathUnderPrefix(t *testing.T) {
	tests := []struct {
		filePath string
		prefix   string
		want     bool
	}{
		{"coding/debugger/SKILL.md", "coding/debugger", true},
		{"coding/debugger", "coding/debugger", true},
		{"coding/debugger-v2/SKILL.md", "coding/debugger", false},
		{"other/file.go", "coding/debugger", false},
		{"", "coding/debugger", false},
		{"coding/debugger/file.go", "", false},
	}
	for _, tt := range tests {
		if got := pathUnderPrefix(tt.filePath, tt.prefix); got != tt.want {
			t.Errorf("pathUnderPrefix(%q, %q) = %v, want %v", tt.filePath, tt.prefix, got, tt.want)
		}
	}
}

func TestSafeShort(t *testing.T) {
	tests := []struct {
		sha  string
		want string
	}{
		{"abcdef1234567890", "abcdef12"},
		{"short", "short"},
		{"", ""},
		{"12345678", "12345678"},
	}
	for _, tt := range tests {
		if got := safeShort(tt.sha); got != tt.want {
			t.Errorf("safeShort(%q) = %q, want %q", tt.sha, got, tt.want)
		}
	}
}

// TestCommitAndPush_PushFailure_NoHeadAdvance is a regression test for issue #71.
//
// When CommitAndPush succeeds at committing but fails at pushing (e.g. auth
// error on a third-party repo), the local cache HEAD must not advance.
// Without a rollback, the dangling commit poisons the cache HEAD SHA and
// causes every other skill in the same repo to appear as needing an update on
// subsequent syncs.
func TestCommitAndPush_PushFailure_NoHeadAdvance(t *testing.T) {
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	// Init a local repo and make an initial commit so HEAD is valid.
	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	initFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(initFile, []byte("init"), 0o644); err != nil {
		t.Fatalf("write init file: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	initialHash, err := wt.Commit("initial commit", &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Add a skill file that CommitAndPush will stage and commit.
	skillRelPath := "skills/my-skill"
	skillDir := filepath.Join(repoDir, skillRelPath)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	// Call CommitAndPush with an unreachable remote — push must fail.
	_, err = CommitAndPush(repoDir, skillRelPath, "test: add skill", "http://localhost:19999/no-such-repo.git", "")
	if err == nil {
		t.Fatal("expected push to fail but got nil error")
	}

	// HEAD must still point to the initial commit — the failed push must not leave
	// a dangling commit that advances the HEAD SHA.
	headSHA, err := HeadSHA(repoDir)
	if err != nil {
		t.Fatalf("HeadSHA after failed push: %v", err)
	}
	if headSHA != initialHash.String() {
		t.Errorf("HEAD advanced after failed push: got %s, want %s", headSHA[:8], initialHash.String()[:8])
	}
}

func TestIsGHOToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"gho_abc123", true},
		{"ghp_abc123", false},
		{"", false},
		{"gho_", true},
		{"random", false},
	}
	for _, tt := range tests {
		if got := isGHOToken(tt.token); got != tt.want {
			t.Errorf("isGHOToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

// TestCommitAndPush_GhoTokenHint verifies that when CommitAndPush fails with a
// gho_ token, the error message includes a hint about refreshing the token.
// This is a behavioural test: it checks the error string contains the expected
// hint text rather than just the raw git error.
func TestCommitAndPush_GhoTokenHint(t *testing.T) {
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	initFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(initFile, []byte("init"), 0o644); err != nil {
		t.Fatalf("write init file: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wt.Commit("initial commit", &gogit.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	skillRelPath := "skills/my-skill"
	skillDir := filepath.Join(repoDir, skillRelPath)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	// Use an unreachable remote with a gho_ token — push will fail with a
	// transport error. The error message should contain our hint.
	_, err = CommitAndPush(repoDir, skillRelPath, "test: add skill", "http://localhost:19999/no-such-repo.git", "gho_test123")
	if err == nil {
		t.Fatal("expected push to fail but got nil error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "hint:") {
		t.Errorf("expected hint in error message, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "gh auth token") {
		t.Errorf("expected 'gh auth token' in hint, got: %s", errMsg)
	}
}
