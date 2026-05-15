package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// UpdateResult describes the state of an installed skill relative to upstream.
type UpdateResult struct {
	Addr        string
	AgentName   string
	HasUpstream bool // upstream has changes since installed_at_sha
	IsModified  bool // local install has been edited since install
	IsConflict  bool // both upstream changed and locally modified
}

// CheckUpdate checks whether an installed skill has upstream changes or local modifications.
func CheckUpdate(addr, agentName string, st *state.State) (*UpdateResult, error) {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return nil, fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return nil, fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	modified, err := IsModified(rec)
	if err != nil {
		return nil, err
	}
	hasUpstream, err := hasUpstreamChange(addr, rec, st)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{
		Addr:        addr,
		AgentName:   agentName,
		HasUpstream: hasUpstream,
		IsModified:  modified,
		IsConflict:  hasUpstream && modified,
	}, nil
}

// ApplyUpdate copies the current cache version of a skill to the installed dir.
// The caller must confirm there is no conflict before calling this.
func ApplyUpdate(addr, agentName string, st *state.State) error {
	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return err
	}
	rec := st.InstalledSkills[addr][agentName]

	if err := os.RemoveAll(rec.LocalPath); err != nil {
		return fmt.Errorf("removing old install: %w", err)
	}
	if err := copyDir(skillInfo.FullPath, rec.LocalPath); err != nil {
		return fmt.Errorf("copying updated skill: %w", err)
	}

	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return err
	}
	sha, err := repo.HeadSHA(skillInfo.RepoName, st)
	if err != nil {
		return err
	}
	st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
		InstalledAtSHA: sha,
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
	}
	return nil
}

// ForceRemote overwrites the installed skill with the cache (upstream) version, discarding local changes.
func ForceRemote(addr, agentName string, st *state.State) error {
	return ApplyUpdate(addr, agentName, st)
}

// ForceLocal copies the installed skill back to the repo cache, commits, pushes to main,
// and updates state so the installed copy is considered canonical.
func ForceLocal(addr, agentName, token string, st *state.State) error {
	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return err
	}
	rec := st.InstalledSkills[addr][agentName]
	repoRec := st.Repos[skillInfo.RepoName]

	// Overwrite cache with installed files
	if err := os.RemoveAll(skillInfo.FullPath); err != nil {
		return fmt.Errorf("clearing cache skill dir: %w", err)
	}
	if err := copyDir(rec.LocalPath, skillInfo.FullPath); err != nil {
		return fmt.Errorf("copying installed to cache: %w", err)
	}

	r, err := gogit.PlainOpen(repoRec.CachePath)
	if err != nil {
		return fmt.Errorf("opening repo cache: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return err
	}

	// Stage all changes under the skill path (additions, modifications, deletions)
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	staged := false
	for path, fs := range status {
		if !pathUnderSkill(path, skillInfo.RelPath) {
			continue
		}
		if fs.Worktree == gogit.Deleted {
			if _, err := w.Remove(path); err != nil {
				return fmt.Errorf("git rm %s: %w", path, err)
			}
		} else {
			if _, err := w.Add(path); err != nil {
				return fmt.Errorf("git add %s: %w", path, err)
			}
		}
		staged = true
	}
	if !staged {
		fmt.Println("  Nothing to commit — skill is identical to cache.")
		return updateStateAfterPush(addr, agentName, rec.LocalPath, r, st)
	}

	sig := defaultSignature()
	commitHash, err := w.Commit(
		fmt.Sprintf("skillpack: update %s", skillInfo.RelPath),
		&gogit.CommitOptions{Author: sig, Committer: sig},
	)
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	pushOpts := &gogit.PushOptions{Progress: os.Stdout}
	if isSSHRemote(repoRec.URL) {
		auth, err := gogitssh.NewSSHAgentAuth("git")
		if err != nil {
			return fmt.Errorf("SSH agent unavailable: %w", err)
		}
		pushOpts.Auth = auth
 } else if token != "" {
		pushOpts.Auth = &gogithttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	if err := r.Push(pushOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("git push: %w", err)
	}

	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return err
	}
	st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
		InstalledAtSHA: commitHash.String(),
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
	}
	return nil
}

