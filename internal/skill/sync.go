package skill

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// SyncAction describes what happened (or would happen) for one installed skill during sync.
type SyncAction string

const (
	SyncUpdated        SyncAction = "updated"          // upstream applied to local
	SyncPublished      SyncAction = "published"         // local edits pushed to remote
	SyncConflict       SyncAction = "skipped-conflict"  // modified locally + upstream changed
	SyncAlreadyCurrent SyncAction = "already-current"   // nothing to do
)

// SyncResult describes the outcome of syncing one installed skill.
type SyncResult struct {
	Addr      string
	AgentName string
	Action    SyncAction
	Err       error
}

// SyncPlanItem is the decision for one installed skill, computed without git or network I/O.
// Err is non-nil when the plan for this skill could not be determined (e.g. repo not
// registered, local-modification check failed). Callers should display Err and skip
// the skill rather than acting on Action when Err != nil.
type SyncPlanItem struct {
	Addr      string
	AgentName string
	Action    SyncAction
	Err       error
}

// ReconcilePlan determines the sync action for every installed skill using only
// the data provided — no git or network I/O is performed inside this function.
//
// repoHeads maps repo name to the current HEAD SHA in the local cache (obtained
// by the caller, e.g. via CollectRepoHeads, after any desired repo pulls).
// Upstream-change detection first uses a coarse repo-HEAD SHA comparison as a
// fast pre-filter. When the SHA indicates a change, the result is refined by
// comparing the upstream cache directory's content hash against InstalledHash.
// Only a skill whose own content differs from the install-time snapshot is
// considered to have an upstream change — commits touching only unrelated paths
// in the same repo do not produce a SyncUpdated action.
//
// Local-modification detection calls IsModified, which hashes the installed
// directory (local file reads; no git or network operations).
//
// If a skill's repo is not present in repoHeads, or if the local-modification
// check fails, the returned SyncPlanItem has Err set and Action = SyncAlreadyCurrent.
// Callers must check Err before acting on Action.
//
// To simulate the second-pass sibling re-check that Sync performs after
// publishing, call ReconcilePlan a second time on the updated state.
func ReconcilePlan(st *state.State, repoHeads map[string]string) []SyncPlanItem {
	var plan []SyncPlanItem
	for addr, agents := range st.InstalledSkills {
		for agentName, rec := range agents {
			item := SyncPlanItem{Addr: addr, AgentName: agentName, Action: SyncAlreadyCurrent}

			headSHA := repoHeadForRecord(addr, rec, repoHeads)
			if headSHA == "" {
				// Repo not found in repoHeads — state is inconsistent or the repo was removed.
				var missingRepo string
				if isFork(rec) {
					missingRepo = strings.SplitN(rec.UpstreamAddr, "/", 2)[0]
				} else {
					missingRepo = strings.SplitN(addr, "/", 2)[0]
				}
				item.Err = fmt.Errorf("repo %q not found in local cache — run 'skillpack repo add' to register it", missingRepo)
				plan = append(plan, item)
				continue
			}

			// For forked skills the relevant baseline is upstream_sha (the upstream
			// HEAD at the time of the last fork/update), not installed_at_sha (which
			// is a SHA from the fork's own repo and cannot be compared against the
			// upstream repo's SHA space).
			baselineSHA := rec.InstalledAtSHA
			if isFork(rec) {
				baselineSHA = rec.UpstreamSHA
			}
			// Coarse pre-filter: if repo HEAD is unchanged, no skill in that repo
			// can have upstream changes.
			hasUpstream := baselineSHA != headSHA
			if hasUpstream {
				// Refine with a per-skill content hash comparison so that commits
				// touching only unrelated paths in the same repo do not produce a
				// spurious SyncUpdated. Fall back to the coarser SHA result only
				// when the upstream cache directory cannot be located or hashed.
				if upstreamDir := upstreamCacheDirFor(addr, rec, st); upstreamDir != "" {
					if upstreamHash, uErr := ComputeHash(upstreamDir); uErr == nil {
						hasUpstream = upstreamHash != rec.InstalledHash
					}
				}
			}

			modified, installedHash, modErr := isModifiedWithHash(rec)
			if modErr != nil {
				item.Err = fmt.Errorf("checking local modifications: %w", modErr)
				plan = append(plan, item)
				continue
			}

			switch {
			case hasUpstream && modified:
				// Before reporting a conflict, check whether the installed files are
				// byte-identical to the upstream cache. If they are, the stored
				// InstalledHash is merely stale (e.g. after a --force-remote reset) and
				// there is no real conflict — treat it as already-current.
				if upstreamDir := upstreamCacheDirFor(addr, rec, st); upstreamDir != "" {
					upstreamHash, uErr := ComputeHash(upstreamDir)
					if uErr == nil && installedHash == upstreamHash {
						item.Action = SyncAlreadyCurrent
						break
					}
				}
				item.Action = SyncConflict
			case hasUpstream && !modified:
				item.Action = SyncUpdated
			case modified && !hasUpstream:
				item.Action = SyncPublished
			}
			plan = append(plan, item)
		}
	}
	return plan
}

