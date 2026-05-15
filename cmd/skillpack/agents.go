package main

import (
	"fmt"

	"github.com/bernard/skillpack/internal/config"
)

// resolveAgents returns the list of agent names targeted by a command.
// If agentName is set, that single agent is used.
// If allAgents is true, all configured agents are returned.
// Otherwise the configured default agent is used.
func resolveAgents(agentName string, allAgents bool, cfg *config.Config) ([]string, error) {
	if allAgents {
		names := make([]string, 0, len(cfg.Agents))
		for name := range cfg.Agents {
			names = append(names, name)
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no agents configured; add one to ~/.skillpack/config.yaml")
		}
		return names, nil
	}
	if agentName == "" {
		agentName = cfg.DefaultAgent
	}
	if agentName == "" {
		return nil, fmt.Errorf("no default agent configured; use --agent <name> or set default_agent in ~/.skillpack/config.yaml")
	}
	if _, ok := cfg.Agents[agentName]; !ok {
		return nil, fmt.Errorf("agent %q not found in config", agentName)
	}
	return []string{agentName}, nil
}
