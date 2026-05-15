package skill

import (
	"fmt"
	"os"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/bernard/skillpack/internal/state"
)

// Publish pushes local edits of an installed skill back to the remote repo.
// Semantically equivalent to ForceLocal: the local copy wins unconditionally.
func Publish(addr, agentName, token string, st *state.State) error {
	return ForceLocal(addr, agentName, token, st)
}

// PublishNew copies a local skill directory into the named repo, commits, and pushes.
// The skill is placed at <repo-root>/<basename-of-localDir>.
// The skill is NOT auto-installed; use `skillpack install` afterwards.
// Returns the new skill address (e.g. "my-repo/my-new-skill") on success.
func PublishNew(localDir, repoName, token string, st *state.State) (string, error) {
	if _, err := os.Stat(localDir); err != nil {
		return "", fmt.Errorf("local dir %q not found: %w", localDir, err)
	}
	if _, err := os.Stat(localDir + "/SKILL.md"); err != nil {
		return "", fmt.Errorf("%s does not contain a SKILL.md — not a valid skill directory", localDir)
	}

	repoRec, ok := st.Repos[repoName]
	if !ok {
		return "", fmt.Errorf("repo %q not registered — add it first with: skillpack repo add <name> <url>", repoName)
	}

	// Use the local dir's basename as the skill name in the repo
	skillName := localDir
	// Strip trailing slashes and leading ./
	for strings.HasSuffix(skillName, "/") {
		skillName = skillName[:len(skillName)-1]
	}
	if idx := strings.LastIndex(skillName, "/"); idx >= 0 {
		skillName = skillName[idx+1:]
	}
	if skillName == "" || skillName == "." {
		return "", fmt.Errorf("cannot determine skill name from path %q", localDir)
	}

	destPath := repoRec.CachePath + "/" + skillName
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf(
			"skill %q already exists in repo %q — use `skillpack publish %s/%s` to update it",
			skillName, repoName, repoName, skillName,
		)
	}

	if err := copyDir(localDir, destPath); err != nil {
		return "", fmt.Errorf("copying skill to repo cache: %w", err)
	}

	r, err := gogit.PlainOpen(repoRec.CachePath)
	if err != nil {
		return "", fmt.Errorf("opening repo cache: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return "", err
	}

	// Stage all files in the new skill directory
	wStatus, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	for path := range wStatus {
		if strings.HasPrefix(path, skillName+"/") || path == skillName {
			if _, err := w.Add(path); err != nil {
				return "", fmt.Errorf("git add %s: %w", path, err)
			}
		}
	}

	sig := defaultSignature()
	if _, err := w.Commit(
		fmt.Sprintf("skillpack: add %s", skillName),
		&gogit.CommitOptions{Author: sig, Committer: sig},
	); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	pushOpts := &gogit.PushOptions{Progress: os.Stdout}
	if isSSHRemote(repoRec.URL) {
		auth, err := gogitssh.NewSSHAgentAuth("git")
		if err != nil {
			return "", fmt.Errorf("SSH agent unavailable: %w", err)
		}
		pushOpts.Auth = auth
	} else if t := resolveToken(token); t != "" {
		pushOpts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: t}
	}
	if err := r.Push(pushOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("git push: %w", err)
	}

	return repoName + "/" + skillName, nil
}
