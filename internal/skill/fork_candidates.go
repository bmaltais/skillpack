package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
// canWrite is an optional predicate: when non-nil, only skills whose own repo
// passes canWrite(repoName) are returned as candidates.  Pass nil to disable
// the filter (all matching skills are returned regardless of write access).
//
// Returns nil (not an error) when no repos are registered or no candidates are
// found. The function is read-only: it makes no writes, no network calls, and
// has no side effects.
func DetectForkCandidates(st *state.State, canWrite func(string) bool) ([]ForkCandidate, error) {
	if len(st.Repos) == 0 {
		return nil, nil
	}

	var candidates []ForkCandidate

	// Sort installed skill addresses for deterministic output.
	addrs := make([]string, 0, len(st.InstalledSkills))
	for addr := range st.InstalledSkills {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	// Sort repo names so the first match is stable across runs.
	repoNames := make([]string, 0, len(st.Repos))
	for name := range st.Repos {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)

	for _, addr := range addrs {
		agents := st.InstalledSkills[addr]
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
		// Only flag skills in repos the caller has write access to. This prevents
		// upstream (read-only) skills from appearing as fork candidates — the
		// [fork?] label and register-provenance action only make sense for repos
		// you own.
		if canWrite != nil && !canWrite(ownRepo) {
			continue
		}
		basename := filepath.Base(addr)

		// Use the first (lexicographically smallest) repo that matches, for stability.
		for _, repoName := range repoNames {
			if repoName == ownRepo {
				continue
			}
			upstream, err := findSkillInRepo(st.Repos[repoName].CachePath, repoName, basename)
			if err != nil || upstream == "" {
				continue
			}
			candidates = append(candidates, ForkCandidate{
				Addr:              addr,
				CandidateUpstream: upstream,
			})
			break // first match wins; repos are sorted so the choice is stable
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

	// Resolve the skill's cache path via its full address (supports nested paths,
	// e.g. "my-repo/coding/debugger" → cachePath/coding/debugger).
	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return fmt.Errorf("resolving skill %q in repo cache: %w", addr, err)
	}
	skillCachePath := skillInfo.FullPath
	ownRepoRec := st.Repos[skillInfo.RepoName]

	// Guard: registering provenance requires write access to the skill's repo.
	// HTTPS repos need a token; SSH repos rely on the agent.  If neither is
	// available the subsequent CommitAndPush will fail with a cryptic auth
	// error, so we surface a clear message here instead.
	if !gitops.IsSSHURL(ownRepoRec.URL) && token == "" {
		return fmt.Errorf(
			"cannot register fork provenance for %q: no write access to repo %q (%s)\n"+
				"Hint: fork the skill into a repo you own first:\n"+
				"  skillpack fork %s <your-repo>",
			addr, skillInfo.RepoName, ownRepoRec.URL, addr,
		)
	}

	// Validate that upstreamAddr points to an existing skill in its repo cache,
	// so we don't write a broken UpstreamAddr into state.
	if _, err := repo.FindSkill(upstreamAddr, st); err != nil {
		return fmt.Errorf("upstream skill %q not found; ensure the repo is registered and cached: %w", upstreamAddr, err)
	}

	// Resolve the upstream repo name and get its HEAD SHA.
	upstreamRepo := strings.SplitN(upstreamAddr, "/", 2)[0]
	upstreamSHA, err := repo.HeadSHA(upstreamRepo, st)
	if err != nil {
		return fmt.Errorf("reading HEAD SHA for upstream repo %q: %w", upstreamRepo, err)
	}

	// 1. Write .skillpack-fork into the repo cache skill dir.
	if err := writeForkMetadata(skillCachePath, upstreamAddr, upstreamSHA); err != nil {
		return fmt.Errorf("writing fork metadata to repo cache: %w", err)
	}

	// 2. Commit and push (skillInfo.RelPath is the path relative to repo root).
	_, err = gitops.CommitAndPush(
		ownRepoRec.CachePath,
		skillInfo.RelPath,
		fmt.Sprintf("skillpack: add fork provenance metadata for %s", addr),
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
// canWrite is forwarded to DetectForkCandidates; pass nil to disable filtering.
func ForkCandidateMap(st *state.State, canWrite func(string) bool) map[string]string {
	candidates, err := DetectForkCandidates(st, canWrite)
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
