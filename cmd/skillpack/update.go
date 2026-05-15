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

var updateCmd = &cobra.Command{
	Use:   "update [<repo>/<path/to/skill>]",
	Short: "Check for and apply upstream updates to installed skills",
	Example: `  skillpack update
  skillpack update my-repo/coding/debugger
  skillpack update --dry-run
  skillpack update --force-remote my-repo/coding/debugger
  skillpack update --force-local  my-repo/coding/debugger
  skillpack update --merge        my-repo/coding/debugger
  skillpack update --merge --llm  my-repo/coding/debugger
  skillpack update --merge --llm claude-code my-repo/coding/debugger`,
	Long: `Check installed skills for upstream changes and apply updates.

Without arguments, checks all installed skills. With a skill address,
checks only that skill.

If a skill has local modifications AND upstream changes, the update is
blocked. Resolve the conflict with one of:

  --force-remote   upstream wins: overwrite local changes with cache
  --force-local    local wins: push your version to the remote repo
  --merge          file-level three-way merge; conflict markers on failure
  --merge --llm    three-way merge + LLM-assisted conflict resolution

For forked skills, --merge uses the upstream origin as 'theirs' and
the upstream_sha recorded at fork time as the common base.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentFilter, _ := cmd.Flags().GetString("agent")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		forceRemote, _ := cmd.Flags().GetBool("force-remote")
		forceLocal, _ := cmd.Flags().GetBool("force-local")
		doMerge, _ := cmd.Flags().GetBool("merge")
		llmAgent, _ := cmd.Flags().GetString("llm")

		resCount := 0
		for _, f := range []bool{forceRemote, forceLocal, doMerge} {
			if f {
				resCount++
			}
		}
		if resCount > 1 {
			return fmt.Errorf("specify at most one of --force-remote, --force-local, --merge")
		}
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
					fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, yellow("locally modified (no upstream change)"))
				}
				continue
			}

			if result.IsConflict {
				switch {
				case forceRemote:
					if !dryRun {
						if err := skill.ForceRemote(t.addr, t.agent, cfg.TokenForRepo(repoNameFromAddr(t.addr)), st); err != nil {
							return err
						}
						fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, green("force-remote applied"))
						changed = true
					} else {
						fmt.Printf("  %-40s  %-14s  [dry-run] would force-remote\n", t.addr, t.agent)
					}

				case forceLocal:
					if !dryRun {
						if err := skill.ForceLocal(t.addr, t.agent, cfg.TokenForRepo(repoNameFromAddr(t.addr)), st); err != nil {
							return err
						}
						fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, green("force-local applied (pushed to remote)"))
						changed = true
					} else {
						fmt.Printf("  %-40s  %-14s  [dry-run] would force-local (push to remote)\n", t.addr, t.agent)
					}

				case doMerge:
					if !dryRun {
						token := cfg.TokenForRepo(repoNameFromAddr(t.addr))
						hadConflicts, err := skill.MergeSkill(t.addr, t.agent, token, st)
						if err != nil {
							return err
						}
						if hadConflicts {
							if llmAgent != "" {
								agentName := llmAgent
								if agentName == "true" || agentName == "" {
									agentName = cfg.DefaultAgent
								}
								resolver, err := skill.NewDefaultLLMResolver(agentName)
								if err != nil {
									return err
								}
								if err := skill.LLMResolveConflicts(t.addr, t.agent, resolver, st); err != nil {
									return err
								}
								// Push resolved result to fork repo if applicable
								rec := st.InstalledSkills[t.addr][t.agent]
								if rec.UpstreamAddr != "" {
									if pushErr := skill.PushForkAfterLLM(t.addr, t.agent, token, st); pushErr != nil {
										return pushErr
									}
								}
								fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, green("merged + LLM resolved"))
							} else {
								fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, yellow("merged — conflicts written, resolve manually or use --llm"))
							}
						} else {
								fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, green("merged cleanly"))
							changed = true
						}
					} else {
						fmt.Printf("  %-40s  %-14s  [dry-run] would merge\n", t.addr, t.agent)
					}

				default:
					fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, red("CONFLICT: local modified + upstream changed — use --force-remote, --force-local, or --merge"))
					conflictCount++
				}
			} else {
				// Safe update: not locally modified
				if !dryRun {
					if err := skill.ApplyUpdate(t.addr, t.agent, cfg.TokenForRepo(repoNameFromAddr(t.addr)), st); err != nil {
						return err
					}
					fmt.Printf("  %-40s  %-14s  %s\n", t.addr, t.agent, green("updated"))
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
	updateCmd.Flags().String("llm", "", "LLM agent for conflict resolution (requires --merge); omit value to use default agent")
	// Allow --llm without a value; sentinel "true" means "use default agent".
	updateCmd.Flags().Lookup("llm").NoOptDefVal = "true"
}

func repoNameFromAddr(addr string) string {
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		return addr[:i]
	}
	return addr
}
