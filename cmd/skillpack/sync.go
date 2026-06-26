package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// llmNoOptDefVal is the sentinel value cobra injects when --llm is given without
// an argument (set via NoOptDefVal in init). Must match the NoOptDefVal assignment.
const llmNoOptDefVal = "true"

var syncCmd = &cobra.Command{
	Use:   "sync [<addr>]",
	Short: "Two-way reconciliation of installed skills",
	Example: `  skillpack sync
  skillpack sync my-repo/coding/debugger
  skillpack sync --dry-run
  skillpack sync --force-remote my-repo/coding/debugger
  skillpack sync --force-local  my-repo/coding/debugger
  skillpack sync --merge        my-repo/coding/debugger
  skillpack sync --merge --llm  my-repo/coding/debugger`,
	Long: `Two-way reconciliation of installed skills:

  1. Pull registered repos (update local cache)
  2. Skills with upstream changes and no local edits  → updated automatically
  3. Skills with local edits and no upstream changes  → pushed to remote
  4. Skills with both local edits and upstream changes → skipped (conflict)

Without arguments, operates on all installed skills.
With a skill address, operates on that single skill only.

Resolve conflicts with:
  skillpack sync --force-remote <addr>   upstream wins (overwrites local)
  skillpack sync --force-local  <addr>   local wins (pushes to remote)
  skillpack sync --merge        <addr>   file-level three-way merge
  skillpack sync --merge --llm  <addr>   merge + LLM-assisted conflict resolution`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		doMerge, _ := cmd.Flags().GetBool("merge")
		forceRemote, _ := cmd.Flags().GetBool("force-remote")
		forceLocal, _ := cmd.Flags().GetBool("force-local")
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

		if len(app.St.InstalledSkills) == 0 {
			fmt.Println("No installed skills to sync.")
			return nil
		}

		// Single-skill path: pull only the relevant repo, then reconcile filtered.
		if len(args) == 1 {
			return syncOne(cmd, args[0], dryRun, forceRemote, forceLocal, doMerge, llmAgent, app)
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
			var staleAddrs [][2]string
			for _, p := range plan {
				if p.Err != nil {
					fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, p.Err)
					errCount++
					continue
				}
				switch p.Action {
				case skill.SyncUpdated:
					fmt.Printf("  %-*s  %-*s  [dry-run] would update\n", addrW, p.Addr, agentW, p.AgentName)
					updated++
				case skill.SyncPublished:
					fmt.Printf("  %-*s  %-*s  [dry-run] would push\n", addrW, p.Addr, agentW, p.AgentName)
					published++
				case skill.SyncConflict:
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, red("CONFLICT — resolve manually"))
					conflicts++
				case skill.SyncAlreadyCurrent:
					current++
				case skill.SyncStaleAddress:
					staleAddrs = append(staleAddrs, [2]string{p.Addr, p.AgentName})
				}
				if p.Warning != "" {
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, "", agentW, "", yellow("warning: "+p.Warning))
				}
			}
			fmt.Printf("\n  %d updated, %d pushed, %d already current", updated, published, current)
			if conflicts > 0 {
				fmt.Printf(", %d conflict(s)", conflicts)
			}
			if errCount > 0 {
				fmt.Printf(", %d error(s)", errCount)
			}
			fmt.Println()
			printStaleSection(staleAddrs, addrW, agentW, app.St)
			if conflicts > 0 {
				return fmt.Errorf(
					"%d conflict(s) skipped — resolve with: skillpack sync --force-remote|--force-local|--merge <addr>",
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
		var staleAddrs [][2]string
		for _, r := range results {
			switch {
			case r.Err != nil:
				fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, r.Addr, agentW, r.AgentName, r.Err)
				errCount++
			case r.Action == skill.SyncUpdated:
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, r.Addr, agentW, r.AgentName, green("updated"))
				updated++
			case r.Action == skill.SyncPublished:
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, r.Addr, agentW, r.AgentName, green("pushed"))
				published++
			case r.Action == skill.SyncAlreadyCurrent:
				current++
			case r.Action == skill.SyncStaleAddress:
				staleAddrs = append(staleAddrs, [2]string{r.Addr, r.AgentName})
			}
			if r.Warning != "" {
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, "", agentW, "", yellow("warning: "+r.Warning))
			}
		}
		for _, c := range conflicts {
			if doMerge {
				token := app.Cfg.TokenForRepo(repoNameFromAddr(c.Addr))
				llmPublished, hadErr := applyMerge(c.Addr, c.AgentName, llmAgent, token, app, addrW, agentW)
				if hadErr {
					errCount++
				} else if llmPublished {
					published++
				}
			} else {
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, c.Addr, agentW, c.AgentName, red("CONFLICT — resolve manually"))
			}
		}

		// Summary line
		fmt.Printf("\n  %d updated, %d pushed, %d already current", updated, published, current)
		if len(conflicts) > 0 {
			fmt.Printf(", %d conflict(s)", len(conflicts))
		}
		if errCount > 0 {
			fmt.Printf(", %d error(s)", errCount)
		}
		fmt.Println()

		// Stale-address remediation section.
		printStaleSection(staleAddrs, addrW, agentW, app.St)

		// Internal functions (skill.Resolve, skill.ApplySync) persist state
		// themselves — no explicit save needed here.

		if len(conflicts) > 0 && !doMerge {
			return fmt.Errorf(
				"%d conflict(s) skipped — resolve with: skillpack sync --force-remote|--force-local|--merge <addr>",
				len(conflicts),
			)
		}
		return nil
	},
}

