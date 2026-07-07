package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// --- View Styles (moved from tui.go in Phase 1) ---

var (
	titleStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	selectedStyle       = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	cellSelStyle        = lipgloss.NewStyle().Background(lipgloss.Color("62")).Bold(true)
	repoStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	dimStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	checkStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	emptyStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	filterStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	msgStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	helpStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	inputStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	updateStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("117")) // cyan - update available
	modifiedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow - locally modified
	forkStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // amber  - forked skill
	conflictStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red - conflict
	bannerStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("220")).Bold(true)
	bannerBtnActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62")).Bold(true)
	bannerBtnInactive   = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Background(lipgloss.Color("238"))
	staleStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true) // orange  - stale address
	brokenUpstreamStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // red - broken upstream
	partialStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true) // yellow - partial pack
)

// panelName returns the display name used in the title bar for p.
func panelName(p panel) string {
	switch p {
	case panelSkills:
		return "Skills"
	case panelStatus:
		return "Status"
	case panelRepos:
		return "Repos"
	case panelUnmanaged:
		return "Unmanaged"
	case panelPacks:
		return "Packs"
	default:
		return ""
	}
}

// renderTitleBar renders the full-width reverse-video title bar: app name,
// version, and the active panel's name, roughly centered.
func renderTitleBar(m model) string {
	text := fmt.Sprintf("SkillPack %s ── %s", Version, panelName(m.activePanel))
	pad := (m.width - lipgloss.Width(text)) / 2
	if pad < 0 {
		pad = 0
	}
	return chromeBarStyle.Render(padLine(strings.Repeat(" ", pad)+text, m.width))
}

// renderMenuBar renders the DOS Shell-style menu bar. Visual only in this
// phase — F10/Alt+letter interaction is wired in a later move.
func renderMenuBar(m model) string {
	plainLen := 1
	var sb strings.Builder
	sb.WriteString(chromeBarStyle.Render(" "))
	for i, menu := range appMenus {
		it := menu.label
		if i > 0 {
			sb.WriteString(chromeBarStyle.Render("  "))
			plainLen += 2
		}
		sb.WriteString(chromeAccentStyle.Render(it[:1]))
		sb.WriteString(chromeBarStyle.Render(it[1:]))
		plainLen += lipgloss.Width(it)
	}
	if pad := m.width - plainLen; pad > 0 {
		sb.WriteString(chromeBarStyle.Render(strings.Repeat(" ", pad)))
	}
	return sb.String()
}

// hintForPanel returns the left-hand key hint shown in the bottom status
// bar for the active panel, mirroring what each panel used to render
// inline as its own footer help line.
func hintForPanel(m model) string {
	switch m.activePanel {
	case panelSkills:
		return "↑↓ navigate  ←→ agents  Space/Enter toggle  f fork  R repair  v view  Tab switch  q quit"
	case panelStatus:
		rHelp := "r refresh"
		if m.statusCursor < len(m.statusRows) {
			row := m.statusRows[m.statusCursor]
			if _, ok := m.forkCandidates[row.addr]; ok {
				rHelp = "r register fork"
			}
		}
		return fmt.Sprintf("↑↓ navigate  u update selected  S sync all  %s  U self-update  Tab switch  q quit", rHelp)
	case panelRepos:
		return "↑↓ navigate  a add  d remove  Tab skills  q quit"
	case panelUnmanaged:
		return "↑↓ navigate  Type to filter  Enter adopt into repo  v view  Tab switch  q quit"
	case panelPacks:
		if m.packDetailOpen && m.packCursor < len(m.packRows) {
			row := m.packRows[m.packCursor]
			if !row.installed {
				return "i install  e edit  Esc back  Tab switch  q quit"
			}
			if row.isPartial {
				return "c complete deployment  e edit  d remove  Esc back  Tab switch  q quit"
			}
			return "e edit  d remove  Esc back  Tab switch  q quit"
		}
		help := "↑↓ navigate  Enter detail  n new  Tab switch  q quit"
		if m.packCursor < len(m.packRows) {
			row := m.packRows[m.packCursor]
			switch {
			case !row.installed:
				help = "↑↓ navigate  Enter detail  i install  n new  e edit  Tab switch  q quit"
			case row.isPartial:
				help = "↑↓ navigate  Enter detail  c complete  n new  e edit  d remove  Tab switch  q quit"
			default:
				help = "↑↓ navigate  Enter detail  n new  e edit  d remove  Tab switch  q quit"
			}
		}
		return help
	default:
		return ""
	}
}

