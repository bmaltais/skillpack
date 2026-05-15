package skill

import (
	"fmt"
	"strings"

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

// Sync performs two-way reconciliation for all installed skills:
//
//  1. Pulls every registered repo (updates the local cache).
//  2. For each installed skill:
//     - No local edits + upstream changed  → update (cache → installed)
//     - Local edits + no upstream change   → publish (installed → cache, commit, push)
//     - Local edits + upstream changed     → skip, append to conflicts
//     - Neither                            → nothing to do
//
// When dryRun is true, repos are still pulled (to get accurate upstream state) but
// no installed-skill files or state records are modified.
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
					if applyErr := ApplyUpdate(addr, agentName, st); applyErr != nil {
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
				}
				results = append(results, SyncResult{addr, agentName, SyncPublished, nil})

			default:
				// Already in sync.
				results = append(results, SyncResult{addr, agentName, SyncAlreadyCurrent, nil})
			}
		}
	}

	return results, conflicts, nil
}
