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
	Conflicts int
	Errors    int
	// Stale lists skills whose path no longer exists upstream.
	Stale []SyncKey
	// BrokenUpstream lists skills whose upstream tracking was disabled because
	// the upstream skill path no longer exists.
	BrokenUpstream []SyncKey
}

// Record classifies one result into the summary. SyncConflict results are not
// counted here: Sync returns conflicts in a separate slice (see Summarize), and
// callers that resolve a conflict inline decide themselves whether it stays one.
func (s *SyncSummary) Record(r SyncResult) {
	key := SyncKey{r.Addr, r.AgentName}
	switch {
	case r.Err != nil:
		s.Errors++
	case r.Action == SyncUpdated:
		s.Updated++
	case r.Action == SyncPublished:
		s.Published++
	case r.Action == SyncAlreadyCurrent:
		s.Current++
	case r.Action == SyncStaleAddress:
		s.Stale = append(s.Stale, key)
	}
	if r.UpstreamPathBroken {
		s.BrokenUpstream = append(s.BrokenUpstream, key)
	}
}

// Summarize tallies the output of Sync or ApplySync. Conflicts come from the
// conflicts slice only, because a second-pass conflict appears in both slices.
func Summarize(results, conflicts []SyncResult) SyncSummary {
	var s SyncSummary
	for _, r := range results {
		s.Record(r)
	}
	s.Conflicts = len(conflicts)
	return s
}

// SummarizePlan tallies a ReconcilePlan (dry-run): each item counts as if it had
// been applied successfully.
func SummarizePlan(plan []SyncPlanItem) SyncSummary {
	var s SyncSummary
	for _, p := range plan {
		if p.Action == SyncConflict && p.Err == nil {
			s.Conflicts++
			continue
		}
		s.Record(SyncResult{
			Addr: p.Addr, AgentName: p.AgentName, Action: p.Action, Err: p.Err,
			Warning: p.Warning, UpstreamPathBroken: p.UpstreamPathBroken,
		})
	}
	return s
}