// MergeSkill performs a file-level three-way merge between the installed skill (ours)
// and the upstream cache (theirs), using the version at installed_at_sha as the common base.
// Returns true if any file had a conflict (conflict markers written to installed files).
// On a clean merge, state is updated to reflect the new HEAD.
func MergeSkill(addr, agentName string, st *state.State) (hasConflicts bool, err error) {
	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return false, err
	}
	rec := st.InstalledSkills[addr][agentName]
	repoRec := st.Repos[skillInfo.RepoName]

	r, err := gogit.PlainOpen(repoRec.CachePath)
	if err != nil {
		return false, fmt.Errorf("opening repo cache: %w", err)
	}

	baseFiles, err := listFilesAtCommit(r, rec.InstalledAtSHA, skillInfo.RelPath)
	if err != nil {
		return false, fmt.Errorf("reading base at %s: %w", rec.InstalledAtSHA[:8], err)
	}
	oursFiles := listFilesOnDisk(rec.LocalPath)
	theirsFiles := listFilesOnDisk(skillInfo.FullPath)

	allFiles := union(keys(baseFiles), keys(oursFiles), keys(theirsFiles))

	for _, relFile := range allFiles {
		base := baseFiles[relFile]
		ours := oursFiles[relFile]
		theirs := theirsFiles[relFile]
		targetPath := filepath.Join(rec.LocalPath, filepath.FromSlash(relFile))

		switch {
		case base == ours:
			// No local change — take upstream
			if err := writeStringToFile(targetPath, theirs); err != nil {
				return false, err
			}
		case base == theirs:
			// No upstream change — keep ours (already on disk, no action needed)
		case ours == theirs:
			// Both sides converged — keep either (ours is already on disk)
		default:
			// True conflict: both sides differ from base
			hasConflicts = true
			if err := writeStringToFile(targetPath, conflictBlock(ours, theirs)); err != nil {
				return false, err
			}
		}
	}

	if !hasConflicts {
		hash, _ := ComputeHash(rec.LocalPath)
		sha, _ := repo.HeadSHA(skillInfo.RepoName, st)
		st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
			InstalledAtSHA: sha,
			InstalledHash:  hash,
			LocalPath:      rec.LocalPath,
		}
	}
	return hasConflicts, nil
}

// hasUpstreamChange returns true if the skill's content changed in the repo
// between installed_at_sha and the current cache HEAD.
func hasUpstreamChange(addr string, rec state.InstalledSkillRecord, st *state.State) (bool, error) {
	parts := strings.SplitN(addr, "/", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid skill address: %s", addr)
	}
	repoName, skillRelPath := parts[0], parts[1]

	repoRec, ok := st.Repos[repoName]
	if !ok {
		return false, fmt.Errorf("repo %q not found", repoName)
	}

	r, err := gogit.PlainOpen(repoRec.CachePath)
	if err != nil {
		return false, err
	}
	headRef, err := r.Head()
	if err != nil {
		return false, err
	}
	if headRef.Hash().String() == rec.InstalledAtSHA {
		return false, nil // repo has not moved at all
	}

	oldCommit, err := r.CommitObject(plumbing.NewHash(rec.InstalledAtSHA))
	if err != nil {
		return false, fmt.Errorf("resolving installed SHA %s: %w", rec.InstalledAtSHA[:8], err)
	}
	newCommit, err := r.CommitObject(headRef.Hash())
	if err != nil {
		return false, err
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
		if pathUnderSkill(change.From.Name, skillRelPath) || pathUnderSkill(change.To.Name, skillRelPath) {
			return true, nil
		}
	}
	return false, nil
}

// listFilesAtCommit returns a map of relPath→content for all files under skillRelPath
// in the repo at the given commit SHA.
func listFilesAtCommit(r *gogit.Repository, commitSHA, skillRelPath string) (map[string]string, error) {
	commit, err := r.CommitObject(plumbing.NewHash(commitSHA))
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	files := make(map[string]string)
	err = tree.Files().ForEach(func(f *object.File) error {
		if !pathUnderSkill(f.Name, skillRelPath) {
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

// listFilesOnDisk returns a map of relPath→content for all files in dir.
func listFilesOnDisk(dir string) map[string]string {
	files := make(map[string]string)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		data, _ := os.ReadFile(path)
		files[rel] = string(data)
		return nil
	})
	return files
}

// pathUnderSkill returns true if filePath is the skill root or a file within it.
func pathUnderSkill(filePath, skillRelPath string) bool {
	if filePath == "" || skillRelPath == "" {
		return false
	}
	return filePath == skillRelPath || strings.HasPrefix(filePath, skillRelPath+"/")
}

func keys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func union(sets ...[]string) []string {
	seen := make(map[string]bool)
	for _, set := range sets {
		for _, v := range set {
			seen[v] = true
		}
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func conflictBlock(ours, theirs string) string {
	return "<<<<<<< ours (local)\n" + ours + "\n=======\n" + theirs + "\n>>>>>>> theirs (upstream)\n"
}

func writeStringToFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}

func updateStateAfterPush(addr, agentName, localPath string, r *gogit.Repository, st *state.State) error {
	hash, err := ComputeHash(localPath)
	if err != nil {
		return err
	}
	ref, err := r.Head()
	if err != nil {
		return err
	}
	st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
		InstalledAtSHA: ref.Hash().String(),
		InstalledHash:  hash,
		LocalPath:      localPath,
	}
	return nil
}

func defaultSignature() *object.Signature {
	return &object.Signature{
		Name:  "skillpack",
		Email: "skillpack@local",
		When:  time.Now(),
	}
}

func isSSHRemote(url string) bool {
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}
