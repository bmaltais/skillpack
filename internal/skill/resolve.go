package skill

import (
	"errors"
	"fmt"

	"github.com/bmaltais/skillpack/internal/state"
)

// ErrMergeConflicts is returned by Resolve when a three-way merge wrote conflict
// markers into one or more installed files. Callers should prompt the user to
// resolve manually or retry with a resolution strategy.
var ErrMergeConflicts = errors.New("merge produced conflicts")

// ResolveStrategy names the conflict resolution strategy for skill.Resolve.
type ResolveStrategy string

const (
	// ResolveForceRemote overwrites the installed skill with the upstream cache,
	// discarding any local modifications.
	ResolveForceRemote ResolveStrategy = "force-remote"

	// ResolveForceLocal pushes the installed skill back to the remote repo cache,
	// making the local version canonical.
	ResolveForceLocal ResolveStrategy = "force-local"

	// ResolveMerge performs a file-level three-way merge. If any file has
	// irreconcilable changes, ErrMergeConflicts is returned and conflict markers
	// are left in the installed files for manual resolution.
	ResolveMerge ResolveStrategy = "merge"

	// ResolveLLM performs a three-way merge and, if conflicts remain, sends each
	// conflicted file to the named LLM agent for automatic resolution.
	ResolveLLM ResolveStrategy = "llm"
)

// Resolve executes the requested conflict resolution strategy for an installed skill.
// token is the auth token for git operations against the skill's remote repo.
// llmAgentName is only used for the ResolveLLM strategy; pass "" for all others.
//
// The returned bool is true when the ResolveLLM pipeline was invoked (MergeSkill
// detected conflicts and LLMResolveConflicts was called). It is only meaningful
// when err == nil; callers must not inspect it when err != nil.
// resolve executes the requested conflict resolution strategy for an installed skill.
// token is the auth token for git operations against the skill's remote repo.
// llmAgentName is only used for the ResolveLLM strategy; pass "" for all others.
//
// The returned bool is true when the ResolveLLM pipeline was invoked (mergeSkill
// detected conflicts and LLMResolveConflicts was called). It is only meaningful
// when err == nil; callers must not inspect it when err != nil.
func resolve(addr, agentName string, strategy ResolveStrategy, token, llmAgentName string, st *state.State) (bool, error) {
	switch strategy {
	case ResolveForceRemote:
		return false, forceRemote(addr, agentName, token, st)
	case ResolveForceLocal:
		return false, forceLocal(addr, agentName, token, st)
	case ResolveMerge:
		hadConflicts, err := mergeSkill(addr, agentName, token, st)
		if err != nil {
			return false, err
		}
		if hadConflicts {
			return false, ErrMergeConflicts
		}
		return false, nil
	case ResolveLLM:
		if llmAgentName == "" {
			return false, fmt.Errorf("ResolveLLM requires a non-empty llmAgentName")
		}
		hadConflicts, err := mergeSkill(addr, agentName, token, st)
		if err != nil {
			return false, err
		}
		if !hadConflicts {
			// Clean merge — no LLM invocation needed.
			return false, nil
		}
		resolver, err := NewDefaultLLMResolver(llmAgentName)
		if err != nil {
			return false, err
		}
		if err := LLMResolveConflicts(addr, agentName, resolver, st); err != nil {
			return false, err
		}
		rec := st.InstalledSkills[addr][agentName]
		if isFork(rec) {
			if err := pushForkAfterLLM(addr, agentName, token, st); err != nil {
				return false, err
			}
			return true, nil
		}
		if err := snapshotInstalled(addr, agentName, st); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown resolve strategy %q", strategy)
	}
}
