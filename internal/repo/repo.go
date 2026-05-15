package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/bernard/skillpack/internal/config"
	"github.com/bernard/skillpack/internal/state"
)

// SkillInfo describes a discovered skill inside a repo clone.
type SkillInfo struct {
	Address  string // e.g. "awesome-skills/coding/debugger"
	RepoName string
	RelPath  string // path relative to the repo root, slash-separated
	FullPath string // absolute path on disk
}

// Add clones the remote repo to the local cache and registers it in state.
func Add(name, url string, st *state.State) error {
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

	cloneOpts := &gogit.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
	}
	if err := applyAuth(url, cloneOpts); err != nil {
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

// Update performs a git pull on the cached repo clone.
func Update(name string, st *state.State) error {
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

	pullOpts := &gogit.PullOptions{Progress: os.Stdout}
	if err := applyPullAuth(rec.URL, pullOpts); err != nil {
		return err
	}

	err = w.Pull(pullOpts)
	if err == gogit.NoErrAlreadyUpToDate {
		fmt.Println("  Already up to date.")
		err = nil
	}
	if err != nil {
		return fmt.Errorf("pulling repo: %w", err)
	}

	rec.LastUpdated = time.Now()
	st.Repos[name] = rec
	return nil
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

// NameFromURL infers a repo name from its URL (last path component, .git stripped).
func NameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimRight(url, "/")
	if idx := strings.LastIndexAny(url, "/:\\"); idx >= 0 {
		url = url[idx+1:]
	}
	return url
}

func walkSkills(repoName, cachePath string) ([]SkillInfo, error) {
	var skills []SkillInfo
	err := filepath.Walk(cachePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden directories (e.g. .git)
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if !info.IsDir() && info.Name() == "SKILL.md" {
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
func applyAuth(url string, opts *gogit.CloneOptions) error {
	if isSSHURL(url) {
		auth, err := ssh.NewSSHAgentAuth("git")
		if err != nil {
			return fmt.Errorf("SSH agent unavailable (ensure ssh-agent is running): %w", err)
		}
		opts.Auth = auth
	}
	// HTTPS public repos: no auth needed.
	// HTTPS private repos: rely on system git credential store via go-git's default behaviour.
	return nil
}

func applyPullAuth(url string, opts *gogit.PullOptions) error {
	if isSSHURL(url) {
		auth, err := ssh.NewSSHAgentAuth("git")
		if err != nil {
			return fmt.Errorf("SSH agent unavailable: %w", err)
		}
		opts.Auth = auth
	}
	return nil
}
