package state_test

import (
	"testing"
	"time"

	"github.com/bmaltais/skillpack/internal/state"
)

// ─── RecordPackInstall / RecordPackRemove ─────────────────────────────────────

func TestRecordPackInstall_StoresRecord(t *testing.T) {
	st := emptyState()
	rec := state.InstalledPackRecord{
		PackAddress: "my-repo/packs/go-dev",
		InstalledAt: time.Now(),
		Agents:      []string{"claude-code"},
		Skills: map[string]map[string]state.PackSkillStatus{
			"my-repo/coding/debugger": {
				"claude-code": {Installed: true},
			},
		},
	}
	if err := st.RecordPackInstall("my-repo/packs/go-dev", rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stored, ok := st.InstalledPacks["my-repo/packs/go-dev"]
	if !ok {
		t.Fatal("pack record not stored")
	}
	if stored.PackAddress != "my-repo/packs/go-dev" {
		t.Errorf("PackAddress = %q", stored.PackAddress)
	}
	if len(stored.Agents) != 1 || stored.Agents[0] != "claude-code" {
		t.Errorf("Agents = %v", stored.Agents)
	}
}

func TestRecordPackInstall_EmptyAddrError(t *testing.T) {
	st := emptyState()
	if err := st.RecordPackInstall("", state.InstalledPackRecord{}); err == nil {
		t.Error("want error for empty packAddr, got nil")
	}
}

func TestRecordPackInstall_ReplacesExisting(t *testing.T) {
	st := emptyState()
	_ = st.RecordPackInstall("my-repo/packs/go-dev", state.InstalledPackRecord{PackAddress: "first"})
	_ = st.RecordPackInstall("my-repo/packs/go-dev", state.InstalledPackRecord{PackAddress: "second"})
	if got := st.InstalledPacks["my-repo/packs/go-dev"].PackAddress; got != "second" {
		t.Errorf("want second, got %q", got)
	}
}

func TestRecordPackRemove_DeletesRecord(t *testing.T) {
	st := emptyState()
	_ = st.RecordPackInstall("my-repo/packs/go-dev", state.InstalledPackRecord{PackAddress: "my-repo/packs/go-dev"})
	if err := st.RecordPackRemove("my-repo/packs/go-dev"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := st.InstalledPacks["my-repo/packs/go-dev"]; ok {
		t.Error("pack record should be removed")
	}
}

func TestRecordPackRemove_EmptyAddrError(t *testing.T) {
	st := emptyState()
	if err := st.RecordPackRemove(""); err == nil {
		t.Error("want error for empty packAddr, got nil")
	}
}

func TestRecordPackRemove_NoopWhenMissing(t *testing.T) {
	st := emptyState()
	if err := st.RecordPackRemove("nonexistent/pack"); err != nil {
		t.Fatalf("unexpected error for missing pack: %v", err)
	}
}

// ─── MarkPackSkillMissing ─────────────────────────────────────────────────────

func TestMarkPackSkillMissing_MarksSkillNotInstalled(t *testing.T) {
	st := emptyState()
	_ = st.RecordPackInstall("my-repo/packs/go-dev", state.InstalledPackRecord{
		PackAddress: "my-repo/packs/go-dev",
		Skills: map[string]map[string]state.PackSkillStatus{
			"my-repo/coding/debugger": {
				"claude-code": {Installed: true},
			},
		},
	})

	st.MarkPackSkillMissing("my-repo/packs/go-dev", "my-repo/coding/debugger", "claude-code", "directly removed by user")

	rec := st.InstalledPacks["my-repo/packs/go-dev"]
	s := rec.Skills["my-repo/coding/debugger"]["claude-code"]
	if s.Installed {
		t.Error("skill should be marked not installed")
	}
	if s.Error != "directly removed by user" {
		t.Errorf("Error = %q, want %q", s.Error, "directly removed by user")
	}
}

func TestMarkPackSkillMissing_NoopWhenPackMissing(t *testing.T) {
	st := emptyState()
	// Should not panic when pack doesn't exist.
	st.MarkPackSkillMissing("nonexistent/pack", "some/skill", "claude-code", "test")
}

func TestMarkPackSkillMissing_CreatesSkillEntry(t *testing.T) {
	st := emptyState()
	_ = st.RecordPackInstall("my-repo/packs/go-dev", state.InstalledPackRecord{
		PackAddress: "my-repo/packs/go-dev",
		Skills:      map[string]map[string]state.PackSkillStatus{},
	})

	// Marking a skill that wasn't in the pack should create the entry.
	st.MarkPackSkillMissing("my-repo/packs/go-dev", "my-repo/coding/linter", "claude-code", "orphaned")

	rec := st.InstalledPacks["my-repo/packs/go-dev"]
	s := rec.Skills["my-repo/coding/linter"]["claude-code"]
	if s.Installed {
		t.Error("skill should be marked not installed")
	}
}

// ─── FindPacksOwningSkill ─────────────────────────────────────────────────────

func TestFindPacksOwningSkill_FindsOwner(t *testing.T) {
	st := emptyState()
	_ = st.RecordPackInstall("my-repo/packs/go-dev", state.InstalledPackRecord{
		PackAddress: "my-repo/packs/go-dev",
		Skills: map[string]map[string]state.PackSkillStatus{
			"my-repo/coding/debugger": {
				"claude-code": {Installed: true},
			},
		},
	})

	packs := st.FindPacksOwningSkill("my-repo/coding/debugger")
	if len(packs) != 1 || packs[0] != "my-repo/packs/go-dev" {
		t.Errorf("FindPacksOwningSkill = %v, want [my-repo/packs/go-dev]", packs)
	}
}

func TestFindPacksOwningSkill_ReturnsNilWhenNone(t *testing.T) {
	st := emptyState()
	packs := st.FindPacksOwningSkill("my-repo/coding/unknown")
	if len(packs) != 0 {
		t.Errorf("expected empty, got %v", packs)
	}
}

func TestFindPacksOwningSkill_FindsMultipleOwners(t *testing.T) {
	st := emptyState()
	_ = st.RecordPackInstall("my-repo/packs/pack-a", state.InstalledPackRecord{
		PackAddress: "my-repo/packs/pack-a",
		Skills: map[string]map[string]state.PackSkillStatus{
			"my-repo/coding/debugger": {"agent1": {Installed: true}},
		},
	})
	_ = st.RecordPackInstall("my-repo/packs/pack-b", state.InstalledPackRecord{
		PackAddress: "my-repo/packs/pack-b",
		Skills: map[string]map[string]state.PackSkillStatus{
			"my-repo/coding/debugger": {"agent1": {Installed: true}},
		},
	})

	packs := st.FindPacksOwningSkill("my-repo/coding/debugger")
	if len(packs) != 2 {
		t.Errorf("expected 2 owning packs, got %d: %v", len(packs), packs)
	}
}

// ─── isPackPartial helper (via round-trip test) ───────────────────────────────

func TestInstalledPacks_PartialDetection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := emptyState()
	st.InstalledPacks = make(map[string]state.InstalledPackRecord)
	st.InstalledPacks["my-repo/packs/mixed"] = state.InstalledPackRecord{
		PackAddress: "my-repo/packs/mixed",
		InstalledAt: time.Now(),
		Agents:      []string{"claude-code"},
		Skills: map[string]map[string]state.PackSkillStatus{
			"my-repo/coding/debugger": {
				"claude-code": {Installed: true},
			},
			"my-repo/coding/linter": {
				"claude-code": {Installed: false, Error: "auth failed"},
			},
		},
	}

	if err := state.Save(st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	rec := loaded.InstalledPacks["my-repo/packs/mixed"]
	// Verify per-skill-per-agent access.
	if !rec.Skills["my-repo/coding/debugger"]["claude-code"].Installed {
		t.Error("debugger should be installed")
	}
	if rec.Skills["my-repo/coding/linter"]["claude-code"].Installed {
		t.Error("linter should NOT be installed")
	}
	if rec.Skills["my-repo/coding/linter"]["claude-code"].Error != "auth failed" {
		t.Errorf("Error = %q", rec.Skills["my-repo/coding/linter"]["claude-code"].Error)
	}
}
