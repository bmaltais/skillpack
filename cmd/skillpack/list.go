package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bernard/skillpack/internal/repo"
	"github.com/bernard/skillpack/internal/skill"
	"github.com/bernard/skillpack/internal/state"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		agentFilter, _ := cmd.Flags().GetString("agent")
		modifiedOnly, _ := cmd.Flags().GetBool("modified")
		available, _ := cmd.Flags().GetString("available")

		st, err := state.Load()
		if err != nil {
			return err
		}

		// --available: browse skills in registered repos
		if cmd.Flags().Changed("available") {
			return listAvailable(available, st)
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

		for _, addr := range addrs {
			agents := st.InstalledSkills[addr]
			for agentName, rec := range agents {
				if agentFilter != "" && agentName != agentFilter {
					continue
				}
				if modifiedOnly {
					modified, err := skill.IsModified(rec)
					if err != nil || !modified {
						continue
					}
				}
				modified, _ := skill.IsModified(rec)
				flag := ""
				if modified {
					flag = " [modified]"
				}
				fmt.Printf("%-40s  %-16s  %s%s\n", addr, agentName, rec.LocalPath, flag)
			}
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
		fmt.Println("No skills found. Register a repo with: skillpack repo add <url>")
		return nil
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Address < skills[j].Address })
	for _, s := range skills {
		installed := ""
		if _, ok := st.InstalledSkills[s.Address]; ok {
			installed = " [installed]"
		}
		fmt.Printf("%-50s%s\n", s.Address, installed)
	}
	return nil
}

func init() {
	listCmd.Flags().String("agent", "", "Filter by agent name")
	listCmd.Flags().Bool("modified", false, "Show only locally-modified skills")
	listCmd.Flags().String("available", "", "Browse skills in registered repos (use repo name or omit for all)")
	listCmd.Flags().Lookup("available").NoOptDefVal = "" // allow --available without a value
}
