package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
)

var relinkCmd = &cobra.Command{
	Use:   "relink <stale-addr> <new-addr>",
	Short: "Repair a stale installed skill by pointing it at a new upstream address",
	Long: `Repair an installed skill whose upstream path no longer exists.

When a skill is moved or renamed upstream, its installed mapping becomes stale
and skillpack sync reports it as a stale address. Instead of removing and
reinstalling, relink re-points the installed skill at a valid replacement
address: the installed copy is refreshed to the replacement's contents and the
install state is updated so a subsequent sync completes cleanly.`,
	Example: `  skillpack relink old-repo/coding/debugger new-repo/skills/debugger
  skillpack relink old-repo/coding/debugger new-repo/skills/debugger --agent claude-code
  skillpack relink old-repo/coding/debugger new-repo/skills/debugger --all-agents`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldAddr, newAddr := args[0], args[1]
		agentName, _ := cmd.Flags().GetString("agent")
		allAgents, _ := cmd.Flags().GetBool("all-agents")
		force, _ := cmd.Flags().GetBool("force")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		targets, err := resolveInstalledTargets(oldAddr, agentName, allAgents, app)
		if err != nil {
			return err
		}

		for _, target := range targets {
			fmt.Printf("Relinking %s → %s for %s...\n", oldAddr, newAddr, target)
			if err := skill.Relink(oldAddr, newAddr, target, force, app.St); err != nil {
				return err
			}
			fmt.Printf("  relinked\n")
		}
		return nil
	},
}

func init() {
	relinkCmd.Flags().String("agent", "", "Target agent (default: configured default_agent)")
	relinkCmd.Flags().Bool("all-agents", false, "Relink for all agents the stale skill is installed for")
	relinkCmd.Flags().Bool("force", false, "Relink even if the installed skill has local modifications")
}
