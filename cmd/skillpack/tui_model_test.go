package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

func TestEditTextInputBackspaceRemovesRune(t *testing.T) {
	input := "agenté"
	editTextInput(&input, tea.KeyMsg{Type: tea.KeyBackspace})
	if input != "agent" {
		t.Fatalf("got %q, want %q", input, "agent")
	}
}

// makeGitClone creates a minimal directory that passes the .git sentinel check.
func makeGitClone(t *testing.T, base string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(base, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return base
}

// makeSkillDir creates a skill directory with a SKILL.md file.
func makeSkillDir(t *testing.T, root, relPath string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Test Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- computeSkillProblems tests ---

// TestComputeSkillProblems_HealthySkill verifies a skill that still exists in the
// repo cache is classified as problemNone.
func TestComputeSkillProblems_HealthySkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "repo-a"))
	makeSkillDir(t, cacheDir, "coding/debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {URL: "https://example.com/repo-a.git", CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo-a/coding/debugger": {
				"copilot": {InstalledAtSHA: "abc", InstalledHash: "def"},
			},
		},
	}

	discovered := map[string]bool{"repo-a/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if p, ok := got["repo-a/coding/debugger"]; ok && p != problemNone {
		t.Errorf("expected problemNone for healthy skill, got %v", p)
	}
}

// TestComputeSkillProblems_StaleSkill verifies a skill absent from the discovered
// set is classified as problemStale.
func TestComputeSkillProblems_StaleSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "repo-a"))
	// Note: the skill directory is NOT created — it's gone from the cache.

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {URL: "https://example.com/repo-a.git", CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo-a/coding/debugger": {
				"copilot": {InstalledAtSHA: "abc", InstalledHash: "def"},
			},
		},
	}

	// The skill is NOT in the discovered set (simulates path gone from cache).
	discovered := map[string]bool{}
	got := computeSkillProblems(st, discovered)

	if got["repo-a/coding/debugger"] != problemStale {
		t.Errorf("expected problemStale, got %v", got["repo-a/coding/debugger"])
	}
}

// TestComputeSkillProblems_BrokenUpstream verifies a fork whose upstream path
// no longer exists is classified as problemBrokenUpstream.
func TestComputeSkillProblems_BrokenUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Fork's own repo cache (with the fork skill still present).
	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger")

	// Upstream repo cache exists but the upstream skill path is gone.
	upstreamCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "upstream-repo"))
	// upstream skill NOT created — it's been deleted upstream.

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo":       {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			"upstream-repo": {URL: "https://example.com/upstream-repo.git", CachePath: upstreamCacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	// Fork itself IS discoverable; its UpstreamAddr is broken.
	discovered := map[string]bool{"my-repo/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if got["my-repo/coding/debugger"] != problemBrokenUpstream {
		t.Errorf("expected problemBrokenUpstream, got %v", got["my-repo/coding/debugger"])
	}
}

// TestComputeSkillProblems_UpstreamRepoNotRegistered verifies that a fork whose
// upstream repo is NOT registered is NOT flagged as broken (unknown state, not broken).
func TestComputeSkillProblems_UpstreamRepoNotRegistered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger")

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo": {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			// "upstream-repo" is NOT registered.
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	discovered := map[string]bool{"my-repo/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if p := got["my-repo/coding/debugger"]; p != problemNone {
		t.Errorf("expected problemNone when upstream repo not registered, got %v", p)
	}
}

// TestComputeSkillProblems_HealthyFork verifies a fork whose upstream skill still
// exists is classified as problemNone.
func TestComputeSkillProblems_HealthyFork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger")

	upstreamCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "upstream-repo"))
	makeSkillDir(t, upstreamCacheDir, "coding/debugger") // upstream skill still exists

	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo":       {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			"upstream-repo": {URL: "https://example.com/upstream-repo.git", CachePath: upstreamCacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	discovered := map[string]bool{"my-repo/coding/debugger": true}
	got := computeSkillProblems(st, discovered)

	if p := got["my-repo/coding/debugger"]; p != problemNone {
		t.Errorf("expected problemNone for healthy fork, got %v", p)
	}
}

