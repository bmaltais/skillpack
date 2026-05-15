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
}

// AgentConfig holds the configuration for a single agent.
type AgentConfig struct {
	SkillDir string `yaml:"skill_dir"`
}

// KnownAgent is an agent bundled with the binary for auto-detection.
type KnownAgent struct {
	Name     string
	SkillDir string
}

// DefaultAgents is the list of known agents the first-run wizard checks for.
var DefaultAgents = []KnownAgent{
	{"claude-code", "~/.claude/skills"},
	{"copilot", "~/.copilot/skills"},
	{"hermes", "~/.hermes/skills"},
	{"opencode", "~/.config/opencode/skills"},
	{"openclaw", "~/.openclaw/skills"},
	{"pi", "~/.pi/agent/skills"},
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
		return &Config{Agents: make(map[string]AgentConfig)}, nil
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
