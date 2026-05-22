package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
)

func parseForkMode(s string) (skill.ForkMode, error) {
	switch s {
	case "":
		return skill.ForkModeAuto, nil
	case "auto":
		return skill.ForkModeAuto, nil
	case "override":
		return skill.ForkModeOverride, nil
	case "register":
		return skill.ForkModeRegister, nil
	default:
		return 0, fmt.Errorf("invalid fork mode %q: must be auto, override, or register", s)
	}
}

var forkCmd = &cobra.Command{
	Use:   "fork <addr> <my-repo>",
	Short: "Fork an installed skill into your own repo",
	Long: `Copy an installed skill into a repo you own, recording the upstream origin
so future updates can detect and merge upstream changes.

<addr>    — address of the installed skill to fork (e.g. matt-skills/debugger)
<my-repo> — name of your writable repo (must be registered via: skillpack repo add)

After forking:
  • The skill is tracked under <my-repo>/<skill-name> in state.
  • skillpack list shows the new fork address.
  • skillpack update detects upstream origin changes as conflicts.
  • skillpack update --merge resolves upstream changes via three-way merge.
  • skillpack update --merge --llm [<agent>] delegates conflict resolution to an LLM.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		forkRepo := args[1]
		agentName, _ := cmd.Flags().GetString("agent")
		forkModeStr, _ := cmd.Flags().GetString("fork-mode")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}
		if agentName == "" {
			agentName = app.Cfg.DefaultAgent
		}

		token := app.Cfg.TokenForRepo(forkRepo)

		mode, err := parseForkMode(forkModeStr)
		if err != nil {
			return err
		}

		is, err := skill.Open(addr, agentName, app.Cfg, app.St)
		if err != nil {
			return err
		}
		newAddr, err := is.Fork(forkRepo, token, mode)
		if err != nil {
			return err
		}

		fmt.Printf("  Forked: %s → %s (%s)\n", addr, newAddr, agentName)
		fmt.Printf("  Upstream origin recorded. Use `skillpack update %s` to check for upstream changes.\n", newAddr)
		return nil
	},
}

func init() {
	forkCmd.Flags().String("agent", "", "Agent whose installed copy to fork (default: default agent)")
	forkCmd.Flags().String("fork-mode", "auto", "How to handle unknown provenance: auto, override, or register")
}
