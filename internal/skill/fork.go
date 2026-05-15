package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gogitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// Fork copies an installed skill into the user's own repo, commits, pushes,
// and re-registers state so the skill is now tracked under the fork address.
// The original skill address and upstream HEAD SHA are recorded in state so
// future update/sync commands can detect and merge upstream origin changes.
//
// addr      — current installed skill address (e.g. "matt-pocock-skills/debugger")
// forkRepo  — name of the target repo (must already be registered via `skillpack repo add`)
// agentName — which agent's installed copy to fork
// token     — optional OAuth/PAT token for pushing to forkRepo over HTTPS
func Fork(addr, forkRepo, agentName, token string, st *state.State) (newAddr string, err error) {
	// Validate skill is installed
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return "", fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return "", fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	// Prevent forking a fork (multi-hop)
	if rec.UpstreamAddr != "" {
		return "", fmt.Errorf("skill %q is already a fork of %q — multi-hop forks are not supported", addr, rec.UpstreamAddr)
	}

	// Validate fork repo is registered
	forkRec, ok := st.Repos[forkRepo]
	if !ok {
		return "", fmt.Errorf("repo %q not registered — add it first with: skillpack repo add %s <url>", forkRepo, forkRepo)
	}

	// Get the upstream SHA (current HEAD of the original repo) before we do anything
	origRepoName := strings.SplitN(addr, "/", 2)[0]
	upstreamSHA, err := repo.HeadSHA(origRepoName, st)
	if err != nil {
		return "", fmt.Errorf("reading upstream SHA for %q: %w", origRepoName, err)
	}

	// Skill name = basename component (last path segment of addr)
	skillName := filepath.Base(addr)
	forkDestPath := filepath.Join(forkRec.CachePath, skillName)

	// Refuse if that skill already exists in the fork repo cache
	if _, statErr := os.Stat(forkDestPath); statErr == nil {
		return "", fmt.Errorf(
			"skill %q already exists in repo %q — use `skillpack publish %s/%s` to update it",
			skillName, forkRepo, forkRepo, skillName,
		)
	}

	// Copy installed files → fork repo cache
	if err := copyDir(rec.LocalPath, forkDestPath); err != nil {
		return "", fmt.Errorf("copying skill to fork repo: %w", err)
	}

	// Open fork repo git object
	r, err := gogit.PlainOpen(forkRec.CachePath)
	if err != nil {
		return "", fmt.Errorf("opening fork repo cache: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return "", err
	}

	// Stage all new files
	wStatus, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	staged := false
	for path := range wStatus {
		if strings.HasPrefix(path, skillName+"/") || path == skillName {
			if _, addErr := w.Add(path); addErr != nil {
				return "", fmt.Errorf("git add %s: %w", path, addErr)
			}
			staged = true
		}
	}
	if !staged {
		return "", fmt.Errorf("no files staged — is %q empty?", rec.LocalPath)
	}

	sig := defaultSignature()
	commitHash, err := w.Commit(
		fmt.Sprintf("skillpack: fork %s from %s", skillName, addr),
		&gogit.CommitOptions{Author: sig, Committer: sig},
	)
	if err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	pushOpts := &gogit.PushOptions{Progress: os.Stdout}
	if isSSHRemote(forkRec.URL) {
		auth, err := gogitssh.NewSSHAgentAuth("git")
		if err != nil {
			return "", fmt.Errorf("SSH agent unavailable: %w", err)
		}
		pushOpts.Auth = auth
	} else if token != "" {
		pushOpts.Auth = &gogithttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	if err := r.Push(pushOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("git push: %w", err)
	}

	// Compute new hash from installed dir (unchanged on disk)
	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return "", fmt.Errorf("computing installed hash: %w", err)
	}

	newAddr = forkRepo + "/" + skillName

	// Register the fork in state
	if st.InstalledSkills[newAddr] == nil {
		st.InstalledSkills[newAddr] = make(map[string]state.InstalledSkillRecord)
	}
	st.InstalledSkills[newAddr][agentName] = state.InstalledSkillRecord{
		InstalledAtSHA: commitHash.String(),
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
		UpstreamAddr:   addr,
		UpstreamSHA:    upstreamSHA,
	}

	// Remove the original state entry for this agent
	delete(st.InstalledSkills[addr], agentName)
	if len(st.InstalledSkills[addr]) == 0 {
		delete(st.InstalledSkills, addr)
	}

	return newAddr, nil
}

// PushForkAfterLLM is the exported counterpart of pushForkAfterMerge for use
// after LLM conflict resolution. It looks up the upstream origin HEAD SHA and
// commits the resolved installed files to the fork repo.
func PushForkAfterLLM(addr, agentName, token string, st *state.State) error {
	rec, ok := st.InstalledSkills[addr][agentName]
	if !ok {
		return fmt.Errorf("skill %q not installed for agent %q", addr, agentName)
	}
	if rec.UpstreamAddr == "" {
		return nil // non-forked skill — nothing to push
	}
	upstreamRepoName := strings.SplitN(rec.UpstreamAddr, "/", 2)[0]
	upstreamHeadSHA, err := repo.HeadSHA(upstreamRepoName, st)
	if err != nil {
		return fmt.Errorf("reading upstream HEAD SHA: %w", err)
	}
	return pushForkAfterMerge(addr, agentName, token, upstreamHeadSHA, st)
}
// commits, pushes, and updates state with the new fork SHA and upstream SHA.
// Called after a clean merge or successful LLM resolution on a forked skill.
func pushForkAfterMerge(addr, agentName, token string, upstreamHeadSHA string, st *state.State) error {
	rec := st.InstalledSkills[addr][agentName]

	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return err
	}
	forkRec := st.Repos[skillInfo.RepoName]

	// Overwrite fork cache with merged installed files
	if err := os.RemoveAll(skillInfo.FullPath); err != nil {
		return fmt.Errorf("clearing fork cache dir: %w", err)
	}
	if err := copyDir(rec.LocalPath, skillInfo.FullPath); err != nil {
		return fmt.Errorf("copying installed to fork cache: %w", err)
	}

	r, err := gogit.PlainOpen(forkRec.CachePath)
	if err != nil {
		return fmt.Errorf("opening fork repo cache: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return err
	}

	wStatus, err := w.Status()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	staged := false
	for path, fs := range wStatus {
		if !pathUnderSkill(path, skillInfo.RelPath) {
			continue
		}
		if fs.Worktree == gogit.Deleted {
			if _, rmErr := w.Remove(path); rmErr != nil {
				return fmt.Errorf("git rm %s: %w", path, rmErr)
			}
		} else {
			if _, addErr := w.Add(path); addErr != nil {
				return fmt.Errorf("git add %s: %w", path, addErr)
			}
		}
		staged = true
	}
	if !staged {
		// No changes to commit — update state without pushing
		hash, _ := ComputeHash(rec.LocalPath)
		ref, _ := r.Head()
		st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
			InstalledAtSHA: ref.Hash().String(),
			InstalledHash:  hash,
			LocalPath:      rec.LocalPath,
			UpstreamAddr:   rec.UpstreamAddr,
			UpstreamSHA:    upstreamHeadSHA,
		}
		return nil
	}

	sig := defaultSignature()
	commitHash, err := w.Commit(
		fmt.Sprintf("skillpack: merge upstream changes into %s", skillInfo.RelPath),
		&gogit.CommitOptions{Author: sig, Committer: sig},
	)
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	pushOpts := &gogit.PushOptions{Progress: os.Stdout}
	if isSSHRemote(forkRec.URL) {
		auth, sshErr := gogitssh.NewSSHAgentAuth("git")
		if sshErr != nil {
			return fmt.Errorf("SSH agent unavailable: %w", sshErr)
		}
		pushOpts.Auth = auth
	} else if token != "" {
		pushOpts.Auth = &gogithttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	if err := r.Push(pushOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return fmt.Errorf("git push fork: %w", err)
	}

	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return err
	}
	st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
		InstalledAtSHA: commitHash.String(),
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
		UpstreamAddr:   rec.UpstreamAddr,
		UpstreamSHA:    upstreamHeadSHA,
	}
	return nil
}
