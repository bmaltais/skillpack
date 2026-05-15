package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage skill repositories",
}

var repoAddCmd = &cobra.Command{
	Use:   "add <url> [--name <name>] [--token <pat>]",
	Short: "Clone and register a skill repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		name, _ := cmd.Flags().GetString("name")
		token, _ := cmd.Flags().GetString("token")
		if name == "" {
			name = repo.NameFromURL(url)
		}

		st, err := state.Load()
		if err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// Check for name/URL collision BEFORE touching credentials.
		if rec, exists := st.Repos[name]; exists {
			if rec.URL != url {
				return fmt.Errorf("repo name %q is already used for %s — use --name to pick a different name", name, rec.URL)
			}
			// Same URL: just update the token (no re-clone needed).
			if token != "" {
				cfg.Credentials[name] = token
				if err := config.Save(cfg); err != nil {
					return err
				}
			}
			skills, _ := repo.DiscoverSkills(name, st)
			fmt.Printf("Repo %q already registered — token updated. %d skill(s) available.\n", name, len(skills))
			return nil
		}

		// Save token AFTER collision check passes.
		if token != "" {
			cfg.Credentials[name] = token
			if err := config.Save(cfg); err != nil {
				return err
			}
		}

		fmt.Printf("Cloning %s as %q...\n", url, name)
		if err := repo.Add(name, url, cfg.TokenForRepo(name), st); err != nil {
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
		cfg, err := config.Load()
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
			if err := repo.Update(name, cfg.TokenForRepo(name), st); err != nil {
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
	repoAddCmd.Flags().String("token", "", "Personal access token for private HTTPS repos (saved to config)")
	repoCmd.AddCommand(repoAddCmd, repoListCmd, repoRemoveCmd, repoUpdateCmd, repoRenameCmd)
}

var repoRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a registered repository",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName, newName := args[0], args[1]

		st, err := state.Load()
		if err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		rec, ok := st.Repos[oldName]
		if !ok {
			return fmt.Errorf("repo %q not found", oldName)
		}
		if _, exists := st.Repos[newName]; exists {
			return fmt.Errorf("repo %q already exists", newName)
		}

		// Compute the new cache path.
		newCachePath, err := repo.NewCachePath(newName)
		if err != nil {
			return err
		}

		// Update in-memory state before touching disk so that if a save
		// fails nothing on disk has changed yet.
		rec.CachePath = newCachePath
		delete(st.Repos, oldName)
		st.Repos[newName] = rec

		// Rekey installed skills. Collect first, then apply to avoid
		// mutating the map while ranging over it.
		type rekey struct{ oldAddr, newAddr string }
		prefix := oldName + "/"
		var rekeys []rekey
		for addr := range st.InstalledSkills {
			if strings.HasPrefix(addr, prefix) {
				rekeys = append(rekeys, rekey{addr, newName + "/" + addr[len(prefix):]})
			}
		}
		for _, rk := range rekeys {
			st.InstalledSkills[rk.newAddr] = st.InstalledSkills[rk.oldAddr]
			delete(st.InstalledSkills, rk.oldAddr)
		}

		// Save state and config before renaming the directory.
		// If either save fails, the disk is unchanged and the user can retry.
		if err := state.Save(st); err != nil {
			return err
		}
		if token, ok := cfg.Credentials[oldName]; ok {
			cfg.Credentials[newName] = token
			delete(cfg.Credentials, oldName)
			if err := config.Save(cfg); err != nil {
				return err
			}
		}

		// Rename the cache directory last. If this fails, report the manual
		// fix command so the user can recover without re-cloning.
		if err := repo.RenameCache(oldName, newName); err != nil {
			return fmt.Errorf("%w\n  state updated; fix manually with: mv ~/.skillpack/repos/%s ~/.skillpack/repos/%s", err, oldName, newName)
		}

		fmt.Printf("Renamed %q → %q\n", oldName, newName)
		return nil
	},
}
