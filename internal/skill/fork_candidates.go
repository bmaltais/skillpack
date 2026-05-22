package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// ForkCandidate is a skill that has no .skillpack-fork provenance metadata but
// whose basename matches a skill directory in another registered repo's cache.
type ForkCandidate struct {
	// Addr is the installed skill address, e.g. "bmaltais-skills/triage".
	Addr string
	// CandidateUpstream is the matching skill address in another repo,
	// e.g. "mattpocock-skills/skills/engineering/triage".
	CandidateUpstream string
}

// DetectForkCandidates scans st.InstalledSkills for skills that have no
// UpstreamAddr set on any agent record, and checks whether their basename
// appears as a skill (directory containing SKILL.md) in any other registered
// repo's cache.
//
// Returns nil (not an error) when no repos are registered or no candidates are
// found. The function is read-only: it makes no writes, no network calls, and
// has no side effects.
func DetectForkCandidates(st *state.State) ([]ForkCandidate, error) {
	if len(st.Repos) == 0 {
		return nil, nil
	}

	var candidates []ForkCandidate

	for addr, agents := range st.InstalledSkills {
		// Skip skills that are already tracked as forks.
		hasProvenance := false
		for _, rec := range agents {
			if rec.UpstreamAddr != "" {
				hasProvenance = true
				break
			}
		}
		if hasProvenance {
			continue
		}

		ownRepo := strings.SplitN(addr, "/", 2)[0]
		basename := filepath.Base(addr)

		for repoName, repoRec := range st.Repos {
			if repoName == ownRepo {
				continue
			}
			upstream, err := findSkillInRepo(repoRec.CachePath, repoName, basename)
			if err != nil || upstream == "" {
				continue
			}
			candidates = append(candidates, ForkCandidate{
				Addr:              addr,
				CandidateUpstream: upstream,
			})
		}
	}

	return candidates, nil
}

// findSkillInRepo walks cachePath searching for a directory whose base name
// equals skillName and which contains a SKILL.md file. On the first match it
// returns the skill address formatted as "<repoName>/<relPath>". Returns ""
// (no error) when no match is found.
func findSkillInRepo(cachePath, repoName, skillName string) (string, error) {
	var found string
	err := filepath.WalkDir(cachePath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() != skillName {
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(path, "SKILL.md")); statErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(cachePath, path)
		if relErr != nil {
			return nil
		}
		found = repoName + "/" + filepath.ToSlash(rel)
			return fs.SkipAll // stop walking on first match
		})
	if err == fs.SkipAll {
		err = nil
	}
	return found, err
}

// RegisterForkProvenance retroactively records that addr is a fork of
// upstreamAddr by writing .skillpack-fork into the repo cache, committing and
// pushing, copying the file to all agent installs, and updating state.
//
// addr must already be installed. upstreamAddr must reference a registered and
// locally-cached repo. token is used for HTTPS push; pass "" when the repo
// uses SSH or does not require auth.
func RegisterForkProvenance(addr, upstreamAddr, token string, st *state.State) error {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return fmt.Errorf("skill %q is not installed", addr)
	}

	// Resolve the skill's own repo cache path.
	ownRepo := strings.SplitN(addr, "/", 2)[0]
	ownRepoRec, ok := st.Repos[ownRepo]
	if !ok {
		return fmt.Errorf("repo %q is not registered", ownRepo)
	}
	skillName := filepath.Base(addr)
	skillCachePath := filepath.Join(ownRepoRec.CachePath, skillName)
	if _, err := os.Stat(skillCachePath); err != nil {
		return fmt.Errorf("skill directory not found in repo cache at %s: %w", skillCachePath, err)
	}

	// Resolve the upstream repo name and get its HEAD SHA.
	upstreamRepo := strings.SplitN(upstreamAddr, "/", 2)[0]
	if _, ok := st.Repos[upstreamRepo]; !ok {
		return fmt.Errorf("upstream repo %q is not registered; run: skillpack repo update %s", upstreamRepo, upstreamRepo)
	}
	upstreamSHA, err := repo.HeadSHA(upstreamRepo, st)
	if err != nil {
		return fmt.Errorf("reading HEAD SHA for upstream repo %q: %w", upstreamRepo, err)
	}

	// 1. Write .skillpack-fork into the repo cache skill dir.
	if err := writeForkMetadata(skillCachePath, upstreamAddr, upstreamSHA); err != nil {
		return fmt.Errorf("writing fork metadata to repo cache: %w", err)
	}

	// 2. Commit and push.
	_, err = gitops.CommitAndPush(
		ownRepoRec.CachePath,
		skillName,
		fmt.Sprintf("skillpack: add fork provenance metadata for %s", skillName),
		ownRepoRec.URL,
		token,
	)
	if err != nil {
		return fmt.Errorf("committing and pushing fork metadata: %w", err)
	}

	// 3. Copy .skillpack-fork to all agent installs.
	forkMetaFile := filepath.Join(skillCachePath, forkMetadataFilename)
	for _, rec := range agents {
		if rec.LocalPath == "" {
			continue
		}
		if _, statErr := os.Stat(rec.LocalPath); statErr != nil {
			continue
		}
		dst := filepath.Join(rec.LocalPath, forkMetadataFilename)
		if err := copyFile(forkMetaFile, dst, 0600); err != nil {
			return fmt.Errorf("copying fork metadata to agent install at %s: %w", rec.LocalPath, err)
		}
	}

	// 4. Patch state: set UpstreamAddr and UpstreamSHA on all agent records.
	for agentName, rec := range agents {
		rec.UpstreamAddr = upstreamAddr
		rec.UpstreamSHA = upstreamSHA
		st.InstalledSkills[addr][agentName] = rec
	}

	return state.Save(st)
}

// ForkCandidateMap calls DetectForkCandidates and returns a map of skill address
// to the first detected candidate upstream address. When multiple repos match
// the same basename, the first candidate wins.
// Returns an empty map (not nil) on error so callers can use it safely.
func ForkCandidateMap(st *state.State) map[string]string {
	candidates, err := DetectForkCandidates(st)
	if err != nil {
		return map[string]string{}
	}
	m := make(map[string]string, len(candidates))
	for _, c := range candidates {
		if _, already := m[c.Addr]; !already {
			m[c.Addr] = c.CandidateUpstream
		}
	}
	return m
}
