package state_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

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

func TestMemoryStore_LoadReturnsIsolatedSnapshot(t *testing.T) {
	st, mem := state.NewMemory()
	fresh, err := mem.Load()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Repos == nil || fresh.InstalledSkills == nil || fresh.InstalledPacks == nil {
		t.Fatal("Load of never-saved store must return non-nil maps")
	}
	_ = st.RecordInstall("r/s", "a", state.InstalledSkillRecord{InstalledHash: "1"})
	_ = st.Save()
	loaded, err := mem.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InstalledSkills["r/s"]["a"].InstalledHash != "1" {
		t.Fatal("Load lost saved record")
	}
	_ = loaded.RecordHash("r/s", "a", "2")
	if mem.Last().InstalledSkills["r/s"]["a"].InstalledHash != "1" {
		t.Error("mutating a loaded state changed the stored snapshot")
	}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	if mem.Saves() != 2 {
		t.Errorf("loaded state should save through the same store, Saves() = %d", mem.Saves())
	}
}

func TestClone_NilMapsBecomeEmpty(t *testing.T) {
	c := (&state.State{}).Clone()
	if c.Repos == nil || c.InstalledSkills == nil || c.InstalledPacks == nil {
		t.Error("Clone must leave the three top-level maps non-nil")
	}
}

// populate sets every exported field of v (recursively) to a non-zero value,
// so a field added later is covered by the clone test without editing it.
func populate(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(7)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		populate(s.Index(0))
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k := reflect.New(v.Type().Key()).Elem()
		populate(k)
		e := reflect.New(v.Type().Elem()).Elem()
		populate(e)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				populate(v.Field(i))
			}
		}
	default:
		panic("populate: unsupported kind " + v.Kind().String())
	}
}

func TestClone_FullyPopulatedStateClonesEqualAndIndependent(t *testing.T) {
	st, mem := state.NewMemory()
	populate(reflect.ValueOf(st).Elem())

	c := st.Clone()
	want, _ := json.Marshal(st)
	got, _ := json.Marshal(c)
	if string(want) != string(got) {
		t.Fatalf("clone differs from source:\n want %s\n got  %s", want, got)
	}

	// Mutate every nested level of the clone; the source must not change.
	c.Repos["x"] = state.RepoRecord{URL: "changed"}
	c.InstalledSkills["x"]["x"] = state.InstalledSkillRecord{InstalledHash: "changed"}
	pack := c.InstalledPacks["x"]
	pack.Agents[0] = "changed"
	pack.Skills["x"]["x"] = state.PackSkillStatus{Error: "changed"}
	after, _ := json.Marshal(st)
	if string(after) != string(want) {
		t.Fatal("mutating the clone changed the source")
	}

	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if mem.Saves() != 1 {
		t.Error("clone should save through the source's store")
	}
}
