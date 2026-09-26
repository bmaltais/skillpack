package pack_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/state"
)

const debuggerAddr = "my-repo/coding/debugger"

// writeFile creates path (and parents) with content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// commitAll git-inits dir and commits every file in it.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init %s: %v", dir, err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("."); err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// skillRepo creates a local git repo holding a coding/debugger skill and returns its path.
func skillRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "coding", "debugger", "SKILL.md"), "# Debugger")
	commitAll(t, dir)
	return dir
}

func newEnv(t *testing.T, agents ...string) (*config.Config, *state.State) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	if len(agents) == 0 {
		agents = []string{"claude-code"}
	}
	cfg := &config.Config{DefaultAgent: agents[0], Agents: make(map[string]config.AgentConfig)}
	for _, a := range agents {
		cfg.Agents[a] = config.AgentConfig{SkillDir: t.TempDir()}
	}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		InstalledPacks:  make(map[string]state.InstalledPackRecord),
	}
	return cfg, st
}

// packFile writes a pack.yaml naming one repo and one skill, returning its directory.
func packFile(t *testing.T, name, repoName, repoURL, skillAddr string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack.yaml"), "name: "+name+"\nrepos:\n  - name: "+repoName+"\n    url: "+repoURL+"\nskills:\n  - "+skillAddr+"\n")
	return dir
}

func TestIsPartial(t *testing.T) {
	complete := state.InstalledPackRecord{Skills: map[string]map[string]state.PackSkillStatus{
		"repo/a": {"claude-code": {Installed: true}},
		"repo/b": {"claude-code": {Installed: true}},
	}}
	partial := state.InstalledPackRecord{Skills: map[string]map[string]state.PackSkillStatus{
		"repo/a": {"claude-code": {Installed: true}},
		"repo/b": {"claude-code": {Error: "auth failed"}},
	}}
	if pack.IsPartial(complete) {
		t.Error("complete record reported partial")
	}
	if !pack.IsPartial(partial) {
		t.Error("partial record reported complete")
	}
	if pack.IsPartial(state.InstalledPackRecord{}) {
		t.Error("empty record reported partial")
	}
}

