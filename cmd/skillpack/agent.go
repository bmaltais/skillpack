package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage configured agents",
}

var agentAddCmd = &cobra.Command{
	Use:   "add [name] [skill-dir]",
	Short: "Register a new agent; with no arguments, lists known agents available to add",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}
		if len(args) == 0 {
			printAgentSuggestions(app.Cfg)
			return nil
		}

		name := args[0]
		skillDir := ""
		if len(args) == 2 {
			skillDir = args[1]
		} else {
			for _, ka := range config.DefaultAgents {
				if ka.Name == name {
					skillDir = ka.SkillDir
					break
				}
			}
			if skillDir == "" {
				return fmt.Errorf("%q is not a known agent — specify a skill directory: skillpack agent add %s <skill-dir>", name, name)
			}
		}

		if err := config.AddAgent(app.Cfg, name, skillDir); err != nil {
			return err
		}
		fmt.Printf("Added agent %q → %s\n", name, skillDir)
		return nil
	},
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}
		if len(app.Cfg.Agents) == 0 {
			fmt.Println("No agents configured. Run: skillpack agent add <name> <skill-dir>")
		} else {
			names := make([]string, 0, len(app.Cfg.Agents))
			for n := range app.Cfg.Agents {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				marker := "  "
				if n == app.Cfg.DefaultAgent {
					marker = "* "
				}
				fmt.Printf("%s%-20s %s\n", marker, n, app.Cfg.Agents[n].SkillDir)
			}
		}

		var detected []config.AgentCandidate
		for _, c := range config.UnconfiguredAgents(app.Cfg) {
			if c.Detected {
				detected = append(detected, c)
			}
		}
		if len(detected) > 0 {
			fmt.Println("\nDetected but not added:")
			for _, c := range detected {
				fmt.Printf("  %-20s %s  (skillpack agent add %s)\n", c.Name, c.SkillDir, c.Name)
			}
		}
		return nil
	},
}

func init() {
	agentCmd.AddCommand(agentAddCmd, agentListCmd)
}

// printAgentSuggestions lists every bundled known agent not yet configured,
// flagging which were found installed on disk.
func printAgentSuggestions(cfg *config.Config) {
	candidates := config.UnconfiguredAgents(cfg)
	if len(candidates) == 0 {
		fmt.Println("All known agents are already configured.")
		fmt.Println("To add a custom agent: skillpack agent add <name> <skill-dir>")
		return
	}
	fmt.Println("Known agents available to add:")
	for _, c := range candidates {
		status := ""
		if c.Detected {
			status = "  (detected on disk)"
		}
		fmt.Printf("  %-20s %s%s\n", c.Name, c.SkillDir, status)
	}
	fmt.Println("\nAdd one with: skillpack agent add <name>")
	fmt.Println("Or a custom agent: skillpack agent add <name> <skill-dir>")
}
