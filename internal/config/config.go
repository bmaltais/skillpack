package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is loaded from ~/.skillpack/config.yaml.
type Config struct {
	DefaultAgent string                 `yaml:"default_agent"`
	Agents       map[string]AgentConfig `yaml:"agents"`
	// Credentials maps repo name → personal access token for private HTTPS repos.
	// Stored in plain text; config.yaml is written with mode 0600.
	Credentials map[string]string `yaml:"credentials,omitempty"`
}

// TokenForRepo returns the best available token for repoName.
// Priority: config credentials → SKILLPACK_GIT_TOKEN → GITHUB_TOKEN → "".
func (c *Config) TokenForRepo(repoName string) string {
	if c.Credentials != nil {
		if t := c.Credentials[repoName]; t != "" {
			return t
		}
	}
	if t := os.Getenv("SKILLPACK_GIT_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GITHUB_TOKEN")
}

// AgentConfig holds the configuration for a single agent.
type AgentConfig struct {
	SkillDir string `yaml:"skill_dir"`
}

// KnownAgent is an agent bundled with the binary for auto-detection.
type KnownAgent struct {
	Name      string
	SkillDir  string
	DetectDir string // if set, check this dir for presence instead of SkillDir
}

var DefaultAgents = []KnownAgent{
	{"claude-code", "~/.claude/skills", ""},
	{"copilot", "~/.copilot/skills", ""},
	{"grok", "~/.grok/skills", ""},
	{"hermes", "~/.hermes/skills", ""},
	{"omp", "~/.omp/agent/skills", ""},
	{"opencode", "~/.config/opencode/skills", ""},
	{"openclaw", "~/.openclaw/skills", ""},
	{"pi", "~/.pi/agent/skills", "~/.pi/agent"},
}

// KnownAgentByName returns the bundled agent with name.
func KnownAgentByName(name string) (KnownAgent, bool) {
	for _, agent := range DefaultAgents {
		if agent.Name == name {
			return agent, true
		}
	}
	return KnownAgent{}, false
}

// Dir returns the path to the ~/.skillpack directory.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".skillpack"), nil
}

// ReposDir returns the path to the repos cache directory.
func ReposDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "repos"), nil
}

// Path returns the path to the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads the config from ~/.skillpack/config.yaml.
// Returns an empty Config (no error) if the file does not exist yet.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := &Config{Agents: make(map[string]AgentConfig)}
		if DetectAgents(cfg) {
			_ = Save(cfg) // best-effort persist
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentConfig)
	}
	if cfg.Credentials == nil {
		cfg.Credentials = make(map[string]string)
	}

	// Auto-detect agents on every load
	if DetectAgents(&cfg) {
		_ = Save(&cfg) // best-effort save
	}

	return &cfg, nil
}

// Save writes the config to ~/.skillpack/config.yaml.
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating skillpack dir: %w", err)
	}
	path := filepath.Join(dir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// DetectAgents scans the system for known agents and adds any newly found
// agents to the config. Returns true if the config was modified.
func DetectAgents(cfg *Config) bool {
	modified := false

	// Add newly detected agents
	for _, agent := range DefaultAgents {
		if _, exists := cfg.Agents[agent.Name]; exists {
			continue
		}
		if agentDetected(agent) {
			cfg.Agents[agent.Name] = AgentConfig{SkillDir: agent.SkillDir}
			modified = true
		}
	}

	return modified
}

// agentDetected reports whether ka's skill directory (or DetectDir, if set)
// exists on disk.
func agentDetected(ka KnownAgent) bool {
	checkDir := ka.SkillDir
	if ka.DetectDir != "" {
		checkDir = ka.DetectDir
	}
	expanded, err := ExpandPath(checkDir)
	if err != nil {
		return false
	}
	info, err := os.Stat(expanded)
	return err == nil && info.IsDir()
}

// AgentCandidate is a bundled known agent (see DefaultAgents) not yet present
// in Config.Agents, annotated with whether it was found installed on disk.
type AgentCandidate struct {
	KnownAgent
	Detected bool
}

// UnconfiguredAgents returns bundled known agents that aren't yet registered
// in cfg.Agents, each flagged with whether it was detected on disk. Used to
// offer known agents the user hasn't added yet — via the CLI (`agent add`
// with no arguments, `agent list`) and the TUI's Add Agent dialog — for
// agents auto-detection missed (e.g. the skill directory didn't exist yet
// when config was last loaded).
func UnconfiguredAgents(cfg *Config) []AgentCandidate {
	var out []AgentCandidate
	for _, ka := range DefaultAgents {
		if _, exists := cfg.Agents[ka.Name]; exists {
			continue
		}
		out = append(out, AgentCandidate{KnownAgent: ka, Detected: agentDetected(ka)})
	}
	return out
}

// ValidateAgentName normalizes and validates a new agent name.
func ValidateAgentName(cfg *Config, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("agent name cannot be empty")
	}
	if _, exists := cfg.Agents[name]; exists {
		return "", fmt.Errorf("agent %q is already configured", name)
	}
	return name, nil
}

// AddAgent registers a new agent in cfg.Agents and persists the config.
// Returns an error if name or skillDir is empty, or if name is already
// configured.
func AddAgent(cfg *Config, name, skillDir string) error {
	name, err := ValidateAgentName(cfg, name)
	if err != nil {
		return err
	}
	skillDir = strings.TrimSpace(skillDir)
	if skillDir == "" {
		return fmt.Errorf("skill directory cannot be empty")
	}

	agents := make(map[string]AgentConfig, len(cfg.Agents)+1)
	for name, agent := range cfg.Agents {
		agents[name] = agent
	}
	agents[name] = AgentConfig{SkillDir: skillDir}
	next := *cfg
	next.Agents = agents
	if err := Save(&next); err != nil {
		return err
	}
	cfg.Agents = agents
	return nil
}

// ExpandPath expands a path starting with ~/ using os.UserHomeDir.
// All paths stored in config use ~/ and must be expanded before use.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}
