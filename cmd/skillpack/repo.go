package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bernard/skillpack/internal/repo"
	"github.com/bernard/skillpack/internal/state"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage skill repositories",
}

var repoAddCmd = &cobra.Command{
	Use:   "add <url> [--name <name>]",
	Short: "Clone and register a skill repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name = repo.NameFromURL(url)
		}

		st, err := state.Load()
		if err != nil {
			return err
		}

		fmt.Printf("Cloning %s as %q...\n", url, name)
		if err := repo.Add(name, url, st); err != nil {
			return err
		}
		if err := state.Save(st); err != nil {
			return err
		}

		skills, _ := repo.DiscoverSkills(name, st)
		fmt.Printf("Repo %q registered — %d skill(s) available.\n", name, len(skills))
		return nil
	},
}

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered skill repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := state.Load()
		if err != nil {
			return err
		}
		if len(st.Repos) == 0 {
			fmt.Println("No repositories registered. Run: skillpack repo add <url>")
			return nil
		}
		for name, rec := range st.Repos {
			skills, _ := repo.DiscoverSkills(name, st)
			fmt.Printf("%-24s  %-50s  %d skill(s)\n", name, rec.URL, len(skills))
		}
		return nil
	},
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Unregister a skill repository (local clone is kept)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		st, err := state.Load()
		if err != nil {
			return err
		}
		if err := repo.Remove(name, st); err != nil {
			return err
		}
		if err := state.Save(st); err != nil {
			return err
		}
		fmt.Printf("Repo %q unregistered (local clone kept at ~/.skillpack/repos/%s)\n", name, name)
		return nil
	},
}

var repoUpdateCmd = &cobra.Command{
	Use:   "update [<name>]",
	Short: "Pull latest changes for one or all registered repos",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := state.Load()
		if err != nil {
			return err
		}

		names := args
		if len(names) == 0 {
			for name := range st.Repos {
				names = append(names, name)
			}
		}

		var errs []string
		for _, name := range names {
			fmt.Printf("Updating %s...\n", name)
			if err := repo.Update(name, st); err != nil {
				errs = append(errs, fmt.Sprintf("  %s: %v", name, err))
			}
		}
		if err := state.Save(st); err != nil {
			return err
		}
		if len(errs) > 0 {
			return fmt.Errorf("some repos failed to update:\n%s", strings.Join(errs, "\n"))
		}
		return nil
	},
}

func init() {
	repoAddCmd.Flags().String("name", "", "Name for the repository (default: inferred from URL)")
	repoCmd.AddCommand(repoAddCmd, repoListCmd, repoRemoveCmd, repoUpdateCmd)
}
