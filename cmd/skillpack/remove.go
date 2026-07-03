package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
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

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		targets, err := resolveInstalledTargets(addr, agentName, allAgents, app)
		if err != nil {
			return err
		}

		// Identify owning packs before removing (InstalledPacks is unaffected by Remove).
		owningPacks := app.St.FindPacksOwningSkill(addr)

		packMarked := false
		for _, target := range targets {
			fmt.Printf("Removing %s from %s...\n", addr, target)
			is, err := skill.Open(addr, target, app.Cfg, app.St)
			if err != nil {
				return err
			}
			if err := is.Remove(force); err != nil {
				return err
			}
			fmt.Printf("  removed\n")

			// Mark owning packs as partial.
			for _, packAddr := range owningPacks {
				app.St.MarkPackSkillMissing(packAddr, addr, target, "directly removed by user")
				fmt.Printf("  %s This skill is part of pack %s — the pack is now %s.\n",
					yellow("notice:"), bold(packAddr), yellow("partially deployed"))
				packMarked = true
			}
		}

		// Persist pack partial marks (is.Remove already called state.Save for skill removal).
		if packMarked {
			if err := state.Save(app.St); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	removeCmd.Flags().String("agent", "", "Target agent (default: configured default_agent)")
	removeCmd.Flags().Bool("all-agents", false, "Remove from all agents it is installed for")
	removeCmd.Flags().Bool("force", false, "Remove even if the skill has local modifications")
}
