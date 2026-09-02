package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/state"
)

// TestSlashFilter_StatusPanel mirrors TestSlashFilter_UnmanagedPanel for the
// Status panel's statusFilter field: bare 'q' still quits until '/' is
// pressed, typed letters feed statusFilter and narrow statusRows, and Esc
// clears the filter and restores the full row set.
func TestSlashFilter_StatusPanel(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelStatus
	m.statusInfo = map[string]map[string]string{
		"repo/foo": {"claude-code": "ok"},
		"repo/bar": {"claude-code": "ok"},
	}
	m.rebuildStatusRows()
	if len(m.statusRows) != 2 {
		t.Fatalf("setup: statusRows = %d, want 2", len(m.statusRows))
	}

	next, cmd := m.Update(keyRune("q"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("'q' should quit when filter mode is off")
	}

	next, _ = m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode on the Status panel")
	}

	for _, ch := range []string{"f", "o", "o"} {
		next, _ = m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.statusFilter != "foo" {
		t.Fatalf("statusFilter = %q, want %q", m.statusFilter, "foo")
	}
	if len(m.statusRows) != 1 || m.statusRows[0].addr != "repo/foo" {
		t.Fatalf("statusRows after filter = %+v, want only repo/foo", m.statusRows)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.statusFilter != "" || m.filterActive {
		t.Fatal("Esc should clear the status filter and exit filter mode")
	}
	if len(m.statusRows) != 2 {
		t.Fatalf("statusRows after Esc clear = %d, want 2", len(m.statusRows))
	}
}

// TestSlashFilter_ReposPanel mirrors TestSlashFilter_UnmanagedPanel for the
// Repos panel's repoFilter field, matching on repo name or URL.
func TestSlashFilter_ReposPanel(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelRepos
	m.st.Repos = map[string]state.RepoRecord{
		"awesome-skills": {URL: "https://example.com/awesome-skills.git"},
		"other-repo":     {URL: "https://example.com/other-repo.git"},
	}
	m.refreshRepos()
	if len(m.repoList) != 2 {
		t.Fatalf("setup: repoList = %d, want 2", len(m.repoList))
	}

	next, cmd := m.Update(keyRune("q"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("'q' should quit when filter mode is off")
	}

	next, _ = m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode on the Repos panel")
	}

	for _, ch := range []string{"a", "w", "e"} {
		next, _ = m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.repoFilter != "awe" {
		t.Fatalf("repoFilter = %q, want %q", m.repoFilter, "awe")
	}
	if len(m.repoList) != 1 || m.repoList[0].name != "awesome-skills" {
		t.Fatalf("repoList after filter = %+v, want only awesome-skills", m.repoList)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.repoFilter != "" || m.filterActive {
		t.Fatal("Esc should clear the repo filter and exit filter mode")
	}
	if len(m.repoList) != 2 {
		t.Fatalf("repoList after Esc clear = %d, want 2", len(m.repoList))
	}
}

// TestSlashFilter_PacksPanel mirrors TestSlashFilter_UnmanagedPanel for the
// Packs panel's packFilter field, matching on pack address or description.
func TestSlashFilter_PacksPanel(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelPacks
	m.st.InstalledPacks = map[string]state.InstalledPackRecord{
		"awesome-skills/packs/go-dev": {
			PackAddress: "awesome-skills/packs/go-dev",
			Agents:      []string{"claude-code"},
			Skills:      map[string]map[string]state.PackSkillStatus{},
		},
		"other-repo/packs/web-dev": {
			PackAddress: "other-repo/packs/web-dev",
			Agents:      []string{"claude-code"},
			Skills:      map[string]map[string]state.PackSkillStatus{},
		},
	}
	m.refreshPacks()
	if len(m.packRows) != 2 {
		t.Fatalf("setup: packRows = %d, want 2", len(m.packRows))
	}

	next, cmd := m.Update(keyRune("q"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("'q' should quit when filter mode is off")
	}

	next, _ = m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode on the Packs panel")
	}

	for _, ch := range []string{"g", "o"} {
		next, _ = m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.packFilter != "go" {
		t.Fatalf("packFilter = %q, want %q", m.packFilter, "go")
	}
	if len(m.packRows) != 1 || m.packRows[0].packAddr != "awesome-skills/packs/go-dev" {
		t.Fatalf("packRows after filter = %+v, want only the go-dev pack", m.packRows)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.packFilter != "" || m.filterActive {
		t.Fatal("Esc should clear the pack filter and exit filter mode")
	}
	if len(m.packRows) != 2 {
		t.Fatalf("packRows after Esc clear = %d, want 2", len(m.packRows))
	}
}

// TestSlashFilter_DoctorPanel mirrors TestSlashFilter_UnmanagedPanel for the
// Doctor panel's doctorFilter field, matching on duplicate-set skill
// addresses. Doctor filters at render time (doctorLines), so doctorSets
// itself is left untouched by typing.
func TestSlashFilter_DoctorPanel(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelDoctor
	m.doctorSets = []doctorSetRow{
		{basename: "debugger", members: []string{"repo-a/debugger", "repo-b/debugger"}},
		{basename: "linter", members: []string{"repo-a/linter", "repo-b/linter"}},
	}

	next, cmd := m.Update(keyRune("q"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("'q' should quit when filter mode is off")
	}

	next, _ = m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode on the Doctor panel")
	}

	for _, ch := range []string{"d", "e", "b"} {
		next, _ = m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.doctorFilter != "deb" {
		t.Fatalf("doctorFilter = %q, want %q", m.doctorFilter, "deb")
	}
	if len(m.doctorSets) != 2 {
		t.Fatal("doctorFilter must not mutate doctorSets itself")
	}
	joined := strings.Join(doctorLines(m), "\n")
	if !strings.Contains(joined, "debugger") || strings.Contains(joined, "linter") {
		t.Fatalf("doctorLines with filter %q did not narrow to the debugger set: %q", m.doctorFilter, joined)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.doctorFilter != "" || m.filterActive {
		t.Fatal("Esc should clear the doctor filter and exit filter mode")
	}
	joined = strings.Join(doctorLines(m), "\n")
	if !strings.Contains(joined, "debugger") || !strings.Contains(joined, "linter") {
		t.Fatalf("doctorLines after Esc clear should show both sets: %q", joined)
	}
}
