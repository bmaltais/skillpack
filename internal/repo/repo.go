package repo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

// SkillInfo describes a discovered skill inside a repo clone.
type SkillInfo struct {
	Address  string // e.g. "awesome-skills/coding/debugger"
	RepoName string
	RelPath  string // path relative to the repo root, slash-separated
	FullPath string // absolute path on disk
}

// Add clones the remote repo to the local cache and registers it in state.
// token is optional; pass "" to rely on env vars (SKILLPACK_GIT_TOKEN, GITHUB_TOKEN).
func Add(name, url, token string, st *state.State) error {
	if _, exists := st.Repos[name]; exists {
		return fmt.Errorf("repo %q is already registered", name)
	}

	reposDir, err := config.ReposDir()
	if err != nil {
		return err
	}
	cachePath := filepath.Join(reposDir, name)

	if err := os.MkdirAll(reposDir, 0700); err != nil {
		return fmt.Errorf("creating repos dir: %w", err)
	}

	// If a directory exists at cachePath but is NOT a valid git repo (e.g. a
	// partial clone from a previously-failed attempt), wipe it so PlainClone
	// can start fresh. If PlainOpen succeeds the directory is a valid clone
	// that the user may have intentionally kept (via `repo remove --keep`), so
	// we leave it alone and let PlainClone return its own "already exists" error.
	if _, statErr := os.Stat(cachePath); statErr == nil {
		if _, openErr := gogit.PlainOpen(cachePath); openErr != nil {
			if err := os.RemoveAll(cachePath); err != nil {
				return fmt.Errorf("removing partial cache dir %s: %w", cachePath, err)
			}
		}
	}

	cloneOpts := &gogit.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
	}
	if err := applyAuth(url, token, cloneOpts); err != nil {
		return err
	}

	if _, err := gogit.PlainClone(cachePath, false, cloneOpts); err != nil {
		return fmt.Errorf("cloning %s: %w", url, err)
	}

	st.Repos[name] = state.RepoRecord{
		URL:         url,
		CachePath:   cachePath,
		LastUpdated: time.Now(),
	}
	return nil
}

// Remove unregisters a repo from state. The local cache clone is kept on disk.
func Remove(name string, st *state.State) error {
	if _, ok := st.Repos[name]; !ok {
		return fmt.Errorf("repo %q not found", name)
	}
	delete(st.Repos, name)
	return nil
}

// NewCachePath returns the absolute path where a repo named name would be cached.
func NewCachePath(name string) (string, error) {
	reposDir, err := config.ReposDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(reposDir, name), nil
}

// RenameCache moves the on-disk cache directory from oldName to newName.
func RenameCache(oldName, newName string) error {
	reposDir, err := config.ReposDir()
	if err != nil {
		return err
	}
	oldPath := filepath.Join(reposDir, oldName)
	newPath := filepath.Join(reposDir, newName)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("cache directory %s already exists", newPath)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("renaming cache dir: %w", err)
	}
	return nil
}

// Update fetches the latest remote state and hard-resets the cache to origin/HEAD.
// This is safe because the cache is a read-only remote mirror — it is never
// directly edited by the user. A hard reset recovers automatically from any
// non-fast-forward divergence (e.g. after a force-push upstream).
// token is optional; pass "" to rely on env vars.
func Update(name, token string, st *state.State) error {
	rec, ok := st.Repos[name]
	if !ok {
		return fmt.Errorf("repo %q not found", name)
	}

	r, err := gogit.PlainOpen(rec.CachePath)
	if err != nil {
		return fmt.Errorf("opening repo cache: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	fetchOpts := &gogit.FetchOptions{
		RemoteName: "origin",
		Progress:   os.Stdout,
	}
	if err := applyFetchAuth(rec.URL, token, fetchOpts); err != nil {
		return err
	}

	err = r.Fetch(fetchOpts)
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetching repo: %w", err)
	}
	alreadyUpToDate := err == gogit.NoErrAlreadyUpToDate

	// Resolve origin/HEAD to the remote tip hash.
	hash, err := resolveRemoteHEAD(r)
	if err != nil {
		return fmt.Errorf("resolving remote HEAD: %w", err)
	}

	// Capture current HEAD before reset to detect whether the worktree actually moved.
	preResetHead, _ := r.Head()

	if err := w.Reset(&gogit.ResetOptions{Commit: *hash, Mode: gogit.HardReset}); err != nil {
		return fmt.Errorf("resetting to remote HEAD: %w", err)
	}

	// Print "Already up to date." only when the remote had nothing new AND the
	// local HEAD was already at the remote tip (i.e. nothing actually changed).
	if alreadyUpToDate && preResetHead != nil && preResetHead.Hash() == *hash {
		fmt.Println("  Already up to date.")
	}

	rec.LastUpdated = time.Now()
	st.Repos[name] = rec
	return nil
}

// resolveRemoteHEAD returns the hash the remote tip the local worktree tracks.
//
// Resolution order:
//  1. refs/remotes/origin/HEAD — set by modern git clones; resolves the symref.
//  2. The remote-tracking ref for the currently checked-out branch
//     (e.g. refs/remotes/origin/develop) — handles any default branch name.
//
// This avoids hard-coding branch names like "main" or "master".
func resolveRemoteHEAD(r *gogit.Repository) (*plumbing.Hash, error) {
	// 1. Try refs/remotes/origin/HEAD (resolves the upstream symref).
	if h, err := r.ResolveRevision(plumbing.Revision("refs/remotes/origin/HEAD")); err == nil {
		return h, nil
	}

	// 2. Derive from the checked-out branch's tracking ref.
	head, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("getting HEAD: %w", err)
	}
	if head.Name().IsBranch() {
		trackingRef := plumbing.NewRemoteReferenceName("origin", head.Name().Short())
		if h, err := r.ResolveRevision(plumbing.Revision(trackingRef)); err == nil {
			return h, nil
		}
	}

	return nil, fmt.Errorf(
		"could not resolve remote HEAD: refs/remotes/origin/HEAD not set and no tracking ref found for branch %q",
		head.Name().Short(),
	)
}