// --- refreshSkills tests ---

// TestRefreshSkills_StaleAddressAppearsInRows verifies that a stale installed skill
// (not discoverable in any repo cache) is added to the row list with problemStale.
func TestRefreshSkills_StaleAddressAppearsInRows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "repo-a"))
	// The skill path is gone from the cache — do NOT create it.

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"repo-a": {URL: "https://example.com/repo-a.git", CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"repo-a/coding/debugger": {
				"copilot": {InstalledAtSHA: "abc", InstalledHash: "def"},
			},
		},
	}

	m := initialModel(cfg, st)

	var found *tuiRow
	for i := range m.rows {
		if m.rows[i].kind == skillRow && m.rows[i].addr == "repo-a/coding/debugger" {
			found = &m.rows[i]
			break
		}
	}

	if found == nil {
		t.Fatal("stale skill was not added to TUI rows")
	}
	if found.problem != problemStale {
		t.Errorf("expected problemStale, got %v", found.problem)
	}
}

// TestRefreshSkills_BrokenUpstreamMarked verifies a fork with a missing upstream
// path is marked as problemBrokenUpstream in the TUI rows.
func TestRefreshSkills_BrokenUpstreamMarked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ownCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "my-repo"))
	makeSkillDir(t, ownCacheDir, "coding/debugger") // fork itself is fine

	upstreamCacheDir := makeGitClone(t, filepath.Join(t.TempDir(), "upstream-repo"))
	// upstream skill path is gone — NOT created

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"my-repo":       {URL: "git@github.com:me/my-repo.git", CachePath: ownCacheDir},
			"upstream-repo": {URL: "https://example.com/upstream-repo.git", CachePath: upstreamCacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			"my-repo/coding/debugger": {
				"copilot": {
					InstalledAtSHA: "abc",
					InstalledHash:  "def",
					UpstreamAddr:   "upstream-repo/coding/debugger",
					UpstreamSHA:    "xyz",
				},
			},
		},
	}

	m := initialModel(cfg, st)

	var found *tuiRow
	for i := range m.rows {
		if m.rows[i].kind == skillRow && m.rows[i].addr == "my-repo/coding/debugger" {
			found = &m.rows[i]
			break
		}
	}

	if found == nil {
		t.Fatal("fork skill was not found in TUI rows")
	}
	if found.problem != problemBrokenUpstream {
		t.Errorf("expected problemBrokenUpstream, got %v", found.problem)
	}
}

// --- refreshPacks tests ---

// TestRefreshPacks_Empty verifies refreshPacks produces no rows when no packs are installed.
func TestRefreshPacks_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}

	m := initialModel(cfg, st)
	if len(m.packRows) != 0 {
		t.Errorf("expected 0 packRows, got %d", len(m.packRows))
	}
}

// TestRefreshPacks_Complete verifies a fully installed pack is flagged isPartial=false.
func TestRefreshPacks_Complete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/go-dev": {
				PackAddress: "my-repo/packs/go-dev",
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/skills/go": {
						"claude-code": {Installed: true},
					},
				},
			},
		},
	}

	m := initialModel(cfg, st)
	if len(m.packRows) != 1 {
		t.Fatalf("expected 1 packRow, got %d", len(m.packRows))
	}
	row := m.packRows[0]
	if row.packAddr != "my-repo/packs/go-dev" {
		t.Errorf("unexpected packAddr: %s", row.packAddr)
	}
	if row.isPartial {
		t.Error("expected isPartial=false for a fully installed pack")
	}
}

// TestRefreshPacks_Partial verifies a pack with a missing skill is flagged isPartial=true.
func TestRefreshPacks_Partial(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/go-dev": {
				PackAddress: "my-repo/packs/go-dev",
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/skills/go": {
						"claude-code": {Installed: true},
					},
					"my-repo/skills/missing": {
						"claude-code": {Installed: false, Error: "repo unavailable"},
					},
				},
			},
		},
	}

	m := initialModel(cfg, st)
	if len(m.packRows) != 1 {
		t.Fatalf("expected 1 packRow, got %d", len(m.packRows))
	}
	if !m.packRows[0].isPartial {
		t.Error("expected isPartial=true when a skill is not installed")
	}
}

