package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current state of all installed skills",
	Long: `Show the state of every installed skill without making any changes.

Each skill is reported as one of:
  up-to-date       — no local edits, no upstream changes
  update available — upstream has new changes (sync would pull these in)
  locally modified — local edits exist, no upstream changes (sync would publish)
  conflict         — both local edits and upstream changes (sync would skip)

By default the registered repos are fetched first so upstream state is accurate.
Use --no-fetch to skip the network call and report against cached state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		noFetch, _ := cmd.Flags().GetBool("no-fetch")

		st, err := state.Load()
		if err != nil {
			fmt.Printf("error: could not load state: %v\n", err)
			return nil
		}

		if len(st.InstalledSkills) == 0 {
			fmt.Println("No installed skills.")
			return nil
		}

		cfg, cfgErr := config.Load()
		tokenFor := func(string) string { return "" }
		if cfgErr == nil {
			tokenFor = cfg.TokenForRepo
		}

		if !noFetch {
			fmt.Println("Fetching repos...")
			for name := range st.Repos {
				if pullErr := repo.Update(name, tokenFor(name), st); pullErr != nil {
					fmt.Printf("  warning: could not fetch %s: %v\n", name, pullErr)
				}
			}
		}

		type row struct {
			addr      string
			agentName string
			result    *skill.UpdateResult
			err       error
		}

		var rows []row
		for addr, agents := range st.InstalledSkills {
			for agentName := range agents {
				r, checkErr := skill.CheckUpdate(addr, agentName, st)
				rows = append(rows, row{addr, agentName, r, checkErr})
			}
		}

		sort.Slice(rows, func(i, j int) bool {
			if rows[i].addr != rows[j].addr {
				return rows[i].addr < rows[j].addr
			}
			return rows[i].agentName < rows[j].agentName
		})

		// Print header — apply bold after padding to avoid ANSI codes skewing column widths.
		fmt.Printf("\n  %s  %s  %s\n",
			bold(fmt.Sprintf("%-40s", "Skill")),
			bold(fmt.Sprintf("%-14s", "Agent")),
			bold("Status"),
		)
		fmt.Printf("  %-40s  %-14s  %s\n", "────────────────────────────────────────", "──────────────", "───────────────────")

		var nCurrent, nUpdate, nModified, nConflict, nErr int
		for _, row := range rows {
			if row.err != nil {
				fmt.Printf("  %-40s  %-14s  %s\n", row.addr, row.agentName, red("error: "+row.err.Error()))
				nErr++
				continue
			}
			var statusStr string
			switch {
			case row.result.IsConflict:
				statusStr = red("conflict")
				nConflict++
			case row.result.IsModified:
				statusStr = yellow("locally modified")
				nModified++
			case row.result.HasUpstream:
				statusStr = cyan("update available")
				nUpdate++
			default:
				statusStr = green("up-to-date")
				nCurrent++
			}
			fmt.Printf("  %-40s  %-14s  %s\n", row.addr, row.agentName, statusStr)
		}

		// Summary line
		fmt.Println()
		parts := []string{}
		if nCurrent > 0 {
			parts = append(parts, fmt.Sprintf("%s %s", green(fmt.Sprintf("%d", nCurrent)), "up-to-date"))
		}
		if nUpdate > 0 {
			parts = append(parts, fmt.Sprintf("%s %s", cyan(fmt.Sprintf("%d", nUpdate)), "update available"))
		}
		if nModified > 0 {
			parts = append(parts, fmt.Sprintf("%s %s", yellow(fmt.Sprintf("%d", nModified)), "locally modified"))
		}
		if nConflict > 0 {
			parts = append(parts, fmt.Sprintf("%s %s", red(fmt.Sprintf("%d", nConflict)), "conflict"))
		}
		if nErr > 0 {
			parts = append(parts, fmt.Sprintf("%d error(s)", nErr))
		}
		summary := ""
		for i, p := range parts {
			if i > 0 {
				summary += ", "
			}
			summary += p
		}
		fmt.Println(summary)
		return nil
	},
}

func init() {
	statusCmd.Flags().Bool("no-fetch", false, "Skip fetching repos; report against cached state")
}
