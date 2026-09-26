package state_test

import (
	"testing"

	"github.com/bmaltais/skillpack/internal/state"
)

func TestMemoryStore_SaveDoesNotTouchDisk(t *testing.T) {
	st, mem := state.NewMemory()
	if err := st.RecordInstall("r/s", "claude-code", state.InstalledSkillRecord{InstalledHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(st); err != nil {
		t.Fatal(err)
	}
	if mem.Saves() != 1 {
		t.Fatalf("Saves() = %d, want 1", mem.Saves())
	}
	if got := mem.Last().InstalledSkills["r/s"]["claude-code"].InstalledHash; got != "h" {
		t.Errorf("last snapshot hash = %q, want h", got)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.InstalledSkills) != 0 {
		t.Error("memory store leaked to state.json")
	}
}

func TestMemoryStore_SnapshotIsIsolated(t *testing.T) {
	st, mem := state.NewMemory()
	_ = st.RecordInstall("r/s", "a", state.InstalledSkillRecord{InstalledHash: "1"})
	_ = st.Save()
	_ = st.RecordHash("r/s", "a", "2")
	if got := mem.Last().InstalledSkills["r/s"]["a"].InstalledHash; got != "1" {
		t.Errorf("snapshot mutated after Save: %q", got)
	}
}

func TestFileStore_SaveMethodRoundTrips(t *testing.T) {
	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	_ = st.RecordInstall("r/s", "a", state.InstalledSkillRecord{InstalledHash: "x"})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if again.InstalledSkills["r/s"]["a"].InstalledHash != "x" {
		t.Error("file round trip lost record")
	}
}

func TestClone_IsDeepAndKeepsStore(t *testing.T) {
	st, mem := state.NewMemory()
	_ = st.RecordInstall("r/s", "a", state.InstalledSkillRecord{InstalledHash: "1"})
	_ = st.RecordPackInstall("p", state.InstalledPackRecord{
		Agents: []string{"a"},
		Skills: map[string]map[string]state.PackSkillStatus{"r/s": {"a": {Installed: true}}},
	})
	c := st.Clone()
	_ = c.RecordHash("r/s", "a", "2")
	c.InstalledPacks["p"].Skills["r/s"]["a"] = state.PackSkillStatus{Error: "boom"}
	if st.InstalledSkills["r/s"]["a"].InstalledHash != "1" || !st.InstalledPacks["p"].Skills["r/s"]["a"].Installed {
		t.Fatal("Clone shares memory with source")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if mem.Saves() != 1 {
		t.Error("clone should save through the same store")
	}
}