// TestRefreshPacks_SortedByAddr verifies pack rows are sorted by address.
func TestRefreshPacks_SortedByAddr(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks: map[string]state.InstalledPackRecord{
			"z-repo/packs/z": {PackAddress: "z-repo/packs/z", Skills: map[string]map[string]state.PackSkillStatus{}},
			"a-repo/packs/a": {PackAddress: "a-repo/packs/a", Skills: map[string]map[string]state.PackSkillStatus{}},
		},
	}

	m := initialModel(cfg, st)
	if len(m.packRows) != 2 {
		t.Fatalf("expected 2 packRows, got %d", len(m.packRows))
	}
	if m.packRows[0].packAddr != "a-repo/packs/a" {
		t.Errorf("expected first row to be a-repo/packs/a, got %s", m.packRows[0].packAddr)
	}
	if m.packRows[1].packAddr != "z-repo/packs/z" {
		t.Errorf("expected second row to be z-repo/packs/z, got %s", m.packRows[1].packAddr)
	}
}

// TestCloneState_CopiesInstalledPacks verifies cloneState correctly copies InstalledPacks.
func TestCloneState_CopiesInstalledPacks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	src := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/go-dev": {
				PackAddress: "my-repo/packs/go-dev",
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/skills/go": {"claude-code": {Installed: true}},
				},
			},
		},
	}

	dst := cloneState(src)

	// Verify the pack was copied
	if len(dst.InstalledPacks) != 1 {
		t.Fatalf("expected 1 InstalledPack in clone, got %d", len(dst.InstalledPacks))
	}
	rec, ok := dst.InstalledPacks["my-repo/packs/go-dev"]
	if !ok {
		t.Fatal("pack not found in cloned state")
	}
	if len(rec.Skills) != 1 {
		t.Errorf("expected 1 skill in cloned pack, got %d", len(rec.Skills))
	}

	// Verify it is a deep copy — mutations don't bleed back
	dst.InstalledPacks["my-repo/packs/go-dev"].Skills["my-repo/skills/go"]["claude-code"] = state.PackSkillStatus{Installed: false}
	origStatus := src.InstalledPacks["my-repo/packs/go-dev"].Skills["my-repo/skills/go"]["claude-code"]
	if !origStatus.Installed {
		t.Error("cloneState is not deep-copying InstalledPacks — mutation in dst affected src")
	}
}

// makePackDir writes a pack.yaml at relPath inside a repo cache dir.
func makePackDir(t *testing.T, cacheDir, relPath, name, desc string) {
	t.Helper()
	dir := filepath.Join(cacheDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: " + name + "\n"
	if desc != "" {
		yaml += "description: " + desc + "\n"
	}
	yaml += "repos:\n  - name: my-repo\n    url: https://example.com/r.git\nskills:\n  - my-repo/skills/go\n"
	if err := os.WriteFile(filepath.Join(dir, "pack.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRefreshPacks_MergesAvailableAndInstalled verifies the packs panel lists
// discoverable-but-uninstalled packs alongside installed ones.
func TestRefreshPacks_MergesAvailableAndInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cache := t.TempDir()
	makePackDir(t, cache, "packs/avail", "avail", "an available pack")
	makePackDir(t, cache, "packs/inst", "inst", "an installed pack")

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           map[string]state.RepoRecord{"my-repo": {URL: "https://example.com/r.git", CachePath: cache}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/inst": {
				PackAddress: "my-repo/packs/inst",
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/skills/go": {"claude-code": {Installed: true}},
				},
			},
		},
	}

	m := initialModel(cfg, st)
	if len(m.packRows) != 2 {
		t.Fatalf("expected 2 packRows (1 available + 1 installed), got %d", len(m.packRows))
	}
	avail, inst := m.packRows[0], m.packRows[1]
	if avail.packAddr != "my-repo/packs/avail" || avail.installed {
		t.Errorf("expected first row to be available my-repo/packs/avail, got %+v", avail)
	}
	if avail.desc != "an available pack" {
		t.Errorf("expected description on available row, got %q", avail.desc)
	}
	if inst.packAddr != "my-repo/packs/inst" || !inst.installed {
		t.Errorf("expected second row to be installed my-repo/packs/inst, got %+v", inst)
	}
	if inst.desc != "an installed pack" {
		t.Errorf("expected discoverable installed pack to keep its description, got %q", inst.desc)
	}
}