// renderStatusBar renders the full-width reverse-video bottom bar: the
// active panel's key hints on the left, the menu-activation hint on the
// right.
func renderStatusBar(m model) string {
	left := " " + hintForPanel(m)
	right := "F10=Menu "
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
		maxLeft := m.width - lipgloss.Width(right) - gap
		if maxLeft < 0 {
			maxLeft = 0
		}
		if lipgloss.Width(left) > maxLeft {
			runes := []rune(left)
			if maxLeft < len(runes) {
				runes = runes[:maxLeft]
			}
			left = string(runes)
		}
	}
	line := left + strings.Repeat(" ", gap) + right
	return chromeBarStyle.Render(padLine(line, m.width))
}

// render pads body to fill the terminal height and pins the status bar to
// the last row, regardless of how much of the height the body used.
func (m model) render(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, renderStatusBar(m))
	return strings.Join(lines, "\n")
}

// View renders the entire TUI (delegates to the four panel-specific view* methods).
func (m model) View() string {
	var b strings.Builder

	b.WriteString(renderTitleBar(m))
	b.WriteString("\n")
	b.WriteString(renderMenuBar(m))
	b.WriteString("\n")

	// Update banner
	if m.updateBanner != "" {
		current := strings.TrimPrefix(Version, "v")
		bannerText := fmt.Sprintf(" ⚠ Update available: v%s → %s ", current, m.updateBanner)
		updateBtn := " Update "
		skipBtn := " Skip "
		if m.bannerSelection == 0 {
			updateBtn = bannerBtnActive.Render(updateBtn)
			skipBtn = bannerBtnInactive.Render(skipBtn)
		} else {
			updateBtn = bannerBtnInactive.Render(updateBtn)
			skipBtn = bannerBtnActive.Render(skipBtn)
		}
		b.WriteString(bannerStyle.Render(bannerText))
		b.WriteString("  ")
		b.WriteString(updateBtn)
		b.WriteString(" ")
		b.WriteString(skipBtn)
		b.WriteString("\n")
	}

	// Busy indicator
	if m.busy != "" {
		b.WriteString(inputStyle.Render(" ⌛ " + m.busy))
		b.WriteString("\n")
	}

	// Embedded pack wizard replaces the active panel while open.
	if m.packWizard != nil {
		b.WriteString("\n")
		b.WriteString(m.packWizard.View())
		return m.render(overlayDropdown(m, overlayDialog(m, b.String())))
	}

	switch m.activePanel {
	case panelSkills:
		m.viewSkills(&b)
	case panelStatus:
		m.viewStatus(&b)
	case panelRepos:
		m.viewRepos(&b)
	case panelUnmanaged:
		m.viewUnmanaged(&b)
	case panelPacks:
		m.viewPacks(&b)
	}

	return m.render(overlayDropdown(m, overlayDialog(m, b.String())))
}