// syncOne performs two-way reconciliation for a single installed skill.
func syncOne(cmd *cobra.Command, addr string, dryRun, forceRemote, forceLocal, doMerge bool, llmAgent string, app *App) error {
	if _, ok := app.St.InstalledSkills[addr]; !ok {
		return fmt.Errorf("skill %q is not installed", addr)
	}

	repoName := repoNameFromAddr(addr)
	token := app.Cfg.TokenForRepo(repoName)

	// Pull just the relevant repo.
	if !dryRun {
		if warn, pullErr := repo.Update(repoName, token, app.St); pullErr != nil {
			fmt.Printf("  warning: could not pull %s: %v\n", repoName, pullErr)
		} else if warn != "" {
			fmt.Printf("  notice: %s\n", warn)
		}
	}

	repoHeads, headErr := skill.CollectRepoHeads(app.St)
	if headErr != nil {
		return headErr
	}

	// Build plan and filter to just this addr.
	fullPlan := skill.ReconcilePlan(app.St, repoHeads)
	var plan []skill.SyncPlanItem
	for _, p := range fullPlan {
		if p.Addr == addr {
			plan = append(plan, p)
		}
	}

	sort.Slice(plan, func(i, j int) bool {
		return plan[i].AgentName < plan[j].AgentName
	})

	addrW := maxInt(5, len(addr))
	agentW := 5
	for _, p := range plan {
		agentW = maxInt(agentW, len(p.AgentName))
	}

	var updated, published, current, conflictCount, errCount int
	var staleAddrs [][2]string

	for _, p := range plan {
		if p.Err != nil {
			fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, p.Err)
			errCount++
			continue
		}
		switch p.Action {
		case skill.SyncAlreadyCurrent:
			current++
		case skill.SyncStaleAddress:
			staleAddrs = append(staleAddrs, [2]string{p.Addr, p.AgentName})
		case skill.SyncUpdated:
			if dryRun {
				fmt.Printf("  %-*s  %-*s  [dry-run] would update\n", addrW, p.Addr, agentW, p.AgentName)
				updated++
			} else {
				results, _, applyErr := skill.ApplySync([]skill.SyncPlanItem{p}, app.Cfg.TokenForRepo, app.St)
				if applyErr != nil {
					return applyErr
				}
				if len(results) > 0 && results[0].Err != nil {
					fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, results[0].Err)
					errCount++
				} else {
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("updated"))
					updated++
				}
			}
		case skill.SyncPublished:
			if dryRun {
				fmt.Printf("  %-*s  %-*s  [dry-run] would push\n", addrW, p.Addr, agentW, p.AgentName)
				published++
			} else {
				results, _, applyErr := skill.ApplySync([]skill.SyncPlanItem{p}, app.Cfg.TokenForRepo, app.St)
				if applyErr != nil {
					return applyErr
				}
				if len(results) > 0 && results[0].Err != nil {
					fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, results[0].Err)
					errCount++
				} else {
					fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("pushed"))
					published++
				}
			}
		case skill.SyncConflict:
			switch {
			case dryRun:
				fmt.Printf("  %-*s  %-*s  [dry-run] CONFLICT — would need resolution\n", addrW, p.Addr, agentW, p.AgentName)
				conflictCount++
			case forceRemote:
				is, openErr := skill.Open(p.Addr, p.AgentName, app.Cfg, app.St)
				if openErr != nil {
					fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, openErr)
					errCount++
					continue
				}
				if _, err := is.Resolve(skill.ResolveForceRemote, token, ""); err != nil {
					return err
				}
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("force-remote applied"))
			case forceLocal:
				is, openErr := skill.Open(p.Addr, p.AgentName, app.Cfg, app.St)
				if openErr != nil {
					fmt.Printf("  %-*s  %-*s  error: %v\n", addrW, p.Addr, agentW, p.AgentName, openErr)
					errCount++
					continue
				}
				if _, err := is.Resolve(skill.ResolveForceLocal, token, ""); err != nil {
					return err
				}
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, green("force-local applied (pushed to remote)"))
			case doMerge:
				_, hadErr := applyMerge(p.Addr, p.AgentName, llmAgent, token, app, addrW, agentW)
				if hadErr {
					errCount++
				}
			default:
				fmt.Printf("  %-*s  %-*s  %s\n", addrW, p.Addr, agentW, p.AgentName, red("CONFLICT — resolve with --force-remote, --force-local, or --merge"))
				conflictCount++
			}
		}
		if p.Warning != "" {
			fmt.Printf("  %-*s  %-*s  %s\n", addrW, "", agentW, "", yellow("warning: "+p.Warning))
		}
	}

	fmt.Printf("\n  %d updated, %d pushed, %d already current", updated, published, current)
	if conflictCount > 0 {
		fmt.Printf(", %d conflict(s)", conflictCount)
	}
	if errCount > 0 {
		fmt.Printf(", %d error(s)", errCount)
	}
	fmt.Println()

	printStaleSection(staleAddrs, addrW, agentW, app.St)

	if conflictCount > 0 {
		return fmt.Errorf("%d conflict(s) skipped — resolve with: skillpack sync --force-remote|--force-local|--merge %s", conflictCount, addr)
	}
	return nil
}

