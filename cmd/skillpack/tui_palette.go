package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// paletteMaxRows caps the result list so the dialog fits small terminals.
const paletteMaxRows = 10

// paletteItem is one palette result. run reuses an existing action or panel
// switch; the palette adds no behavior of its own.
type paletteItem struct {
	label  string
	detail string
	run    func(*model) tea.Cmd
	score  int
}

func (m *model) openPalette() {
	m.inputMode = modePalette
	m.paletteQuery = ""
	m.paletteCursor = 0
	m.menuOpen = false
}

// fuzzyScore reports whether query's characters appear in s in order
// (case-insensitive) and how good the match is: contiguous runs and word
// starts score higher. An empty query matches everything with score 0.
func fuzzyScore(query, s string) (int, bool) {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	score, qi, prev := 0, 0, -2
	prevRune := ' '
	for i, r := range []rune(strings.ToLower(s)) {
		if qi < len(q) && r == q[qi] {
			score++
			if i == prev+1 {
				score += 3
			}
			if !unicode.IsLetter(prevRune) && !unicode.IsDigit(prevRune) {
				score += 2
			}
			prev = i
			qi++
		}
		prevRune = r
	}
	return score, qi == len(q)
}

// paletteCandidates lists everything the palette can run right now: the
// enabled menu actions, and (once something is typed) jumps to individual
// skills, repos and packs.
func (m *model) paletteCandidates() []paletteItem {
	var items []paletteItem
	for _, menu := range appMenus {
		for _, it := range menu.items {
			if !it.enabled(m) {
				continue
			}
			it := it
			items = append(items, paletteItem{label: it.label, detail: menu.label + "  " + it.shortcut, run: it.action})
		}
	}
	if m.paletteQuery == "" {
		return items
	}
	for i, r := range m.rows {
		if r.kind != skillRow {
			continue
		}
		i, addr := i, r.addr
		items = append(items, paletteItem{label: addr, detail: "Skill", run: func(m *model) tea.Cmd {
			cmd := m.switchPanel(panelSkills)
			m.filter = addr
			m.cursorRow = i
			return cmd
		}})
	}
	for _, r := range m.repoList {
		name := r.name
		items = append(items, paletteItem{label: name, detail: "Repo", run: func(m *model) tea.Cmd {
			cmd := m.switchPanel(panelRepos)
			m.repoFilter = name
			m.refreshRepos()
			return cmd
		}})
	}
	for _, r := range m.packRows {
		addr := r.packAddr
		items = append(items, paletteItem{label: addr, detail: "Pack", run: func(m *model) tea.Cmd {
			cmd := m.switchPanel(panelPacks)
			m.packFilter = addr
			m.refreshPacks()
			return cmd
		}})
	}
	return items
}

// paletteMatches filters and ranks candidates for the current query.
func (m *model) paletteMatches() []paletteItem {
	var out []paletteItem
	for _, it := range m.paletteCandidates() {
		if sc, ok := fuzzyScore(m.paletteQuery, it.label); ok {
			it.score = sc
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

// handlePaletteKey drives the palette while modePalette is active.
func (m *model) handlePaletteKey(msg tea.KeyMsg) (model, tea.Cmd) {
	matches := m.paletteMatches()
	switch msg.Type {
	case tea.KeyEsc:
		m.inputMode = modeNormal
	case tea.KeyUp:
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
	case tea.KeyDown:
		if m.paletteCursor < min(len(matches), paletteMaxRows)-1 {
			m.paletteCursor++
		}
	case tea.KeyEnter:
		if m.paletteCursor < len(matches) {
			run := matches[m.paletteCursor].run
			m.inputMode = modeNormal
			return *m, run(m)
		}
	case tea.KeyBackspace:
		if m.paletteQuery != "" {
			_, n := utf8.DecodeLastRuneInString(m.paletteQuery)
			m.paletteQuery = m.paletteQuery[:len(m.paletteQuery)-n]
			m.paletteCursor = 0
		}
	case tea.KeyRunes:
		m.paletteQuery += msg.String()
		m.paletteCursor = 0
	case tea.KeySpace:
		m.paletteQuery += " "
		m.paletteCursor = 0
	case tea.KeyCtrlC:
		return *m, tea.Quit
	}
	return *m, nil
}

func paletteDialog(m model) (string, []string, bool) {
	lines := []string{fmt.Sprintf("> %s▌", m.paletteQuery), ""}
	matches := m.paletteMatches()
	if len(matches) == 0 {
		lines = append(lines, dimStyle.Render("No matches"))
	}
	for i, it := range matches {
		if i >= paletteMaxRows {
			break
		}
		lines = append(lines, listLine(m, fmt.Sprintf("%-34s %s", truncate(it.label, 34), dimStyle.Render(it.detail)), i == m.paletteCursor))
	}
	lines = append(lines, "", dimStyle.Render("type to search • ↑↓ select • Enter run • Esc close"))
	return "Command Palette", lines, true
}
