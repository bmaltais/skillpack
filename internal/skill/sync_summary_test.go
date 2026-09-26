package skill

import (
	"errors"
	"testing"
)

func TestSummarizeRows_Classifies(t *testing.T) {
	rows := []SyncRow{
		{SyncResult: SyncResult{Addr: "r/a", AgentName: "x", Action: SyncUpdated}},
		{SyncResult: SyncResult{Addr: "r/b", AgentName: "x", Action: SyncPublished}},
		{SyncResult: SyncResult{Addr: "r/c", AgentName: "x", Action: SyncAlreadyCurrent}},
		{SyncResult: SyncResult{Addr: "r/d", AgentName: "x", Err: errors.New("boom")}},
		{SyncResult: SyncResult{Addr: "r/e", AgentName: "y", Action: SyncStaleAddress}},
		{SyncResult: SyncResult{Addr: "r/f", AgentName: "y", Action: SyncUpdated, UpstreamPathBroken: true}},
		{SyncResult: SyncResult{Addr: "r/g", AgentName: "x", Action: SyncConflict, UpstreamPathBroken: true}},
		{SyncResult: SyncResult{Addr: "r/h", AgentName: "x", Action: SyncConflict}, Resolved: ResolveMerge},
		{SyncResult: SyncResult{Addr: "r/i", AgentName: "x", Action: SyncConflict}, Resolved: ResolveMerge, MergeConflicts: true},
		{SyncResult: SyncResult{Addr: "r/j", AgentName: "x", Action: SyncConflict}, Resolved: ResolveLLM, LLMResolved: true},
		{SyncResult: SyncResult{Addr: "r/k", AgentName: "x", Action: SyncConflict}, Resolved: ResolveMerge},
	}
	s := summarizeRows(rows)
	if s.Updated != 2 || s.Published != 2 || s.Current != 1 || s.Errors != 1 {
		t.Errorf("counts = %+v", s)
	}
	if s.Conflicts != 1 || s.MergedWithConflicts != 1 {
		t.Errorf("conflict counts = %+v", s)
	}
	if len(s.Stale) != 1 || s.Stale[0] != (SyncKey{"r/e", "y"}) {
		t.Errorf("Stale = %v", s.Stale)
	}
	if len(s.BrokenUpstream) != 2 {
		t.Errorf("BrokenUpstream = %v (conflicting items must be included)", s.BrokenUpstream)
	}
}

func TestRowsFor_ConflictsListedOnce(t *testing.T) {
	results := []SyncResult{
		{Addr: "r/b", AgentName: "x", Action: SyncUpdated},
		{Addr: "r/a", AgentName: "x", Action: SyncConflict}, // second-pass duplicate
	}
	conflicts := []SyncResult{{Addr: "r/a", AgentName: "x", Action: SyncConflict}}
	rows := rowsFor(results, conflicts)
	if len(rows) != 2 || rows[0].Addr != "r/b" || rows[1].Action != SyncConflict {
		t.Errorf("rows = %+v", rows)
	}
	if got := summarizeRows(rows).Conflicts; got != 1 {
		t.Errorf("Conflicts = %d, want 1", got)
	}
}