func TestInstall_RegisteredRepo(t *testing.T) {
	cfg, st := newEnv(t)
	st.Repos["my-repo"] = state.RepoRecord{URL: "fake://my-repo", CachePath: skillRepo(t)}
	def, err := pack.Resolve(packFile(t, "my-test-pack", "my-repo", "fake://my-repo", debuggerAddr), st)
	if err != nil {
		t.Fatal(err)
	}

	var events []pack.EventKind
	res, err := pack.Install(cfg, st, def, []string{"claude-code"}, pack.Options{
		Progress: func(e pack.Event) { events = append(events, e.Kind) },
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if res.Partial() || res.Installed != 1 || res.Failed != 0 {
		t.Errorf("result = installed %d, failed %d, partial %v; want 1, 0, false", res.Installed, res.Failed, res.Partial())
	}
	rec, ok := st.InstalledPacks["my-test-pack"]
	if !ok {
		t.Fatalf("pack record not saved; InstalledPacks = %v", st.InstalledPacks)
	}
	if len(rec.Agents) != 1 || rec.Agents[0] != "claude-code" {
		t.Errorf("Agents = %v", rec.Agents)
	}
	if !rec.Skills[debuggerAddr]["claude-code"].Installed {
		t.Errorf("skill not installed: %+v", rec.Skills[debuggerAddr]["claude-code"])
	}
	if _, ok := st.InstalledSkills[debuggerAddr]["claude-code"]; !ok {
		t.Error("skill not recorded in InstalledSkills")
	}
	if len(events) != 2 || events[0] != pack.SkillInstalling || events[1] != pack.SkillInstalled {
		t.Errorf("events = %v, want [SkillInstalling SkillInstalled]", events)
	}
}

func TestInstall_PartialOnUnavailableRepo(t *testing.T) {
	cfg, st := newEnv(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	def, err := pack.Resolve(packFile(t, "partial-pack", "my-repo", missing, debuggerAddr), st)
	if err != nil {
		t.Fatal(err)
	}

	var kinds []pack.EventKind
	res, err := pack.Install(cfg, st, def, []string{"claude-code"}, pack.Options{
		Progress: func(e pack.Event) { kinds = append(kinds, e.Kind) },
	})
	if err != nil {
		t.Fatalf("Install must not fail on a per-repo error: %v", err)
	}

	if !res.Partial() || res.Installed != 0 || res.Failed != 1 {
		t.Errorf("result = installed %d, failed %d, partial %v; want 0, 1, true", res.Installed, res.Failed, res.Partial())
	}
	status := st.InstalledPacks["partial-pack"].Skills[debuggerAddr]["claude-code"]
	if status.Installed || !strings.Contains(status.Error, "repo unavailable") {
		t.Errorf("status = %+v, want not installed with 'repo unavailable'", status)
	}
	want := []pack.EventKind{pack.RepoRegistering, pack.RepoFailed, pack.SkillSkipped}
	if len(kinds) != len(want) {
		t.Fatalf("events = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("events = %v, want %v", kinds, want)
			break
		}
	}
}

func TestInstall_RecordsSkillInstallFailure(t *testing.T) {
	cfg, st := newEnv(t)
	st.Repos["my-repo"] = state.RepoRecord{URL: "fake://my-repo", CachePath: skillRepo(t)}
	def, err := pack.Resolve(packFile(t, "bad-skill", "my-repo", "fake://my-repo", "my-repo/coding/nope"), st)
	if err != nil {
		t.Fatal(err)
	}

	res, err := pack.Install(cfg, st, def, []string{"claude-code"}, pack.Options{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Partial() || res.Failed != 1 {
		t.Errorf("failed = %d, partial = %v; want 1, true", res.Failed, res.Partial())
	}
	if st.InstalledPacks["bad-skill"].Skills["my-repo/coding/nope"]["claude-code"].Error == "" {
		t.Error("install error not recorded")
	}
}

func TestComplete_RetriesFailedSkills(t *testing.T) {
	cfg, st := newEnv(t)
	st.Repos["my-repo"] = state.RepoRecord{URL: "fake://my-repo", CachePath: skillRepo(t)}
	// A synthetic (non-registered) pack address: the recipe cannot be resolved,
	// so only the installs are retried.
	st.InstalledPacks["my-pack"] = state.InstalledPackRecord{
		PackAddress: "my-pack",
		Agents:      []string{"claude-code"},
		Skills: map[string]map[string]state.PackSkillStatus{
			debuggerAddr: {"claude-code": {Error: "earlier failure"}},
		},
	}

	res, err := pack.Complete(cfg, st, "my-pack", pack.Options{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Installed != 1 || res.Failed != 0 || res.Partial() {
		t.Errorf("result = installed %d, failed %d, partial %v; want 1, 0, false", res.Installed, res.Failed, res.Partial())
	}
	if !st.InstalledPacks["my-pack"].Skills[debuggerAddr]["claude-code"].Installed {
		t.Error("pack record not updated in state")
	}
}

func TestComplete_SkipsAlreadyInstalled(t *testing.T) {
	cfg, st := newEnv(t)
	st.InstalledPacks["my-pack"] = state.InstalledPackRecord{
		PackAddress: "my-pack",
		Skills: map[string]map[string]state.PackSkillStatus{
			debuggerAddr: {"claude-code": {Installed: true}},
		},
	}
	res, err := pack.Complete(cfg, st, "my-pack", pack.Options{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(res.Outcomes) != 0 {
		t.Errorf("Outcomes = %v, want none", res.Outcomes)
	}
}

func TestComplete_UnknownPack(t *testing.T) {
	cfg, st := newEnv(t)
	if _, err := pack.Complete(cfg, st, "nope", pack.Options{}); err == nil {
		t.Error("expected error for a pack that is not installed")
	}
}

// A pack from a registered repo whose recipe lists a second repo that failed to
// register the first time: Complete registers it again, then installs the skill.
func TestComplete_ReRegistersMissingRepos(t *testing.T) {
	cfg, st := newEnv(t)

	packRepo := t.TempDir()
	skillsRepo := skillRepo(t)
	writeFile(t, filepath.Join(packRepo, "packs", "p", "pack.yaml"),
		"name: p\nrepos:\n  - name: my-repo\n    url: "+skillsRepo+"\nskills:\n  - "+debuggerAddr+"\n")
	commitAll(t, packRepo)
	st.Repos["pack-repo"] = state.RepoRecord{URL: "fake://pack-repo", CachePath: packRepo}

	const packAddr = "pack-repo/packs/p"
	st.InstalledPacks[packAddr] = state.InstalledPackRecord{
		PackAddress: packAddr,
		Agents:      []string{"claude-code"},
		Skills: map[string]map[string]state.PackSkillStatus{
			debuggerAddr: {"claude-code": {Error: "repo unavailable: auth failed"}},
		},
	}

	res, err := pack.Complete(cfg, st, packAddr, pack.Options{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, ok := st.Repos["my-repo"]; !ok {
		t.Fatal("missing repo was not re-registered")
	}
	if res.Installed != 1 || res.Partial() {
		t.Errorf("installed = %d, partial = %v; want 1, false", res.Installed, res.Partial())
	}
}