func (m model) viewSkills(b *strings.Builder) {
	// Fork/repair overlays render as centered dialogs (tui_dialogs.go); the
	// panel keeps rendering its normal content underneath.

	// Filter
	if m.filter != "" {
		b.WriteString(filterStyle.Render(fmt.Sprintf(" Filter: %s▌", m.filter)))
	} else {
		b.WriteString(dimStyle.Render(" Type to filter…"))
	}
	b.WriteString("\n\n")

	// Column widths — dynamic based on terminal width
	agentColW := 7
	for _, a := range m.agents {
		if len(a)+2 > agentColW {
			agentColW = len(a) + 2
		}
	}

	// Ensure agent columns fit within terminal; shrink agentColW if needed
	nAgents := len(m.agents)
	if nAgents > 0 {
		maxAgentW := (m.width - 12) / nAgents // reserve 12 for name col minimum + padding
		if maxAgentW < 5 {
			maxAgentW = 5
		}
		if agentColW > maxAgentW {
			agentColW = maxAgentW
		}
	}

	// Available width for the name column: total - agent columns - leading space
	totalAgentW := agentColW * nAgents
	nameColW := m.width - totalAgentW - 2
	if nameColW < 10 {
		nameColW = 10
	}
	// Hard clamp: total must not exceed terminal width
	if nameColW+totalAgentW+2 > m.width {
		nameColW = m.width - totalAgentW - 2
		if nameColW < 5 {
			nameColW = 5
		}
	}

	// Compute dynamic name column width based on longest visible skill (don't exceed available)
	vis := m.visibleRows()
	contentW := 34 // minimum default
	for _, idx := range vis {
		row := m.rows[idx]
		if row.kind == skillRow {
			w := len(row.relPath) + 6 // indent + padding
			if w > contentW {
				contentW = w
			}
		}
	}
	if contentW < nameColW {
		nameColW = contentW
	}

	// Header row
	header := fmt.Sprintf(" %-*s", nameColW, "SKILL")
	for _, a := range m.agents {
		name := a
		if len(name) > agentColW-1 {
			name = name[:agentColW-2] + "…"
		}
		header += fmt.Sprintf(" %-*s", agentColW-1, name)
	}
	b.WriteString(dimStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")

	// Scrolling
	maxRows := m.height - 10 // header(4) + footer(5) + chrome(1)
	if maxRows < 5 {
		maxRows = 5
	}

	// Find cursor position in visible list for scroll offset
	curVisIdx := 0
	for i, idx := range vis {
		if idx == m.cursorRow {
			curVisIdx = i
			break
		}
	}
	offset := 0
	if curVisIdx >= maxRows {
		offset = curVisIdx - maxRows + 1
	}

	// Render visible rows
	displayed := 0
	for i := offset; i < len(vis) && displayed < maxRows; i++ {
		idx := vis[i]
		row := m.rows[idx]
		isSelected := idx == m.cursorRow

		if row.kind == repoRow {
			arrow := "▼"
			if !row.expanded {
				arrow = "▶"
			}
			label := fmt.Sprintf(" %s %s", arrow, row.repoName)
			padded := fmt.Sprintf("%-*s", nameColW+1+(agentColW*len(m.agents)), label)
			if isSelected {
				b.WriteString(selectedStyle.Render(padded))
			} else {
				b.WriteString(repoStyle.Render(padded))
			}
		} else {
			// Skill name with optional glyphs/badges
			upstream := m.upstreamAddr(row.addr)
			isFork := upstream != ""

			const forkGlyph = " ⑂"
			const forkGlyphW = 2
			const staleGlyph = " [stale]"
			const staleGlyphW = 8
			const brokenGlyph = " [broken upstream]"
			const brokenGlyphW = 18

			label := fmt.Sprintf("     %s", row.relPath)

			switch {
			case row.problem == problemStale:
				// [stale] badge — skill is no longer discoverable upstream
				maxLabelW := nameColW - staleGlyphW
				if maxLabelW < 1 {
					maxLabelW = 1
				}
				if len(label) > maxLabelW {
					label = label[:maxLabelW-1] + "…"
				}
				nameStr := fmt.Sprintf("%-*s", nameColW+1-staleGlyphW, label)
				if isSelected {
					b.WriteString(selectedStyle.Render(nameStr))
				} else {
					b.WriteString(nameStr)
				}
				b.WriteString(staleStyle.Render(staleGlyph))
			case row.problem == problemBrokenUpstream:
				// fork glyph + [broken upstream] badge
				maxLabelW := nameColW - forkGlyphW - brokenGlyphW
				if maxLabelW < 1 {
					maxLabelW = 1
				}
				if len(label) > maxLabelW {
					label = label[:maxLabelW-1] + "…"
				}
				nameStr := fmt.Sprintf("%-*s", nameColW+1-forkGlyphW-brokenGlyphW, label)
				if isSelected {
					b.WriteString(selectedStyle.Render(nameStr))
				} else {
					b.WriteString(nameStr)
				}
				b.WriteString(forkStyle.Render(forkGlyph))
				b.WriteString(brokenUpstreamStyle.Render(brokenGlyph))
			case isFork:
				// Normal fork: fork glyph only
				maxLabelW := nameColW - forkGlyphW
				if len(label) > maxLabelW {
					label = label[:maxLabelW-1] + "…"
				}
				nameStr := fmt.Sprintf("%-*s", nameColW+1-forkGlyphW, label)
				if isSelected {
					b.WriteString(selectedStyle.Render(nameStr))
				} else {
					b.WriteString(nameStr)
				}
				b.WriteString(forkStyle.Render(forkGlyph))
			default:
				if len(label) > nameColW {
					label = label[:nameColW-1] + "…"
				}
				nameStr := fmt.Sprintf("%-*s", nameColW+1, label)
				if isSelected {
					b.WriteString(selectedStyle.Render(nameStr))
				} else {
					b.WriteString(nameStr)
				}
			}

			// Agent cells
			for j, agent := range m.agents {
				isInstalled := m.installed[row.addr] != nil && m.installed[row.addr][agent]
				var cell string
				if isInstalled {
					// Show status indicator if available
					status := ""
					if m.statusInfo != nil && m.statusInfo[row.addr] != nil {
						status = m.statusInfo[row.addr][agent]
					}
					switch status {
					case "update":
						cell = " [↑] "
					case "modified":
						cell = " [~] "
					case "conflict":
						cell = " [!] "
					case "error":
						cell = " [?] "
					default:
						cell = " [✓] "
					}
				} else {
					cell = " [ ] "
				}
				padded := fmt.Sprintf("%-*s", agentColW, cell)

				if isSelected && j == m.cursorCol {
					b.WriteString(cellSelStyle.Render(padded))
				} else if isInstalled {
					// Color based on status
					status := ""
					if m.statusInfo != nil && m.statusInfo[row.addr] != nil {
						status = m.statusInfo[row.addr][agent]
					}
					switch status {
					case "update":
						b.WriteString(updateStyle.Render(padded))
					case "modified":
						b.WriteString(modifiedStyle.Render(padded))
					case "conflict":
						b.WriteString(conflictStyle.Render(padded))
					case "error":
						b.WriteString(conflictStyle.Render(padded))
					default:
						b.WriteString(checkStyle.Render(padded))
					}
				} else {
					b.WriteString(emptyStyle.Render(padded))
				}
			}
		}
		b.WriteString("\n")
		displayed++
	}

	// Pad remaining space
	for i := displayed; i < maxRows; i++ {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		// Show problem badge hint or fork provenance when cursor is on a skill
		if m.cursorRow >= 0 && m.cursorRow < len(m.rows) {
			selRow := m.rows[m.cursorRow]
			if selRow.kind == skillRow {
				switch selRow.problem {
				case problemStale:
					b.WriteString(staleStyle.Render(" [stale] skill path no longer exists upstream — press R to repair"))
					return
				case problemBrokenUpstream:
					b.WriteString(brokenUpstreamStyle.Render(" [broken upstream] upstream tracking pointer is invalid — press R to repair"))
					return
				default:
					if upstream := m.upstreamAddr(selRow.addr); upstream != "" {
						line := fmt.Sprintf(" ⑂ forked from %s", upstream)
						if m.width > 2 && len(line) > m.width-2 {
							line = line[:m.width-3] + "…"
						}
						b.WriteString(forkStyle.Render(line))
						return
					}
				}
			}
		}
		skillCount := 0
		repoCount := 0
		for _, r := range m.rows {
			if r.kind == repoRow {
				repoCount++
			} else {
				skillCount++
			}
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d skills in %d repos  •  %d agents", skillCount, repoCount, len(m.agents))))
	}
}

// upstreamAddr returns the upstream skill address for a forked skill, or "" if none.
func (m model) upstreamAddr(addr string) string {
	for _, rec := range m.st.InstalledSkills[addr] {
		if rec.UpstreamAddr != "" {
			return rec.UpstreamAddr
		}
	}
	return ""
}

func (m model) viewStatus(b *strings.Builder) {
	// Register-fork-provenance renders as a centered dialog (tui_dialogs.go).
	b.WriteString("\n")

	if len(m.statusRows) == 0 {
		b.WriteString(dimStyle.Render("   No status data. Press 'r' to refresh."))
		b.WriteString("\n")
		if len(m.st.InstalledSkills) == 0 {
			b.WriteString(dimStyle.Render("   (No skills installed)"))
			b.WriteString("\n")
		}
	} else {
		// Compute column widths
		addrW := 5
		agentW := 5
		for _, row := range m.statusRows {
			if len(row.addr) > addrW {
				addrW = len(row.addr)
			}
			if len(row.agentName) > agentW {
				agentW = len(row.agentName)
			}
		}
		// Cap addrW to half the terminal width
		if addrW > m.width/2 {
			addrW = m.width / 2
		}
		// Enforce minimum only if terminal is wide enough
		if addrW < 10 && m.width >= 30 {
			addrW = 10
		} else if addrW < 5 {
			addrW = 5
		}
		// Ensure total columns fit: addrW + agentW + status(~20) + padding(5)
		maxAddr := m.width - agentW - 25
		if maxAddr < 5 {
			maxAddr = 5
		}
		if addrW > maxAddr {
			addrW = maxAddr
		}

		// Header
		header := fmt.Sprintf(" %-*s  %-*s  %s", addrW, "SKILL", agentW, "AGENT", "STATUS")
		b.WriteString(dimStyle.Render(header))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
		b.WriteString("\n")

		// Scrolling
		maxRows := m.height - 11
		if maxRows < 5 {
			maxRows = 5
		}

		offset := 0
		if m.statusCursor >= maxRows {
			offset = m.statusCursor - maxRows + 1
		}

		displayed := 0
		for i := offset; i < len(m.statusRows) && displayed < maxRows; i++ {
			row := m.statusRows[i]
			isSelected := i == m.statusCursor

			// Status label with color
			var statusLabel string
			var statusStyled string
			switch row.status {
			case "ok":
				statusLabel = "✓ up-to-date"
				statusStyled = checkStyle.Render(statusLabel)
			case "update":
				statusLabel = "↑ update available"
				statusStyled = updateStyle.Render(statusLabel)
			case "modified":
				statusLabel = "~ locally modified"
				statusStyled = modifiedStyle.Render(statusLabel)
			case "conflict":
				statusLabel = "! conflict"
				statusStyled = conflictStyle.Render(statusLabel)
			case "error":
				statusLabel = "? error"
				statusStyled = conflictStyle.Render(statusLabel)
			default:
				statusLabel = row.status
				statusStyled = dimStyle.Render(statusLabel)
			}

			addr := row.addr
			if len(addr) > addrW {
				addr = addr[:addrW-1] + "…"
			}

			// Append [fork?] badge when the skill is a detected fork candidate.
			if _, isCandidate := m.forkCandidates[row.addr]; isCandidate {
				statusStyled += "  " + modifiedStyle.Render("[fork?]")
			}

			line := fmt.Sprintf(" %-*s  %-*s  ", addrW, addr, agentW, row.agentName)
			if isSelected {
				b.WriteString(selectedStyle.Render(line))
				b.WriteString(statusStyled)
			} else {
				b.WriteString(line)
				b.WriteString(statusStyled)
			}
			b.WriteString("\n")
			displayed++
		}

		for i := displayed; i < maxRows; i++ {
			b.WriteString("\n")
		}
	}

	// Summary counts
	var nOK, nUpdate, nModified, nConflict int
	for _, row := range m.statusRows {
		switch row.status {
		case "ok":
			nOK++
		case "update":
			nUpdate++
		case "modified":
			nModified++
		case "conflict":
			nConflict++
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")

	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else if len(m.statusRows) > 0 {
		parts := []string{}
		if nOK > 0 {
			parts = append(parts, fmt.Sprintf("%d up-to-date", nOK))
		}
		if nUpdate > 0 {
			parts = append(parts, fmt.Sprintf("%d update available", nUpdate))
		}
		if nModified > 0 {
			parts = append(parts, fmt.Sprintf("%d modified", nModified))
		}
		if nConflict > 0 {
			parts = append(parts, fmt.Sprintf("%d conflict(s)", nConflict))
		}
		b.WriteString(dimStyle.Render(" " + strings.Join(parts, "  •  ")))
	}
}

func (m model) viewRepos(b *strings.Builder) {
	// Add-repo name/URL prompts and remove confirmation render as centered
	// dialogs (tui_dialogs.go).
	b.WriteString("\n")
	b.WriteString("\n")

	// Compute dynamic NAME column width from longest repo name
	repoNameColW := 10
	for _, entry := range m.repoList {
		if len(entry.name) > repoNameColW {
			repoNameColW = len(entry.name)
		}
	}
	repoNameColW += 2 // padding
	if repoNameColW > m.width/2 {
		repoNameColW = m.width / 2
	}

	// URL column gets the remaining width
	urlColW := m.width - repoNameColW - 4 // leading space + gap

	// Ensure both columns fit within terminal; shrink proportionally if needed
	overhead := 4 // leading space + gap between columns
	if repoNameColW+urlColW+overhead > m.width {
		urlColW = m.width - repoNameColW - overhead
	}
	if urlColW < 5 {
		// Shrink name column to give URL more room
		urlColW = 5
		repoNameColW = m.width - urlColW - overhead
		if repoNameColW < 5 {
			repoNameColW = 5
		}
	}

	// Header
	header := fmt.Sprintf(" %-*s  %s", repoNameColW, "NAME", "URL")
	b.WriteString(dimStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")

	// Repo list
	maxRows := m.height - 13
	if maxRows < 3 {
		maxRows = 3
	}

	if len(m.repoList) == 0 {
		b.WriteString(dimStyle.Render("   No repos registered. Press 'a' to add one."))
		b.WriteString("\n")
	}

	// Scroll offset: keep cursor within viewport
	offset := 0
	if m.repoCursor >= maxRows {
		offset = m.repoCursor - maxRows + 1
	}

	displayed := 0
	for i := offset; i < len(m.repoList) && displayed < maxRows; i++ {
		entry := m.repoList[i]
		name := entry.name
		if len(name) > repoNameColW {
			name = name[:repoNameColW-1] + "…"
		}
		url := entry.url
		if len(url) > urlColW {
			url = url[:urlColW-1] + "…"
		}
		line := fmt.Sprintf(" %-*s  %s", repoNameColW, name, url)
		if i == m.repoCursor {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("%-*s", m.width-1, line)))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
		displayed++
	}

	// Pad
	for i := displayed; i < maxRows; i++ {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d repo(s) registered", len(m.repoList))))
	}
}

func (m model) viewUnmanaged(b *strings.Builder) {
	// Adopt repo-selection renders as a centered dialog (tui_dialogs.go).
	b.WriteString("\n")

	// Filter indicator
	if m.unmanagedFilter != "" {
		b.WriteString(filterStyle.Render(fmt.Sprintf(" Filter: %s▌", m.unmanagedFilter)))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	if len(m.unmanagedEntries) == 0 {
		b.WriteString(emptyStyle.Render("   No unmanaged skills found."))
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("   Skills installed by skillpack are tracked automatically."))
		b.WriteString("\n")
	} else {
		// Column widths
		nameColW := 10
		agentColW := 5
		for _, e := range m.unmanagedEntries {
			if len(e.skillName) > nameColW {
				nameColW = len(e.skillName)
			}
			if len(e.agentName) > agentColW {
				agentColW = len(e.agentName)
			}
		}
		nameColW += 2
		agentColW += 2
		if nameColW > m.width/2 {
			nameColW = m.width / 2
		}

		header := fmt.Sprintf(" %-*s  %-*s  %s", nameColW, "SKILL", agentColW, "AGENT", "PATH")
		b.WriteString(dimStyle.Render(header))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
		b.WriteString("\n")

		maxRows := m.height - 11
		if maxRows < 3 {
			maxRows = 3
		}
		pathColW := m.width - nameColW - agentColW - 8
		if pathColW < 5 {
			pathColW = 5
		}

		// Scroll offset: keep cursor within viewport
		offset := 0
		if m.unmanagedCursor >= maxRows {
			offset = m.unmanagedCursor - maxRows + 1
		}

		displayed := 0
		for i := offset; i < len(m.unmanagedEntries) && displayed < maxRows; i++ {
			entry := m.unmanagedEntries[i]
			name := entry.skillName
			if len(name) > nameColW {
				name = name[:nameColW-1] + "…"
			}
			agent := entry.agentName
			if len(agent) > agentColW {
				agent = agent[:agentColW-1] + "…"
			}
			path := entry.localPath
			if len(path) > pathColW {
				path = "…" + path[len(path)-pathColW+1:]
			}
			line := fmt.Sprintf(" %-*s  %-*s  %s", nameColW, name, agentColW, agent, dimStyle.Render(path))
			if i == m.unmanagedCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf(" %-*s  %-*s  %-*s", nameColW, name, agentColW, agent, pathColW, path)))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
			displayed++
		}

		for i := displayed; i < maxRows; i++ {
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d unmanaged skill(s) found", len(m.unmanagedEntries))))
	}
}

func (m model) viewPacks(b *strings.Builder) {
	// Pack agent-install select and remove confirmation render as centered
	// dialogs (tui_dialogs.go).
	b.WriteString("\n")

	// Detail overlay for a pack that is not installed: show its definition.
	if m.packDetailOpen && m.packCursor < len(m.packRows) && !m.packRows[m.packCursor].installed {
		m.viewAvailablePackDetail(b, m.packRows[m.packCursor])
		return
	}

	// Detail overlay: show per-skill, per-agent status for the selected pack.
	if m.packDetailOpen && m.packCursor < len(m.packRows) {
		row := m.packRows[m.packCursor]
		rec, ok := m.st.InstalledPacks[row.packAddr]
		if !ok {
			b.WriteString(dimStyle.Render(" Pack record not found."))
			b.WriteString("\n")
			return
		}
		statusLabel := checkStyle.Render("complete")
		if row.isPartial {
			statusLabel = partialStyle.Render("partial")
		}
		b.WriteString(fmt.Sprintf(" Pack: %s  [%s]\n", bold(row.packAddr), statusLabel))
		b.WriteString(fmt.Sprintf(" Installed: %s\n", rec.InstalledAt.Format("2006-01-02 15:04:05")))
		b.WriteString(fmt.Sprintf(" Agents:    %s\n\n", strings.Join(rec.Agents, ", ")))

		// Collect and sort skills
		var skillAddrs []string
		for s := range rec.Skills {
			skillAddrs = append(skillAddrs, s)
		}
		sort.Strings(skillAddrs)

		skillW, agentW := 5, 5
		for _, sa := range skillAddrs {
			if len(sa) > skillW {
				skillW = len(sa)
			}
			for ag := range rec.Skills[sa] {
				if len(ag) > agentW {
					agentW = len(ag)
				}
			}
		}
		if skillW > m.width/2 {
			skillW = m.width / 2
		}

		header := fmt.Sprintf("  %-*s  %-*s  %s", skillW, "SKILL", agentW, "AGENT", "STATUS")
		b.WriteString(dimStyle.Render(header))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  " + safeRepeat("─", m.width-4)))
		b.WriteString("\n")

		maxRows := m.height - 13
		if maxRows < 3 {
			maxRows = 3
		}
		displayed := 0
		for _, sa := range skillAddrs {
			if displayed >= maxRows {
				break
			}
			agentStatuses := rec.Skills[sa]
			var agents []string
			for ag := range agentStatuses {
				agents = append(agents, ag)
			}
			sort.Strings(agents)
			for _, ag := range agents {
				if displayed >= maxRows {
					break
				}
				s := agentStatuses[ag]
				var statusStr string
				if s.Installed {
					statusStr = checkStyle.Render("✓ installed")
				} else if s.Error != "" {
					statusStr = conflictStyle.Render("✗ error: " + s.Error)
				} else {
					statusStr = partialStyle.Render("⚠ missing")
				}
				skillShort := sa
				if len(skillShort) > skillW {
					skillShort = skillShort[:skillW-1] + "…"
				}
				fmt.Fprintf(b, "  %-*s  %-*s  ", skillW, skillShort, agentW, ag)
				b.WriteString(statusStr)
				b.WriteString("\n")
				displayed++
			}
		}
		for i := displayed; i < maxRows; i++ {
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

	// List view
	if len(m.packRows) == 0 {
		b.WriteString(emptyStyle.Render("   No packs found."))
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("   Press n to create a pack, or register a repo containing packs (Tab → Repos → a)."))
		b.WriteString("\n")
	} else {
		// Column widths
		addrW := 12
		for _, row := range m.packRows {
			if len(row.packAddr) > addrW {
				addrW = len(row.packAddr)
			}
		}
		addrW += 2
		if addrW > m.width*2/3 {
			addrW = m.width * 2 / 3
		}
		agentsColW := m.width - addrW - 16 // STATUS col is ~10 + padding
		if agentsColW < 8 {
			agentsColW = 8
		}

		header := fmt.Sprintf(" %-*s  %-*s  %s", addrW, "PACK", agentsColW, "AGENTS", "STATUS")
		b.WriteString(dimStyle.Render(header))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
		b.WriteString("\n")

		maxRows := m.height - 11
		if maxRows < 3 {
			maxRows = 3
		}

		offset := 0
		if m.packCursor >= maxRows {
			offset = m.packCursor - maxRows + 1
		}

		displayed := 0
		for i := offset; i < len(m.packRows) && displayed < maxRows; i++ {
			row := m.packRows[i]
			isSelected := i == m.packCursor

			addr := row.packAddr
			if len(addr) > addrW {
				addr = addr[:addrW-1] + "…"
			}

			agentsStr := strings.Join(row.agents, ", ")
			if len(agentsStr) > agentsColW {
				agentsStr = agentsStr[:agentsColW-1] + "…"
			}

			var statusStr string
			switch {
			case !row.installed:
				statusStr = dimStyle.Render("· available")
			case row.isPartial:
				statusStr = partialStyle.Render("⚠ partial")
			default:
				statusStr = checkStyle.Render("✓ complete")
			}

			line := fmt.Sprintf(" %-*s  %-*s  ", addrW, addr, agentsColW, agentsStr)
			if isSelected {
				b.WriteString(selectedStyle.Render(fmt.Sprintf(" %-*s  %-*s ", addrW, addr, agentsColW, agentsStr)))
			} else {
				switch {
				case !row.installed:
					b.WriteString(dimStyle.Render(line))
				case row.isPartial:
					b.WriteString(partialStyle.Render(fmt.Sprintf(" %-*s", addrW, addr)))
					b.WriteString(fmt.Sprintf("  %-*s  ", agentsColW, agentsStr))
				default:
					b.WriteString(line)
				}
			}
			b.WriteString(statusStr)
			b.WriteString("\n")
			displayed++
		}

		for i := displayed; i < maxRows; i++ {
			b.WriteString("\n")
		}
	}

	// Footer — contextual help for the selected row now lives in the status bar.
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
		return
	}
	// Show the selected pack's description when it has one.
	if m.packCursor < len(m.packRows) && m.packRows[m.packCursor].desc != "" {
		desc := " " + m.packRows[m.packCursor].desc
		if m.width > 2 && len(desc) > m.width-2 {
			desc = desc[:m.width-3] + "…"
		}
		b.WriteString(dimStyle.Render(desc))
		return
	}
	installedCount, partialCount := 0, 0
	for _, row := range m.packRows {
		if row.installed {
			installedCount++
		}
		if row.isPartial {
			partialCount++
		}
	}
	availableCount := len(m.packRows) - installedCount
	parts := []string{fmt.Sprintf("%d installed", installedCount)}
	if partialCount > 0 {
		parts = append(parts, partialStyle.Render(fmt.Sprintf("%d partial", partialCount)))
	}
	if availableCount > 0 {
		parts = append(parts, fmt.Sprintf("%d available", availableCount))
	}
	b.WriteString(dimStyle.Render(" " + strings.Join(parts, "  •  ")))
}

// viewAvailablePackDetail renders the detail overlay for a pack that is
// discoverable in a repo cache but not installed. The pack definition is
// parsed once when the overlay opens (loadPackDetail) — never per frame.
func (m model) viewAvailablePackDetail(b *strings.Builder, row packRow) {
	if m.packDetailErr != nil {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" Cannot read pack: %v", m.packDetailErr)))
		b.WriteString("\n")
		return
	}
	pk := m.packDetailDef
	if pk == nil {
		b.WriteString(dimStyle.Render(" Pack definition not loaded."))
		b.WriteString("\n")
		return
	}

	b.WriteString(fmt.Sprintf(" Pack: %s  [%s]\n", bold(row.packAddr), dimStyle.Render("available")))
	if pk.Description != "" {
		b.WriteString(fmt.Sprintf(" Desc: %s\n", pk.Description))
	}
	b.WriteString(fmt.Sprintf(" Skills: %d\n\n", len(pk.Skills)))

	maxRows := m.height - 11
	if maxRows < 3 {
		maxRows = 3
	}
	displayed := 0
	for _, sa := range pk.Skills {
		if displayed >= maxRows {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  … and %d more", len(pk.Skills)-displayed)))
			b.WriteString("\n")
			break
		}
		installedMark := "  "
		if _, ok := m.st.InstalledSkills[sa]; ok {
			installedMark = checkStyle.Render("✓ ")
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", installedMark, sa))
		displayed++
	}
	for i := displayed; i < maxRows; i++ {
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + safeRepeat("─", m.width-2)))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	}
}

