package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
)

var publishCmd = &cobra.Command{
	Use:   "publish (<repo>/<path/to/skill> | <local-dir> --repo <name>)",
	Short: "Push local skill edits to a remote repo",
	Long: `Push local edits back to the remote skill repo.

Two modes:

  skillpack publish <repo>/<path/to/skill>
    Publish an installed skill's local edits. The local copy always wins.
    Use --agent to select which agent's copy to publish (default: default agent).

  skillpack publish ./my-new-skill --repo <name>
    Add a brand-new local directory as a skill in the named repo.
    The directory must contain a SKILL.md file.
    After publishing, install it with: skillpack install <repo>/my-new-skill`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName, _ := cmd.Flags().GetString("agent")
		repoFlag, _ := cmd.Flags().GetString("repo")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		// New-skill mode: --repo flag provided
		if repoFlag != "" {
			localDir := args[0]
			if dryRun {
				fmt.Printf("  [dry-run] would add %q to repo %q\n", localDir, repoFlag)
				return nil
			}
			addr, err := skill.PublishNew(localDir, repoFlag, app.Cfg.TokenForRepo(repoFlag), app.St)
			if err != nil {
				return err
			}
			fmt.Printf("  Published: %s\n", addr)
			fmt.Printf("  Install with: skillpack install %s\n", addr)
			return nil
		}

		// Existing-skill mode: publish installed edits for an address
		addr := args[0]
		if agentName == "" {
			agentName = app.Cfg.DefaultAgent
		}
		repoName := strings.SplitN(addr, "/", 2)[0]

		// Verify it's installed
		if agents, ok := app.St.InstalledSkills[addr]; !ok {
			return fmt.Errorf("skill %q is not installed", addr)
		} else if _, ok := agents[agentName]; !ok {
			return fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
		}

		if dryRun {
			modified, err := skill.IsModified(app.St.InstalledSkills[addr][agentName])
			if err != nil {
				return err
			}
			if modified {
				fmt.Printf("  [dry-run] would publish %s (%s) — has local edits\n", addr, agentName)
			} else {
				fmt.Printf("  [dry-run] %s (%s) — no local edits to publish\n", addr, agentName)
			}
			return nil
		}

		if err := skill.Publish(addr, agentName, app.Cfg.TokenForRepo(repoName), app.St); err != nil {
			return err
		}
		fmt.Printf("  Published: %s (%s)\n", addr, agentName)
		return nil
	},
}

func init() {
	publishCmd.Flags().String("agent", "", "Agent whose copy to publish (default: config default agent)")
	publishCmd.Flags().String("repo", "", "Repo name for publishing a new local skill directory")
	publishCmd.Flags().Bool("dry-run", false, "Show what would be published without making changes")
}
