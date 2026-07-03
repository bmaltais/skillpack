package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/state"
)

// SkillInfo describes a discovered skill inside a repo clone.
type SkillInfo struct {
	Address  string // e.g. "awesome-skills/coding/debugger"
	RepoName string
	RelPath  string // path relative to the repo root, slash-separated
	FullPath string // absolute path on disk
}

// PackInfo describes a discovered pack inside a repo clone.
type PackInfo struct {
	Address  string // e.g. "awesome-skills/packs/go-dev"
	RepoName string
	RelPath  string // path relative to the repo root, slash-separated
	FullPath string // absolute path on disk
}

// Add clones the remote repo to the local cache and registers it in state.
// recovered is true when an existing cache directory was reused instead of
// cloned fresh (e.g. after a previous repo remove left the clone on disk).
// token is optional; pass "" to rely on env vars (SKILLPACK_GIT_TOKEN, GITHUB_TOKEN).
func Add(name, url, token string, st *state.State) (recovered bool, err error) {
	if _, exists := st.Repos[name]; exists {
		return false, fmt.Errorf("repo %q is already registered", name)
	}

	reposDir, err := config.ReposDir()
	if err != nil {
		return false, err
	}
	cachePath := filepath.Join(reposDir, name)

	if err := os.MkdirAll(reposDir, 0700); err != nil {
		return false, fmt.Errorf("creating repos dir: %w", err)
	}

	// If a directory exists at cachePath, check whether it is a valid git repo.
	if _, statErr := os.Stat(cachePath); statErr == nil {
		if _, openErr := gogit.PlainOpen(cachePath); openErr != nil {
			// Partial/corrupt clone — wipe it so PlainClone can start fresh.
			if err := os.RemoveAll(cachePath); err != nil {
				return false, fmt.Errorf("removing partial cache dir %s: %w", cachePath, err)
			}
		} else {
			// Valid git repo left behind by a previous repo remove — reuse it
			// rather than failing. The caller reports this outcome to the user.
			recovered = true
		}
	}

	if !recovered {
		cloneOpts := &gogit.CloneOptions{
			URL:      url,
			Progress: os.Stdout,
		}
		auth, err := gitops.Auth(url, token)
		if err != nil {
			return false, err
		}
		cloneOpts.Auth = auth

		if _, err := gogit.PlainClone(cachePath, false, cloneOpts); err != nil {
			return false, fmt.Errorf("cloning %s: %w", url, err)
		}
	}

	st.Repos[name] = state.RepoRecord{
		URL:         url,
		CachePath:   cachePath,
		LastUpdated: time.Now(),
	}
	return recovered, state.Save(st)
}

// Remove unregisters a repo from state. The local cache clone is kept on disk.
func Remove(name string, st *state.State) error {
	if _, ok := st.Repos[name]; !ok {
		return fmt.Errorf("repo %q not found", name)
	}
	delete(st.Repos, name)
	return state.Save(st)
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
//
// Returns (warning, error). warning is non-empty when a configured credential
// failed but the repo was successfully fetched anonymously (stale credential on
// a public repo). In that case error is nil.
func Update(name, token string, st *state.State) (string, error) {
	rec, ok := st.Repos[name]
	if !ok {
		return "", fmt.Errorf("repo %q not found", name)
	}

	r, err := gogit.PlainOpen(rec.CachePath)
	if err != nil {
		return "", fmt.Errorf("opening repo cache: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return "", fmt.Errorf("getting worktree: %w", err)
	}

	fetchOpts := &gogit.FetchOptions{
		RemoteName: "origin",
		Progress:   os.Stdout,
	}
	auth, err := gitops.Auth(rec.URL, token)
	if err != nil {
		return "", err
	}
	fetchOpts.Auth = auth

	var warning string
	fetchErr := r.Fetch(fetchOpts)
	switch {
	case fetchErr == nil || fetchErr == gogit.NoErrAlreadyUpToDate:
		// success
	case isAuthError(fetchErr) && !gitops.IsSSHURL(rec.URL) && token != "":
		// Credential failed for an HTTPS repo. Retry anonymously — if the repo
		// is public, anonymous fetch succeeds and we surface a stale-credential
		// notice rather than a hard error.
		anonOpts := *fetchOpts
		anonOpts.Auth = nil
		anonErr := r.Fetch(&anonOpts)
		if anonErr == nil || anonErr == gogit.NoErrAlreadyUpToDate {
			warning = fmt.Sprintf("stale credential for %q ignored; repo is public and was fetched anonymously", name)
		} else {
			// Both authenticated and anonymous fetches failed: genuine private-repo auth failure.
			return "", fmt.Errorf("private repo auth failed for %q (check your token): %w", name, fetchErr)
		}
	default:
		return "", fmt.Errorf("fetching repo: %w", fetchErr)
	}

	// Resolve origin/HEAD to the remote tip hash.
	hash, err := resolveRemoteHEAD(r)
	if err != nil {
		return "", fmt.Errorf("resolving remote HEAD: %w", err)
	}

	if err := w.Reset(&gogit.ResetOptions{Commit: *hash, Mode: gogit.HardReset}); err != nil {
		return "", fmt.Errorf("resetting to remote HEAD: %w", err)
	}

	rec.LastUpdated = time.Now()
	st.Repos[name] = rec
	return warning, state.Save(st)
}

// isAuthError reports whether err indicates an authentication or authorisation
// failure from the remote git transport.
func isAuthError(err error) bool {
	return errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed)
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
		if !d.IsDir() {
			return nil
		}
		// Skip .git only — other hidden directories (e.g. .agents/) may contain skills.
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Detect SKILL.md while visiting the directory itself so pruning is
		// order-independent and does not rely on sibling traversal order.
		if _, statErr := os.Stat(filepath.Join(path, "SKILL.md")); statErr == nil {
			skillDir := path
			relPath, _ := filepath.Rel(cachePath, skillDir)
			relPath = filepath.ToSlash(relPath)
			skills = append(skills, SkillInfo{
				Address:  repoName + "/" + relPath,
				RepoName: repoName,
				RelPath:  relPath,
				FullPath: skillDir,
			})
			return filepath.SkipDir
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		return nil
	})
	return skills, err
}

// DiscoverPacks walks the cached repo and returns all packs (dirs containing pack.yaml).
func DiscoverPacks(repoName string, st *state.State) ([]PackInfo, error) {
	rec, ok := st.Repos[repoName]
	if !ok {
		return nil, fmt.Errorf("repo %q not found", repoName)
	}
	return walkPacks(repoName, rec.CachePath)
}

// DiscoverAllPacks walks all registered repos and returns every pack found.
func DiscoverAllPacks(st *state.State) ([]PackInfo, error) {
	var all []PackInfo
	for name, rec := range st.Repos {
		packs, err := walkPacks(name, rec.CachePath)
		if err != nil {
			return nil, err
		}
		all = append(all, packs...)
	}
	return all, nil
}

func walkPacks(repoName, cachePath string) ([]PackInfo, error) {
	var packs []PackInfo
	err := filepath.WalkDir(cachePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if _, statErr := os.Stat(filepath.Join(path, "pack.yaml")); statErr == nil {
			relPath, _ := filepath.Rel(cachePath, path)
			relPath = filepath.ToSlash(relPath)
			packs = append(packs, PackInfo{
				Address:  repoName + "/" + relPath,
				RepoName: repoName,
				RelPath:  relPath,
				FullPath: path,
			})
			return filepath.SkipDir
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		return nil
	})
	return packs, err
}

