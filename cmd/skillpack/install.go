package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
)

var installCmd = &cobra.Command{
	Use:   "install <repo>/<path/to/skill>",
	Short: "Install a skill into an agent's skill directory",
	Example: `  skillpack install my-repo/coding/debugger
  skillpack install my-repo/coding/debugger --agent claude-code
  skillpack install my-repo/coding/debugger --all-agents
  skillpack install my-repo/coding/debugger --skip-existing`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		agentName, _ := cmd.Flags().GetString("agent")
		allAgents, _ := cmd.Flags().GetBool("all-agents")
		skipExisting, _ := cmd.Flags().GetBool("skip-existing")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		targets, err := resolveAgents(agentName, allAgents, app.Cfg)
		if err != nil {
			return err
		}

		for _, target := range targets {
			fmt.Printf("Installing %s for %s...\n", addr, target)
			if err := skill.Install(addr, target, app.Cfg, app.St, skipExisting); err != nil {
				return err
			}
			fmt.Printf("  installed\n")
		}
		return nil
	},
}

func init() {
	installCmd.Flags().String("agent", "", "Target agent (default: configured default_agent)")
	installCmd.Flags().Bool("all-agents", false, "Install for all configured agents")
	installCmd.Flags().Bool("skip-existing", false, "No-op if the skill is already installed")
}
