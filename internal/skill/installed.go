package skill

import (
	"fmt"
	"strings"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

// InstalledSkill is a handle to a skill that has already been installed for a
// specific agent. Obtain one via Open; all mutating operations are methods.
//
// The struct is a value type — methods do not mutate the receiver. Each method
// re-reads necessary fields from st (the shared state pointer) so that
// sequential calls see up-to-date records.
type InstalledSkill struct {
	// Addr is the canonical skill address, e.g. "bmaltais-skills/diagnose".
	Addr string
	// AgentName is the agent this installation belongs to, e.g. "claude-code".
	AgentName string
	// Rec is the installed-skill record as it existed at Open() time.
	Rec state.InstalledSkillRecord
	// CachePath is the repo cache directory, pre-resolved at Open() time.
	CachePath string

	st  *state.State   // shared state; methods call state.Save via delegates
	cfg *config.Config // agent config
}

// Open returns an InstalledSkill handle for addr+agentName. It errors if the
// skill is not currently installed (i.e. not recorded in st.InstalledSkills).
// CachePath is resolved from st.Repos using the repo portion of addr.
func Open(addr, agentName string, cfg *config.Config, st *state.State) (InstalledSkill, error) {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return InstalledSkill{}, fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return InstalledSkill{}, fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	// Resolve the cache path from the repo name portion of the address.
	cachePath, err := resolveCachePath(addr, st)
	if err != nil {
		return InstalledSkill{}, err
	}

	return InstalledSkill{
		Addr:      addr,
		AgentName: agentName,
		Rec:       rec,
		CachePath: cachePath,
		st:        st,
		cfg:       cfg,
	}, nil
}

// resolveCachePath extracts the repo name from addr and returns its CachePath.
func resolveCachePath(addr string, st *state.State) (string, error) {
	parts := strings.SplitN(addr, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid skill address %q: must be <repo>/<path>", addr)
	}
	repoName := parts[0]
	rec, ok := st.Repos[repoName]
	if !ok {
		return "", fmt.Errorf("repo %q not registered; run 'skillpack repo add' to register it", repoName)
	}
	return rec.CachePath, nil
}

// Remove removes the installed skill. Delegates to skill.Remove.
func (s InstalledSkill) Remove(force bool) error {
	return Remove(s.Addr, s.AgentName, s.cfg, s.st, force)
}

// Update checks for an upstream change and applies it. Delegates to
// skill.ApplyUpdate. The token is used for forked skills that need a push.
func (s InstalledSkill) Update(token string) error {
	return ApplyUpdate(s.Addr, s.AgentName, token, s.st)
}

// ForceRemote overwrites the installed skill with the upstream cache version.
// Delegates to skill.ForceRemote.
func (s InstalledSkill) ForceRemote(token string) error {
	return ForceRemote(s.Addr, s.AgentName, token, s.st)
}

// ForceLocal copies the locally-installed skill back into the repo cache and
// pushes. Delegates to skill.ForceLocal.
func (s InstalledSkill) ForceLocal(token string) error {
	return ForceLocal(s.Addr, s.AgentName, token, s.st)
}

// Merge performs a three-way merge between the installed skill and the upstream
// cache. Delegates to skill.MergeSkill.
func (s InstalledSkill) Merge(token string) (hasConflicts bool, err error) {
	return MergeSkill(s.Addr, s.AgentName, token, s.st)
}

// Fork forks the skill into forkRepo. Delegates to skill.Fork.
func (s InstalledSkill) Fork(forkRepo, token string, mode ForkMode) (newAddr string, err error) {
	return Fork(s.Addr, forkRepo, s.AgentName, token, mode, s.st)
}

// Publish copies the locally-modified skill back to the repo cache and pushes.
// Delegates to skill.Publish.
func (s InstalledSkill) Publish(token string) error {
	return Publish(s.Addr, s.AgentName, token, s.st)
}

// Resolve resolves a conflict using the given strategy. Delegates to
// skill.Resolve.
func (s InstalledSkill) Resolve(strategy ResolveStrategy, token, llmAgentName string) (bool, error) {
	return Resolve(s.Addr, s.AgentName, strategy, token, llmAgentName, s.st)
}

// PushForkAfterLLM pushes a fork after an LLM-assisted edit. Delegates to
// skill.PushForkAfterLLM.
func (s InstalledSkill) PushForkAfterLLM(token string) error {
	return PushForkAfterLLM(s.Addr, s.AgentName, token, s.st)
}

// IsModified reports whether the installed skill has been locally modified
// since install time. It re-reads the current record from state so that the
// result is accurate even after Update/Merge/Fork operations that may have
// changed the stored InstalledHash. Returns an error if the record is no
// longer present in state.
func (s InstalledSkill) IsModified() (bool, error) {
	agents, ok := s.st.InstalledSkills[s.Addr]
	if !ok {
		return false, fmt.Errorf("skill %q is no longer installed", s.Addr)
	}
	rec, ok := agents[s.AgentName]
	if !ok {
		return false, fmt.Errorf("skill %q is no longer installed for agent %q", s.Addr, s.AgentName)
	}
	return IsModified(rec)
}

// IsFork reports whether this installed skill is a fork of an upstream skill.
// It re-reads the current record from state so that the result reflects any
// Fork/Resolve operations performed after Open() was called. Falls back to the
// snapshot record captured at Open() time if the entry is no longer in state.
func (s InstalledSkill) IsFork() bool {
	if agents, ok := s.st.InstalledSkills[s.Addr]; ok {
		if rec, ok := agents[s.AgentName]; ok {
			return IsFork(rec)
		}
	}
	return IsFork(s.Rec)
}
