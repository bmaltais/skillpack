package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

func emptyTestModel() model {
	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}
	return initialModel(cfg, st)
}

func keyF(n int) tea.KeyMsg {
	fkeys := []tea.KeyType{tea.KeyF1, tea.KeyF2, tea.KeyF3, tea.KeyF4, tea.KeyF5, tea.KeyF6, tea.KeyF7, tea.KeyF8, tea.KeyF9, tea.KeyF10}
	return tea.KeyMsg{Type: fkeys[n-1]}
}

func keyAlt(ch string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(ch), Alt: true}
}

// TestMenu_F10OpensAndEscCloses verifies F10 opens the File menu and Esc
// closes it without running any action.
func TestMenu_F10OpensAndEscCloses(t *testing.T) {
	m := emptyTestModel()
	next, _ := m.Update(keyF(10))
	m = next.(model)
	if !m.menuOpen {
		t.Fatal("F10 did not open the menu")
	}
	if appMenus[m.menuIndex].label != "File" {
		t.Fatalf("F10 opened %q, want File", appMenus[m.menuIndex].label)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.menuOpen {
		t.Fatal("Esc did not close the menu")
	}
}

// TestMenu_ArrowNavigation walks Right across all five menus and confirms
// wraparound back to File.
func TestMenu_ArrowNavigation(t *testing.T) {
	m := emptyTestModel()
	next, _ := m.Update(keyF(10))
	m = next.(model)

	for i := 0; i < len(appMenus); i++ {
		want := appMenus[i].label
		if appMenus[m.menuIndex].label != want {
			t.Fatalf("step %d: menu is %q, want %q", i, appMenus[m.menuIndex].label, want)
		}
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(model)
	}
	// One more Right should wrap back to File.
	if appMenus[m.menuIndex].label != "File" {
		t.Fatalf("menu did not wrap around to File, got %q", appMenus[m.menuIndex].label)
	}
}

// TestMenu_AltLetterOpensCorrectMenu checks each Alt+letter opens its menu.
func TestMenu_AltLetterOpensCorrectMenu(t *testing.T) {
	cases := map[string]string{"f": "File", "v": "View", "a": "Actions", "p": "Packs", "h": "Help"}
	for letter, want := range cases {
		m := emptyTestModel()
		next, _ := m.Update(keyAlt(letter))
		m = next.(model)
		if !m.menuOpen {
			t.Fatalf("alt+%s did not open a menu", letter)
		}
		if got := appMenus[m.menuIndex].label; got != want {
			t.Fatalf("alt+%s opened %q, want %q", letter, got, want)
		}
	}
}

// TestMenu_F2ThroughF7SwitchPanels checks direct panel-jump keys work from
// any starting panel without opening a menu.
func TestMenu_F2ThroughF7SwitchPanels(t *testing.T) {
	cases := []struct {
		key   tea.KeyMsg
		panel panel
	}{
		{keyF(2), panelSkills},
		{keyF(3), panelStatus},
		{keyF(4), panelRepos},
		{keyF(5), panelUnmanaged},
		{keyF(6), panelPacks},
		{keyF(7), panelDoctor},
	}
	for _, c := range cases {
		m := emptyTestModel()
		next, _ := m.Update(c.key)
		m = next.(model)
		if m.activePanel != c.panel {
			t.Errorf("key did not switch to panel %v, got %v", c.panel, m.activePanel)
		}
		if m.menuOpen {
			t.Errorf("F-key panel switch should not open the menu")
		}
	}
}

// TestMenu_FilterUnaffectedByBareLetters confirms bare letters no longer
// filter on their own (they require '/' first) and are left for menu/
// shortcut handling, while '/'-prefixed typing still fills the filter and
// Esc still clears it (menu stays closed throughout).
func TestMenu_FilterUnaffectedByBareLetters(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelSkills
	for _, ch := range []string{"d", "e", "b"} {
		next, _ := m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.filter != "" {
		t.Fatalf("filter = %q, want %q (bare letters must not filter without '/')", m.filter, "")
	}

	next, _ := m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode")
	}
	for _, ch := range []string{"d", "e", "b"} {
		next, _ := m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.filter != "deb" {
		t.Fatalf("filter = %q, want %q (menu must not consume filter-mode letters)", m.filter, "deb")
	}
	if m.menuOpen {
		t.Fatal("typing filter letters must not open the menu")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.filter != "" {
		t.Fatal("Esc should clear the filter when no menu/input mode is active")
	}
	if m.filterActive {
		t.Fatal("Esc should exit filter mode")
	}
}

// TestSlashFilter_ShortcutsWorkUntilSlashPressed confirms bare single-letter
// shortcuts ('q', 'v') still fire on the Skills and Unmanaged panels when
// filter mode is off, but the same letters feed the filter instead once '/'
// has been pressed.
func TestSlashFilter_ShortcutsWorkUntilSlashPressed(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelSkills

	next, cmd := m.Update(keyRune("q"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("'q' should quit when filter mode is off")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("'q' should send tea.QuitMsg, got %T", cmd())
	}

	next, _ = m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode")
	}

	next, cmd = m.Update(keyRune("q"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("'q' should feed the filter, not quit, once filter mode is active")
	}
	if m.filter != "q" {
		t.Fatalf("filter = %q, want %q", m.filter, "q")
	}
}

// TestSlashFilter_EscRightAfterSlashExitsFilterModeWithoutQuitting confirms
// pressing Esc immediately after '/' (before typing anything) exits filter
// mode rather than quitting the app.
func TestSlashFilter_EscRightAfterSlashExitsFilterModeWithoutQuitting(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelSkills

	next, _ := m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("expected filter mode active after '/'")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if cmd != nil {
		t.Fatal("Esc right after '/' should exit filter mode, not quit")
	}
	if m.filterActive {
		t.Fatal("Esc should have exited filter mode")
	}
}

// TestSlashFilter_UnmanagedPanel mirrors the Skills-panel behavior for the
// Unmanaged panel's 'v' shortcut and its unmanagedFilter field.
func TestSlashFilter_UnmanagedPanel(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelUnmanaged

	next, _ := m.Update(keyRune("/"))
	m = next.(model)
	if !m.filterActive {
		t.Fatal("'/' did not enter filter mode on the Unmanaged panel")
	}

	next, _ = m.Update(keyRune("v"))
	m = next.(model)
	if m.unmanagedFilter != "v" {
		t.Fatalf("unmanagedFilter = %q, want %q", m.unmanagedFilter, "v")
	}
}

// TestMenu_InputModeBeatsMenu confirms a key that would otherwise open a
// menu (F10) is not swallowed by an active input mode's own key handling —
// dialogs take precedence over menu activation, per the precedence chain.
func TestMenu_InputModeBeatsMenu(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelRepos
	m.inputMode = modeAddRepoName
	m.inputBuffer = ""

	next, _ := m.Update(keyF(10))
	m = next.(model)
	if m.menuOpen {
		t.Fatal("F10 opened the menu while an input mode dialog was active")
	}
	if m.inputMode != modeAddRepoName {
		t.Fatalf("input mode changed unexpectedly: %v", m.inputMode)
	}
}

// TestAppMenus_WellFormed asserts every menu item has a non-nil action and
// enabled predicate, and a shortcut recognizable from the docs (or empty,
// for items with no single-key equivalent like "About").
func TestAppMenus_WellFormed(t *testing.T) {
	for _, menu := range appMenus {
		if menu.label == "" {
			t.Errorf("menu with empty label")
		}
		for _, item := range menu.items {
			if item.label == "" {
				t.Errorf("%s: item with empty label", menu.label)
			}
			if item.action == nil {
				t.Errorf("%s > %s: nil action", menu.label, item.label)
			}
			if item.enabled == nil {
				t.Errorf("%s > %s: nil enabled predicate", menu.label, item.label)
			}
		}
	}
}

// TestAppMenus_NoPanicOnEmptyModel walks every menu item's enabled predicate
// and, when true, its action, against a freshly initialized model with no
// repos, skills, or selection — the state a menu item might see on first
// launch. Actions must not panic even when their preferred selection
// context is absent.
func TestAppMenus_NoPanicOnEmptyModel(t *testing.T) {
	cfg := &config.Config{Agents: map[string]config.AgentConfig{}}
	st := &state.State{
		Repos:           make(map[string]state.RepoRecord),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
	}

	for _, menu := range appMenus {
		for _, item := range menu.items {
			m := initialModel(cfg, st)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s > %s panicked on empty model: %v", menu.label, item.label, r)
					}
				}()
				if item.enabled(&m) {
					item.action(&m)
				}
			}()
		}
	}
}
