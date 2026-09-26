package pack_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

const registeredPack = "my-repo/packs/p"

// deployment is a pack installed from a local origin repo that tests can advance.
type deployment struct {
	cfg    *config.Config
	st     *state.State
	origin string
}

// deploy registers a local origin repo holding the debugger skill and a pack
// recipe, then installs the pack for agents.
func deploy(t *testing.T, agents ...string) *deployment {
	t.Helper()
	cfg, st := newEnv(t, agents...)
	origin := skillRepo(t)
	writeFile(t, filepath.Join(origin, "packs", "p", "pack.yaml"),
		"name: p\nrepos:\n  - name: my-repo\n    url: "+origin+"\nskills:\n  - "+debuggerAddr+"\n")
	commitFile(t, origin, "packs/p/pack.yaml")
	if _, err := repo.Add("my-repo", origin, "", st); err != nil {
		t.Fatalf("repo.Add: %v", err)
	}
	def, err := pack.Resolve(registeredPack, st)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(cfg.Agents))
	for a := range cfg.Agents {
		names = append(names, a)
	}
	res, err := pack.Install(cfg, st, def, names, pack.Options{})
	if err != nil || res.Partial() {
		t.Fatalf("Install: err=%v partial=%v", err, res != nil && res.Partial())
	}
	return &deployment{cfg: cfg, st: st, origin: origin}
}

