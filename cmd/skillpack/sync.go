package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bernard/skillpack/internal/skill"
	"github.com/bernard/skillpack/internal/state"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Two-way reconciliation of all installed skills",
	Long: `Two-way reconciliation across all installed skills:

  1. Pull all registered repos (update local cache)
  2. Skills with upstream changes and no local edits  → updated automatically
  3. Skills with local edits and no upstream changes  → published to remote
  4. Skills with both local edits and upstream changes → skipped (conflict)

Resolve conflicts with:
  skillpack update --force-remote <addr>   upstream wins
  skillpack update --force-local  <addr>   local wins (push to remote)
  skillpack update --merge        <addr>   three-way merge`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		st, err := state.Load()
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

		results, conflicts, err := skill.Sync(dryRun, st)
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
				tag := "updated"
				if dryRun {
					tag = "[dry-run] would update"
				}
				fmt.Printf("  %-40s  %-14s  %s\n", r.Addr, r.AgentName, tag)
				updated++
			case r.Action == skill.SyncPublished:
				tag := "published"
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
			fmt.Printf("  %-40s  %-14s  CONFLICT — resolve manually\n", c.Addr, c.AgentName)
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

		if len(conflicts) > 0 {
			return fmt.Errorf(
				"%d conflict(s) skipped — resolve with: skillpack update --force-remote|--force-local|--merge <addr>",
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
}
