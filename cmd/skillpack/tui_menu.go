package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// menuItem is one selectable row in a dropdown. action calls into an
// existing handler — menus are a second door to the same code paths a
// keypress would trigger, never a reimplementation. Items whose enabled
// predicate returns false render dim and are skipped by Enter, DOS Shell
// style, but stay navigable.
type menuItem struct {
	label    string
	shortcut string
	enabled  func(m *model) bool
	action   func(m *model) tea.Cmd
}

// menuDef is one top-level menu (File, View, ...). The mnemonic is always
// the first letter of label.
type menuDef struct {
	label string
	items []menuItem
}

func alwaysEnabled(*model) bool { return true }

// skillRowSelected reports whether the cursor sits on a skill row in the
// Skills panel — the precondition for Toggle Install, Fork, and Repair.
func skillRowSelected(m *model) bool {
	return m.activePanel == panelSkills && m.cursorRow >= 0 && m.cursorRow < len(m.rows) && m.rows[m.cursorRow].kind == skillRow
}

// appMenus is the single source of truth for the menu bar, the dropdown
// contents, and (later) the F1 help screen.
var appMenus = []menuDef{
	{
		label: "File",
		items: []menuItem{
			{"Add Repo", "a", func(m *model) bool { return m.activePanel == panelRepos }, func(m *model) tea.Cmd { m.startAddRepo(); return nil }},
			{"Remove Repo", "d", func(m *model) bool { return m.activePanel == panelRepos && len(m.repoList) > 0 }, func(m *model) tea.Cmd { m.startRemoveRepo(); return nil }},
			{"Add Agent", "", alwaysEnabled, func(m *model) tea.Cmd { m.startAddAgent(); return nil }},
			{"Self-Update", "U", alwaysEnabled, func(m *model) tea.Cmd { return m.startSelfUpdate() }},
			{"Exit", "q", alwaysEnabled, func(m *model) tea.Cmd { return tea.Quit }},
		},
	},
	{
		label: "View",
		items: []menuItem{
			{"Skills", "F2", alwaysEnabled, func(m *model) tea.Cmd { return m.switchPanel(panelSkills) }},
			{"Status", "F3", alwaysEnabled, func(m *model) tea.Cmd { return m.switchPanel(panelStatus) }},
			{"Repos", "F4", alwaysEnabled, func(m *model) tea.Cmd { return m.switchPanel(panelRepos) }},
			{"Unmanaged", "F5", alwaysEnabled, func(m *model) tea.Cmd { return m.switchPanel(panelUnmanaged) }},
			{"Packs", "F6", alwaysEnabled, func(m *model) tea.Cmd { return m.switchPanel(panelPacks) }},
			{"Refresh Status", "r", alwaysEnabled, func(m *model) tea.Cmd {
				m.busy = "Refreshing status..."
				m.message = ""
				return m.cmdCheckStatus()
			}},
		},
	},
	{
		label: "Actions",
		items: []menuItem{
			{"Toggle Install", "Space", skillRowSelected, func(m *model) tea.Cmd { m.handleEnter(); return nil }},
			{"Fork", "f", skillRowSelected, func(m *model) tea.Cmd { m.startFork(); return nil }},
			{"Repair", "R", func(m *model) bool {
				return skillRowSelected(m) && m.rows[m.cursorRow].problem != problemNone
			}, func(m *model) tea.Cmd { m.startRepair(); return nil }},
			{"View SKILL.md", "v", func(m *model) bool {
				if m.activePanel == panelSkills {
					return skillRowSelected(m)
				}
				return m.activePanel == panelUnmanaged && m.unmanagedCursor >= 0 && m.unmanagedCursor < len(m.unmanagedEntries)
			}, func(m *model) tea.Cmd { return m.startViewSkillMd() }},
			{"Update Skill", "u", func(m *model) bool {
				return m.activePanel == panelStatus && m.statusCursor < len(m.statusRows)
			}, func(m *model) tea.Cmd { m.updateSelectedSkill(); return nil }},
			{"Sync All", "S", alwaysEnabled, func(m *model) tea.Cmd {
				m.busy = "Syncing all..."
				m.message = ""
				return m.cmdSync()
			}},
			{"Adopt", "Enter", func(m *model) bool {
				return m.activePanel == panelUnmanaged && len(m.unmanagedEntries) > 0
			}, func(m *model) tea.Cmd { m.startAdopt(); return nil }},
		},
	},
	{
		label: "Packs",
		items: []menuItem{
			{"Create Pack", "n", alwaysEnabled, func(m *model) tea.Cmd { m.startPackCreate(); return nil }},
			{"Edit Pack", "e", func(m *model) bool { return m.packCursor < len(m.packRows) }, func(m *model) tea.Cmd { m.startPackEdit(); return nil }},
			{"Install Pack", "i", func(m *model) bool {
				return m.packCursor < len(m.packRows) && !m.packRows[m.packCursor].installed
			}, func(m *model) tea.Cmd { m.startPackInstall(); return nil }},
			{"Remove Pack", "d", func(m *model) bool {
				return m.packCursor < len(m.packRows) && m.packRows[m.packCursor].installed
			}, func(m *model) tea.Cmd { m.startPackRemove(); return nil }},
		},
	},
	{
		label: "Help",
		items: []menuItem{
			{"Keys", "F1", alwaysEnabled, func(m *model) tea.Cmd {
				m.inputMode = modeHelp
				m.helpScroll = 0
				return nil
			}},
			{"About", "", alwaysEnabled, func(m *model) tea.Cmd {
				m.message = fmt.Sprintf("SkillPack %s", Version)
				return nil
			}},
		},
	},
}

