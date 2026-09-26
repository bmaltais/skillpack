package main

import (
	"fmt"

	"github.com/spf13/cobra"

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

		opts := skill.SyncOptions{DryRun: dryRun}
		if len(args) == 1 {
			opts.Addr = args[0]
		}
		switch {
		case forceRemote:
			opts.Resolve = skill.ResolveForceRemote
		case forceLocal:
			opts.Resolve = skill.ResolveForceLocal
		case doMerge:
			opts.Resolve = skill.ResolveMerge
			if llmAgent != "" {
				opts.Resolve = skill.ResolveLLM
				opts.LLMAgent = llmAgent
				if llmAgent == llmNoOptDefVal {
					opts.LLMAgent = app.Cfg.DefaultAgent
				}
			}
		}
		scoped := opts.Addr != ""
		if !scoped {
			// Force flags only apply to a named skill; bulk sync resolves by merge only.
			if opts.Resolve == skill.ResolveForceRemote || opts.Resolve == skill.ResolveForceLocal {
				opts.Resolve = ""
			}
			prefix := ""
			if dryRun {
				prefix = "[dry-run] "
			}
			fmt.Printf("%sSyncing %d installed skill(s)...\n", prefix, countInstalled(app.St))
		}

		rep, err := skill.RunSync(opts, app.Cfg, app.St)
		renderSyncReport(rep, scoped, app.St)
		if err != nil {
			return err
		}

		resolveHint := "--force-remote|--force-local|--merge"
		if n := rep.Summary.Conflicts; n > 0 {
			if scoped {
				return fmt.Errorf("%d conflict(s) skipped — resolve with: skillpack sync %s %s", n, resolveHint, opts.Addr)
			}
			return fmt.Errorf("%d conflict(s) skipped — resolve with: skillpack sync %s <addr>", n, resolveHint)
		}
		if dryRun && !scoped && rep.Summary.Errors > 0 {
			return fmt.Errorf("%d skill(s) could not be planned — see errors above", rep.Summary.Errors)
		}
		return nil
	},
}

// renderSyncReport prints a report's notices, one line per row, and the summary.
func renderSyncReport(rep skill.SyncReport, scoped bool, st *state.State) {
	for _, n := range rep.Notices {
		fmt.Printf("  %s\n", n)
	}
	addrW, agentW := 5, 5
	for _, r := range rep.Rows {
		addrW = maxInt(addrW, len(r.Addr))
		agentW = maxInt(agentW, len(r.AgentName))
	}
	for _, r := range rep.Rows {
		if text := syncRowText(r, rep.DryRun, scoped); text != "" {
			fmt.Printf("  %-*s  %-*s  %s\n", addrW, r.Addr, agentW, r.AgentName, text)
		}
		if r.Warning != "" {
			fmt.Printf("  %-*s  %-*s  %s\n", addrW, "", agentW, "", yellow("warning: "+r.Warning))
		}
	}
	printSyncSummary(rep.Summary, addrW, agentW, st)
}

// syncRowText is the status text for one row; empty when the row prints nothing.
func syncRowText(r skill.SyncRow, dryRun, scoped bool) string {
	switch {
	case r.Err != nil && (r.Resolved == skill.ResolveMerge || r.Resolved == skill.ResolveLLM):
		return fmt.Sprintf("merge error: %v", r.Err)
	case r.Err != nil:
		return fmt.Sprintf("error: %v", r.Err)
	case r.Action == skill.SyncUpdated && dryRun:
		return "[dry-run] would update"
	case r.Action == skill.SyncUpdated:
		return green("updated")
	case r.Action == skill.SyncPublished && dryRun:
		return "[dry-run] would push"
	case r.Action == skill.SyncPublished:
		return green("pushed")
	case r.Action != skill.SyncConflict:
		return ""
	}
	switch {
	case r.MergeConflicts:
		return yellow("merged — conflicts written, resolve manually or use --llm")
	case r.LLMResolved:
		return green("merged + LLM resolved")
	case r.Resolved == skill.ResolveMerge || r.Resolved == skill.ResolveLLM:
		return green("merged cleanly")
	case r.Resolved == skill.ResolveForceRemote:
		return green("force-remote applied")
	case r.Resolved == skill.ResolveForceLocal:
		return green("force-local applied (pushed to remote)")
	case !scoped:
		return red("CONFLICT — resolve manually")
	case dryRun:
		return "[dry-run] CONFLICT — would need resolution"
	default:
		return red("CONFLICT — resolve with --force-remote, --force-local, or --merge")
	}
}

// printSyncSummary prints the totals line followed by the stale-address and
// broken-upstream remediation sections.
func printSyncSummary(sum skill.SyncSummary, addrW, agentW int, st *state.State) {
	fmt.Printf("\n  %d updated, %d pushed, %d already current", sum.Updated, sum.Published, sum.Current)
	if sum.Conflicts > 0 {
		fmt.Printf(", %d conflict(s)", sum.Conflicts)
	}
	if sum.MergedWithConflicts > 0 {
		fmt.Printf(", %d merged with conflicts", sum.MergedWithConflicts)
	}
	if sum.Errors > 0 {
		fmt.Printf(", %d error(s)", sum.Errors)
	}
	fmt.Println()
	printStaleSection(sum.Stale, addrW, agentW, st)
	printBrokenUpstreamSection(sum.BrokenUpstream, addrW, agentW)
}

// printBrokenUpstreamSection prints the broken-upstream-pointer remediation block
// shared by the sync output paths. rows holds (addr, agent) pairs where upstream
// tracking was disabled because the upstream skill path no longer exists.
// It prints nothing when rows is empty.
func printBrokenUpstreamSection(rows []skill.SyncKey, addrW, agentW int) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("\n  %s\n", yellow(fmt.Sprintf("%d broken upstream pointer(s) — upstream skill path no longer exists:", len(rows))))
	for _, row := range rows {
		fmt.Printf("    %-*s  %-*s\n", addrW, row.Addr, agentW, row.AgentName)
	}
	fmt.Printf("\n  Upstream tracking was disabled for this run; the installed skill was synced against its own repo.\n")
	fmt.Printf("  To re-point upstream tracking:  skillpack relink <fork-addr> --set-upstream <new-upstream-addr>\n")
	fmt.Printf("  To disable tracking permanently: skillpack relink <fork-addr> --clear-upstream\n")
}

// printStaleSection prints the stale-address remediation block shared by the
// sync output paths. rows holds the (addr, agent) pairs whose skill path no
// longer exists upstream. For each stale mapping it surfaces likely replacement
// addresses found in registered repos and the relink command to repair it. It
// prints nothing when rows is empty.
func printStaleSection(rows []skill.SyncKey, addrW, agentW int, st *state.State) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("\n  %s\n", yellow(fmt.Sprintf("%d stale skill address(es) — skill path no longer exists upstream:", len(rows))))

	// Cache suggestions per address so we don't re-scan repos for the same addr
	// when it is stale across multiple agents.
	suggestions := make(map[string][]string)
	for _, s := range rows {
		addr := s.Addr
		fmt.Printf("    %-*s  %-*s\n", addrW, addr, agentW, s.AgentName)
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
