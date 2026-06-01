package skill

import (
	"fmt"
	"strings"

	"github.com/bmaltais/skillpack/internal/state"
)

// UpdateAction describes the update status of one installed skill.
type UpdateAction string

const (
	UpdateAlreadyCurrent UpdateAction = "up-to-date"     // no upstream changes, no local edits
	UpdateAvailable      UpdateAction = "update-available" // upstream has new changes
	UpdateLocallyModified UpdateAction = "locally-modified" // local edits, no upstream changes
	UpdateConflict       UpdateAction = "conflict"         // both local edits and upstream changes
)

// UpdatePlanItem is the decision for one installed skill, computed without
// git or network I/O. Err is non-nil when the plan for this skill could not
// be determined (e.g. repo not registered, local-modification check failed).
// Callers should display Err and skip the skill rather than acting on Action
// when Err != nil.
type UpdatePlanItem struct {
	Addr      string
	AgentName string
	Action    UpdateAction
	Err       error
	// Warning is a non-fatal message (e.g. upstream repo not registered).
	// When set, Action is still valid and should be acted on.
	Warning string
	// UpstreamDisabled is true when a fork's upstream repo is not registered.
	// Callers should use the fork's own repo cache for updates instead of the
	// upstream origin.
	UpstreamDisabled bool
}

// PlanUpdate determines the update action for every installed skill using only
// the data provided — no git or network I/O is performed inside this function.
//
// repoHeads maps repo name to the current HEAD SHA in the local cache (obtained
// by the caller, e.g. via CollectRepoHeads).
// Upstream-change detection compares InstalledAtSHA (non-forks) or UpstreamSHA
// (forks) against the provided HEAD. This is a coarser check than a file-level
// diff — a commit touching only unrelated parts of the repo will still produce
// an UpdateAvailable action.
//
// Local-modification detection calls isModified, which hashes the installed
// directory (local file reads; no git or network operations).
//
// If a skill's repo is not present in repoHeads, or if the local-modification
// check fails, the returned UpdatePlanItem has Err set and Action = UpdateAlreadyCurrent.
// Callers must check Err before acting on Action.
func PlanUpdate(st *state.State, repoHeads map[string]string) []UpdatePlanItem {
	var plan []UpdatePlanItem
	for addr, agents := range st.InstalledSkills {
		for agentName, rec := range agents {
			item := UpdatePlanItem{Addr: addr, AgentName: agentName, Action: UpdateAlreadyCurrent}

			headSHA := repoHeadForRecord(addr, rec, repoHeads)
			if headSHA == "" {
				if isFork(rec) {
					// Upstream repo not registered — fall back to evaluating
					// against the skill's own repo HEAD.
					ownRepo := strings.SplitN(addr, "/", 2)[0]
					headSHA = repoHeads[ownRepo]
					if headSHA == "" {
						item.Err = fmt.Errorf("repo %q not found in local cache — run 'skillpack repo add' to register it", ownRepo)
						plan = append(plan, item)
						continue
					}
					upstreamRepo := strings.SplitN(rec.UpstreamAddr, "/", 2)[0]
					item.Warning = fmt.Sprintf("upstream repo %q not registered — skipping upstream tracking", upstreamRepo)
					item.UpstreamDisabled = true
				} else {
					missingRepo := strings.SplitN(addr, "/", 2)[0]
					item.Err = fmt.Errorf("repo %q not found in local cache — run 'skillpack repo add' to register it", missingRepo)
					plan = append(plan, item)
					continue
				}
			}

			baselineSHA := rec.InstalledAtSHA
			if isFork(rec) && item.Warning == "" {
				baselineSHA = rec.UpstreamSHA
			}
			hasUpstream := baselineSHA != headSHA

			modified, _, modErr := isModifiedWithHash(rec)
			if modErr != nil {
				item.Err = fmt.Errorf("checking local modifications: %w", modErr)
				plan = append(plan, item)
				continue
			}

			switch {
			case hasUpstream && modified:
				// Check whether installed files are byte-identical to upstream cache
				// to avoid phantom conflicts after a --force-remote reset.
				var checkDir string
				if item.UpstreamDisabled {
					checkDir = ownRepoCacheDirFor(addr, st)
				} else {
					checkDir = upstreamCacheDirFor(addr, rec, st)
				}
				if checkDir != "" {
					cacheHash, uErr := ComputeHash(checkDir)
					if uErr == nil {
						currentHash, _ := ComputeHash(rec.LocalPath)
						if currentHash == cacheHash {
							item.Action = UpdateAlreadyCurrent
							break
						}
					}
				}
				item.Action = UpdateConflict
			case hasUpstream && !modified:
				item.Action = UpdateAvailable
			case modified && !hasUpstream:
				item.Action = UpdateLocallyModified
			}
			plan = append(plan, item)
		}
	}
	return plan
}
