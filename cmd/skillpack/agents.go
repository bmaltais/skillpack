package main

import (
	"fmt"
	"sort"

	"github.com/bmaltais/skillpack/internal/config"
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

// resolveInstalledTargets returns the agent names a command should act on for an
// already-installed skill. When allAgents is true it returns every agent the
// skill is installed for (erroring if none); otherwise it falls back to
// resolveAgents (the --agent flag or the configured default).
func resolveInstalledTargets(addr, agentName string, allAgents bool, app *App) ([]string, error) {
	if allAgents {
		var targets []string
		if agents, ok := app.St.InstalledSkills[addr]; ok {
			for name := range agents {
				targets = append(targets, name)
			}
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("skill %q is not installed for any agent", addr)
		}
		sort.Strings(targets)
		return targets, nil
	}
	return resolveAgents(agentName, false, app.Cfg)
}
