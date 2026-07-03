package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// --- View Styles (moved from tui.go in Phase 1) ---

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	tabActive     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Underline(true)
	tabInactive   = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	cellSelStyle  = lipgloss.NewStyle().Background(lipgloss.Color("62")).Bold(true)
	repoStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	checkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	emptyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	filterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	msgStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	inputStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	updateStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")) // cyan - update available
	modifiedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow - locally modified
	forkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // amber  - forked skill
	conflictStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red - conflict
	bannerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("220")).Bold(true)
	bannerBtnActive   = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62")).Bold(true)
	bannerBtnInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Background(lipgloss.Color("238"))
	staleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true) // orange  - stale address
	brokenUpstreamStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // red - broken upstream
)

// View renders the entire TUI (delegates to the four panel-specific view* methods).
func (m model) View() string {
	var b strings.Builder

	// Title + tabs
	b.WriteString(titleStyle.Render(" SkillPack"))
	b.WriteString("  ")
	switch m.activePanel {
	case panelSkills:
		b.WriteString(tabActive.Render("[Skills]"))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Status "))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Repos "))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Unmanaged "))
	case panelStatus:
		b.WriteString(tabInactive.Render(" Skills "))
		b.WriteString("  ")
		b.WriteString(tabActive.Render("[Status]"))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Repos "))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Unmanaged "))
	case panelRepos:
		b.WriteString(tabInactive.Render(" Skills "))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Status "))
		b.WriteString("  ")
		b.WriteString(tabActive.Render("[Repos]"))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Unmanaged "))
	case panelUnmanaged:
		b.WriteString(tabInactive.Render(" Skills "))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Status "))
		b.WriteString("  ")
		b.WriteString(tabInactive.Render(" Repos "))
		b.WriteString("  ")
		b.WriteString(tabActive.Render("[Unmanaged]"))
	}
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

	switch m.activePanel {
	case panelSkills:
		m.viewSkills(&b)
	case panelStatus:
		m.viewStatus(&b)
	case panelRepos:
		m.viewRepos(&b)
	case panelUnmanaged:
		m.viewUnmanaged(&b)
	}

	return b.String()
}

