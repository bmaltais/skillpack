// Package gitops provides deep git operations for the skillpack codebase.
// It consolidates all go-git ceremony (auth, stage, commit, push, diff) behind
// a small interface so callers express intent rather than reimplementing git plumbing.
package gitops

import (
	"errors"
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
// token is optional; pass "" to skip HTTPS auth (callers are responsible for
// resolving tokens from config/env before calling this function).
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
		// Decide add vs remove based on worktree state: if the file is
		// deleted from the working tree, remove it from the index.
		// Otherwise add it (even if the index has a stale staged-delete,
		// w.Add will reflect the current worktree state).
		if fs.Worktree == gogit.Deleted {
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

	// Record the current HEAD ref before committing so we can roll back if the
	// push fails. Without this, a dangling local commit advances the cache HEAD
	// SHA and causes every other skill in the repo to appear as needing an
	// update on subsequent syncs (issue #71).
	headRef, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("getting HEAD before commit: %w", err)
	}
	preCommitRef := plumbing.NewHashReference(headRef.Name(), headRef.Hash())

	sig := DefaultSignature()
	commitHash, err := w.Commit(message, &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		return nil, fmt.Errorf("git commit: %w", err)
	}

	if err := push(r, remoteURL, token); err != nil {
		// Roll back the local commit so the cache HEAD stays at the
		// pre-commit SHA. Best-effort: ignore the reset error since we
		// already have a push error to return.
		_ = r.Storer.SetReference(preCommitRef)
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
	return diffSkillInRepo(r, oldSHA, newSHA, skillRelPath)
}

// DiffSkillChangedFromHEAD is like DiffSkillChanged but resolves HEAD internally,
// avoiding a redundant repo open when the caller needs both HEAD SHA and a diff check.
// Returns (headSHA, changed, error).
func DiffSkillChangedFromHEAD(cachePath, oldSHA, skillRelPath string) (string, bool, error) {
	r, err := gogit.PlainOpen(cachePath)
	if err != nil {
		return "", false, fmt.Errorf("opening repo at %s: %w", cachePath, err)
	}
	ref, err := r.Head()
	if err != nil {
		return "", false, fmt.Errorf("getting HEAD: %w", err)
	}
	headSHA := ref.Hash().String()
	if headSHA == oldSHA {
		return headSHA, false, nil
	}
	changed, err := diffSkillInRepo(r, oldSHA, headSHA, skillRelPath)
	return headSHA, changed, err
}

// diffSkillInRepo performs the actual tree diff on an already-open repository.
func diffSkillInRepo(r *gogit.Repository, oldSHA, newSHA, skillRelPath string) (bool, error) {

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
		if isGHOToken(token) && isTransportAuthError(err) {
			return fmt.Errorf("push failed: %v\nhint: your gho_ token appears to be expired or revoked\nhint: run: gh auth token -h github.com -u <user>\nhint: then update credentials in ~/.skillpack/config.yaml", err)
		}
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

// isTransportAuthError reports whether err is an authentication/authorisation
// failure from the go-git transport layer, as opposed to a network, DNS, or
// repository-not-found failure that would make a token-refresh hint misleading.
func isTransportAuthError(err error) bool {
	return errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed)
}

// isGHOToken reports whether token is a GitHub OAuth token (gho_ prefix).
// These tokens can expire or be revoked after inactivity, producing
// "Invalid username or token" errors that are hard to diagnose.
//
// Defined here in gitops to avoid importing from the higher-level repo package.
func isGHOToken(token string) bool {
	return strings.HasPrefix(token, "gho_")
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
