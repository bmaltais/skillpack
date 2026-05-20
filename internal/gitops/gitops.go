// Package gitops provides deep git operations for the skillpack codebase.
// It consolidates all go-git ceremony (auth, stage, commit, push, diff) behind
// a small interface so callers express intent rather than reimplementing git plumbing.
package gitops

import (
	"fmt"
	"os"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// IsSSHURL reports whether the URL uses SSH transport.
func IsSSHURL(url string) bool {
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}

// Auth resolves the appropriate transport.AuthMethod for a remote URL.
// token is optional; pass "" for SSH-only or env-based auth.
func Auth(url, token string) (transport.AuthMethod, error) {
	if IsSSHURL(url) {
		auth, err := gitssh.NewSSHAgentAuth("git")
		if err != nil {
			return nil, fmt.Errorf("SSH agent unavailable (ensure ssh-agent is running): %w", err)
		}
		return auth, nil
	}
	if token != "" {
		return &githttp.BasicAuth{Username: "x-access-token", Password: token}, nil
	}
	return nil, nil
}

// DefaultSignature returns the standard commit signature for skillpack operations.
func DefaultSignature() *object.Signature {
	return &object.Signature{
		Name:  "skillpack",
		Email: "skillpack@local",
		When:  time.Now(),
	}
}

// CommitResult holds the outcome of a CommitAndPush operation.
type CommitResult struct {
	Committed  bool   // true if there were changes to commit
	CommitHash string // the new commit SHA (empty if !Committed)
}

// CommitAndPush stages all changes under skillRelPath in the repo at cachePath,
// commits with the given message, and pushes to origin.
//
// If no files under skillRelPath have changed, it returns Committed=false and
// does not create an empty commit or push.
//
// remoteURL and token are used for push authentication.
func CommitAndPush(cachePath, skillRelPath, message, remoteURL, token string) (*CommitResult, error) {
	r, err := gogit.PlainOpen(cachePath)
	if err != nil {
		return nil, fmt.Errorf("opening repo at %s: %w", cachePath, err)
	}
	w, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("getting worktree: %w", err)
	}

	// Stage all changes under the skill path
	status, err := w.Status()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	staged := false
	for path, fs := range status {
		if !pathUnderPrefix(path, skillRelPath) {
			continue
		}
		if fs.Worktree == gogit.Deleted || fs.Staging == gogit.Deleted {
			if _, err := w.Remove(path); err != nil {
				return nil, fmt.Errorf("git rm %s: %w", path, err)
			}
		} else {
			if _, err := w.Add(path); err != nil {
				return nil, fmt.Errorf("git add %s: %w", path, err)
			}
		}
		staged = true
	}

	if !staged {
		return &CommitResult{Committed: false}, nil
	}

	sig := DefaultSignature()
	commitHash, err := w.Commit(message, &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		return nil, fmt.Errorf("git commit: %w", err)
	}

	if err := push(r, remoteURL, token); err != nil {
		return nil, err
	}

	return &CommitResult{Committed: true, CommitHash: commitHash.String()}, nil
}

// HeadSHA returns the current HEAD commit SHA of the repo at cachePath.
func HeadSHA(cachePath string) (string, error) {
	r, err := gogit.PlainOpen(cachePath)
	if err != nil {
		return "", fmt.Errorf("opening repo at %s: %w", cachePath, err)
	}
	ref, err := r.Head()
	if err != nil {
		return "", fmt.Errorf("getting HEAD: %w", err)
	}
	return ref.Hash().String(), nil
}

// DiffSkillChanged reports whether any file under skillRelPath changed between
// two commits in the repo at cachePath.
func DiffSkillChanged(cachePath, oldSHA, newSHA, skillRelPath string) (bool, error) {
	r, err := gogit.PlainOpen(cachePath)
	if err != nil {
		return false, fmt.Errorf("opening repo at %s: %w", cachePath, err)
	}

	oldCommit, err := r.CommitObject(plumbing.NewHash(oldSHA))
	if err != nil {
		return false, fmt.Errorf("resolving commit %s: %w", safeShort(oldSHA), err)
	}
	newCommit, err := r.CommitObject(plumbing.NewHash(newSHA))
	if err != nil {
		return false, fmt.Errorf("resolving commit %s: %w", safeShort(newSHA), err)
	}

	oldTree, err := oldCommit.Tree()
	if err != nil {
		return false, err
	}
	newTree, err := newCommit.Tree()
	if err != nil {
		return false, err
	}

	changes, err := object.DiffTree(oldTree, newTree)
	if err != nil {
		return false, err
	}
	for _, change := range changes {
		if pathUnderPrefix(change.From.Name, skillRelPath) || pathUnderPrefix(change.To.Name, skillRelPath) {
			return true, nil
		}
	}
	return false, nil
}

// ListFilesAtCommit returns a map of relPath→content for all files under
// skillRelPath in the repo at cachePath at the given commit SHA.
func ListFilesAtCommit(cachePath, commitSHA, skillRelPath string) (map[string]string, error) {
	r, err := gogit.PlainOpen(cachePath)
	if err != nil {
		return nil, fmt.Errorf("opening repo at %s: %w", cachePath, err)
	}

	commit, err := r.CommitObject(plumbing.NewHash(commitSHA))
	if err != nil {
		return nil, fmt.Errorf("resolving commit %s: %w", safeShort(commitSHA), err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	files := make(map[string]string)
	err = tree.Files().ForEach(func(f *object.File) error {
		if !pathUnderPrefix(f.Name, skillRelPath) {
			return nil
		}
		rel := strings.TrimPrefix(f.Name, skillRelPath+"/")
		content, err := f.Contents()
		if err != nil {
			return err
		}
		files[rel] = content
		return nil
	})
	return files, err
}

// push pushes the repo to origin with appropriate auth.
func push(r *gogit.Repository, remoteURL, token string) error {
	pushOpts := &gogit.PushOptions{Progress: os.Stdout}
	auth, err := Auth(remoteURL, token)
	if err != nil {
		return err
	}
	pushOpts.Auth = auth

	if err := r.Push(pushOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

// pathUnderPrefix reports whether filePath equals prefix or starts with prefix+"/".
func pathUnderPrefix(filePath, prefix string) bool {
	if filePath == "" || prefix == "" {
		return false
	}
	return filePath == prefix || strings.HasPrefix(filePath, prefix+"/")
}

func safeShort(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}
