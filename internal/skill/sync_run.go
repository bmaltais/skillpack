package skill

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// SyncRow is one line of a sync report: the outcome for one installed skill.
type SyncRow struct {
	SyncResult
	// Resolved is the strategy applied to a SyncConflict row; empty when the
	// conflict was left unresolved.
	Resolved ResolveStrategy
	// MergeConflicts is true when a merge left conflict markers to resolve by hand.
	MergeConflicts bool
	// LLMResolved is true when the LLM pipeline resolved merge conflicts.
	LLMResolved bool
}

// SyncReport is the structured outcome of a sync run. Callers render it and
// never re-tally it.
type SyncReport struct {
	// DryRun is true when nothing was applied; rows show what would happen.
	DryRun bool
	// Rows are sorted by address then agent, with conflicts last.
	Rows []SyncRow
	// Notices are non-fatal messages (e.g. a repo that could not be pulled).
	Notices []string
	Summary SyncSummary
}

// SyncOptions selects what RunSync does.
type SyncOptions struct {
	// Addr limits the run to one installed skill; empty syncs every skill.
	Addr string
	// DryRun plans without pulling or applying.
	DryRun bool
	// Resolve is applied to conflicts; empty leaves them unresolved. Force
	// strategies are ignored unless Addr names a skill.
	Resolve ResolveStrategy
	// LLMAgent names the agent used when Resolve is ResolveLLM.
	LLMAgent string
}

// Sync pulls every registered repo, reconciles all installed skills and applies
// the result (see syncAll), returning the classified report.
func Sync(tokenFor func(string) string, st *state.State) (SyncReport, error) {
	results, conflicts, notices, err := syncAll(tokenFor, st)
	rep := SyncReport{Notices: notices, Rows: rowsFor(results, conflicts)}
	rep.Summary = summarizeRows(rep.Rows)
	return rep, err
}

// RunSync performs two-way reconciliation per opts and returns a report. On a
// fatal error the report holds the rows produced so far.
func RunSync(opts SyncOptions, cfg *config.Config, st *state.State) (SyncReport, error) {
	var (
		rep SyncReport
		err error
	)
	rep.DryRun = opts.DryRun
	if cfg == nil {
		cfg = &config.Config{}
	}
	if opts.Addr == "" && (opts.Resolve == ResolveForceRemote || opts.Resolve == ResolveForceLocal) {
		// Force strategies overwrite one side wholesale; only ever on a named skill.
		opts.Resolve = ""
	}
	switch {
	case opts.Addr != "":
		err = syncOne(opts, cfg, st, &rep)
	case opts.DryRun:
		err = syncPlanOnly(st, &rep)
	default:
		err = syncEverything(opts, cfg, st, &rep)
	}
	rep.Summary = summarizeRows(rep.Rows)
	return rep, err
}

func syncPlanOnly(st *state.State, rep *SyncReport) error {
	heads, err := CollectRepoHeads(st)
	if err != nil {
		return err
	}
	for _, p := range ReconcilePlan(st, heads) {
		rep.Rows = append(rep.Rows, SyncRow{SyncResult: p.Result()})
	}
	sortRows(rep.Rows)
	return nil
}

func syncEverything(opts SyncOptions, cfg *config.Config, st *state.State, rep *SyncReport) error {
	results, conflicts, notices, err := syncAll(cfg.TokenForRepo, st)
	rep.Notices = notices
	if err != nil {
		return err
	}
	rep.Rows = rowsFor(results, conflicts)
	for i, r := range rep.Rows {
		if r.Action != SyncConflict || opts.Resolve == "" {
			continue
		}
		row, fatal := resolveConflict(r, opts, cfg, st)
		rep.Rows[i] = row
		if fatal != nil {
			return fatal
		}
	}
	return nil
}

func syncOne(opts SyncOptions, cfg *config.Config, st *state.State, rep *SyncReport) error {
	addr := opts.Addr
	if _, ok := st.InstalledSkills[addr]; !ok {
		return fmt.Errorf("skill %q is not installed", addr)
	}
	repoName := strings.SplitN(addr, "/", 2)[0]
	token := cfg.TokenForRepo(repoName)

	if !opts.DryRun {
		if warn, pullErr := repo.Update(repoName, token, st); pullErr != nil {
			rep.Notices = append(rep.Notices, fmt.Sprintf("warning: could not pull %s: %v", repoName, pullErr))
		} else if warn != "" {
			rep.Notices = append(rep.Notices, "notice: "+warn)
		}
	}

	heads, err := CollectRepoHeads(st)
	if err != nil {
		return err
	}
	var plan []SyncPlanItem
	for _, p := range ReconcilePlan(st, heads) {
		if p.Addr == addr {
			plan = append(plan, p)
		}
	}
	sort.Slice(plan, func(i, j int) bool { return plan[i].AgentName < plan[j].AgentName })

	for _, p := range plan {
		row := SyncRow{SyncResult: p.Result()}
		switch {
		case p.Err != nil:
		case p.Action == SyncConflict && !opts.DryRun && opts.Resolve != "":
			var fatal error
			row, fatal = resolveConflict(row, opts, cfg, st)
			if fatal != nil {
				rep.Rows = append(rep.Rows, row)
				return fatal
			}
		case (p.Action == SyncUpdated || p.Action == SyncPublished) && !opts.DryRun:
			results, _, applyErr := ApplySync([]SyncPlanItem{p}, cfg.TokenForRepo, st)
			if applyErr != nil {
				return applyErr
			}
			if len(results) > 0 {
				row.SyncResult = results[0]
			}
		}
		rep.Rows = append(rep.Rows, row)
	}
	return nil
}

// resolveConflict applies opts.Resolve to a conflict row. Per-skill failures
// land in the row's Err; a failure of a force strategy is also returned as fatal.
func resolveConflict(row SyncRow, opts SyncOptions, cfg *config.Config, st *state.State) (SyncRow, error) {
	row.Resolved = opts.Resolve
	is, err := Open(row.Addr, row.AgentName, cfg, st)
	if err != nil {
		row.Err = err
		return row, nil
	}
	token := cfg.TokenForRepo(strings.SplitN(row.Addr, "/", 2)[0])
	llmResolved, err := is.Resolve(opts.Resolve, token, opts.LLMAgent)
	switch {
	case errors.Is(err, ErrMergeConflicts):
		row.MergeConflicts = true
	case err != nil && (opts.Resolve == ResolveForceRemote || opts.Resolve == ResolveForceLocal):
		row.Err = err
		return row, err
	case err != nil:
		row.Err = err
	default:
		row.LLMResolved = llmResolved
	}
	return row, nil
}

// rowsFor builds report rows from Sync output: results sorted, then the
// conflicts. A second-pass conflict appears in both slices, so SyncConflict
// results are skipped and the conflicts slice is the only source of those rows.
func rowsFor(results, conflicts []SyncResult) []SyncRow {
	rows := make([]SyncRow, 0, len(results)+len(conflicts))
	for _, r := range results {
		if r.Action != SyncConflict || r.Err != nil {
			rows = append(rows, SyncRow{SyncResult: r})
		}
	}
	sortRows(rows)
	start := len(rows)
	for _, c := range conflicts {
		rows = append(rows, SyncRow{SyncResult: c})
	}
	sortRows(rows[start:])
	return rows
}

func sortRows(rows []SyncRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Addr != rows[j].Addr {
			return rows[i].Addr < rows[j].Addr
		}
		return rows[i].AgentName < rows[j].AgentName
	})
}
