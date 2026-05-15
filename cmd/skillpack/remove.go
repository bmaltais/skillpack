package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bernard/skillpack/internal/config"
	"github.com/bernard/skillpack/internal/skill"
	"github.com/bernard/skillpack/internal/state"
)

var removeCmd = &cobra.Command{
	Use:   "remove <repo>/<path/to/skill>",
	Short: "Remove an installed skill from an agent's skill directory",
	Example: `  skillpack remove my-repo/coding/debugger
  skillpack remove my-repo/coding/debugger --agent claude-code
  skillpack remove my-repo/coding/debugger --all-agents
  skillpack remove my-repo/coding/debugger --force`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		agentName, _ := cmd.Flags().GetString("agent")
		allAgents, _ := cmd.Flags().GetBool("all-agents")
		force, _ := cmd.Flags().GetBool("force")

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		st, err := state.Load()
		if err != nil {
			return err
		}

		var targets []string
		if allAgents {
			if agents, ok := st.InstalledSkills[addr]; ok {
				for name := range agents {
					targets = append(targets, name)
				}
			}
			if len(targets) == 0 {
				return fmt.Errorf("skill %q is not installed for any agent", addr)
			}
		} else {
			var err error
			targets, err = resolveAgents(agentName, false, cfg)
			if err != nil {
				return err
			}
		}

		for _, target := range targets {
			fmt.Printf("Removing %s from %s...\n", addr, target)
			if err := skill.Remove(addr, target, cfg, st, force); err != nil {
				return err
			}
			fmt.Printf("  removed\n")
		}
		return state.Save(st)
	},
}

func init() {
	removeCmd.Flags().String("agent", "", "Target agent (default: configured default_agent)")
	removeCmd.Flags().Bool("all-agents", false, "Remove from all agents it is installed for")
	removeCmd.Flags().Bool("force", false, "Remove even if the skill has local modifications")
}
