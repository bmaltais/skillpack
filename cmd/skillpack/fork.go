package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

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

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if agentName == "" {
			agentName = cfg.DefaultAgent
		}

		token := cfg.TokenForRepo(forkRepo)

		st, err := state.Load()
		if err != nil {
			return err
		}

		newAddr, err := skill.Fork(addr, forkRepo, agentName, token, st)
		if err != nil {
			return err
		}

		if err := state.Save(st); err != nil {
			return err
		}

		fmt.Printf("  Forked: %s → %s (%s)\n", addr, newAddr, agentName)
		fmt.Printf("  Upstream origin recorded. Use `skillpack update %s` to check for upstream changes.\n", newAddr)
		return nil
	},
}

func init() {
	forkCmd.Flags().String("agent", "", "Agent whose installed copy to fork (default: default agent)")
}
