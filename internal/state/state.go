package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bernard/skillpack/internal/config"
)

// State is persisted to ~/.skillpack/state.json.
type State struct {
	Repos           map[string]RepoRecord                      `json:"repos"`
	InstalledSkills map[string]map[string]InstalledSkillRecord `json:"installed_skills"`
	// InstalledSkills key: skill address (e.g. "awesome-skills/coding/debugger")
	// inner key: agent name (e.g. "claude-code")
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
	}
}
