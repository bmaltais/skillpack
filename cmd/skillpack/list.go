package main

import (
	"fmt"
	"path"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills (or browse available skills with --available)",
	Example: `  skillpack list
  skillpack list --agent claude-code
  skillpack list --modified
  skillpack list --available
  skillpack list --available --repo my-repo`,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentFilter, _ := cmd.Flags().GetString("agent")
		modifiedOnly, _ := cmd.Flags().GetBool("modified")
		available, _ := cmd.Flags().GetBool("available")
		repoFilter, _ := cmd.Flags().GetString("repo")

		st, err := state.Load()
		if err != nil {
			return err
		}

		// --available: browse skills in registered repos
		if available {
			return listAvailable(repoFilter, st)
		}

		// Default: list installed skills
		if len(st.InstalledSkills) == 0 {
			fmt.Println("No skills installed. Run: skillpack install <repo>/<skill>")
			return nil
		}

		// Collect and sort addresses for stable output
		addrs := make([]string, 0, len(st.InstalledSkills))
		for addr := range st.InstalledSkills {
			addrs = append(addrs, addr)
		}
		sort.Strings(addrs)

		// Collect entries for width computation.
		type entry struct {
			addr, agentName, localPath string
			modified                   bool
		}
		var entries []entry
		for _, addr := range addrs {
			agents := st.InstalledSkills[addr]
			for agentName, rec := range agents {
				if agentFilter != "" && agentName != agentFilter {
					continue
				}
				mod, _ := skill.IsModified(rec)
				if modifiedOnly && !mod {
					continue
				}
				entries = append(entries, entry{addr, agentName, rec.LocalPath, mod})
			}
		}

		addrW := 5
		agentW := 5
		for _, e := range entries {
			addrW = maxInt(addrW, len(e.addr))
			agentW = maxInt(agentW, len(e.agentName))
		}

		for _, e := range entries {
			flag := ""
			if e.modified {
				flag = "  " + yellow("[modified]")
			}
			fmt.Printf("%-*s  %-*s  %s%s\n", addrW, e.addr, agentW, e.agentName, e.localPath, flag)
		}
		return nil
	},
}

func listAvailable(repoFilter string, st *state.State) error {
	var skills []repo.SkillInfo
	var err error

	if repoFilter != "" {
		skills, err = repo.DiscoverSkills(repoFilter, st)
	} else {
		skills, err = repo.DiscoverAllSkills(st)
	}
	if err != nil {
		return err
	}

	if len(skills) == 0 {
		fmt.Println("No skills found. Register a repo with: skillpack repo add <name> <url>")
		return nil
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Address < skills[j].Address })

	// Group skills by their parent path (repo + category prefix)
	type group struct {
		prefix string
		items  []repo.SkillInfo
	}
	var groups []group
	curPrefix := ""
	for _, s := range skills {
		prefix := path.Dir(s.Address) // e.g. "my-repo/coding" or "my-repo"
		if prefix != curPrefix {
			groups = append(groups, group{prefix: prefix})
			curPrefix = prefix
		}
		groups[len(groups)-1].items = append(groups[len(groups)-1].items, s)
	}

	total := 0
	for _, g := range groups {
		fmt.Printf("%s/\n", bold(g.prefix))
		for _, s := range g.items {
			name := path.Base(s.Address)
			installed := ""
			if _, ok := st.InstalledSkills[s.Address]; ok {
				installed = "  " + green("[installed]")
			}
			fmt.Printf("  %-38s%s\n", name, installed)
			total++
		}
	}
	fmt.Printf("\n%s available\n", bold(fmt.Sprintf("%d skill(s)", total)))
	return nil
}

func init() {
	listCmd.Flags().String("agent", "", "Filter by agent name")
	listCmd.Flags().Bool("modified", false, "Show only locally-modified skills")
	listCmd.Flags().Bool("available", false, "Browse skills available in registered repos")
	listCmd.Flags().String("repo", "", "Filter available skills by repo name (used with --available)")
}
