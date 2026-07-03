package main

import (
	"fmt"
	"path"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Manage skill packs",
}

var packListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed packs (or browse available packs with --available)",
	Example: `  skillpack pack list
  skillpack pack list --available
  skillpack pack list --available --repo my-repo`,
	RunE: func(cmd *cobra.Command, args []string) error {
		available, _ := cmd.Flags().GetBool("available")
		repoFilter, _ := cmd.Flags().GetString("repo")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		if available {
			return packListAvailable(repoFilter, app.St)
		}
		return packListInstalled(app.St)
	},
}

// packListInstalled prints all installed packs with their completion status.
func packListInstalled(st *state.State) error {
	if len(st.InstalledPacks) == 0 {
		fmt.Println("No packs installed. Run: skillpack pack install <repo>/<path/to/pack>")
		return nil
	}

	addrs := make([]string, 0, len(st.InstalledPacks))
	for addr := range st.InstalledPacks {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	for _, addr := range addrs {
		rec := st.InstalledPacks[addr]

		partial := false
		for _, s := range rec.Skills {
			if !s.Installed {
				partial = true
				break
			}
		}
		status := green("complete")
		if partial {
			status = yellow("partial")
		}

		agents := ""
		for i, a := range rec.Agents {
			if i > 0 {
				agents += ", "
			}
			agents += a
		}
		fmt.Printf("%-48s  [%s]  agents: %s\n", addr, status, agents)
	}
	return nil
}

// packListAvailable lists all packs discoverable from registered repos.
func packListAvailable(repoFilter string, st *state.State) error {
	var packs []repo.PackInfo
	var err error

	if repoFilter != "" {
		packs, err = repo.DiscoverPacks(repoFilter, st)
	} else {
		packs, err = repo.DiscoverAllPacks(st)
	}
	if err != nil {
		return err
	}

	if len(packs) == 0 {
		fmt.Println("No packs found. Register a repo with: skillpack repo add <name> <url>")
		return nil
	}

	sort.Slice(packs, func(i, j int) bool { return packs[i].Address < packs[j].Address })

	// Group packs by their parent path (repo + category prefix).
	type group struct {
		prefix string
		items  []repo.PackInfo
	}
	var groups []group
	curPrefix := ""
	for _, p := range packs {
		prefix := path.Dir(p.Address) // e.g. "my-repo/packs"
		if prefix != curPrefix {
			groups = append(groups, group{prefix: prefix})
			curPrefix = prefix
		}
		groups[len(groups)-1].items = append(groups[len(groups)-1].items, p)
	}

	total := 0
	for _, g := range groups {
		fmt.Printf("%s/\n", bold(g.prefix))
		for _, p := range g.items {
			name := path.Base(p.Address)
			installed := ""
			if _, ok := st.InstalledPacks[p.Address]; ok {
				installed = "  " + green("[installed]")
			}
			fmt.Printf("  %-38s%s\n", name, installed)
			total++
		}
	}
	fmt.Printf("\n%s available\n", bold(fmt.Sprintf("%d pack(s)", total)))
	return nil
}

func init() {
	packListCmd.Flags().Bool("available", false, "Browse packs available in registered repos")
	packListCmd.Flags().String("repo", "", "Filter available packs by repo name (used with --available)")
	packCmd.AddCommand(packListCmd)
}
