package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Two-way reconciliation of all installed skills",
	Long: `Two-way reconciliation across all installed skills:

  1. Pull all registered repos (update local cache)
  2. Skills with upstream changes and no local edits  → updated automatically
  3. Skills with local edits and no upstream changes  → published to remote
  4. Skills with both local edits and upstream changes → skipped (conflict)

Resolve conflicts at sync time with:
  skillpack sync --merge          three-way merge for all conflicts
  skillpack sync --merge --llm    merge + LLM-assisted conflict resolution`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		doMerge, _ := cmd.Flags().GetBool("merge")
		llmAgent, _ := cmd.Flags().GetString("llm")

		if llmAgent != "" && !doMerge {
			return fmt.Errorf("--llm requires --merge")
		}

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		if len(app.St.InstalledSkills) == 0 {
			fmt.Println("No installed skills to sync.")
			return nil
		}

		prefix := ""
		if dryRun {
			prefix = "[dry-run] "
		}
		fmt.Printf("%sSyncing %d installed skill(s)...\n", prefix, countInstalled(app.St))

		// Dry-run: use ReconcilePlan (pure, no applying changes) to show what
		// would happen. Repo caches are not pulled — output reflects current
		// local cache state, matching the previous behaviour.
		if dryRun {
			repoHeads, headErr := skill.CollectRepoHeads(app.St)
			if headErr != nil {
				return headErr
			}
			plan := skill.ReconcilePlan(app.St, repoHeads)

			// Sort for stable output.
			sort.Slice(plan, func(i, j int) bool {
				if plan[i].Addr != plan[j].Addr {
					return plan[i].Addr < plan[j].Addr
				}
				return plan[i].AgentName < plan[j].AgentName
			})

			addrW := 5
			agentW := 5
			for _, p := range plan {
				addrW = maxInt(addrW, len(p.Addr))
				agentW = maxInt(agentW, len(p.AgentName))
			}

			var updated, published, current, conflicts, errCount int
			for _, p := range plan {
				if p.Err != nil {
					fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, p.Err)
					errCount++
					continue
				}
				switch p.Action {
				case skill.SyncUpdated:
					// Note: upstream detection uses InstalledAtSHA vs HEAD SHA (coarser than
					// a file-level diff). A commit touching only unrelated paths in the repo
					// will still appear as "would update" here.
					fmt.Printf("  %-*s  %-*s  [dry-run] would update\n", addrW, p.Addr, agentW, p.AgentName)
					updated++
				case skill.SyncPublished:
					fmt.Printf("  %-*s  %-*s  [dry-run] would publish\n", addrW, p.Addr, agentW, p.AgentName)
					published++
				case skill.SyncConflict:
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, red("CONFLICT — resolve manually"))
					conflicts++
				case skill.SyncAlreadyCurrent:
					current++
				}
			}
			fmt.Printf("\n  %d updated, %d published, %d already current", updated, published, current)
			if conflicts > 0 {
				fmt.Printf(", %d conflict(s)", conflicts)
			}
			if errCount > 0 {
				fmt.Printf(", %d error(s)", errCount)
			}
			fmt.Println()
			if conflicts > 0 {
				return fmt.Errorf(
					"%d conflict(s) skipped — resolve with: skillpack update --force-remote|--force-local|--merge <addr>  (or rerun sync with --merge)",
					conflicts,
				)
			}
			if errCount > 0 {
				return fmt.Errorf("%d skill(s) could not be planned — see errors above", errCount)
			}
			return nil
		}

		results, conflicts, err := skill.Sync(false, app.Cfg.TokenForRepo, app.St)
		if err != nil {
			return err
		}

		// Sort results for stable output
		sort.Slice(results, func(i, j int) bool {
			if results[i].Addr != results[j].Addr {
				return results[i].Addr < results[j].Addr
			}
			return results[i].AgentName < results[j].AgentName
		})

		// Compute column widths from actual data.
		addrW := 5 // len("Skill") minimum
		agentW := 5
		for _, r := range results {
			addrW = maxInt(addrW, len(r.Addr))
			agentW = maxInt(agentW, len(r.AgentName))
		}
		for _, c := range conflicts {
			addrW = maxInt(addrW, len(c.Addr))
			agentW = maxInt(agentW, len(c.AgentName))
		}

		// Tally
		var updated, published, current, errCount int
		for _, r := range results {
			switch {
			case r.Err != nil:
				fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, r.Addr, agentW, r.AgentName, r.Err)
				errCount++
			case r.Action == skill.SyncUpdated:
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, r.Addr, agentW, r.AgentName, green("updated"))
				updated++
			case r.Action == skill.SyncPublished:
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, r.Addr, agentW, r.AgentName, green("published"))
				published++
			case r.Action == skill.SyncAlreadyCurrent:
				current++
			}
		}
		for _, c := range conflicts {
			if doMerge {
				token := app.Cfg.TokenForRepo(repoNameFromAddr(c.Addr))
				mergeStrategy := skill.ResolveMerge
				effectiveLLMAgent := llmAgent
				if llmAgent != "" {
					mergeStrategy = skill.ResolveLLM
					if effectiveLLMAgent == llmNoOptDefVal {
						effectiveLLMAgent = app.Cfg.DefaultAgent
					}
				}
				is, openErr := skill.Open(c.Addr, c.AgentName, app.Cfg, app.St)
				if openErr != nil {
					fmt.Printf("  %-*s  %-*s  merge error: %v\n", addrW, c.Addr, agentW, c.AgentName, openErr)
					errCount++
					continue
				}
				llmResolved, mergeErr := is.Resolve(mergeStrategy, token, effectiveLLMAgent)
				switch {
				case errors.Is(mergeErr, skill.ErrMergeConflicts):
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, c.Addr, agentW, c.AgentName, yellow("merged — conflicts written, resolve manually or use --llm"))
				case mergeErr != nil:
					fmt.Printf("  %-*s  %-*s  merge error: %v\n", addrW, c.Addr, agentW, c.AgentName, mergeErr)
					errCount++
				case llmResolved:
					published++
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, c.Addr, agentW, c.AgentName, green("merged + LLM resolved"))
				default:
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, c.Addr, agentW, c.AgentName, green("merged cleanly"))
				}
			} else {
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, c.Addr, agentW, c.AgentName, red("CONFLICT — resolve manually"))
			}
		}

		// Summary line
		fmt.Printf("\n  %d updated, %d published, %d already current", updated, published, current)
		if len(conflicts) > 0 {
			fmt.Printf(", %d conflict(s)", len(conflicts))
		}
		if errCount > 0 {
			fmt.Printf(", %d error(s)", errCount)
		}
		fmt.Println()

		// Internal functions (skill.Resolve, skill.ApplySync) persist state
		// themselves — no explicit save needed here.

		if len(conflicts) > 0 && !doMerge {
			return fmt.Errorf(
				"%d conflict(s) skipped — resolve with: skillpack update --force-remote|--force-local|--merge <addr>  (or rerun sync with --merge)",
				len(conflicts),
			)
		}
		return nil
	},
}

func countInstalled(st *state.State) int {
	n := 0
	for _, agents := range st.InstalledSkills {
		n += len(agents)
	}
	return n
}

func init() {
	syncCmd.Flags().Bool("dry-run", false, "Show what would change without applying")
	syncCmd.Flags().Bool("merge", false, "Attempt three-way merge for all conflicts")
	syncCmd.Flags().String("llm", "", "LLM agent for conflict resolution (requires --merge); omit value to use default agent")
	syncCmd.Flags().Lookup("llm").NoOptDefVal = llmNoOptDefVal
}