// upstreamCacheDirFor returns the filesystem path of the upstream cache
// directory for the given skill record. For non-forked skills this is
// <repo.CachePath>/<relPath>; for forked skills the upstream repo is used.
// Returns "" if the path cannot be determined (missing repo, malformed addr).
func upstreamCacheDirFor(addr string, rec state.InstalledSkillRecord, st *state.State) string {
	srcAddr := addr
	if isFork(rec) {
		srcAddr = rec.UpstreamAddr
	}
	parts := strings.SplitN(srcAddr, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	repoRec, ok := st.Repos[parts[0]]
	if !ok {
		return ""
	}
	return filepath.Join(repoRec.CachePath, filepath.FromSlash(parts[1]))
}

// repoHeadForRecord returns the relevant HEAD SHA for a skill record:
// the upstream repo HEAD for forked skills, the skill's own repo HEAD otherwise.
func repoHeadForRecord(addr string, rec state.InstalledSkillRecord, repoHeads map[string]string) string {
	if isFork(rec) {
		upstreamRepoName := strings.SplitN(rec.UpstreamAddr, "/", 2)[0]
		return repoHeads[upstreamRepoName]
	}
	repoName := strings.SplitN(addr, "/", 2)[0]
	return repoHeads[repoName]
}

// CollectRepoHeads reads the current HEAD SHA from each registered repo's local
// cache. This is a local git read (no network). The resulting map is suitable for
// passing directly to ReconcilePlan.
func CollectRepoHeads(st *state.State) (map[string]string, error) {
	heads := make(map[string]string, len(st.Repos))
	for name, rec := range st.Repos {
		sha, err := gitops.HeadSHA(rec.CachePath)
		if err != nil {
			return nil, fmt.Errorf("reading HEAD for repo %q: %w", name, err)
		}
		heads[name] = sha
	}
	return heads, nil
}

// ApplySync executes a plan produced by ReconcilePlan, applying updates,
// publishing local edits, and collecting conflicts. State persistence is
// handled by applyUpdate and publish individually — ApplySync does not write
// state itself.
//
// SyncConflict items are collected into the returned conflicts slice without
// being applied; callers that wish to resolve them should do so and call
// ApplySync again with a fresh plan from ReconcilePlan.
//
// Items whose Err field is non-nil are reported as error results and skipped.
// tokenFor is called with a repo name to resolve its auth token; pass nil to
// rely on environment variables only.
func ApplySync(plan []SyncPlanItem, tokenFor func(string) string, st *state.State) (results []SyncResult, conflicts []SyncResult, err error) {
	if tokenFor == nil {
		tokenFor = func(string) string { return "" }
	}
	results = make([]SyncResult, 0, len(plan))
	conflicts = make([]SyncResult, 0, len(plan))
	for _, item := range plan {
		if item.Err != nil {
			results = append(results, SyncResult{Addr: item.Addr, AgentName: item.AgentName, Err: item.Err})
			continue
		}
		repoName := strings.SplitN(item.Addr, "/", 2)[0]
		switch item.Action {
		case SyncConflict:
			conflicts = append(conflicts, SyncResult{item.Addr, item.AgentName, SyncConflict, nil})
		case SyncUpdated:
			if applyErr := applyUpdate(item.Addr, item.AgentName, tokenFor(repoName), st); applyErr != nil {
				results = append(results, SyncResult{Addr: item.Addr, AgentName: item.AgentName, Err: applyErr})
				continue
			}
			results = append(results, SyncResult{item.Addr, item.AgentName, SyncUpdated, nil})
		case SyncPublished:
			if pubErr := publish(item.Addr, item.AgentName, tokenFor(repoName), st); pubErr != nil {
				results = append(results, SyncResult{Addr: item.Addr, AgentName: item.AgentName, Err: pubErr})
				continue
			}
			results = append(results, SyncResult{item.Addr, item.AgentName, SyncPublished, nil})
		default: // SyncAlreadyCurrent
			// For non-forked skills, refresh a stale InstalledHash/InstalledAtSHA so
			// the phantom conflict cannot reappear on the next sync.
			// Forked skills are skipped: snapshotInstalled only refreshes InstalledAtSHA
			// (from the fork's own repo), not UpstreamSHA, so calling it on a fork
			// would leave hasUpstream=true on the next ReconcilePlan and trigger a
			// spurious SyncUpdated.
			if rec, ok := st.InstalledSkills[item.Addr][item.AgentName]; ok && !isFork(rec) {
				if currentHash, hashErr := ComputeHash(rec.LocalPath); hashErr == nil && currentHash != rec.InstalledHash {
					if snapErr := snapshotInstalled(item.Addr, item.AgentName, st); snapErr != nil {
						results = append(results, SyncResult{item.Addr, item.AgentName, SyncAlreadyCurrent, snapErr})
						continue
					}
				}
			}
			results = append(results, SyncResult{item.Addr, item.AgentName, SyncAlreadyCurrent, nil})
		}
	}
	return results, conflicts, nil
}

// Sync performs two-way reconciliation for all installed skills:
//
//  1. Pulls every registered repo (updates the local cache).
//  2. Calls ReconcilePlan to determine the action for each installed skill,
//     then ApplySync to execute those actions.
//  3. Second-pass sibling re-check: a publish in step 2 advances the cache HEAD.
//     ReconcilePlan is called a second time on the updated state, and ApplySync
//     applies any updates that are now visible to sibling agents that were
//     marked already-current before the publish.
//
// When dryRun is true, repo pulls are skipped and only a status message is
// printed per repo. ApplySync is not called — steps 2 and 3 are skipped.
//
// tokenFor is called with a repo name to resolve its token; pass nil to rely
// on environment variables only.
//
// Returns all results and a separate slice for conflicts.
func Sync(dryRun bool, tokenFor func(string) string, st *state.State) (results []SyncResult, conflicts []SyncResult, err error) {
	if tokenFor == nil {
		tokenFor = func(string) string { return "" }
	}
	// Step 1: Pull all registered repos so we have fresh upstream state.
	for name := range st.Repos {
		if dryRun {
			fmt.Printf("  would pull repo %s (skipped in dry-run)\n", name)
		} else {
			if pullErr := repo.Update(name, tokenFor(name), st); pullErr != nil {
				// Non-fatal: report but keep going with stale cache.
				fmt.Printf("  warning: could not pull %s: %v\n", name, pullErr)
			}
		}
	}

	if dryRun {
		return nil, nil, nil
	}

	// Step 2: Reconcile and apply.
	heads, headsErr := CollectRepoHeads(st)
	if headsErr != nil {
		return nil, nil, headsErr
	}
	plan := ReconcilePlan(st, heads)
	results, conflicts, err = ApplySync(plan, tokenFor, st)
	if err != nil {
		return results, conflicts, err
	}

	// Step 3: Second-pass sibling re-check.
	//
	// Any publish in step 2 advanced the cache HEAD. Re-running ReconcilePlan
	// on the updated state surfaces sibling agents that were marked
	// already-current before the publish. Apply those updates now so a single
	// Sync call fully converges.
	heads2, heads2Err := CollectRepoHeads(st)
	if heads2Err != nil {
		return results, conflicts, fmt.Errorf("second-pass head collection: %w", heads2Err)
	}
	plan2 := ReconcilePlan(st, heads2)
	secondResults, secondConflicts, err2 := ApplySync(plan2, tokenFor, st)
	if err2 != nil {
		return results, conflicts, err2
	}

	// Merge second-pass outcomes into the main results.
	// A skill that was already-current in pass 1 may have become updated,
	// errored, or conflicted in pass 2 (e.g. after a sibling publish advanced
	// the cache HEAD). Surface all such changes so callers see the final state.
	type skillKey struct{ addr, agentName string }
	resultIdx := make(map[skillKey]int, len(results))
	for i, r := range results {
		resultIdx[skillKey{r.Addr, r.AgentName}] = i
	}
	for _, r := range secondResults {
		key := skillKey{r.Addr, r.AgentName}
		if i, ok := resultIdx[key]; ok && results[i].Action == SyncAlreadyCurrent && r.Action != SyncAlreadyCurrent {
			results[i] = r
		}
	}
	for _, c := range secondConflicts {
		key := skillKey{c.Addr, c.AgentName}
		if i, ok := resultIdx[key]; ok && results[i].Action == SyncAlreadyCurrent {
			results[i] = SyncResult{Addr: c.Addr, AgentName: c.AgentName, Action: SyncConflict}
			conflicts = append(conflicts, c)
		}
	}

	return results, conflicts, nil
}
