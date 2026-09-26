package skill

// SyncKey identifies one installed skill for one agent.
type SyncKey struct {
	Addr      string
	AgentName string
}

// SyncSummary is the classified tally of a sync run (real or dry-run). It is the
// single owner of the classification rules so CLI and TUI only render it.
type SyncSummary struct {
	Updated   int
	Published int
	Current   int
	// Conflicts counts conflicts left unresolved.
	Conflicts int
	// MergedWithConflicts counts conflicts a merge resolved only partially:
	// conflict markers were written and need manual attention.
	MergedWithConflicts int
	Errors              int
	// Stale lists skills whose path no longer exists upstream.
	Stale []SyncKey
	// BrokenUpstream lists skills whose upstream tracking was disabled because
	// the upstream skill path no longer exists.
	BrokenUpstream []SyncKey
}

// summarizeRows classifies every row of a report into a SyncSummary.
func summarizeRows(rows []SyncRow) SyncSummary {
	var s SyncSummary
	for _, r := range rows {
		s.record(r)
	}
	return s
}

func (s *SyncSummary) record(r SyncRow) {
	switch {
	case r.Err != nil:
		s.Errors++
	case r.Action == SyncConflict && r.Resolved == "":
		s.Conflicts++
	case r.Action == SyncConflict && r.MergeConflicts:
		s.MergedWithConflicts++
	case r.Action == SyncConflict && r.LLMResolved:
		s.Published++
	case r.Action == SyncUpdated:
		s.Updated++
	case r.Action == SyncPublished:
		s.Published++
	case r.Action == SyncAlreadyCurrent:
		s.Current++
	case r.Action == SyncStaleAddress:
		s.Stale = append(s.Stale, SyncKey{r.Addr, r.AgentName})
	}
	if r.UpstreamPathBroken {
		s.BrokenUpstream = append(s.BrokenUpstream, SyncKey{r.Addr, r.AgentName})
	}
}

// Result converts a plan item to the result it would produce if applied successfully.
func (p SyncPlanItem) Result() SyncResult {
	return SyncResult{
		Addr: p.Addr, AgentName: p.AgentName, Action: p.Action, Err: p.Err,
		Warning: p.Warning, UpstreamPathBroken: p.UpstreamPathBroken,
	}
}
