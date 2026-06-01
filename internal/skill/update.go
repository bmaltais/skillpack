package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// safeShortSHA returns the first 8 characters of a SHA, or the full string if
// it is shorter than 8 — prevents a slice-bounds panic on empty/corrupted state.
func safeShortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// snapshotInstalled refreshes InstalledHash and InstalledAtSHA in state to
// reflect the current on-disk contents of the installed skill directory.
// Call this after a successful merge or LLM conflict resolution on a
// non-forked skill so that subsequent update/sync commands use the resolved
// files as the new baseline.
func snapshotInstalled(addr, agentName string, st *state.State) error {
	rec, ok := st.InstalledSkills[addr][agentName]
	if !ok {
		return fmt.Errorf("skill %q not installed for agent %q", addr, agentName)
	}
	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return fmt.Errorf("computing installed hash: %w", err)
	}
	repoName := strings.SplitN(addr, "/", 2)[0]
	headSHA, err := repo.HeadSHA(repoName, st)
	if err != nil {
		return fmt.Errorf("reading repo HEAD SHA: %w", err)
	}
	rec.InstalledHash = hash
	rec.InstalledAtSHA = headSHA
	if err := st.RecordInstall(addr, agentName, rec); err != nil {
		return err
	}
	return state.Save(st)
}

// UpdateResult describes the state of an installed skill relative to upstream.
type UpdateResult struct {
	Addr        string
	AgentName   string
	HasUpstream bool // upstream has changes since installed_at_sha
	IsModified  bool // local install has been edited since install
	IsConflict  bool // both upstream changed and locally modified
}

// checkUpdate checks whether an installed skill has upstream changes or local modifications.
func checkUpdate(addr, agentName string, st *state.State) (*UpdateResult, error) {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return nil, fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return nil, fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	modified, err := isModified(rec)
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
// For forked skills it copies from the upstream origin, then commits and pushes to the fork repo.
// The caller must confirm there is no conflict before calling this.
func applyUpdate(addr, agentName, token string, st *state.State) error {
	rec := st.InstalledSkills[addr][agentName]

	if isFork(rec) {
		// Forked skill: take files from the upstream origin, then push to fork.
		upstreamInfo, err := repo.FindSkill(rec.UpstreamAddr, st)
		if err != nil {
			return fmt.Errorf("finding upstream skill %q: %w", rec.UpstreamAddr, err)
		}
		upstreamRepoName := strings.SplitN(rec.UpstreamAddr, "/", 2)[0]
		upstreamHeadSHA, err := repo.HeadSHA(upstreamRepoName, st)
		if err != nil {
			return err
		}

		if err := os.RemoveAll(rec.LocalPath); err != nil {
			return fmt.Errorf("removing old install: %w", err)
		}
		if err := copyDir(upstreamInfo.FullPath, rec.LocalPath); err != nil {
			return fmt.Errorf("copying upstream skill: %w", err)
		}
		if err := pushForkAfterMerge(addr, agentName, token, upstreamHeadSHA, st); err != nil {
			return err
		}
		return nil
	}

	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return err
	}

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
	if err := st.RecordInstall(addr, agentName, state.InstalledSkillRecord{
		InstalledAtSHA: sha,
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
	}); err != nil {
		return err
	}
	return state.Save(st)
}

// ForceRemote overwrites the installed skill with the cache (upstream) version, discarding local changes.
func forceRemote(addr, agentName, token string, st *state.State) error {
	return applyUpdate(addr, agentName, token, st)
}

// applyUpdateFromOwnRepo updates a forked skill using its own repo cache
// (not the upstream origin). Used when upstream tracking is disabled because
// the upstream repo is not registered.
func applyUpdateFromOwnRepo(addr, agentName string, st *state.State) error {
	rec := st.InstalledSkills[addr][agentName]

	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return err
	}

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
	// Preserve fork metadata fields but update hash and SHA against own repo.
	rec.InstalledHash = hash
	rec.InstalledAtSHA = sha
	if err := st.RecordInstall(addr, agentName, rec); err != nil {
		return err
	}
	return state.Save(st)
}

// ForceLocal copies the installed skill back to the repo cache, commits, pushes to main,
// and updates state so the installed copy is considered canonical.
func forceLocal(addr, agentName, token string, st *state.State) error {
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

	result, err := gitops.CommitAndPush(
		repoRec.CachePath,
		skillInfo.RelPath,
		fmt.Sprintf("skillpack: update %s", skillInfo.RelPath),
		repoRec.URL,
		token,
	)
	if err != nil {
		return err
	}

	var commitSHA string
	if !result.Committed {
		fmt.Println("  Nothing to commit — skill is identical to cache.")
		commitSHA, err = gitops.HeadSHA(repoRec.CachePath)
		if err != nil {
			return err
		}
	} else {
		commitSHA = result.CommitHash
	}

	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return err
	}
	newRec := state.InstalledSkillRecord{
		InstalledAtSHA: commitSHA,
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
		UpstreamAddr:   rec.UpstreamAddr,
		UpstreamSHA:    rec.UpstreamSHA,
	}
	// For forked skills, acknowledge that upstream changes have been dealt with
	// by updating UpstreamSHA to the current upstream HEAD.
	if isFork(rec) {
		upstreamRepoName := strings.SplitN(rec.UpstreamAddr, "/", 2)[0]
		if upstreamSHA, shaErr := repo.HeadSHA(upstreamRepoName, st); shaErr == nil {
			newRec.UpstreamSHA = upstreamSHA
		}
	}
	if err := st.RecordInstall(addr, agentName, newRec); err != nil {
		return err
	}
	return state.Save(st)
}

