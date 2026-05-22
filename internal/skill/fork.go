package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// IsFork reports whether a skill record represents a forked skill — one that
// was copied from an upstream origin and may need upstream-sync operations.
func IsFork(rec state.InstalledSkillRecord) bool {
	return rec.UpstreamAddr != ""
}

// loadForkProvenance reads the .skillpack-fork metadata file from skillDir and
// populates UpstreamAddr and UpstreamSHA on rec when the file is present.
// Callers should use this instead of readForkMetadata directly so that
// provenance loading stays co-located with its error-wrapping context.
func loadForkProvenance(skillDir string, rec *state.InstalledSkillRecord) error {
	meta, err := readForkMetadata(skillDir)
	if err != nil {
		return fmt.Errorf("reading fork provenance metadata: %w", err)
	}
	if meta != nil {
		rec.UpstreamAddr = meta.UpstreamAddr
		rec.UpstreamSHA = meta.UpstreamSHA
	}
	return nil
}

// ForkMode controls how Fork handles a destination that already contains a
// skill directory with unknown provenance.
type ForkMode int

const (
	// ForkModeAuto is the default: error on unknown provenance.
	ForkModeAuto ForkMode = iota
	// ForkModeOverride removes the existing destination and forks fresh.
	ForkModeOverride
	// ForkModeRegister keeps the existing directory and retroactively
	// registers it as a fork of the source.
	ForkModeRegister
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
// mode      — how to handle unknown provenance at the destination
func Fork(addr, forkRepo, agentName, token string, mode ForkMode, st *state.State) (newAddr string, err error) {
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
	for sourceAgentName, sourceRec := range agents {
		if IsFork(sourceRec) {
			return "", fmt.Errorf(
				"skill %q is already a fork of %q for agent %q — multi-hop forks are not supported",
				addr, sourceRec.UpstreamAddr, sourceAgentName,
			)
		}
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
	newAddr = forkRepo + "/" + skillName
	forkDestPath := filepath.Join(forkRec.CachePath, skillName)

	destExists := false
	if _, statErr := os.Stat(forkDestPath); statErr == nil {
		destExists = true
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("checking fork destination path: %w", statErr)
	}
	if mode == ForkModeRegister && !destExists {
		return "", fmt.Errorf(
			"register mode requires an existing skill directory at %q: nothing to register",
			forkDestPath,
		)
	}

	if destExists {
		forkAgents, ok := st.InstalledSkills[newAddr]
		if !ok {
			switch mode {
			case ForkModeOverride:
				// User chose to overwrite — fall through to remove + copy
			case ForkModeRegister:
				// User chose to register existing dir — skip copy below
			default:
				return "", fmt.Errorf(
					"skill %q already exists in repo %q but has unknown fork provenance in state; install and register %q first or remove it from the destination repo",
					skillName, forkRepo, newAddr,
				)
			}
		} else {
			for _, forkStateRec := range forkAgents {
				if forkStateRec.UpstreamAddr != addr {
					conflictingUpstream := forkStateRec.UpstreamAddr
					if conflictingUpstream == "" {
						conflictingUpstream = "(not tracked as a fork)"
					}
					return "", fmt.Errorf(
						"skill %q already exists in repo %q and is tracked as a fork of %q, not %q",
						skillName, forkRepo, conflictingUpstream, addr,
					)
				}
			}
		}
	}

	switch mode {
	case ForkModeRegister:
		// Keep existing directory — only write fork metadata, skip copy
	default:
		// ForkModeAuto and ForkModeOverride: remove existing (if any) and copy fresh
		if destExists {
			if err := os.RemoveAll(forkDestPath); err != nil {
				return "", fmt.Errorf("clearing existing skill in fork repo: %w", err)
			}
		}
		if err := copyDir(rec.LocalPath, forkDestPath); err != nil {
			return "", fmt.Errorf("copying skill to fork repo: %w", err)
		}
	}
	if err := writeForkMetadata(forkDestPath, addr, upstreamSHA); err != nil {
		return "", fmt.Errorf("writing fork provenance metadata in fork repo: %w", err)
	}
	for sourceAgentName, sourceRec := range agents {
		if err := writeForkMetadata(sourceRec.LocalPath, addr, upstreamSHA); err != nil {
			return "", fmt.Errorf(
				"writing fork provenance metadata in installed skill for agent %q: %w",
				sourceAgentName, err,
			)
		}
	}

	// Commit and push the fork
	result, err := gitops.CommitAndPush(
		forkRec.CachePath,
		skillName,
		fmt.Sprintf("skillpack: fork %s from %s", skillName, addr),
		forkRec.URL,
		token,
	)
	if err != nil {
		return "", err
	}

	var commitHash string
	if result.Committed {
		commitHash = result.CommitHash
	} else {
		commitHash, err = gitops.HeadSHA(forkRec.CachePath)
		if err != nil {
			return "", fmt.Errorf("reading fork repo HEAD: %w", err)
		}
	}

	// Compute all hashes before touching state to avoid partial migration on error.
	newRecords := make(map[string]state.InstalledSkillRecord, len(agents))
	for sourceAgentName, sourceRec := range agents {
		hash, hashErr := ComputeHash(sourceRec.LocalPath)
		if hashErr != nil {
			return "", fmt.Errorf("computing installed hash for agent %q: %w", sourceAgentName, hashErr)
		}
		newRecords[sourceAgentName] = state.InstalledSkillRecord{
			InstalledAtSHA: commitHash,
			InstalledHash:  hash,
			LocalPath:      sourceRec.LocalPath,
			UpstreamAddr:   addr,
			UpstreamSHA:    upstreamSHA,
		}
	}

	// All hashes succeeded — apply state rewrite.
	for sourceAgentName, newRec := range newRecords {
		if err := st.RecordInstall(newAddr, sourceAgentName, newRec); err != nil {
			return "", fmt.Errorf("recording fork install for agent %q: %w", sourceAgentName, err)
		}
	}

	// Remove original state entry only when the fork address differs.
	// If newAddr == addr, deleting addr would also delete the records we just wrote.
	if newAddr != addr {
		if err := st.RecordRemoveAll(addr); err != nil {
			return "", fmt.Errorf("removing old fork address %q from state: %w", addr, err)
		}
	}

	if err := state.Save(st); err != nil {
		return "", err
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
	if !IsFork(rec) {
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

	result, err := gitops.CommitAndPush(
		forkRec.CachePath,
		skillInfo.RelPath,
		fmt.Sprintf("skillpack: merge upstream changes into %s", skillInfo.RelPath),
		forkRec.URL,
		token,
	)
	if err != nil {
		return err
	}

	hash, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return err
	}

	var commitSHA string
	if !result.Committed {
		// No changes to commit — use current HEAD
		commitSHA, err = gitops.HeadSHA(forkRec.CachePath)
		if err != nil {
			return fmt.Errorf("reading fork repo HEAD: %w", err)
		}
	} else {
		commitSHA = result.CommitHash
	}

	if err := st.RecordInstall(addr, agentName, state.InstalledSkillRecord{
		InstalledAtSHA: commitSHA,
		InstalledHash:  hash,
		LocalPath:      rec.LocalPath,
		UpstreamAddr:   rec.UpstreamAddr,
		UpstreamSHA:    upstreamHeadSHA,
	}); err != nil {
		return err
	}
	return state.Save(st)
}
