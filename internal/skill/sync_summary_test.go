package skill

import (
	"errors"
	"testing"
)

func TestSummarize_ClassifiesResults(t *testing.T) {
	results := []SyncResult{
		{Addr: "r/a", AgentName: "x", Action: SyncUpdated},
		{Addr: "r/b", AgentName: "x", Action: SyncPublished},
		{Addr: "r/c", AgentName: "x", Action: SyncAlreadyCurrent},
		{Addr: "r/d", AgentName: "x", Action: SyncAlreadyCurrent, Err: errors.New("boom")},
		{Addr: "r/e", AgentName: "y", Action: SyncStaleAddress},
		{Addr: "r/f", AgentName: "y", Action: SyncUpdated, UpstreamPathBroken: true},
		// second-pass conflict appears in results and conflicts; counted once.
		{Addr: "r/g", AgentName: "x", Action: SyncConflict},
	}
	conflicts := []SyncResult{
		{Addr: "r/h", AgentName: "x", Action: SyncConflict},
		{Addr: "r/g", AgentName: "x", Action: SyncConflict},
	}
	s := Summarize(results, conflicts)

	if s.Updated != 2 || s.Published != 1 || s.Current != 1 || s.Errors != 1 || s.Conflicts != 2 {
		t.Errorf("counts = %+v", s)
	}
	if len(s.Stale) != 1 || s.Stale[0] != (SyncKey{"r/e", "y"}) {
		t.Errorf("Stale = %v", s.Stale)
	}
	if len(s.BrokenUpstream) != 1 || s.BrokenUpstream[0] != (SyncKey{"r/f", "y"}) {
		t.Errorf("BrokenUpstream = %v", s.BrokenUpstream)
	}
}

func TestSummarizePlan_SplitsConflicts(t *testing.T) {
	plan := []SyncPlanItem{
		{Addr: "r/a", AgentName: "x", Action: SyncUpdated},
		{Addr: "r/b", AgentName: "x", Action: SyncConflict, UpstreamPathBroken: true},
		{Addr: "r/c", AgentName: "x", Err: errors.New("no repo")},
		{Addr: "r/d", AgentName: "x", Action: SyncStaleAddress},
	}
	s := SummarizePlan(plan)
	if s.Updated != 1 || s.Conflicts != 1 || s.Errors != 1 || len(s.Stale) != 1 {
		t.Errorf("summary = %+v", s)
	}
	if len(s.BrokenUpstream) != 1 || s.BrokenUpstream[0] != (SyncKey{"r/b", "x"}) {
		t.Errorf("conflicting item lost from BrokenUpstream: %v", s.BrokenUpstream)
	}
}

func TestSyncSummary_RecordAccumulates(t *testing.T) {
	var s SyncSummary
	s.Record(SyncResult{Action: SyncPublished})
	s.Record(SyncResult{Err: errors.New("x")})
	if s.Published != 1 || s.Errors != 1 {
		t.Errorf("summary = %+v", s)
	}
}