func (m model) viewSkills(b *strings.Builder) {
	// Fork unknown-provenance resolution overlay
	if m.inputMode == modeForkResolveChoice {
		skillName := filepath.Base(m.forkAddr)
		b.WriteString(inputStyle.Render(fmt.Sprintf(" %q already exists in %q with unknown provenance", skillName, m.forkTargetRepo)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" This skill was not installed by skillpack. How should we proceed?"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf(" %s  Override — replace existing with a fresh fork\n", inputStyle.Render(" 1 ")))
		b.WriteString(fmt.Sprintf(" %s  Register — keep existing, record it as a fork of %s\n", inputStyle.Render(" 2 "), m.forkAddr))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" Press 1 or 2 to choose • Esc to cancel"))
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

	// Fork repo selection overlay
	if m.inputMode == modeForkSelectRepo {
		b.WriteString(inputStyle.Render(fmt.Sprintf(" Fork %q into which repo?", m.forkAddr)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" (↑↓ to select, Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
		for i, entry := range m.repoList {
			line := fmt.Sprintf("   %s  %s", entry.name, dimStyle.Render(entry.url))
			if i == m.forkCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf(" ▶ %-*s", m.width-4, fmt.Sprintf("%s  %s", entry.name, entry.url))))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

	// Stale skill repair overlay
	if m.inputMode == modeRelinkStaleInput {
		b.WriteString(staleStyle.Render(fmt.Sprintf(" Relink stale skill: %q", m.relinkAddr)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf(" Agent: %s", m.relinkAgentName)))
		b.WriteString("\n\n")
		if m.relinkCandidateMode && len(m.relinkCandidates) > 0 {
			b.WriteString(dimStyle.Render(" Suggested replacements (↑↓ to select, Tab to type manually, Enter to confirm):"))
			b.WriteString("\n\n")
			for i, c := range m.relinkCandidates {
				if i == m.relinkCandidateCursor {
					b.WriteString(selectedStyle.Render(fmt.Sprintf(" ▶ %-*s", m.width-4, c)))
				} else {
					b.WriteString(fmt.Sprintf("   %s", c))
				}
				b.WriteString("\n")
			}
		} else {
			if len(m.relinkCandidates) > 0 {
				b.WriteString(dimStyle.Render(" Enter replacement address (Tab to use candidates list):"))
			} else {
				b.WriteString(dimStyle.Render(" Enter replacement address:"))
			}
			b.WriteString("\n")
			b.WriteString(inputStyle.Render(fmt.Sprintf("   %s▌", m.relinkInput)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" Enter to confirm • Esc to cancel"))
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

	// Broken-upstream repair overlay: choose repair action
	if m.inputMode == modeRelinkBrokenChoice {
		b.WriteString(brokenUpstreamStyle.Render(fmt.Sprintf(" Repair broken upstream pointer: %q", m.relinkAddr)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf(" Agent: %s", m.relinkAgentName)))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf(" %s  Set new upstream address (--set-upstream)\n", inputStyle.Render(" 1 ")))
		b.WriteString(fmt.Sprintf(" %s  Clear upstream pointer (--clear-upstream)\n", inputStyle.Render(" 2 ")))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" Press 1 or 2 to choose • Esc to cancel"))
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

	// Broken-upstream repair overlay: set upstream address input
	if m.inputMode == modeRelinkBrokenSetInput {
		b.WriteString(brokenUpstreamStyle.Render(fmt.Sprintf(" Set upstream for: %q", m.relinkAddr)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf(" Agent: %s", m.relinkAgentName)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render(" New upstream skill address:"))
		b.WriteString("\n")
		b.WriteString(inputStyle.Render(fmt.Sprintf("   %s▌", m.relinkInput)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render(" Enter to confirm • Esc to cancel"))
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

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
	maxRows := m.height - 9 // header(4) + footer(5)
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
	b.WriteString(helpStyle.Render(" ↑↓ navigate  ←→ agents  Space/Enter toggle  f fork  R repair  v view  Tab switch  q quit"))
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
	// Register fork provenance input overlay
	if m.inputMode == modeRegisterForkInput {
		b.WriteString("\n")
		b.WriteString(inputStyle.Render(fmt.Sprintf(" Register fork provenance for %q", m.registerForkAddr)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" Upstream skill address:"))
		b.WriteString("\n")
		b.WriteString(inputStyle.Render(fmt.Sprintf("   %s▌", m.registerForkInput)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render(" Enter to confirm • Esc to cancel"))
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

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
		maxRows := m.height - 10
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

	// Show context-sensitive help for r key depending on selected row.
	rHelp := "r refresh"
	if m.statusCursor < len(m.statusRows) {
		row := m.statusRows[m.statusCursor]
		if _, ok := m.forkCandidates[row.addr]; ok {
			rHelp = "r register fork"
		}
	}
	b.WriteString(helpStyle.Render(fmt.Sprintf(" ↑↓ navigate  u update selected  S sync all  %s  U self-update  Tab switch  q quit", rHelp)))
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
	b.WriteString("\n")

	// Input prompts
	if m.inputMode == modeAddRepoName {
		b.WriteString(inputStyle.Render(fmt.Sprintf(" Repo name: %s▌", m.inputBuffer)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" (Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
	} else if m.inputMode == modeAddRepoURL {
		b.WriteString(inputStyle.Render(fmt.Sprintf(" Repo URL for %q: %s▌", m.newRepoName, m.inputBuffer)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" (Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}

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
	maxRows := m.height - 12
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
	b.WriteString(helpStyle.Render(" ↑↓ navigate  a add  d remove  Tab skills  q quit"))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d repo(s) registered", len(m.repoList))))
	}
}

func (m model) viewUnmanaged(b *strings.Builder) {
	b.WriteString("\n")

	// Adopt repo-selection overlay
	if m.inputMode == modeAdoptSelectRepo && m.unmanagedCursor < len(m.unmanagedEntries) {
		entry := m.unmanagedEntries[m.unmanagedCursor]
		b.WriteString(inputStyle.Render(fmt.Sprintf(" Adopt %q (%s) into which repo?", entry.skillName, entry.agentName)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(" (↑↓ to select, Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
		for i, re := range m.repoList {
			if i == m.adoptCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf(" ▶ %-*s", m.width-4, fmt.Sprintf("%s  %s", re.name, re.url))))
			} else {
				b.WriteString(fmt.Sprintf("   %s  %s", re.name, dimStyle.Render(re.url)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString(msgStyle.Render(" " + m.message))
		}
		return
	}

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

		maxRows := m.height - 10
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
	b.WriteString(helpStyle.Render(" ↑↓ navigate  Type to filter  Enter adopt into repo  v view  Tab switch  q quit"))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d unmanaged skill(s) found", len(m.unmanagedEntries))))
	}
}

// --- Small pure helpers moved with the view layer (Phase 1) ---

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
