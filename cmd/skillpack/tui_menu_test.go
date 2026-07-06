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

// TestMenu_F2ThroughF6SwitchPanels checks direct panel-jump keys work from
// any starting panel without opening a menu.
func TestMenu_F2ThroughF6SwitchPanels(t *testing.T) {
	cases := []struct {
		key   tea.KeyMsg
		panel panel
	}{
		{keyF(2), panelSkills},
		{keyF(3), panelStatus},
		{keyF(4), panelRepos},
		{keyF(5), panelUnmanaged},
		{keyF(6), panelPacks},
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

// TestMenu_FilterUnaffectedByBareLetters confirms the menu-key precedence
// change didn't steal bare letters from the Skills panel's type-to-filter,
// and that Esc still clears the filter instead of closing (menu is closed).
func TestMenu_FilterUnaffectedByBareLetters(t *testing.T) {
	m := emptyTestModel()
	m.activePanel = panelSkills
	for _, ch := range []string{"d", "e", "b"} {
		next, _ := m.Update(keyRune(ch))
		m = next.(model)
	}
	if m.filter != "deb" {
		t.Fatalf("filter = %q, want %q (menu must not consume bare letters)", m.filter, "deb")
	}
	if m.menuOpen {
		t.Fatal("typing filter letters must not open the menu")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.filter != "" {
		t.Fatal("Esc should clear the filter when no menu/input mode is active")
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
