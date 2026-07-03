package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bmaltais/skillpack/internal/config"
)

// State is persisted to ~/.skillpack/state.json.
type State struct {
	Repos           map[string]RepoRecord                      `json:"repos"`
	InstalledSkills map[string]map[string]InstalledSkillRecord `json:"installed_skills"`
	// InstalledSkills key: skill address (e.g. "awesome-skills/coding/debugger")
	// inner key: agent name (e.g. "claude-code")
	InstalledPacks map[string]InstalledPackRecord `json:"installed_packs,omitempty"`
	// InstalledPacks key: pack address (e.g. "awesome-skills/packs/go-dev")
}

// InstalledPackRecord holds the state for one installed pack.
type InstalledPackRecord struct {
	PackAddress string                     `json:"pack_address"`
	InstalledAt time.Time                  `json:"installed_at"`
	Agents      []string                   `json:"agents"`
	Skills      map[string]PackSkillStatus `json:"skills"` // key: skill address
}

// PackSkillStatus describes whether a single skill in a pack was successfully installed.
type PackSkillStatus struct {
	Installed bool   `json:"installed"`
	Agent     string `json:"agent"`
	Error     string `json:"error,omitempty"`
}

// RepoRecord holds metadata for a registered skill repo.
type RepoRecord struct {
	URL         string    `json:"url"`
	CachePath   string    `json:"cache_path"`
	LastUpdated time.Time `json:"last_updated"`
}

// InstalledSkillRecord holds the install-time snapshot for one agent.
type InstalledSkillRecord struct {
	InstalledAtSHA string `json:"installed_at_sha"`
	InstalledHash  string `json:"installed_hash"` // SHA-256 of installed dir contents
	LocalPath      string `json:"local_path"`
	// UpstreamAddr is non-empty for forked skills. It holds the original skill
	// address before forking (e.g. "matt-pocock-skills/debugger").
	UpstreamAddr string `json:"upstream_addr,omitempty"`
	// UpstreamSHA is the upstream repo HEAD SHA at the moment the fork was cut.
	// Used as the three-way merge base when reconciling upstream changes.
	UpstreamSHA string `json:"upstream_sha,omitempty"`
}

// Load reads state from ~/.skillpack/state.json.
// Returns an empty State (no error) if the file does not exist yet.
func Load() (*State, error) {
	p, err := statePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return empty(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	if st.Repos == nil {
		st.Repos = make(map[string]RepoRecord)
	}
	if st.InstalledSkills == nil {
		st.InstalledSkills = make(map[string]map[string]InstalledSkillRecord)
	}
	if st.InstalledPacks == nil {
		st.InstalledPacks = make(map[string]InstalledPackRecord)
	}
	return &st, nil
}

// Save writes state to ~/.skillpack/state.json.
func Save(st *State) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating skillpack dir: %w", err)
	}
	p := filepath.Join(dir, "state.json")
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(p, data, 0600)
}

func statePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func empty() *State {
	return &State{
		Repos:           make(map[string]RepoRecord),
		InstalledSkills: make(map[string]map[string]InstalledSkillRecord),
		InstalledPacks:  make(map[string]InstalledPackRecord),
	}
}

// validateMutation checks that addr and agentName are non-empty.
func validateMutation(addr, agentName string) error {
	if addr == "" {
		return fmt.Errorf("addr must not be empty")
	}
	if agentName == "" {
		return fmt.Errorf("agentName must not be empty")
	}
	return nil
}

// RecordInstall stores or replaces the installation record for one agent.
// Creates the per-address inner map if it does not already exist.
func (st *State) RecordInstall(addr, agentName string, rec InstalledSkillRecord) error {
	if err := validateMutation(addr, agentName); err != nil {
		return err
	}
	if st.InstalledSkills[addr] == nil {
		st.InstalledSkills[addr] = make(map[string]InstalledSkillRecord)
	}
	st.InstalledSkills[addr][agentName] = rec
	return nil
}

// RecordRemove deletes the installation record for one agent.
// Removes the per-address inner map entirely when it becomes empty.
func (st *State) RecordRemove(addr, agentName string) error {
	if err := validateMutation(addr, agentName); err != nil {
		return err
	}
	delete(st.InstalledSkills[addr], agentName)
	if len(st.InstalledSkills[addr]) == 0 {
		delete(st.InstalledSkills, addr)
	}
	return nil
}

// RecordRemoveAll deletes all installation records for an address (all agents).
// No-op when no entry exists for addr.
func (st *State) RecordRemoveAll(addr string) error {
	if addr == "" {
		return fmt.Errorf("addr must not be empty")
	}
	delete(st.InstalledSkills, addr)
	return nil
}

// RecordHash updates InstalledHash for an existing installation record.
// Returns an error when no record exists for the given addr/agentName.
func (st *State) RecordHash(addr, agentName, hash string) error {
	if err := validateMutation(addr, agentName); err != nil {
		return err
	}
	rec, ok := st.InstalledSkills[addr][agentName]
	if !ok {
		return fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}
	rec.InstalledHash = hash
	st.InstalledSkills[addr][agentName] = rec
	return nil
}

// RecordSHA updates InstalledAtSHA for an existing installation record.
// Returns an error when no record exists for the given addr/agentName.
func (st *State) RecordSHA(addr, agentName, sha string) error {
	if err := validateMutation(addr, agentName); err != nil {
		return err
	}
	rec, ok := st.InstalledSkills[addr][agentName]
	if !ok {
		return fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}
	rec.InstalledAtSHA = sha
	st.InstalledSkills[addr][agentName] = rec
	return nil
}

// RecordRenameAddr moves all installed-skill entries from oldAddr to newAddr.
// Used when a repo is renamed. No-op when no entry exists for oldAddr.
func (st *State) RecordRenameAddr(oldAddr, newAddr string) error {
	if oldAddr == "" {
		return fmt.Errorf("oldAddr must not be empty")
	}
	if newAddr == "" {
		return fmt.Errorf("newAddr must not be empty")
	}
	agents, ok := st.InstalledSkills[oldAddr]
	if !ok {
		return nil
	}
	st.InstalledSkills[newAddr] = agents
	delete(st.InstalledSkills, oldAddr)
	return nil
}
