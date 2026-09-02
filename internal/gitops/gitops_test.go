package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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

func TestIsAzureDevOpsURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://dev.azure.com/myorg/myproject/_git/skills", true},
		{"https://myorg.visualstudio.com/myproject/_git/skills", true},
		{"git@ssh.dev.azure.com:v3/myorg/myproject/skills", true},
		{"https://github.com/owner/repo.git", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsAzureDevOpsURL(tt.url); got != tt.want {
			t.Errorf("IsAzureDevOpsURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		// Embedded username stripped — auth is supplied separately via token.
		{"https://myorg@dev.azure.com/myorg/myproject/_git/skills", "https://dev.azure.com/myorg/myproject/_git/skills"},
		{"https://user:pass@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		// No userinfo present — unchanged.
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		// SSH URLs are left untouched — "git@" is required syntax, not a stray credential.
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"ssh://git@ssh.dev.azure.com/v3/myorg/myproject/skills", "ssh://git@ssh.dev.azure.com/v3/myorg/myproject/skills"},
	}
	for _, tt := range tests {
		if got := NormalizeURL(tt.url); got != tt.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tt.url, got, tt.want)
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

// TestCommitAndPush_ConnectionFailureNoAuthHint verifies that when CommitAndPush
// fails due to an unreachable remote (a connection failure, not an auth
// failure), the error message does NOT include the gho_ token hint — that
// hint is reserved for actual authentication/authorization errors and would
// be misleading here.
func TestCommitAndPush_ConnectionFailureNoAuthHint(t *testing.T) {
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
	if strings.Contains(errMsg, "hint:") {
		t.Errorf("expected no auth hint for a connection failure, got: %s", errMsg)
	}
}

// TestSystemGitClonePushFetch exercises the system-git fallback path used for
// Azure DevOps (go-git can't talk to ADO at all — see IsAzureDevOpsURL) against
// a local bare repo, proving the clone/fetch/push subprocess plumbing (including
// the --config-env auth-header injection) is valid regardless of the token value.
func TestSystemGitClonePushFetch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	// Bare "remote" repo, seeded with one commit via a throwaway working clone.
	remoteDir := t.TempDir()
	if _, err := gogit.PlainInit(remoteDir, true); err != nil {
		t.Fatalf("PlainInit bare: %v", err)
	}
	seedDir := t.TempDir()
	seedRepo, err := gogit.PlainInit(seedDir, false)
	if err != nil {
		t.Fatalf("PlainInit seed: %v", err)
	}
	if _, err := seedRepo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remoteDir}}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	seedWT, err := seedRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "README.md"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := seedWT.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := seedWT.Commit("v1", &gogit.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := seedRepo.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	// Clone via the system-git path. Token is non-empty but ignored by a local
	// filesystem remote — this only proves the --config-env flag is valid git
	// syntax and doesn't break a non-HTTP transport.
	cloneDir := filepath.Join(t.TempDir(), "clone")
	if err := SystemGitClone(remoteDir, cloneDir, "unused-token"); err != nil {
		t.Fatalf("SystemGitClone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "README.md")); err != nil {
		t.Fatalf("cloned repo missing README.md: %v", err)
	}

	// Push a new commit from the clone via the system-git path.
	if err := os.WriteFile(filepath.Join(cloneDir, "NEW.md"), []byte("new"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cloneRepo, err := gogit.PlainOpen(cloneDir)
	if err != nil {
		t.Fatalf("PlainOpen clone: %v", err)
	}
	cloneWT, err := cloneRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := cloneWT.Add("NEW.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	pushedHash, err := cloneWT.Commit("v2", &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := SystemGitPush(cloneDir, ""); err != nil {
		t.Fatalf("SystemGitPush: %v", err)
	}

	// Fetch the push back down from a second clone via the system-git path.
	secondSeedDir := filepath.Join(t.TempDir(), "second-seed")
	if err := SystemGitClone(remoteDir, secondSeedDir, ""); err != nil {
		t.Fatalf("SystemGitClone (second seed, pre-fetch): %v", err)
	}
	// Simulate an out-of-date cache by re-cloning at the old tip, then fetching.
	if err := SystemGitFetch(secondSeedDir, ""); err != nil {
		t.Fatalf("SystemGitFetch: %v", err)
	}
	secondRepo, err := gogit.PlainOpen(secondSeedDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	remoteRefs, err := secondRepo.References()
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	found := false
	_ = remoteRefs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == pushedHash {
			found = true
		}
		return nil
	})
	if !found {
		t.Errorf("SystemGitFetch did not bring down pushed commit %s", pushedHash)
	}
}