// commitFile stages relPath in the repo at dir and commits it.
func commitFile(t *testing.T, dir, relPath string) {
	t.Helper()
	r, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add(relPath); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit("change "+relPath, &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
}

// upstreamChange commits a new SKILL.md to the origin repo.
func (d *deployment) upstreamChange(t *testing.T, content string) {
	t.Helper()
	writeFile(t, filepath.Join(d.origin, "coding", "debugger", "SKILL.md"), content)
	commitFile(t, d.origin, "coding/debugger/SKILL.md")
}

func (d *deployment) installedFile(agent string) string {
	return filepath.Join(d.st.InstalledSkills[debuggerAddr][agent].LocalPath, "SKILL.md")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUpdate_UpToDate(t *testing.T) {
	d := deploy(t)
	res, err := pack.Update(d.cfg, d.st, registeredPack, pack.Options{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(res.Outcomes) != 0 {
		t.Errorf("Outcomes = %v, want none", res.Outcomes)
	}
}

func TestUpdate_AppliesUpstreamChange(t *testing.T) {
	d := deploy(t)
	d.upstreamChange(t, "# Debugger v2")

	res, err := pack.Update(d.cfg, d.st, registeredPack, pack.Options{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Updated != 1 || res.Failed != 0 || res.Blocked != 0 {
		t.Errorf("updated %d, failed %d, blocked %d; want 1, 0, 0", res.Updated, res.Failed, res.Blocked)
	}
	if got := readFile(t, d.installedFile("claude-code")); got != "# Debugger v2" {
		t.Errorf("installed SKILL.md = %q, want the upstream change", got)
	}
}

func TestUpdate_BlocksConflictAndKeepsLocalEdits(t *testing.T) {
	d := deploy(t)
	if err := os.WriteFile(d.installedFile("claude-code"), []byte("# my local edit"), 0600); err != nil {
		t.Fatal(err)
	}
	d.upstreamChange(t, "# Debugger v2")

	res, err := pack.Update(d.cfg, d.st, registeredPack, pack.Options{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Blocked != 1 || res.Updated != 0 {
		t.Errorf("blocked %d, updated %d; want 1, 0", res.Blocked, res.Updated)
	}
	if got := readFile(t, d.installedFile("claude-code")); got != "# my local edit" {
		t.Errorf("local edit was overwritten: %q", got)
	}
	if res.Partial() || !d.st.InstalledPacks[registeredPack].Skills[debuggerAddr]["claude-code"].Installed {
		t.Error("a blocked update must not make the deployment partial")
	}
}

func TestUpdate_InstallsMissingSkill(t *testing.T) {
	d := deploy(t)
	is, err := skill.Open(debuggerAddr, "claude-code", d.cfg, d.st)
	if err != nil {
		t.Fatal(err)
	}
	if err := is.Remove(true); err != nil {
		t.Fatal(err)
	}

	res, err := pack.Update(d.cfg, d.st, registeredPack, pack.Options{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Installed != 1 {
		t.Errorf("installed = %d, want 1", res.Installed)
	}
	if _, ok := d.st.InstalledSkills[debuggerAddr]["claude-code"]; !ok {
		t.Error("missing skill was not reinstalled")
	}
}

func TestUpdate_UnknownPack(t *testing.T) {
	cfg, st := newEnv(t)
	if _, err := pack.Update(cfg, st, "nope", pack.Options{}); err == nil {
		t.Error("expected error for a pack that is not installed")
	}
}

func TestRemove_RemovesSkillsAndDropsRecord(t *testing.T) {
	d := deploy(t)
	path := d.installedFile("claude-code")

	res, err := pack.Remove(d.cfg, d.st, registeredPack, nil, pack.RemoveOptions{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res.Removed != 1 || res.Kept != 0 || !res.PackRemoved {
		t.Errorf("removed %d, kept %d, packRemoved %v; want 1, 0, true", res.Removed, res.Kept, res.PackRemoved)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("skill file still present (stat err = %v)", err)
	}
	if _, ok := d.st.InstalledPacks[registeredPack]; ok {
		t.Error("pack record not dropped")
	}
}

func TestRemove_KeepsLocallyModifiedSkill(t *testing.T) {
	d := deploy(t)
	path := d.installedFile("claude-code")
	if err := os.WriteFile(path, []byte("# my local edit"), 0600); err != nil {
		t.Fatal(err)
	}

	res, err := pack.Remove(d.cfg, d.st, registeredPack, nil, pack.RemoveOptions{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res.Kept != 1 || res.Removed != 0 || !res.PackRemoved {
		t.Errorf("kept %d, removed %d, packRemoved %v; want 1, 0, true", res.Kept, res.Removed, res.PackRemoved)
	}
	if got := readFile(t, path); got != "# my local edit" {
		t.Errorf("local edit lost: %q", got)
	}
	if _, ok := d.st.InstalledSkills[debuggerAddr]["claude-code"]; !ok {
		t.Error("kept skill must remain an Installed Skill")
	}
	if _, ok := d.st.InstalledPacks[registeredPack]; ok {
		t.Error("pack record should still be dropped")
	}
}

func TestRemove_ForceRemovesLocallyModifiedSkill(t *testing.T) {
	d := deploy(t)
	path := d.installedFile("claude-code")
	if err := os.WriteFile(path, []byte("# my local edit"), 0600); err != nil {
		t.Fatal(err)
	}

	res, err := pack.Remove(d.cfg, d.st, registeredPack, nil, pack.RemoveOptions{Force: true})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res.Removed != 1 || res.Kept != 0 {
		t.Errorf("removed %d, kept %d; want 1, 0", res.Removed, res.Kept)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("forced removal left the skill on disk")
	}
}

func TestRemove_OneAgentKeepsPackForOthers(t *testing.T) {
	d := deploy(t, "claude-code", "copilot")

	res, err := pack.Remove(d.cfg, d.st, registeredPack, []string{"copilot"}, pack.RemoveOptions{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res.PackRemoved || len(res.RemainingAgents) != 1 || res.RemainingAgents[0] != "claude-code" {
		t.Errorf("packRemoved %v, remaining %v; want false, [claude-code]", res.PackRemoved, res.RemainingAgents)
	}
	rec := d.st.InstalledPacks[registeredPack]
	if len(rec.Agents) != 1 || rec.Agents[0] != "claude-code" {
		t.Errorf("record agents = %v", rec.Agents)
	}
	if _, ok := rec.Skills[debuggerAddr]["copilot"]; ok {
		t.Error("removed agent's status not pruned from the record")
	}
	if !rec.Skills[debuggerAddr]["claude-code"].Installed {
		t.Error("remaining agent's status lost")
	}
	if _, ok := d.st.InstalledSkills[debuggerAddr]["claude-code"]; !ok {
		t.Error("remaining agent's skill was removed")
	}
}

func TestRemove_UnknownPack(t *testing.T) {
	cfg, st := newEnv(t)
	if _, err := pack.Remove(cfg, st, "nope", nil, pack.RemoveOptions{}); err == nil {
		t.Error("expected error for a pack that is not installed")
	}
}
