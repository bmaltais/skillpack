package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
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

		st, err := state.Load()
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if len(st.InstalledSkills) == 0 {
			fmt.Println("No installed skills to sync.")
			return nil
		}

		prefix := ""
		if dryRun {
			prefix = "[dry-run] "
		}
		fmt.Printf("%sSyncing %d installed skill(s)...\n", prefix, countInstalled(st))

		results, conflicts, err := skill.Sync(dryRun, cfg.TokenForRepo, st)
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

		// Tally
		var updated, published, current, errCount int
		for _, r := range results {
			switch {
			case r.Err != nil:
				fmt.Printf("  %-40s  %-14s  error: %v\n", r.Addr, r.AgentName, r.Err)
				errCount++
			case r.Action == skill.SyncUpdated:
				tag := green("updated")
				if dryRun {
					tag = "[dry-run] would update"
				}
				fmt.Printf("  %-40s  %-14s  %s\n", r.Addr, r.AgentName, tag)
				updated++
			case r.Action == skill.SyncPublished:
				tag := green("published")
				if dryRun {
					tag = "[dry-run] would publish"
				}
				fmt.Printf("  %-40s  %-14s  %s\n", r.Addr, r.AgentName, tag)
				published++
			case r.Action == skill.SyncAlreadyCurrent:
				current++
			}
		}
		for _, c := range conflicts {
			if doMerge && !dryRun {
				token := cfg.TokenForRepo(strings.SplitN(c.Addr, "/", 2)[0])
				hadConflicts, mergeErr := skill.MergeSkill(c.Addr, c.AgentName, token, st)
				if mergeErr != nil {
					fmt.Printf("  %-40s  %-14s  merge error: %v\n", c.Addr, c.AgentName, mergeErr)
					continue
				}
				if hadConflicts {
					if llmAgent != "" {
						agentName := llmAgent
						if agentName == "true" || agentName == "" {
							agentName = cfg.DefaultAgent
						}
						resolver, resolverErr := skill.NewDefaultLLMResolver(agentName)
						if resolverErr != nil {
							fmt.Printf("  %-40s  %-14s  %v\n", c.Addr, c.AgentName, resolverErr)
							continue
						}
						if llmErr := skill.LLMResolveConflicts(c.Addr, c.AgentName, resolver, st); llmErr != nil {
							fmt.Printf("  %-40s  %-14s  LLM error: %v\n", c.Addr, c.AgentName, llmErr)
							continue
						}
						rec := st.InstalledSkills[c.Addr][c.AgentName]
						if rec.UpstreamAddr != "" {
							if pushErr := skill.PushForkAfterLLM(c.Addr, c.AgentName, token, st); pushErr != nil {
								fmt.Printf("  %-40s  %-14s  push error: %v\n", c.Addr, c.AgentName, pushErr)
								continue
							}
						}
						fmt.Printf("  %-40s  %-14s  %s\n", c.Addr, c.AgentName, green("merged + LLM resolved"))
					} else {
						fmt.Printf("  %-40s  %-14s  %s\n", c.Addr, c.AgentName, yellow("merged — conflicts written, resolve manually or use --llm"))
					}
				} else {
					fmt.Printf("  %-40s  %-14s  %s\n", c.Addr, c.AgentName, green("merged cleanly"))
				}
			} else {
				fmt.Printf("  %-40s  %-14s  %s\n", c.Addr, c.AgentName, red("CONFLICT — resolve manually"))
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

		if !dryRun && (updated > 0 || published > 0) {
			if err := state.Save(st); err != nil {
				return err
			}
		}

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
	syncCmd.Flags().Lookup("llm").NoOptDefVal = "true"
}
