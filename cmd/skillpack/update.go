package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bernard/skillpack/internal/skill"
	"github.com/bernard/skillpack/internal/state"
)

var updateCmd = &cobra.Command{
	Use:   "update [<repo>/<path/to/skill>]",
	Short: "Check for and apply upstream updates to installed skills",
	Long: `Check installed skills for upstream changes and apply updates.

Without arguments, checks all installed skills. With a skill address,
checks only that skill.

If a skill has local modifications AND upstream changes, the update is
blocked. Resolve the conflict with one of:

  --force-remote   upstream wins: overwrite local changes with cache
  --force-local    local wins: push your version to the remote repo
  --merge          file-level three-way merge; conflict markers on failure`,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentFilter, _ := cmd.Flags().GetString("agent")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		forceRemote, _ := cmd.Flags().GetBool("force-remote")
		forceLocal, _ := cmd.Flags().GetBool("force-local")
		doMerge, _ := cmd.Flags().GetBool("merge")

		resCount := 0
		for _, f := range []bool{forceRemote, forceLocal, doMerge} {
			if f {
				resCount++
			}
		}
		if resCount > 1 {
			return fmt.Errorf("specify at most one of --force-remote, --force-local, --merge")
		}

		st, err := state.Load()
		if err != nil {
			return err
		}

		type target struct{ addr, agent string }
		var targets []target

		if len(args) > 0 {
			// Specific skill: check for all agents that have it installed (or filter by --agent)
			addr := args[0]
			agents, ok := st.InstalledSkills[addr]
			if !ok {
				return fmt.Errorf("skill %q is not installed", addr)
			}
			for agent := range agents {
				if agentFilter != "" && agent != agentFilter {
					continue
				}
				targets = append(targets, target{addr, agent})
			}
		} else {
			// All installed skills
			addrs := make([]string, 0, len(st.InstalledSkills))
			for addr := range st.InstalledSkills {
				addrs = append(addrs, addr)
			}
			sort.Strings(addrs)
			for _, addr := range addrs {
				for agent := range st.InstalledSkills[addr] {
					if agentFilter != "" && agent != agentFilter {
						continue
					}
					targets = append(targets, target{addr, agent})
				}
			}
		}

		if len(targets) == 0 {
			fmt.Println("No installed skills to update.")
			return nil
		}

		var conflictCount int
		changed := false

		for _, t := range targets {
			result, err := skill.CheckUpdate(t.addr, t.agent, st)
			if err != nil {
				fmt.Printf("  %-40s  %-14s  error: %v\n", t.addr, t.agent, err)
				continue
			}

			if !result.HasUpstream {
				// Nothing to do — but report if locally modified
				if result.IsModified {
					fmt.Printf("  %-40s  %-14s  locally modified (no upstream change)\n", t.addr, t.agent)
				}
				continue
			}

			if result.IsConflict {
				switch {
				case forceRemote:
					if !dryRun {
						if err := skill.ForceRemote(t.addr, t.agent, st); err != nil {
							return err
						}
						fmt.Printf("  %-40s  %-14s  force-remote applied\n", t.addr, t.agent)
						changed = true
					} else {
						fmt.Printf("  %-40s  %-14s  [dry-run] would force-remote\n", t.addr, t.agent)
					}

				case forceLocal:
					if !dryRun {
						if err := skill.ForceLocal(t.addr, t.agent, st); err != nil {
							return err
						}
						fmt.Printf("  %-40s  %-14s  force-local applied (pushed to remote)\n", t.addr, t.agent)
						changed = true
					} else {
						fmt.Printf("  %-40s  %-14s  [dry-run] would force-local (push to remote)\n", t.addr, t.agent)
					}

				case doMerge:
					if !dryRun {
						hadConflicts, err := skill.MergeSkill(t.addr, t.agent, st)
						if err != nil {
							return err
						}
						if hadConflicts {
							fmt.Printf("  %-40s  %-14s  merged — conflicts written, resolve manually\n", t.addr, t.agent)
						} else {
							fmt.Printf("  %-40s  %-14s  merged cleanly\n", t.addr, t.agent)
							changed = true
						}
					} else {
						fmt.Printf("  %-40s  %-14s  [dry-run] would merge\n", t.addr, t.agent)
					}

				default:
					fmt.Printf("  %-40s  %-14s  CONFLICT: local modified + upstream changed — use --force-remote, --force-local, or --merge\n", t.addr, t.agent)
					conflictCount++
				}
			} else {
				// Safe update: not locally modified
				if !dryRun {
					if err := skill.ApplyUpdate(t.addr, t.agent, st); err != nil {
						return err
					}
					fmt.Printf("  %-40s  %-14s  updated\n", t.addr, t.agent)
					changed = true
				} else {
					fmt.Printf("  %-40s  %-14s  [dry-run] would update\n", t.addr, t.agent)
				}
			}
		}

		if changed {
			if err := state.Save(st); err != nil {
				return err
			}
		}

		if conflictCount > 0 {
			return fmt.Errorf("%d skill(s) blocked by conflicts — rerun with --force-remote, --force-local, or --merge", conflictCount)
		}
		return nil
	},
}

func init() {
	updateCmd.Flags().String("agent", "", "Filter by agent name")
	updateCmd.Flags().Bool("dry-run", false, "Show what would change without applying")
	updateCmd.Flags().Bool("force-remote", false, "Conflict resolution: upstream wins (overwrites local)")
	updateCmd.Flags().Bool("force-local", false, "Conflict resolution: local wins (pushes to remote)")
	updateCmd.Flags().Bool("merge", false, "Conflict resolution: three-way file-level merge")
}