// MergeSkill performs a file-level three-way merge between the installed skill (ours)
// and the upstream (theirs), using an appropriate base commit.
//
// For normal skills: base = installed_at_sha in the skill repo; theirs = repo HEAD.
// For forked skills: base = upstream_sha in the upstream origin; theirs = upstream HEAD.
//
// Returns true if any file had a conflict (conflict markers written to installed files).
// On a clean merge, state is updated. For forked skills, the merged result is also
// committed and pushed to the fork repo.
func mergeSkill(addr, agentName, token string, st *state.State) (hasConflicts bool, err error) {
	rec := st.InstalledSkills[addr][agentName]

	if isFork(rec) {
		return mergeForkSkill(addr, agentName, token, rec, st)
	}

	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return false, err
	}
	repoRec := st.Repos[skillInfo.RepoName]

	baseFiles, err := gitops.ListFilesAtCommit(repoRec.CachePath, rec.InstalledAtSHA, skillInfo.RelPath)
	if err != nil {
		return false, fmt.Errorf("reading base at %s: %w", safeShortSHA(rec.InstalledAtSHA), err)
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
		if err := snapshotInstalled(addr, agentName, st); err != nil {
			return false, err
		}
	}
	return hasConflicts, nil
}

// mergeForkSkill performs a three-way merge for a forked skill.
// base = upstream origin at UpstreamSHA; ours = installed; theirs = upstream HEAD.
// On a clean merge it commits and pushes to the fork repo.
func mergeForkSkill(addr, agentName, token string, rec state.InstalledSkillRecord, st *state.State) (bool, error) {
	upstreamParts := strings.SplitN(rec.UpstreamAddr, "/", 2)
	if len(upstreamParts) != 2 {
		return false, fmt.Errorf("invalid upstream addr: %s", rec.UpstreamAddr)
	}
	upstreamRepoName, upstreamRelPath := upstreamParts[0], upstreamParts[1]

	upstreamRepoRec, ok := st.Repos[upstreamRepoName]
	if !ok {
		return false, fmt.Errorf("upstream repo %q not found", upstreamRepoName)
	}

	baseFiles, err := gitops.ListFilesAtCommit(upstreamRepoRec.CachePath, rec.UpstreamSHA, upstreamRelPath)
	if err != nil {
		return false, fmt.Errorf("reading upstream base at %s: %w", safeShortSHA(rec.UpstreamSHA), err)
	}

	// Upstream HEAD files come from the cache dir (already fetched by update/sync)
	upstreamInfo, err := repo.FindSkill(rec.UpstreamAddr, st)
	if err != nil {
		return false, fmt.Errorf("finding upstream skill: %w", err)
	}

	upstreamHeadSHA, err := repo.HeadSHA(upstreamRepoName, st)
	if err != nil {
		return false, err
	}

	oursFiles := listFilesOnDisk(rec.LocalPath)
	theirsFiles := listFilesOnDisk(upstreamInfo.FullPath)
	allFiles := union(keys(baseFiles), keys(oursFiles), keys(theirsFiles))

	hasConflicts := false
	for _, relFile := range allFiles {
		base := baseFiles[relFile]
		ours := oursFiles[relFile]
		theirs := theirsFiles[relFile]
		targetPath := filepath.Join(rec.LocalPath, filepath.FromSlash(relFile))

		switch {
		case base == ours:
			if err := writeStringToFile(targetPath, theirs); err != nil {
				return false, err
			}
		case base == theirs:
			// keep ours
		case ours == theirs:
			// both converged — keep ours
		default:
			hasConflicts = true
			if err := writeStringToFile(targetPath, conflictBlock(ours, theirs)); err != nil {
				return false, err
			}
		}
	}

	if !hasConflicts {
		// Clean merge: commit+push to fork repo and update state.
		if err := pushForkAfterMerge(addr, agentName, token, upstreamHeadSHA, st); err != nil {
			return false, err
		}
	}
	return hasConflicts, nil
}

// hasUpstreamChange returns true if the skill's content changed in the relevant repo
// since the baseline SHA recorded in state.
//
// For normal skills: checks between installed_at_sha and repo HEAD.
// For forked skills: checks upstream origin between upstream_sha and upstream HEAD.
func hasUpstreamChange(addr string, rec state.InstalledSkillRecord, st *state.State) (bool, error) {
	if isFork(rec) {
		return hasUpstreamOriginChange(rec, st)
	}

	parts := strings.SplitN(addr, "/", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid skill address: %s", addr)
	}
	repoName, skillRelPath := parts[0], parts[1]

	repoRec, ok := st.Repos[repoName]
	if !ok {
		return false, fmt.Errorf("repo %q not found", repoName)
	}

	_, changed, err := gitops.DiffSkillChangedFromHEAD(repoRec.CachePath, rec.InstalledAtSHA, skillRelPath)
	return changed, err
}

// hasUpstreamOriginChange checks whether the upstream origin repo has new commits
// to the skill dir beyond the upstream_sha recorded at fork time.
func hasUpstreamOriginChange(rec state.InstalledSkillRecord, st *state.State) (bool, error) {
	parts := strings.SplitN(rec.UpstreamAddr, "/", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid upstream addr: %s", rec.UpstreamAddr)
	}
	upstreamRepoName, skillRelPath := parts[0], parts[1]

	repoRec, ok := st.Repos[upstreamRepoName]
	if !ok {
		return false, fmt.Errorf("upstream repo %q not found — re-add it with: skillpack repo add %s <url>", upstreamRepoName, upstreamRepoName)
	}

	_, changed, err := gitops.DiffSkillChangedFromHEAD(repoRec.CachePath, rec.UpstreamSHA, skillRelPath)
	return changed, err
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


