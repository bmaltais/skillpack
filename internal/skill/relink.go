package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// SuggestReplacements returns likely replacement addresses for a stale installed
// skill whose upstream path no longer exists. It scans every registered repo for
// skills whose final path component (basename) matches staleAddr's basename and
// returns their addresses, excluding staleAddr itself.
//
// The result is sorted for deterministic output and is empty (not nil) when no
// candidate exists. The function is read-only: no writes, no network calls.
func SuggestReplacements(staleAddr string, st *state.State) []string {
	basename := filepath.Base(staleAddr)
	skills, err := repo.DiscoverAllSkills(st)
	if err != nil {
		return []string{}
	}
	matches := []string{}
	for _, s := range skills {
		if s.Address == staleAddr {
			continue
		}
		if filepath.Base(s.Address) == basename {
			matches = append(matches, s.Address)
		}
	}
	sort.Strings(matches)
	return matches
}

// Relink re-points an installed skill mapping from oldAddr to newAddr for a
// single agent. It is the first-class repair path for a stale installed skill
// whose upstream address no longer exists: instead of removing and reinstalling,
// the user supplies a valid replacement address and the installed copy is
// refreshed to track it.
//
// newAddr must reference a skill that exists in a registered, locally-cached
// repo. The installed directory is replaced with the replacement skill's
// contents, the install-time snapshot (SHA, content hash, fork provenance) is
// recomputed, and the state record is moved from oldAddr to newAddr so a
// subsequent sync no longer reports a stale mapping.
//
// When force is false and the installed copy has local modifications, Relink
// refuses (mirroring remove) so unsaved edits are not silently discarded.
func Relink(oldAddr, newAddr, agentName string, force bool, st *state.State) error {
	if oldAddr == newAddr {
		return fmt.Errorf("replacement address is identical to the current address %q", oldAddr)
	}

	agents, ok := st.InstalledSkills[oldAddr]
	if !ok {
		return fmt.Errorf("skill %q is not installed", oldAddr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return fmt.Errorf("skill %q is not installed for agent %q", oldAddr, agentName)
	}

	if !force {
		modified, err := isModified(rec)
		if err != nil {
			return err
		}
		if modified {
			return fmt.Errorf("skill %q has local modifications — use --force to relink anyway", oldAddr)
		}
	}

	newInfo, err := repo.FindSkill(newAddr, st)
	if err != nil {
		return fmt.Errorf("replacement skill %q not found: %w", newAddr, err)
	}

	sha, err := repo.HeadSHA(newInfo.RepoName, st)
	if err != nil {
		return fmt.Errorf("getting repo HEAD SHA for %q: %w", newInfo.RepoName, err)
	}

	// Replace the installed directory contents with the replacement skill so the
	// installed copy tracks newAddr (equivalent to a remove-and-reinstall, but in
	// place). Stale provenance from the old skill is cleared before re-loading any
	// fork metadata carried by the replacement.
	if rec.LocalPath != "" {
		if err := os.RemoveAll(rec.LocalPath); err != nil {
			return fmt.Errorf("clearing installed directory %s: %w", rec.LocalPath, err)
		}
		if err := copyDir(newInfo.FullPath, rec.LocalPath); err != nil {
			return fmt.Errorf("copying replacement skill files: %w", err)
		}
		hash, err := ComputeHash(rec.LocalPath)
		if err != nil {
			return fmt.Errorf("computing installed hash: %w", err)
		}
		rec.InstalledHash = hash
	}

	rec.InstalledAtSHA = sha
	rec.UpstreamAddr = ""
	rec.UpstreamSHA = ""
	if rec.LocalPath != "" {
		if err := loadForkProvenance(rec.LocalPath, &rec); err != nil {
			return err
		}
	}

	// Move the record from oldAddr to newAddr.
	if err := st.RecordRemove(oldAddr, agentName); err != nil {
		return err
	}
	if err := st.RecordInstall(newAddr, agentName, rec); err != nil {
		return err
	}
	return state.Save(st)
}

// RelinkUpstream updates the upstream tracking pointer on an installed fork
// without touching the installed files or the fork's own address.
//
// When newUpstreamAddr is non-empty (--set-upstream), the address must resolve
// to a valid skill in a registered, locally-cached repo; its repo HEAD SHA is
// recorded as the new UpstreamSHA.
//
// When newUpstreamAddr is empty (--clear-upstream), both UpstreamAddr and
// UpstreamSHA are cleared, converting the fork to a regular installed skill.
func RelinkUpstream(addr, newUpstreamAddr, agentName string, st *state.State) error {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	if newUpstreamAddr == "" {
		// --clear-upstream: strip both fields.
		rec.UpstreamAddr = ""
		rec.UpstreamSHA = ""
	} else {
		// --set-upstream: resolve addr and capture repo HEAD SHA.
		info, err := repo.FindSkill(newUpstreamAddr, st)
		if err != nil {
			return fmt.Errorf("upstream skill %q not found: %w", newUpstreamAddr, err)
		}
		sha, err := repo.HeadSHA(info.RepoName, st)
		if err != nil {
			return fmt.Errorf("getting repo HEAD SHA for %q: %w", info.RepoName, err)
		}
		rec.UpstreamAddr = newUpstreamAddr
		rec.UpstreamSHA = sha
	}

	agents[agentName] = rec
	return state.Save(st)
}
