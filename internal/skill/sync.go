package skill

import (
	"fmt"
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

// SyncPlanItem is the decision for one installed skill, computed without I/O.
type SyncPlanItem struct {
	Addr      string
	AgentName string
	Action    SyncAction
}

// ReconcilePlan determines the sync action for every installed skill using only
// the data provided — no git or network I/O is performed inside this function.
//
// repoHeads maps repo name to the current HEAD SHA in the local cache (obtained
// by the caller, e.g. via CollectRepoHeads, after any desired repo pulls).
// Upstream-change detection compares InstalledAtSHA against the provided HEAD;
// this is a coarser check than a file-level diff — a commit touching unrelated
// parts of the repo will still produce a SyncUpdated action.
//
// Local-modification detection calls IsModified, which hashes the installed
// directory (local file reads; no git operations).
//
// To simulate the second-pass sibling re-check that Sync performs after
// publishing, call ReconcilePlan a second time on the updated state.
func ReconcilePlan(st *state.State, repoHeads map[string]string) []SyncPlanItem {
	var plan []SyncPlanItem
	for addr, agents := range st.InstalledSkills {
		for agentName, rec := range agents {
			headSHA := repoHeadForRecord(addr, rec, repoHeads)
			hasUpstream := headSHA != "" && rec.InstalledAtSHA != headSHA

			modified, _ := IsModified(rec) // local hash check; error → assume not modified

			var action SyncAction
			switch {
			case hasUpstream && modified:
				action = SyncConflict
			case hasUpstream && !modified:
				action = SyncUpdated
			case modified && !hasUpstream:
				action = SyncPublished
			default:
				action = SyncAlreadyCurrent
			}
			plan = append(plan, SyncPlanItem{Addr: addr, AgentName: agentName, Action: action})
		}
	}
	return plan
}

// repoHeadForRecord returns the relevant HEAD SHA for a skill record:
// the upstream repo HEAD for forked skills, the skill's own repo HEAD otherwise.
func repoHeadForRecord(addr string, rec state.InstalledSkillRecord, repoHeads map[string]string) string {
	if rec.UpstreamAddr != "" {
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

// Sync performs two-way reconciliation for all installed skills:
//
//  1. Pulls every registered repo (updates the local cache).
//  2. For each installed skill:
//     - No local edits + upstream changed  → update (cache → installed)
//     - Local edits + no upstream change   → publish (installed → cache, commit, push)
//     - Local edits + upstream changed     → skip, append to conflicts
//     - Neither                            → nothing to do
//  3. Second-pass update for sibling agents: if a skill was published in step 2,
//     the cache HEAD advanced. Any agent whose copy of that skill was evaluated
//     before the publish (and marked already-current) is re-checked and updated
//     if it is now behind. This ensures a single sync run fully converges.
//
// When dryRun is true, repos are still pulled (to get accurate upstream state) but
// no installed-skill files or state records are modified (steps 2 and 3 are skipped).
//
// tokenFor is called with a repo name to resolve its token; pass nil to rely on env vars only.
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

	// Step 2: Reconcile each installed skill.
	//
	// publishedAddrs tracks skill addresses where a publish occurred in this
	// pass. A publish advances the cache HEAD; sibling agents sharing the same
	// address may have been evaluated (and marked already-current) before that
	// new HEAD was visible, so they need a second look in step 3.
	publishedAddrs := make(map[string]bool)

	for addr, agents := range st.InstalledSkills {
		for agentName := range agents {
			result, checkErr := CheckUpdate(addr, agentName, st)
			if checkErr != nil {
				results = append(results, SyncResult{addr, agentName, "", checkErr})
				continue
			}

			switch {
			case result.IsConflict:
				// Both sides changed — user must resolve manually.
				conflicts = append(conflicts, SyncResult{addr, agentName, SyncConflict, nil})

			case result.HasUpstream && !result.IsModified:
				// Safe upstream update.
				if !dryRun {
					if applyErr := ApplyUpdate(addr, agentName, tokenFor(strings.SplitN(addr, "/", 2)[0]), st); applyErr != nil {
						results = append(results, SyncResult{addr, agentName, "", applyErr})
						continue
					}
				}
				results = append(results, SyncResult{addr, agentName, SyncUpdated, nil})

			case result.IsModified && !result.HasUpstream:
				// Local edits, nothing upstream — publish.
				if !dryRun {
					repoName := strings.SplitN(addr, "/", 2)[0]
					if pubErr := Publish(addr, agentName, tokenFor(repoName), st); pubErr != nil {
						results = append(results, SyncResult{addr, agentName, "", pubErr})
						continue
					}
					publishedAddrs[addr] = true
				}
				results = append(results, SyncResult{addr, agentName, SyncPublished, nil})

			default:
				// Already in sync.
				results = append(results, SyncResult{addr, agentName, SyncAlreadyCurrent, nil})
			}
		}
	}

	// Step 3: Second-pass update for sibling agents.
	//
	// When agent A's skill was published in step 2, the cache HEAD advanced.
	// Agent B's copy of the same skill may have been evaluated before that
	// publish and incorrectly marked already-current. Re-check every
	// already-current result whose address was published and apply any
	// update that is now visible.
	if !dryRun {
		for i, r := range results {
			if r.Err != nil || r.Action != SyncAlreadyCurrent || !publishedAddrs[r.Addr] {
				continue
			}
			recheck, checkErr := CheckUpdate(r.Addr, r.AgentName, st)
			if checkErr != nil {
				results[i] = SyncResult{r.Addr, r.AgentName, "", checkErr}
				continue
			}
			if recheck.HasUpstream && !recheck.IsModified {
				if applyErr := ApplyUpdate(r.Addr, r.AgentName, tokenFor(strings.SplitN(r.Addr, "/", 2)[0]), st); applyErr != nil {
					results[i] = SyncResult{r.Addr, r.AgentName, "", applyErr}
					continue
				}
				results[i] = SyncResult{r.Addr, r.AgentName, SyncUpdated, nil}
			}
		}
	}

	return results, conflicts, nil
}
