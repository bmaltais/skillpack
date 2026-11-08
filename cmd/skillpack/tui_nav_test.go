package main

import (
	"fmt"
	"github.com/bmaltais/skillpack/internal/doctor"
	"github.com/bmaltais/skillpack/internal/state"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMenuMnemonicsUniquePerMenu(t *testing.T) {
	for _, menu := range appMenus {
		seen := map[string]string{}
		for i, at := range mnemonicIndexes(menu.items) {
			if at < 0 {
				t.Errorf("%s/%s has no mnemonic", menu.label, menu.items[i].label)
				continue
			}
			c := strings.ToLower(menu.items[i].label[at : at+1])
			if prev, dup := seen[c]; dup {
				t.Errorf("%s: %q and %q share mnemonic %q", menu.label, prev, menu.items[i].label, c)
			}
			seen[c] = menu.items[i].label
		}
	}
}

func TestMenuMnemonicReachesShadowedItem(t *testing.T) {
	m := emptyTestModel()
	m.menuOpen, m.menuIndex = true, menuIndexByLabel("View")
	// "Status" used to be shadowed by "Skills" (both start with S).
	next, _ := m.Update(keyRune("t"))
	if got := next.(model).activePanel; got != panelStatus {
		t.Fatalf("mnemonic t should open Status, got panel %v", got)
	}
}

func TestTabWalksPanelOrderAndMatchesFKeys(t *testing.T) {
	m := emptyTestModel()
	for i, p := range panelOrder {
		if m.activePanel != p {
			t.Fatalf("after %d Tabs: panel %v, want %v", i, m.activePanel, p)
		}
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(model)
	}
	if m.activePanel != panelOrder[0] {
		t.Fatal("Tab should wrap to the first panel")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(model)
	if m.activePanel != panelOrder[len(panelOrder)-1] {
		t.Fatal("Shift+Tab should wrap backwards")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF4})
	if got := next.(model).activePanel; got != panelOrder[2] {
		t.Fatalf("F4 should select panelOrder[2], got %v", got)
	}
}

func TestEscNeverQuits(t *testing.T) {
	m := emptyTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc with nothing to dismiss must not quit")
	}
}

func TestFilterBackspaceIsRuneSafe(t *testing.T) {
	m := emptyTestModel()
	m.filterActive = true
	m.filter = "café"
	m.backspaceFilter()
	if m.filter != "caf" {
		t.Fatalf("got %q, want %q", m.filter, "caf")
	}
}

func TestQuestionMarkOpensHelp(t *testing.T) {
	m := emptyTestModel()
	next, _ := m.Update(keyRune("?"))
	if next.(model).inputMode != modeHelp {
		t.Fatal("? should open help")
	}
}

func TestPanelStripListsEveryPanel(t *testing.T) {
	m := emptyTestModel()
	m.width = 120
	strip := renderPanelStrip(m)
	for _, p := range panelOrder {
		if !strings.Contains(strip, panelName(p)) {
			t.Errorf("strip missing %q", panelName(p))
		}
	}
}

// A frame taller than the terminal scrolls the screen and loses the top bars.
func TestViewNeverExceedsTerminalHeight(t *testing.T) {
	for _, p := range panelOrder {
		for h := 12; h <= 40; h += 7 {
			m := emptyTestModel()
			m.width, m.height = 100, h
			m.activePanel = p
			m.busy = "Checking status..."
			if got := strings.Count(m.View(), "\n") + 1; got != h {
				t.Errorf("panel %v height %d: frame has %d lines", p, h, got)
			}
		}
	}
}

// --- Doctor fixes ---

func doctorModel(findings ...doctor.Finding) model {
	m := emptyTestModel()
	m.activePanel = panelDoctor
	m.doctorFindings = findings
	return m
}

