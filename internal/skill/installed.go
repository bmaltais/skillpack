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
//
// InstalledSkill is a value type (not a pointer) — it holds a snapshot of the
// record at construction time plus pointers to the shared cfg and st so that
// method calls operate on live state. Callers should not store InstalledSkill
// values across mutations made by other callers.
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
// Returns an empty string (no error) when the repo is not registered — callers
// that need a valid CachePath (e.g. Update, Fork) will fail with a clear message
// at use time; callers that do not (e.g. Remove) work correctly with an empty path.
func resolveCachePath(addr string, st *state.State) (string, error) {
	parts := strings.SplitN(addr, "/", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid skill address %q: must be <repo>/<path>", addr)
	}
	repoName := parts[0]
	rec, ok := st.Repos[repoName]
	if !ok {
		return "", nil
	}
	return rec.CachePath, nil
}

// Remove removes the installed skill.
func (s InstalledSkill) Remove(force bool) error {
	return remove(s.Addr, s.AgentName, s.cfg, s.st, force)
}

// Update checks for an upstream change and applies it.
// The token is used for forked skills that need a push.
func (s InstalledSkill) Update(token string) error {
	return applyUpdate(s.Addr, s.AgentName, token, s.st)
}

// ForceRemote overwrites the installed skill with the upstream cache version.
func (s InstalledSkill) ForceRemote(token string) error {
	return forceRemote(s.Addr, s.AgentName, token, s.st)
}

// ForceLocal copies the locally-installed skill back into the repo cache and pushes.
func (s InstalledSkill) ForceLocal(token string) error {
	return forceLocal(s.Addr, s.AgentName, token, s.st)
}

// Merge performs a three-way merge between the installed skill and the upstream cache.
func (s InstalledSkill) Merge(token string) (hasConflicts bool, err error) {
	return mergeSkill(s.Addr, s.AgentName, token, s.st)
}

// Fork forks the skill into forkRepo.
func (s InstalledSkill) Fork(forkRepo, token string, mode ForkMode) (newAddr string, err error) {
	return fork(s.Addr, forkRepo, s.AgentName, token, mode, s.st)
}

// Publish copies the locally-modified skill back to the repo cache and pushes.
func (s InstalledSkill) Publish(token string) error {
	return publish(s.Addr, s.AgentName, token, s.st)
}

// Resolve resolves a conflict using the given strategy.
func (s InstalledSkill) Resolve(strategy ResolveStrategy, token, llmAgentName string) (bool, error) {
	return resolve(s.Addr, s.AgentName, strategy, token, llmAgentName, s.st)
}

// PushForkAfterLLM pushes a fork after an LLM-assisted edit.
func (s InstalledSkill) PushForkAfterLLM(token string) error {
	return pushForkAfterLLM(s.Addr, s.AgentName, token, s.st)
}

// Status returns the update state of this installed skill relative to upstream.
func (s InstalledSkill) Status() (*UpdateResult, error) {
	return checkUpdate(s.Addr, s.AgentName, s.st)
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
	return isModified(rec)
}

// IsFork reports whether this installed skill is a fork of an upstream skill.
// It re-reads the current record from state so that the result reflects any
// Fork/Resolve operations performed after Open() was called. Falls back to the
// snapshot record captured at Open() time if the entry is no longer in state.
func (s InstalledSkill) IsFork() bool {
	if agents, ok := s.st.InstalledSkills[s.Addr]; ok {
		if rec, ok := agents[s.AgentName]; ok {
			return isFork(rec)
		}
	}
	return isFork(s.Rec)
}