// TestPackWizard_OpenAndCancel verifies 'n' opens the embedded wizard and Esc
// on the first step closes it without quitting the program.
func TestPackWizard_OpenAndCancel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}
	m := initialModel(cfg, st)
	m.activePanel = panelPacks

	next, _ := m.Update(keyRune("n"))
	m = next.(model)
	if m.packWizard == nil {
		t.Fatal("expected 'n' to open the embedded pack wizard")
	}
	if !m.packWizard.embedded {
		t.Error("expected wizard to be marked embedded")
	}

	next, _ = m.Update(keyEsc())
	m = next.(model)
	if m.packWizard != nil {
		t.Error("expected Esc on first step to close the wizard")
	}
}

// TestPackWizard_DoneScreenClosesAndReportsResult verifies a keypress on the
// wizard done screen returns to the packs panel and surfaces the result.
func TestPackWizard_DoneScreenClosesAndReportsResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}
	m := initialModel(cfg, st)
	m.activePanel = panelPacks
	w := initialPackCreateModel(cfg, st)
	w.embedded = true
	w.step = createStepDone
	w.doneResult = "published"
	m.packWizard = &w

	next, _ := m.Update(keyRune("x"))
	m = next.(model)
	if m.packWizard != nil {
		t.Fatal("expected done-screen keypress to close the wizard")
	}
	if m.message != "✓ published" {
		t.Errorf("expected result message, got %q", m.message)
	}
}

// TestStartPackInstall_PreselectsDefaultAgent verifies the install overlay
// opens with the default agent preselected.
func TestStartPackInstall_PreselectsDefaultAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cache := t.TempDir()
	makePackDir(t, cache, "packs/avail", "avail", "")

	cfg := &config.Config{
		DefaultAgent: "pi",
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: t.TempDir()},
			"pi":          {SkillDir: t.TempDir()},
		},
	}
	st := &state.State{
		Repos:           map[string]state.RepoRecord{"my-repo": {URL: "https://example.com/r.git", CachePath: cache}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}
	m := initialModel(cfg, st)
	m.activePanel = panelPacks

	next, _ := m.Update(keyRune("i"))
	m = next.(model)
	if m.inputMode != modePackInstallAgents {
		t.Fatalf("expected modePackInstallAgents, got %v", m.inputMode)
	}
	if m.packInstallAddr != "my-repo/packs/avail" {
		t.Errorf("unexpected packInstallAddr %q", m.packInstallAddr)
	}
	// agents are sorted: [claude-code pi]; default agent "pi" is index 1.
	if !m.packAgentSel[1] || m.packAgentSel[0] {
		t.Errorf("expected only default agent preselected, got %v", m.packAgentSel)
	}
}

// TestViewPacks_RendersMergedList is a render smoke test: the packs panel
// shows available and installed rows with their status labels.
func TestViewPacks_RendersMergedList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cache := t.TempDir()
	makePackDir(t, cache, "packs/avail", "avail", "a description here")

	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           map[string]state.RepoRecord{"my-repo": {URL: "https://example.com/r.git", CachePath: cache}},
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks: map[string]state.InstalledPackRecord{
			"my-repo/packs/inst": {
				PackAddress: "my-repo/packs/inst",
				Agents:      []string{"claude-code"},
				Skills: map[string]map[string]state.PackSkillStatus{
					"my-repo/skills/go": {"claude-code": {Installed: true}},
				},
			},
		},
	}
	m := initialModel(cfg, st)
	m.activePanel = panelPacks
	out := m.View()

	for _, want := range []string{"my-repo/packs/avail", "· available", "my-repo/packs/inst", "✓ complete", "a description here"} {
		if !strings.Contains(out, want) {
			t.Errorf("packs view missing %q", want)
		}
	}
}