func TestDoctorFix_StaleOpensRelinkDialog(t *testing.T) {
	m := doctorModel(doctor.Finding{Kind: doctor.StaleSkill, Subject: "r/gone", Agent: "claude", Fix: "Relink"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.inputMode != modeRelinkStaleInput || m.relinkAddr != "r/gone" || m.relinkAgentName != "claude" {
		t.Fatalf("mode=%v addr=%q agent=%q", m.inputMode, m.relinkAddr, m.relinkAgentName)
	}
	if !m.doctorDirty {
		t.Fatal("fix should mark the panel for rescan")
	}
}

func TestDoctorFix_BrokenUpstreamOpensChoice(t *testing.T) {
	m := doctorModel(doctor.Finding{Kind: doctor.BrokenUpstream, Subject: "r/fork", Agent: "pi", Fix: "Set"})
	next, _ := m.Update(keyRune("f"))
	if got := next.(model).inputMode; got != modeRelinkBrokenChoice {
		t.Fatalf("mode=%v", got)
	}
}

func TestDoctorFix_ForkCandidatePrefillsUpstream(t *testing.T) {
	m := doctorModel(doctor.Finding{Kind: doctor.ForkCandidate, Subject: "mine/t", Related: "theirs/t", Fix: "Register"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.inputMode != modeRegisterForkInput || m.registerForkAddr != "mine/t" || m.registerForkInput != "theirs/t" {
		t.Fatalf("mode=%v addr=%q input=%q", m.inputMode, m.registerForkAddr, m.registerForkInput)
	}
}

func TestDoctorFix_PartialPackRunsCompleteAsync(t *testing.T) {
	m := doctorModel(doctor.Finding{Kind: doctor.PartialPack, Subject: "r/packs/p", Fix: "Complete"})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if cmd == nil || m.busy == "" || !m.doctorDirty {
		t.Fatalf("cmd=%v busy=%q dirty=%v", cmd != nil, m.busy, m.doctorDirty)
	}
}

func TestDoctorFix_InformationalDoesNothing(t *testing.T) {
	m := doctorModel(doctor.Finding{Kind: doctor.Duplicate, Subject: "a/t", Related: "b/t"})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if cmd != nil || m.inputMode != modeNormal || m.doctorDirty || m.message == "" {
		t.Fatalf("informational finding must only explain: cmd=%v mode=%v msg=%q", cmd != nil, m.inputMode, m.message)
	}
}

func TestDoctorFix_RescansWhenDialogCloses(t *testing.T) {
	m := doctorModel(doctor.Finding{Kind: doctor.ForkCandidate, Subject: "mine/t", Related: "theirs/t", Fix: "Register"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(model).doctorFindings == nil {
		t.Fatal("findings must stay while the dialog is open")
	}
	next, _ = next.(model).Update(tea.KeyMsg{Type: tea.KeyEsc}) // cancel the dialog
	m = next.(model)
	if m.doctorDirty || len(m.doctorFindings) != 0 {
		t.Fatalf("closing the dialog should rescan (state has no problems): dirty=%v findings=%v", m.doctorDirty, m.doctorFindings)
	}
}

func TestDoctorPanel_ArrowsMoveSelection(t *testing.T) {
	m := doctorModel(doctor.Finding{Subject: "a"}, doctor.Finding{Subject: "b"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.selectedFinding().Subject != "b" {
		t.Fatalf("got %+v", m.selectedFinding())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if next.(model).doctorCursor != 1 {
		t.Fatal("cursor must stop at the last finding")
	}
}

// --- Phase 2: verbs and dialogs ---

func TestAddRepo_SingleStepNamesFromURL(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelRepos
	m.startAddRepo()
	if m.inputMode != modeAddRepoURL {
		t.Fatalf("Add Repo should open straight at the URL prompt, got %v", m.inputMode)
	}
	if _, lines, _ := dialogContent(m); !strings.Contains(strings.Join(lines, "\n"), "Named after the URL") {
		t.Fatal("dialog should say how the repo will be named")
	}
}

func TestRepairChoice_ArrowsAndEnter(t *testing.T) {
	m := emptyTestModel()
	m.inputMode = modeRelinkBrokenChoice
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.relinkChoice != 1 {
		t.Fatalf("Down should select 'clear', got %d", m.relinkChoice)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.(model).inputMode; got != modeRelinkBrokenSetInput {
		t.Fatalf("Enter on 'set' should ask for the address, got mode %v", got)
	}
}

func TestAddAgentKeyWorksFromAnyPanel(t *testing.T) {
	for _, p := range panelOrder {
		m := emptyTestModel()
		m.activePanel = p
		next, _ := m.Update(keyRune("A"))
		if next.(model).inputMode != modeAddAgentPick {
			t.Errorf("panel %v: A should start Add Agent", p)
		}
	}
}

func TestPackRemove_UnavailablePackCannotBeMarkedOrRemoved(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelPacks
	m.packRows = []packRow{{packAddr: "r/packs/avail"}} // available, not installed
	next, _ := m.Update(keyRune("m"))
	m = next.(model)
	if len(m.packMarks) != 0 || m.message == "" {
		t.Fatalf("marks=%v msg=%q", m.packMarks, m.message)
	}
	next, _ = m.Update(keyRune("d"))
	if next.(model).inputMode == modePackConfirmRemove {
		t.Fatal("d must not offer to remove a pack that is not installed")
	}
}

func TestRemovePacksPanelIsGone(t *testing.T) {
	for _, p := range panelOrder {
		if panelName(p) == "Remove Packs" {
			t.Fatal("Remove Packs is part of the Packs panel now")
		}
	}
}

func TestHintsFitHundredColumns(t *testing.T) {
	for _, p := range panelOrder {
		m := emptyTestModel()
		m.activePanel = p
		right := len("Tab :palette ?help F10 menu ")
		if w := lipgloss.Width(" "+hintForPanel(m)) + right; w > 100 {
			t.Errorf("panel %v: hint+menu hint is %d columns, want <= 100", p, w)
		}
	}
}

func TestPackMenuHasNoDuplicateRemoveEntry(t *testing.T) {
	for _, it := range appMenus[menuIndexByLabel("Packs")].items {
		if strings.HasPrefix(it.label, "Remove Packs") {
			t.Fatal("Remove Packs is reached from the View menu only")
		}
	}
}

// --- Phase 3: command palette ---

func TestFuzzyScore(t *testing.T) {
	if _, ok := fuzzyScore("xyz", "Add Repo"); ok {
		t.Fatal("non-subsequence must not match")
	}
	contig, _ := fuzzyScore("rep", "Add Repo")
	scattered, _ := fuzzyScore("rep", "Remove Packs Entire")
	if contig <= scattered {
		t.Fatalf("contiguous word-start match should outrank scattered: %d vs %d", contig, scattered)
	}
	if _, ok := fuzzyScore("", "anything"); !ok {
		t.Fatal("empty query matches everything")
	}
}

func TestPalette_OpensWithCtrlPAndColon_EscCloses(t *testing.T) {
	for _, open := range []tea.KeyMsg{{Type: tea.KeyCtrlP}, keyRune(":")} {
		m := emptyTestModel()
		next, _ := m.Update(open)
		m = next.(model)
		if m.inputMode != modePalette {
			t.Fatalf("%v should open the palette", open)
		}
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if next.(model).inputMode != modeNormal {
			t.Fatal("Esc should close the palette")
		}
	}
}

func TestPalette_ColonTypesIntoActiveFilter(t *testing.T) {
	m := emptyTestModel()
	m.filterActive = true
	next, _ := m.Update(keyRune(":"))
	if next.(model).inputMode == modePalette {
		t.Fatal(": must be filter text while filtering")
	}
}

func TestPalette_RunsMenuActionAndJumpsToEntities(t *testing.T) {
	m := emptyTestModel()
	m.openPalette()
	for _, r := range "stat" { // View → Status
		next, _ := m.Update(keyRune(string(r)))
		m = next.(model)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.inputMode != modeNormal || m.activePanel != panelStatus {
		t.Fatalf("palette should have switched to Status; mode=%v panel=%v", m.inputMode, m.activePanel)
	}

	m = emptyTestModel()
	m.repoList = []repoEntry{{name: "community-skills"}}
	m.openPalette()
	m.paletteQuery = "community"
	matches := m.paletteMatches()
	var repoHit bool
	for _, it := range matches {
		repoHit = repoHit || (it.detail == "Repo" && it.label == "community-skills")
	}
	if !repoHit {
		t.Fatalf("repo should be searchable: %+v", matches)
	}
}

func TestPalette_OmitsDisabledActions(t *testing.T) {
	m := emptyTestModel() // Skills panel, nothing selected for Fork etc.
	m.openPalette()
	for _, it := range m.paletteCandidates() {
		if it.label == "Add Repo" {
			t.Fatal("Add Repo is disabled off the Repos panel and must not be offered")
		}
	}
}

func TestPaletteDialogFitsTerminal(t *testing.T) {
	m := emptyTestModel()
	m.width, m.height = 80, 20
	m.openPalette()
	if got := strings.Count(m.View(), "\n") + 1; got != 20 {
		t.Fatalf("frame has %d lines, want 20", got)
	}
}

func TestPalette_DirectSkillAndPackActions(t *testing.T) {
	m := emptyTestModel()
	m.agents = []string{"claude-code"}
	m.rows = []tuiRow{{kind: skillRow, addr: "r/triage"}}
	m.installed = map[string]map[string]bool{"r/triage": {"claude-code": true}}
	m.st.InstalledPacks = map[string]state.InstalledPackRecord{"r/packs/p": {Agents: []string{"claude-code"}}}
	m.packRows = []packRow{{packAddr: "r/packs/p", installed: true, cells: map[string]packCell{"claude-code": packCellOn}}}
	m.openPalette()

	m.paletteQuery = "triage claude"
	got := m.paletteMatches()
	if len(got) == 0 || got[0].label != "Remove r/triage from claude-code" {
		t.Fatalf("installed skill should offer Remove first, got %+v", got)
	}

	m.paletteQuery = "disable p claude"
	if got := m.paletteMatches(); len(got) == 0 || got[0].detail != "Disable" {
		t.Fatalf("enabled pack should offer Disable, got %+v", got)
	}

	m.paletteQuery = "remove pack"
	got = m.paletteMatches()
	if len(got) == 0 || got[0].label != "Remove pack r/packs/p" {
		t.Fatalf("got %+v", got)
	}
	m.inputMode = modeNormal
	got[0].run(&m)
	if m.inputMode != modePackConfirmRemove {
		t.Fatalf("removing a pack from the palette must still confirm, mode=%v", m.inputMode)
	}
}

// --- Doctor duplicate actions (these write to repos, so they confirm) ---

func dupModel(identical bool, aURL, bURL string) model {
	m := emptyTestModel()
	m.activePanel = panelDoctor
	m.st.Repos = map[string]state.RepoRecord{"a": {URL: aURL}, "b": {URL: bURL}}
	m.doctorFindings = []doctor.Finding{{Kind: doctor.Duplicate, Subject: "a/t", Related: "b/t", Identical: identical, Fix: "x"}}
	return m
}

func choiceLabels(m model) []string {
	var out []string
	for _, o := range m.doctorChoices {
		out = append(out, o.label)
	}
	return out
}

func TestDuplicateChoices_DivergedOffersOverwriteOnlyWhereWritable(t *testing.T) {
	m := dupModel(false, "git@h:a.git", "https://h/b.git") // b has no token: read-only
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	got := strings.Join(choiceLabels(m), "|")
	if m.inputMode != modeDoctorChoice || !strings.Contains(got, "View diff") || !strings.Contains(got, "Make a/t match b/t") {
		t.Fatalf("mode=%v choices=%s", m.inputMode, got)
	}
	if strings.Contains(got, "Make b/t match a/t") || strings.Contains(got, "Delete") {
		t.Fatalf("must not offer writing to a read-only repo or deleting a diverged copy: %s", got)
	}
}

func TestDuplicateChoices_IdenticalOffersDeleteNotInstalled(t *testing.T) {
	m := dupModel(true, "git@h:a.git", "git@h:b.git")
	m.st.InstalledSkills["a/t"] = map[string]state.InstalledSkillRecord{"claude": {}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := strings.Join(choiceLabels(next.(model)), "|")
	if !strings.Contains(got, "Delete b/t") || strings.Contains(got, "Delete a/t") {
		t.Fatalf("an installed copy must not be offered for deletion: %s", got)
	}
}

func TestDuplicateChoices_WriteNeedsConfirmAndDefaultsToNo(t *testing.T) {
	m := dupModel(false, "git@h:a.git", "git@h:b.git")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	m.doctorChoice = 1 // "Make b/t match a/t"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.inputMode != modeConfirmAction || m.confirmYes {
		t.Fatalf("a write must open a confirm that defaults to No: mode=%v yes=%v", m.inputMode, m.confirmYes)
	}
	// Enter on the default (No) must do nothing.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if cmd != nil || m.busy != "" || m.inputMode != modeNormal {
		t.Fatalf("declining must not start work: cmd=%v busy=%q mode=%v", cmd != nil, m.busy, m.inputMode)
	}
}

func TestDuplicateChoices_YesStartsAsyncWrite(t *testing.T) {
	m := dupModel(true, "git@h:a.git", "git@h:b.git")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // first choice: View diff is index 0; pick a Delete
	_ = next
	m.doctorChoice = 1
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, cmd := m.Update(keyRune("y"))
	m = next.(model)
	if !m.doctorDirty {
		t.Fatal("a write must flag the panel for a rescan")
	}
	if cmd == nil || !strings.HasPrefix(m.busy, "Deleting") {
		t.Fatalf("y should start the delete: cmd=%v busy=%q", cmd != nil, m.busy)
	}
}

func TestDoctorWriteDone_SurfacesErrorAndKeepsState(t *testing.T) {
	m := dupModel(true, "git@h:a.git", "git@h:b.git")
	m.busy = "Deleting b/t..."
	next, _ := m.Update(doctorWriteDoneMsg{summary: "Deleted b/t", err: fmt.Errorf("push rejected")})
	m = next.(model)
	if m.busy != "" || !strings.Contains(m.message, "push rejected") {
		t.Fatalf("busy=%q message=%q", m.busy, m.message)
	}
}