// visibleRows returns indices into m.rows that should be displayed given
// the current filter and expand/collapse state.
func (m *model) visibleRows() []int {
	var indices []int
	for i, row := range m.rows {
		if row.kind == repoRow {
			if m.filter == "" || m.repoHasMatch(row.repoName) {
				indices = append(indices, i)
			}
		} else {
			if m.filter != "" {
				// When filtering, show matching skills regardless of collapse state
				if strings.Contains(strings.ToLower(row.addr), strings.ToLower(m.filter)) {
					indices = append(indices, i)
				}
			} else if m.isParentExpanded(i) {
				indices = append(indices, i)
			}
		}
	}
	return indices
}

func (m *model) repoHasMatch(repoName string) bool {
	if strings.Contains(strings.ToLower(repoName), strings.ToLower(m.filter)) {
		return true
	}
	for _, row := range m.rows {
		if row.kind == skillRow && row.repoName == repoName {
			if strings.Contains(strings.ToLower(row.addr), strings.ToLower(m.filter)) {
				return true
			}
		}
	}
	return false
}

func (m *model) isParentExpanded(idx int) bool {
	for i := idx - 1; i >= 0; i-- {
		if m.rows[i].kind == repoRow {
			return m.rows[i].expanded
		}
	}
	return true
}

func (m *model) moveCursor(dir int) {
	vis := m.visibleRows()
	if len(vis) == 0 {
		return
	}

	// Find current position in visible list
	curVis := -1
	for i, idx := range vis {
		if idx == m.cursorRow {
			curVis = i
			break
		}
	}

	if curVis == -1 {
		m.cursorRow = vis[0]
		return
	}

	next := curVis + dir
	if next < 0 {
		next = 0
	}
	if next >= len(vis) {
		next = len(vis) - 1
	}
	m.cursorRow = vis[next]
}

// safeRepeat returns a string of n repetitions of s, or empty if n <= 0.
func safeRepeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}
