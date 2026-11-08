package main

import (
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
