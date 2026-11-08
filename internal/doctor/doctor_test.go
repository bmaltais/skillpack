package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmaltais/skillpack/internal/doctor"
	"github.com/bmaltais/skillpack/internal/state"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newState returns a state with one git-looking repo per name; skills maps
// "repo/relpath" to SKILL.md content.
func newState(t *testing.T, skills map[string]string, repos ...string) *state.State {
	t.Helper()
	st := &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
		InstalledPacks:  map[string]state.InstalledPackRecord{},
	}
	for _, r := range repos {
		cache := t.TempDir()
		if err := os.MkdirAll(filepath.Join(cache, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		st.Repos[r] = state.RepoRecord{CachePath: cache}
	}
	for addr, body := range skills {
		repoName, rel, _ := cutFirst(addr)
		write(t, filepath.Join(st.Repos[repoName].CachePath, filepath.FromSlash(rel), "SKILL.md"), body)
	}
	return st
}

func cutFirst(s string) (string, string, bool) {
	for i := range s {
		if s[i] == '/' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func byKind(fs []doctor.Finding, k doctor.Kind) []doctor.Finding {
	var out []doctor.Finding
	for _, f := range fs {
		if f.Kind == k {
			out = append(out, f)
		}
	}
	return out
}

func TestDiagnose_HealthyIsEmpty(t *testing.T) {
	st := newState(t, map[string]string{"a/one": "# one"}, "a")
	st.InstalledSkills["a/one"] = map[string]state.InstalledSkillRecord{"claude": {}}
	got, err := doctor.Diagnose(st, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("want no findings, got %v err %v", got, err)
	}
}

func TestDiagnose_StaleSkill(t *testing.T) {
	st := newState(t, nil, "a")
	st.InstalledSkills["a/gone"] = map[string]state.InstalledSkillRecord{"pi": {}, "claude": {}}
	got, _ := doctor.Diagnose(st, nil)
	f := byKind(got, doctor.StaleSkill)
	if len(f) != 1 || f[0].Subject != "a/gone" || f[0].Agent != "claude" || f[0].Fix == "" {
		t.Fatalf("got %+v", got)
	}
}

func TestDiagnose_BrokenUpstream(t *testing.T) {
	st := newState(t, map[string]string{"a/fork": "# fork", "b/orig": "# orig"}, "a", "b")
	st.InstalledSkills["a/fork"] = map[string]state.InstalledSkillRecord{"claude": {UpstreamAddr: "b/orig"}}
	if got, _ := doctor.Diagnose(st, nil); len(byKind(got, doctor.BrokenUpstream)) != 0 {
		t.Fatalf("upstream exists, must not flag: %+v", got)
	}
	_ = os.Remove(filepath.Join(st.Repos["b"].CachePath, "orig", "SKILL.md"))
	got, _ := doctor.Diagnose(st, nil)
	f := byKind(got, doctor.BrokenUpstream)
	if len(f) != 1 || f[0].Related != "b/orig" {
		t.Fatalf("got %+v", got)
	}
}

func TestDiagnose_UnregisteredUpstreamIsNotBroken(t *testing.T) {
	st := newState(t, map[string]string{"a/fork": "# fork"}, "a")
	st.InstalledSkills["a/fork"] = map[string]state.InstalledSkillRecord{"claude": {UpstreamAddr: "nowhere/x"}}
	if got, _ := doctor.Diagnose(st, nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestDiagnose_PartialPack(t *testing.T) {
	st := newState(t, nil, "a")
	st.InstalledPacks["a/packs/p"] = state.InstalledPackRecord{
		Skills: map[string]map[string]state.PackSkillStatus{"a/x": {"claude": {Installed: false}}},
	}
	st.InstalledPacks["a/packs/ok"] = state.InstalledPackRecord{
		Skills: map[string]map[string]state.PackSkillStatus{"a/x": {"claude": {Installed: true}}},
	}
	got, _ := doctor.Diagnose(st, nil)
	f := byKind(got, doctor.PartialPack)
	if len(f) != 1 || f[0].Subject != "a/packs/p" {
		t.Fatalf("got %+v", got)
	}
}

func TestDiagnose_ForkCandidateAndDuplicate(t *testing.T) {
	st := newState(t, map[string]string{"mine/triage": "# mine", "theirs/triage": "# theirs"}, "mine", "theirs")
	st.InstalledSkills["mine/triage"] = map[string]state.InstalledSkillRecord{"claude": {}}
	got, _ := doctor.Diagnose(st, nil)
	if f := byKind(got, doctor.ForkCandidate); len(f) != 1 || f[0].Related != "theirs/triage" {
		t.Fatalf("fork candidate: %+v", got)
	}
	d := byKind(got, doctor.Duplicate)
	if len(d) != 1 || d[0].Identical || d[0].Fix == "" {
		t.Fatalf("diverged duplicate should offer fixes: %+v", d)
	}
}

func TestDiagnose_IdenticalDuplicateOffersDelete(t *testing.T) {
	st := newState(t, map[string]string{"a/t": "# same", "b/t": "# same"}, "a", "b")
	got, _ := doctor.Diagnose(st, nil)
	d := byKind(got, doctor.Duplicate)
	if len(d) != 1 || !d[0].Identical || !strings.Contains(d[0].Fix, "Delete") {
		t.Fatalf("got %+v", d)
	}
}

func TestDiagnose_OrderedByKind(t *testing.T) {
	st := newState(t, map[string]string{"a/t": "# 1", "b/t": "# 2"}, "a", "b")
	st.InstalledSkills["a/gone"] = map[string]state.InstalledSkillRecord{"claude": {}}
	got, _ := doctor.Diagnose(st, nil)
	if len(got) < 2 || got[0].Kind != doctor.StaleSkill || got[len(got)-1].Kind != doctor.Duplicate {
		t.Fatalf("got %+v", got)
	}
}

func TestDiff_ChangedOnlyAndMissingFiles(t *testing.T) {
	st := newState(t, map[string]string{"a/t": "# t\nsame\nold line\n", "b/t": "# t\nsame\nnew line\n"}, "a", "b")
	write(t, filepath.Join(st.Repos["b"].CachePath, "t", "extra.md"), "x")
	write(t, filepath.Join(st.Repos["a"].CachePath, "t", ".skillpack-fork"), "meta")
	got, err := doctor.Diff(st, "a/t", "b/t")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- old line", "+ new line", "Only in b/t: extra.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, ".skillpack-fork") {
		t.Errorf("provenance file must be ignored:\n%s", got)
	}
}

func TestDiff_IdenticalSaysSo(t *testing.T) {
	st := newState(t, map[string]string{"a/t": "# same", "b/t": "# same"}, "a", "b")
	got, _ := doctor.Diff(st, "a/t", "b/t")
	if !strings.Contains(got, "no differences") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestSkillProblems(t *testing.T) {
	st := newState(t, map[string]string{"a/ok": "# ok"}, "a")
	st.InstalledSkills["a/ok"] = map[string]state.InstalledSkillRecord{"claude": {}}
	st.InstalledSkills["a/gone"] = map[string]state.InstalledSkillRecord{"claude": {}}
	p := doctor.SkillProblems(st, map[string]bool{"a/ok": true})
	if len(p) != 1 || p["a/gone"].Kind != doctor.StaleSkill {
		t.Fatalf("got %+v", p)
	}
}