// applyMerge attempts a three-way merge (optionally LLM-assisted) for a conflict.
// It prints the outcome and returns (llmPublished, hadErr).
func applyMerge(addr, agentName, llmAgent, token string, app *App, addrW, agentW int) (llmPublished bool, hadErr bool) {
	mergeStrategy := skill.ResolveMerge
	effectiveLLMAgent := llmAgent
	if llmAgent != "" {
		mergeStrategy = skill.ResolveLLM
		if effectiveLLMAgent == llmNoOptDefVal {
			effectiveLLMAgent = app.Cfg.DefaultAgent
		}
	}
	is, openErr := skill.Open(addr, agentName, app.Cfg, app.St)
	if openErr != nil {
		fmt.Printf("  %-*s  %-*s  merge error: %v\n", addrW, addr, agentW, agentName, openErr)
		return false, true
	}
	llmResolved, mergeErr := is.Resolve(mergeStrategy, token, effectiveLLMAgent)
	switch {
	case errors.Is(mergeErr, skill.ErrMergeConflicts):
		fmt.Printf("  %-*s  %-*s  %s\n", addrW, addr, agentW, agentName, yellow("merged — conflicts written, resolve manually or use --llm"))
	case mergeErr != nil:
		fmt.Printf("  %-*s  %-*s  merge error: %v\n", addrW, addr, agentW, agentName, mergeErr)
		return false, true
	case llmResolved:
		fmt.Printf("  %-*s  %-*s  %s\n", addrW, addr, agentW, agentName, green("merged + LLM resolved"))
		return true, false
	default:
		fmt.Printf("  %-*s  %-*s  %s\n", addrW, addr, agentW, agentName, green("merged cleanly"))
	}
	return false, false
}

// printStaleSection prints the stale-address remediation block shared by the
// sync output paths. rows holds the (addr, agent) pairs whose skill path no
// longer exists upstream. For each stale mapping it surfaces likely replacement
// addresses found in registered repos and the relink command to repair it. It
// prints nothing when rows is empty.
func printStaleSection(rows [][2]string, addrW, agentW int, st *state.State) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("\n  %s\n", yellow(fmt.Sprintf("%d stale skill address(es) — skill path no longer exists upstream:", len(rows))))

	// Cache suggestions per address so we don't re-scan repos for the same addr
	// when it is stale across multiple agents.
	suggestions := make(map[string][]string)
	for _, s := range rows {
		addr := s[0]
		fmt.Printf("    %-*s  %-*s\n", addrW, addr, agentW, s[1])
		if _, done := suggestions[addr]; !done {
			suggestions[addr] = skill.SuggestReplacements(addr, st)
		}
		for _, cand := range suggestions[addr] {
			fmt.Printf("      %s %s\n", green("→ possible replacement:"), cand)
		}
	}

	fmt.Printf("\n  To repair a stale mapping: skillpack relink <stale-addr> <new-addr> [--agent <name>]\n")
	fmt.Printf("  To remove a stale mapping: skillpack remove <addr> [--agent <name>]\n")
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
	syncCmd.Flags().Bool("force-remote", false, "Conflict resolution: upstream wins (overwrites local)")
	syncCmd.Flags().Bool("force-local", false, "Conflict resolution: local wins (pushes to remote)")
	syncCmd.Flags().Bool("merge", false, "Conflict resolution: three-way file-level merge")
	syncCmd.Flags().String("llm", "", "LLM agent for conflict resolution (requires --merge); omit value to use default agent")
	syncCmd.Flags().Lookup("llm").NoOptDefVal = llmNoOptDefVal
}
