package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
)

var relinkCmd = &cobra.Command{
	Use:   "relink <addr> [<new-addr>]",
	Short: "Repair a stale installed skill by pointing it at a new upstream address",
	Long: `Repair an installed skill whose upstream path no longer exists.

When a skill is moved or renamed upstream, its installed mapping becomes stale
and skillpack sync reports it as a stale address. Instead of removing and
reinstalling, relink re-points the installed skill at a valid replacement
address: the installed copy is refreshed to the replacement's contents and the
install state is updated so a subsequent sync completes cleanly.

To repair a fork's broken upstream tracking pointer without replacing installed
files, use --set-upstream or --clear-upstream instead of a positional new-addr.`,
	Example: `  skillpack relink old-repo/coding/debugger new-repo/skills/debugger
  skillpack relink old-repo/coding/debugger new-repo/skills/debugger --agent claude-code
  skillpack relink old-repo/coding/debugger new-repo/skills/debugger --all-agents
  skillpack relink my-skills/debugger --set-upstream upstream-repo/debugger
  skillpack relink my-skills/debugger --clear-upstream`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		agentName, _ := cmd.Flags().GetString("agent")
		allAgents, _ := cmd.Flags().GetBool("all-agents")
		force, _ := cmd.Flags().GetBool("force")
		setUpstream, _ := cmd.Flags().GetString("set-upstream")
		clearUpstream, _ := cmd.Flags().GetBool("clear-upstream")

		// Validate flag combinations and argument requirements.
		if err := validateRelinkFlags(len(args), setUpstream, clearUpstream); err != nil {
			return err
		}

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		// Upstream repair path: --set-upstream or --clear-upstream.
		if setUpstream != "" || clearUpstream {
			newUpstream := setUpstream // empty string = clear-upstream
			targets, err := resolveAgents(agentName, false, app.Cfg)
			if err != nil {
				return err
			}
			target := targets[0]
			upstreamLabel := newUpstream
			if upstreamLabel == "" {
				upstreamLabel = "(cleared)"
			}
			fmt.Printf("Updating upstream tracking for %s [%s] → %s...\n", addr, target, upstreamLabel)
			if err := skill.RelinkUpstream(addr, newUpstream, target, app.St); err != nil {
				return err
			}
			fmt.Printf("  done\n")
			return nil
		}

		// Stale-address repair path: requires exactly two positional args.
		if len(args) < 2 {
			return fmt.Errorf("relink requires a <new-addr> argument (or use --set-upstream / --clear-upstream to repair upstream tracking)")
		}
		newAddr := args[1]

		targets, err := resolveInstalledTargets(addr, agentName, allAgents, app)
		if err != nil {
			return err
		}

		for _, target := range targets {
			fmt.Printf("Relinking %s → %s for %s...\n", addr, newAddr, target)
			if err := skill.Relink(addr, newAddr, target, force, app.St); err != nil {
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
	relinkCmd.Flags().String("set-upstream", "", "Set the fork's upstream tracking address to this skill addr")
	relinkCmd.Flags().Bool("clear-upstream", false, "Clear the fork's upstream tracking, converting it to a plain installed skill")
}

// validateRelinkFlags returns an error if the --set-upstream / --clear-upstream
// flags are used in a mutually-exclusive or incompatible way.
func validateRelinkFlags(positionalCount int, setUpstream string, clearUpstream bool) error {
	if setUpstream != "" && clearUpstream {
		return fmt.Errorf("--set-upstream and --clear-upstream are mutually exclusive")
	}
	if positionalCount == 2 && (setUpstream != "" || clearUpstream) {
		return fmt.Errorf("positional <new-addr> cannot be combined with --set-upstream or --clear-upstream")
	}
	return nil
}
