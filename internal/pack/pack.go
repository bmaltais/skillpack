package pack

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RepoRef describes a repo referenced by a pack.
type RepoRef struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Pack holds the parsed contents of a pack.yaml file.
type Pack struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Repos       []RepoRef `yaml:"repos"`
	Skills      []string  `yaml:"skills"`
}

// ParseFile reads and parses a pack.yaml from the given file path.
func ParseFile(path string) (*Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pack.yaml: %w", err)
	}
	return Parse(data)
}

// Parse decodes pack.yaml content from raw bytes and validates it.
func Parse(data []byte) (*Pack, error) {
	var p Pack
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing pack.yaml: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks that all required fields are present.
func (p *Pack) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("pack.yaml: name is required")
	}
	if len(p.Repos) == 0 {
		return fmt.Errorf("pack.yaml: at least one repo is required")
	}
	for i, r := range p.Repos {
		if r.Name == "" {
			return fmt.Errorf("pack.yaml: repos[%d].name is required", i)
		}
		if r.URL == "" {
			return fmt.Errorf("pack.yaml: repos[%d].url is required", i)
		}
	}
	if len(p.Skills) == 0 {
		return fmt.Errorf("pack.yaml: at least one skill is required")
	}
	return nil
}
