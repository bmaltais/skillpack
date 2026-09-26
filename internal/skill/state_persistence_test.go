package skill

// Tests that internal mutating functions persist state to disk themselves,
// without any explicit state.Save call at the cmd layer.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// setupHome redirects os.UserHomeDir() to a temporary directory for the
// duration of the test by setting $HOME. Returns the fake home path.
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// readStateFromDisk reads and decodes state.json from the fake home.
func readStateFromDisk(t *testing.T, home string) *state.State {
	t.Helper()
	p := filepath.Join(home, ".skillpack", "state.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading state.json from disk: %v", err)
	}
	var st state.State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("decoding state.json: %v", err)
	}
	return &st
}

// TestInstall_PersistsStateToDisk verifies that skill.Install writes state.json
// to disk without any explicit state.Save call in the test.
func TestInstall_PersistsStateToDisk(t *testing.T) {
	home := setupHome(t)

	// Build a fake git repo cache containing one skill.
	cacheDir := t.TempDir()
	skillRelPath := "my-skill"
	writeFile(t, filepath.Join(cacheDir, skillRelPath, "SKILL.md"), "# My Skill")
	_, _ = initRepoWithCommit(t, cacheDir, "initial commit")

	// Agent skill dir
	agentSkillDir := t.TempDir()

	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"test-agent": {SkillDir: agentSkillDir},
		},
	}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"test-repo": {CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	addr := "test-repo/my-skill"
	if err := Install(addr, "test-agent", cfg, st, false); err != nil {
		t.Fatalf("skill.Install: %v", err)
	}

	// Verify: state.json must now exist on disk — no cmd-layer save was called.
	onDisk := readStateFromDisk(t, home)

	if _, ok := onDisk.InstalledSkills[addr]["test-agent"]; !ok {
		t.Errorf("state.json on disk does not contain installed record for %q / test-agent", addr)
	}
}

// TestRemove_PersistsStateToDisk verifies that skill.Remove writes state.json
// to disk without any explicit state.Save call in the test.
func TestRemove_PersistsStateToDisk(t *testing.T) {
	home := setupHome(t)

	// Pre-populate an installed skill dir on disk.
	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# My Skill")

	hash, err := ComputeHash(installedDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	addr := "test-repo/my-skill"
	st := &state.State{
		Repos: map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"test-agent": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
		},
	}

	// Write initial state so disk has a baseline before the Remove.
	if err := state.Save(st); err != nil {
		t.Fatalf("saving initial state: %v", err)
	}

	cfg := &config.Config{}

	if err := remove(addr, "test-agent", cfg, st, false); err != nil {
		t.Fatalf("skill.Remove: %v", err)
	}

	// Verify: state.json on disk must reflect the removal.
	onDisk := readStateFromDisk(t, home)

	if _, ok := onDisk.InstalledSkills[addr]; ok {
		t.Errorf("state.json on disk still contains removed record for %q", addr)
	}
}

// TestRepoRemove_PersistsStateToDisk verifies that repo.Remove writes state.json
// to disk without any explicit state.Save call in the test.
func TestRepoRemove_PersistsStateToDisk(t *testing.T) {
	home := setupHome(t)

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {URL: "https://example.com/repo.git", CachePath: "/tmp/repo"},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	// Write initial state to disk.
	if err := state.Save(st); err != nil {
		t.Fatalf("saving initial state: %v", err)
	}

	if err := repo.Remove("my-repo", st); err != nil {
		t.Fatalf("repo.Remove: %v", err)
	}

	// Verify: state.json on disk no longer has the repo.
	onDisk := readStateFromDisk(t, home)

	if _, ok := onDisk.Repos["my-repo"]; ok {
		t.Errorf("state.json on disk still contains removed repo %q", "my-repo")
	}
}

// assertNotOnDisk fails if state.json was written under the fake home.
func assertNotOnDisk(t *testing.T, home string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, ".skillpack", "state.json")); err == nil {
		t.Error("state.json was written; a memory-backed state must not touch disk")
	}
}

func TestInstall_SavesExactlyOnce(t *testing.T) {
	home := setupHome(t)
	cacheDir := t.TempDir()
	writeFile(t, filepath.Join(cacheDir, "my-skill", "SKILL.md"), "# My Skill")
	_, _ = initRepoWithCommit(t, cacheDir, "initial commit")
	cfg := &config.Config{Agents: map[string]config.AgentConfig{"test-agent": {SkillDir: t.TempDir()}}}
	st, mem := state.NewMemoryFrom(&state.State{
		Repos: map[string]state.RepoRecord{"test-repo": {CachePath: cacheDir}},
	})

	if err := Install("test-repo/my-skill", "test-agent", cfg, st, false); err != nil {
		t.Fatal(err)
	}
	if mem.Saves() != 1 {
		t.Errorf("Saves() = %d, want 1", mem.Saves())
	}
	if _, ok := mem.Last().InstalledSkills["test-repo/my-skill"]["test-agent"]; !ok {
		t.Error("saved snapshot lacks the install record")
	}
	assertNotOnDisk(t, home)
}

func removeFixture(t *testing.T) (*state.State, *state.MemoryStore, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# My Skill")
	hash, err := ComputeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	addr := "test-repo/my-skill"
	st, mem := state.NewMemoryFrom(&state.State{
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {"test-agent": {InstalledHash: hash, LocalPath: dir}},
		},
		InstalledPacks: map[string]state.InstalledPackRecord{
			"test-repo/packs/p": {
				PackAddress: "test-repo/packs/p",
				Agents:      []string{"test-agent"},
				Skills:      map[string]map[string]state.PackSkillStatus{addr: {"test-agent": {Installed: true}}},
			},
		},
	})
	return st, mem, addr
}

func TestRemove_SavesExactlyOnce(t *testing.T) {
	home := setupHome(t)
	st, mem, addr := removeFixture(t)
	if err := remove(addr, "test-agent", &config.Config{}, st, false); err != nil {
		t.Fatal(err)
	}
	if mem.Saves() != 1 {
		t.Errorf("Saves() = %d, want 1", mem.Saves())
	}
	if len(mem.Last().InstalledSkills) != 0 {
		t.Error("saved snapshot still has the skill")
	}
	assertNotOnDisk(t, home)
}

func TestRemoveDirect_MarksOwningPacksAndSavesOnce(t *testing.T) {
	home := setupHome(t)
	st, mem, addr := removeFixture(t)
	is := InstalledSkill{Addr: addr, AgentName: "test-agent", cfg: &config.Config{}, st: st}

	marked, err := is.RemoveDirect(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(marked) != 1 || marked[0] != "test-repo/packs/p" {
		t.Errorf("marked = %v", marked)
	}
	if mem.Saves() != 1 {
		t.Errorf("Saves() = %d, want 1", mem.Saves())
	}
	got := mem.Last().InstalledPacks["test-repo/packs/p"].Skills[addr]["test-agent"]
	if got.Installed || got.Error == "" {
		t.Errorf("pack skill status not marked missing in saved snapshot: %+v", got)
	}
	assertNotOnDisk(t, home)
}
