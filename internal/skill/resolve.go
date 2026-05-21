package skill

import (
	"fmt"

	"github.com/bmaltais/skillpack/internal/state"
)

// ResolveStrategy names the conflict resolution strategy for skill.Resolve.
type ResolveStrategy string

const (
	// ResolveForceRemote overwrites the installed skill with the upstream cache,
	// discarding any local modifications.
	ResolveForceRemote ResolveStrategy = "force-remote"

	// ResolveForceLocal pushes the installed skill back to the remote repo cache,
	// making the local version canonical.
	ResolveForceLocal ResolveStrategy = "force-local"
)

// Resolve executes the requested conflict resolution strategy for an installed skill.
// token is the auth token for git operations against the skill's remote repo.
func Resolve(addr, agentName string, strategy ResolveStrategy, token string, st *state.State) error {
	switch strategy {
	case ResolveForceRemote:
		return ForceRemote(addr, agentName, token, st)
	case ResolveForceLocal:
		return ForceLocal(addr, agentName, token, st)
	default:
		return fmt.Errorf("unknown resolve strategy %q", strategy)
	}
}
