package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
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

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		st := app.St

		if len(st.InstalledSkills) == 0 {
			fmt.Println("No installed skills.")
			return nil
		}

		tokenFor := app.Cfg.TokenForRepo

		if !noFetch {
			fmt.Println("Fetching repos...")
			for name := range st.Repos {
				if pullErr := repo.Update(name, tokenFor(name), st); pullErr != nil {
					fmt.Printf("  warning: could not fetch %s: %v\n", name, pullErr)
				}
			}
		}

		// Collect repo heads so PlanUpdate can determine upstream changes.
		repoHeads, headErr := skill.CollectRepoHeads(st)
		if headErr != nil {
			return headErr
		}

		plan := skill.PlanUpdate(st, repoHeads)

		// Sort for stable output.
		sort.Slice(plan, func(i, j int) bool {
			if plan[i].Addr != plan[j].Addr {
				return plan[i].Addr < plan[j].Addr
			}
			return plan[i].AgentName < plan[j].AgentName
		})

		// Detect skills with missing fork provenance (best-effort heuristic).
		// Only flag skills in repos the user can write to (SSH or token configured)
		// so upstream read-only skills don't appear as fork candidates.
		canWrite := func(name string) bool {
			rec, ok := st.Repos[name]
			if !ok {
				return false
			}
			return strings.HasPrefix(rec.URL, "git@") || strings.HasPrefix(rec.URL, "ssh://") || tokenFor(name) != ""
		}
		forkCandidateUpstream := skill.ForkCandidateMap(st, canWrite)

		// Compute column widths from actual data.
		addrW := len("Skill")
		agentW := len("Agent")
		for _, p := range plan {
			addrW = maxInt(addrW, len(p.Addr))
			agentW = maxInt(agentW, len(p.AgentName))
		}
		sep := func(n int) string {
			return strings.Repeat("─", n)
		}

		// Print header — apply bold after padding to avoid ANSI codes skewing column widths.
		fmt.Printf("\n  %s  %s  %s\n",
			bold(fmt.Sprintf("%-*s", addrW, "Skill")),
			bold(fmt.Sprintf("%-*s", agentW, "Agent")),
			bold("Status"),
		)
		fmt.Printf("  %-*s  %-*s  %s\n", addrW, sep(addrW), agentW, sep(agentW), "───────────────────")

		var nCurrent, nUpdate, nModified, nConflict, nErr int
		for _, p := range plan {
			if p.Err != nil {
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, red("error: "+p.Err.Error()))
				nErr++
				continue
			}
			var statusStr string
			rec := st.InstalledSkills[p.Addr][p.AgentName]
			switch p.Action {
			case skill.UpdateConflict:
				statusStr = red("conflict")
				nConflict++
			case skill.UpdateLocallyModified:
				statusStr = yellow("locally modified")
				nModified++
			case skill.UpdateAvailable:
				statusStr = cyan("update available")
				nUpdate++
			default:
				statusStr = green("up-to-date")
				nCurrent++
			}
			if rec.UpstreamAddr != "" {
				statusStr += "  [fork of " + rec.UpstreamAddr + "]"
			} else if upstream, ok := forkCandidateUpstream[p.Addr]; ok {
				statusStr += "  " + yellow("[fork? → "+upstream+"]")
			}
			fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, statusStr)
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