// DiscoverSkills walks the cached repo and returns all skills (dirs containing SKILL.md).
func DiscoverSkills(repoName string, st *state.State) ([]SkillInfo, error) {
	rec, ok := st.Repos[repoName]
	if !ok {
		return nil, fmt.Errorf("repo %q not found", repoName)
	}
	return walkSkills(repoName, rec.CachePath)
}

// DiscoverAllSkills walks all registered repos and returns every skill found.
func DiscoverAllSkills(st *state.State) ([]SkillInfo, error) {
	var all []SkillInfo
	for name, rec := range st.Repos {
		skills, err := walkSkills(name, rec.CachePath)
		if err != nil {
			return nil, err
		}
		all = append(all, skills...)
	}
	return all, nil
}

// FindSkill resolves a skill address (e.g. "awesome-skills/coding/debugger") to its location on disk.
func FindSkill(addr string, st *state.State) (*SkillInfo, error) {
	parts := strings.SplitN(addr, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid skill address %q: expected <repo>/<path>", addr)
	}
	repoName, relPath := parts[0], parts[1]

	rec, ok := st.Repos[repoName]
	if !ok {
		return nil, fmt.Errorf("repo %q not found; register it with: skillpack repo add <url>", repoName)
	}

	skillPath := filepath.Join(rec.CachePath, filepath.FromSlash(relPath))
	if _, err := os.Stat(filepath.Join(skillPath, "SKILL.md")); err != nil {
		return nil, fmt.Errorf("skill %q not found in repo %q (expected SKILL.md at %s)", relPath, repoName, skillPath)
	}

	return &SkillInfo{
		Address:  addr,
		RepoName: repoName,
		RelPath:  relPath,
		FullPath: skillPath,
	}, nil
}

// HeadSHA returns the current HEAD commit SHA for a registered repo.
func HeadSHA(repoName string, st *state.State) (string, error) {
	rec, ok := st.Repos[repoName]
	if !ok {
		return "", fmt.Errorf("repo %q not found", repoName)
	}
	r, err := gogit.PlainOpen(rec.CachePath)
	if err != nil {
		return "", fmt.Errorf("opening repo cache: %w", err)
	}
	ref, err := r.Head()
	if err != nil {
		return "", fmt.Errorf("getting HEAD: %w", err)
	}
	return ref.Hash().String(), nil
}

// NameFromURL infers a repo name from its URL as "<owner>-<repo>".
// Examples:
//
//	https://github.com/mattpocock/skills.git  → mattpocock-skills
//	git@github.com:bmaltais/skillpack.git     → bmaltais-skillpack
//	https://internal.host/myrepo.git          → myrepo  (no owner segment)
func NameFromURL(rawURL string) string {
	s := strings.TrimRight(rawURL, "/")
	s = strings.TrimSuffix(s, ".git")

	// Normalise SSH syntax: git@host:owner/repo → host/owner/repo
	if strings.HasPrefix(s, "git@") {
		s = strings.TrimPrefix(s, "git@")
		s = strings.Replace(s, ":", "/", 1)
	}

	// Strip scheme (https://, ssh://, etc.)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}

	// s is now "host/owner/repo" or "host/repo"
	parts := strings.Split(s, "/")
	path := parts[1:] // drop host
	switch len(path) {
	case 0:
		return s
	case 1:
		return path[0]
	default:
		// Use last two segments: owner + repo
		return path[len(path)-2] + "-" + path[len(path)-1]
	}
}

func walkSkills(repoName, cachePath string) ([]SkillInfo, error) {
	var skills []SkillInfo
	// Using WalkDir instead of Walk avoids unnecessary Stat calls for every file,
	// significantly improving performance when scanning large repositories.
	err := filepath.WalkDir(cachePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden directories (e.g. .git)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			skillDir := filepath.Dir(path)
			relPath, _ := filepath.Rel(cachePath, skillDir)
			relPath = filepath.ToSlash(relPath)
			skills = append(skills, SkillInfo{
				Address:  repoName + "/" + relPath,
				RepoName: repoName,
				RelPath:  relPath,
				FullPath: skillDir,
			})
		}
		return nil
	})
	return skills, err
}

func isSSHURL(url string) bool {
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}

// applyAuth sets auth on CloneOptions based on URL scheme.
// token should already be resolved by the caller (e.g. via Config.TokenForRepo).
func applyAuth(url, token string, opts *gogit.CloneOptions) error {
	if isSSHURL(url) {
		auth, err := ssh.NewSSHAgentAuth("git")
		if err != nil {
			return fmt.Errorf("SSH agent unavailable (ensure ssh-agent is running): %w", err)
		}
		opts.Auth = auth
	} else if token != "" {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	return nil
}

func applyFetchAuth(url, token string, opts *gogit.FetchOptions) error {
	if isSSHURL(url) {
		auth, err := ssh.NewSSHAgentAuth("git")
		if err != nil {
			return fmt.Errorf("SSH agent unavailable: %w", err)
		}
		opts.Auth = auth
	} else if token != "" {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	return nil
}
