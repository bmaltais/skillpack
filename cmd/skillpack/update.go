package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/skill"
)

// llmNoOptDefVal is the sentinel value cobra injects when --llm is given without
// an argument (set via NoOptDefVal in init). Must match the NoOptDefVal assignment.
const llmNoOptDefVal = "true"

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

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		// Collect repo heads so PlanUpdate can determine upstream changes.
		repoHeads, headErr := skill.CollectRepoHeads(app.St)
		if headErr != nil {
			return headErr
		}

		type target struct{ addr, agent string }
		var targets []target
		targetsByRepo := make(map[string]bool)

		if len(args) > 0 {
			// Specific skill: check for all agents that have it installed (or filter by --agent)
			addr := args[0]
			agents, ok := app.St.InstalledSkills[addr]
			if !ok {
				return fmt.Errorf("skill %q is not installed", addr)
			}
			targetsByRepo[repoNameFromAddr(addr)] = true
			for agent := range agents {
				if agentFilter != "" && agent != agentFilter {
					continue
				}
				targets = append(targets, target{addr, agent})
			}
		} else {
			// All installed skills
			addrs := make([]string, 0, len(app.St.InstalledSkills))
			for addr := range app.St.InstalledSkills {
				addrs = append(addrs, addr)
			}
			sort.Strings(addrs)
			for _, addr := range addrs {
				targetsByRepo[repoNameFromAddr(addr)] = true
				for agent := range app.St.InstalledSkills[addr] {
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

		// Build a filtered repoHeads map limited to the repos referenced by targets.
		filteredHeads := make(map[string]string)
		for repoName := range targetsByRepo {
			if sha, ok := repoHeads[repoName]; ok {
				filteredHeads[repoName] = sha
			}
		}

		plan := skill.PlanUpdate(app.St, filteredHeads)

		// Filter plan items to only those matching the targets.
		targetSet := make(map[string]bool)
		for _, t := range targets {
			key := t.addr + "/" + t.agent
			targetSet[key] = true
		}
		var filteredPlan []skill.UpdatePlanItem
		for _, p := range plan {
			key := p.Addr + "/" + p.AgentName
			if targetSet[key] {
				filteredPlan = append(filteredPlan, p)
			}
		}

		// Sort for stable output.
		sort.Slice(filteredPlan, func(i, j int) bool {
			if filteredPlan[i].Addr != filteredPlan[j].Addr {
				return filteredPlan[i].Addr < filteredPlan[j].Addr
			}
			return filteredPlan[i].AgentName < filteredPlan[j].AgentName
		})

		// Compute column widths from actual data.
		addrW := 5
		agentW := 5
		for _, p := range filteredPlan {
			addrW = maxInt(addrW, len(p.Addr))
			agentW = maxInt(agentW, len(p.AgentName))
		}

		var conflictCount int

		for _, p := range filteredPlan {
			if p.Err != nil {
				fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, p.Err)
				continue
			}

			switch p.Action {
			case skill.UpdateAlreadyCurrent:
				// Nothing to do
			case skill.UpdateAvailable:
				token := app.Cfg.TokenForRepo(repoNameFromAddr(p.Addr))
				if !dryRun {
					is, openErr := skill.Open(p.Addr, p.AgentName, app.Cfg, app.St)
					if openErr != nil {
						fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, openErr)
						continue
					}
					if err := is.Update(token); err != nil {
						return err
					}
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("updated"))
				} else {
					fmt.Printf("  %-*s  %-*s  [dry-run] would update\n", addrW, p.Addr, agentW, p.AgentName)
				}
			case skill.UpdateLocallyModified:
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, yellow("locally modified (no upstream change)"))
			case skill.UpdateConflict:
				token := app.Cfg.TokenForRepo(repoNameFromAddr(p.Addr))
				switch {
				case forceRemote:
					if !dryRun {
						is, openErr := skill.Open(p.Addr, p.AgentName, app.Cfg, app.St)
						if openErr != nil {
							fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, openErr)
							continue
						}
						if _, err := is.Resolve(skill.ResolveForceRemote, token, ""); err != nil {
							return err
						}
						fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("force-remote applied"))
					} else {
						fmt.Printf("  %-*s  %-*s  [dry-run] would force-remote\n", addrW, p.Addr, agentW, p.AgentName)
					}

				case forceLocal:
					if !dryRun {
						is, openErr := skill.Open(p.Addr, p.AgentName, app.Cfg, app.St)
						if openErr != nil {
							fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, openErr)
							continue
						}
						if _, err := is.Resolve(skill.ResolveForceLocal, token, ""); err != nil {
							return err
						}
						fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("force-local applied (pushed to remote)"))
					} else {
						fmt.Printf("  %-*s  %-*s  [dry-run] would force-local (push to remote)\n", addrW, p.Addr, agentW, p.AgentName)
					}

				case doMerge:
					if !dryRun {
						is, openErr := skill.Open(p.Addr, p.AgentName, app.Cfg, app.St)
						if openErr != nil {
							fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, openErr)
							continue
						}
						mergeStrategy := skill.ResolveMerge
						effectiveLLMAgent := llmAgent
						if llmAgent != "" {
							mergeStrategy = skill.ResolveLLM
							if effectiveLLMAgent == llmNoOptDefVal {
								effectiveLLMAgent = app.Cfg.DefaultAgent
							}
						}
						llmResolved, err := is.Resolve(mergeStrategy, token, effectiveLLMAgent)
						switch {
						case errors.Is(err, skill.ErrMergeConflicts):
							fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, yellow("merged — conflicts written, resolve manually or use --llm"))
						case err != nil:
							return err
						case llmResolved:
							fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("merged + LLM resolved"))
						default:
							fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("merged cleanly"))
						}
					} else {
						fmt.Printf("  %-*s  %-*s  [dry-run] would merge\n", addrW, p.Addr, agentW, p.AgentName)
					}

				default:
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, red("CONFLICT: local modified + upstream changed — use --force-remote, --force-local, or --merge"))
					conflictCount++
				}
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
	// Allow --llm without a value; llmNoOptDefVal ("true") means "use default agent".
	updateCmd.Flags().Lookup("llm").NoOptDefVal = llmNoOptDefVal
}

func repoNameFromAddr(addr string) string {
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		return addr[:i]
	}
	return addr
}