// menuIndexByLabel returns the appMenus index for a top-level menu label, or
// 0 if not found.
func menuIndexByLabel(label string) int {
	for i, mnu := range appMenus {
		if mnu.label == label {
			return i
		}
	}
	return 0
}

// handleGlobalKey processes F-keys and Alt+letter menu activation, available
// from any panel regardless of what's selected. handled=false means the key
// wasn't ours — the caller falls through to panel-specific key handling
// (bare letters remain filter input / existing single-letter shortcuts).
func (m *model) handleGlobalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	openMenu := func(label string) (tea.Cmd, bool) {
		m.menuOpen = true
		m.menuIndex = menuIndexByLabel(label)
		m.menuItemIndex = 0
		return nil, true
	}
	switch msg.String() {
	case "f10", "alt+f":
		return openMenu("File")
	case "alt+v":
		return openMenu("View")
	case "alt+a":
		return openMenu("Actions")
	case "alt+p":
		return openMenu("Packs")
	case "alt+h":
		return openMenu("Help")
	case "f1":
		m.inputMode = modeHelp
		m.helpScroll = 0
		return nil, true
	case "f2":
		return m.switchPanel(panelSkills), true
	case "f3":
		return m.switchPanel(panelStatus), true
	case "f4":
		return m.switchPanel(panelRepos), true
	case "f5":
		return m.switchPanel(panelUnmanaged), true
	case "f6":
		return m.switchPanel(panelPacks), true
	}
	return nil, false
}

// handleMenuKey processes a key press while a dropdown menu is open: arrows
// navigate, Enter or a mnemonic letter runs the highlighted/matched item
// (if enabled) and closes the menu, Esc/F10 closes without action.
func (m *model) handleMenuKey(msg tea.KeyMsg) (model, tea.Cmd) {
	items := appMenus[m.menuIndex].items

	runItem := func(item menuItem) (model, tea.Cmd) {
		m.menuOpen = false
		if item.enabled(m) {
			return *m, item.action(m)
		}
		return *m, nil
	}

	switch msg.String() {
	case "esc", "f10":
		m.menuOpen = false
		return *m, nil
	case "left":
		m.menuIndex = (m.menuIndex - 1 + len(appMenus)) % len(appMenus)
		m.menuItemIndex = 0
		return *m, nil
	case "right":
		m.menuIndex = (m.menuIndex + 1) % len(appMenus)
		m.menuItemIndex = 0
		return *m, nil
	case "up":
		m.menuItemIndex = (m.menuItemIndex - 1 + len(items)) % len(items)
		return *m, nil
	case "down":
		m.menuItemIndex = (m.menuItemIndex + 1) % len(items)
		return *m, nil
	case "enter":
		return runItem(items[m.menuItemIndex])
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		ch := strings.ToLower(string(msg.Runes))
		for _, item := range items {
			if strings.ToLower(item.label[:1]) == ch {
				return runItem(item)
			}
		}
	}
	return *m, nil
}

// menuLabelOffset returns the column (0-indexed) where appMenus[idx]'s label
// begins in the rendered menu bar, mirroring renderMenuBar's layout math.
func menuLabelOffset(idx int) int {
	offset := 1 // leading space
	for i := 0; i < idx; i++ {
		offset += len(appMenus[i].label) + 2 // label + 2-space gap
	}
	return offset
}

// renderDropdownBox renders the open menu as a single-line-bordered box:
// items as " Label        shortcut ", shortcut right-aligned, the
// highlighted item reverse-video, disabled items dim.
func renderDropdownBox(m *model) []string {
	menu := appMenus[m.menuIndex]

	width := 0
	for _, it := range menu.items {
		w := len(it.label) + 2 + len(it.shortcut)
		if w > width {
			width = w
		}
	}
	width += 2 // padding

	lines := make([]string, 0, len(menu.items)+2)
	lines = append(lines, "┌"+strings.Repeat("─", width)+"┐")
	for i, it := range menu.items {
		gap := width - 1 - len(it.label) - len(it.shortcut)
		if gap < 1 {
			gap = 1
		}
		text := " " + it.label + strings.Repeat(" ", gap) + it.shortcut
		if len(text) > width {
			text = text[:width]
		} else if len(text) < width {
			text += strings.Repeat(" ", width-len(text))
		}

		switch {
		case i == m.menuItemIndex:
			text = selectedStyle.Render(text)
		case !it.enabled(m):
			text = dimStyle.Render(text)
		}
		lines = append(lines, "│"+text+"│")
	}
	lines = append(lines, "└"+strings.Repeat("─", width)+"┘")
	return lines
}

// overlayDropdown, when a menu is open, occludes the top rows of body
// (starting right under the menu bar) with the dropdown box positioned
// under its label. Whole-line replacement — never mid-line ANSI splicing —
// so it can't garble styled panel content underneath.
func overlayDropdown(m model, body string) string {
	if !m.menuOpen {
		return body
	}
	lines := strings.Split(body, "\n")
	offset := menuLabelOffset(m.menuIndex)
	prefix := strings.Repeat(" ", offset)
	for i, boxLine := range renderDropdownBox(&m) {
		row := 2 + i // row 0 = title bar, row 1 = menu bar
		for row >= len(lines) {
			lines = append(lines, "")
		}
		lines[row] = prefix + boxLine
	}
	return strings.Join(lines, "\n")
}
