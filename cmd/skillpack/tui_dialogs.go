package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderDialog draws a double-bordered box titled title around bodyLines
// (already-styled content), capped to maxWidth. One shared look for every
// inputMode overlay and the pack wizard alike — text prompts, confirms,
// list-selects, and multi-step forms.
func renderDialog(maxWidth int, title string, bodyLines []string) []string {
	width := lipgloss.Width(title) + 4
	for _, l := range bodyLines {
		if w := lipgloss.Width(l) + 4; w > width {
			width = w
		}
	}
	maxW := maxWidth - 4
	if maxW < 20 {
		maxW = 20
	}
	if width > maxW {
		width = maxW
	}

	lines := make([]string, 0, len(bodyLines)+2)
	lines = append(lines, "╔"+centerInBorder(title, width)+"╗")
	for _, l := range bodyLines {
		lines = append(lines, "║"+padLine(" "+l, width)+"║")
	}
	lines = append(lines, "╚"+strings.Repeat("═", width)+"╝")
	return lines
}

// centerInBorder centers " title " within a double-line border of the given
// width, e.g. "══ Add Repo ══".
func centerInBorder(title string, width int) string {
	label := " " + title + " "
	if len(label) >= width {
		return strings.Repeat("═", width)
	}
	left := (width - len(label)) / 2
	right := width - len(label) - left
	return strings.Repeat("═", left) + label + strings.Repeat("═", right)
}

// listLine renders one row of a list-select dialog (fork/adopt repo choice,
// relink candidates, pack agent multi-select): "▶ " and reverse-video when
// selected, plain otherwise.
func listLine(m model, text string, selected bool) string {
	width := m.width - 8
	if width < 10 {
		width = 10
	}
	if selected {
		return selectedStyle.Render(fmt.Sprintf("▶ %-*s", width, text))
	}
	return "  " + text
}

// dialogContent returns the title and body lines for the active inputMode,
// or ok=false if the mode has no dialog (modeNormal, or a mode whose UI
// isn't a dialog).
func dialogContent(m model) (title string, lines []string, ok bool) {
	switch m.inputMode {
	case modeAddRepoName:
		return "Add Repo", []string{
			fmt.Sprintf("Repo name: %s▌", m.inputBuffer),
			"",
			dimStyle.Render("Enter to confirm • Esc to cancel"),
		}, true

	case modeAddRepoURL:
		return "Add Repo", []string{
			fmt.Sprintf("Repo URL for %q:", m.newRepoName),
			fmt.Sprintf("%s▌", m.inputBuffer),
			"",
			dimStyle.Render("Enter to confirm • Esc to cancel"),
		}, true

	case modeConfirmRemove:
		name := ""
		if m.repoCursor < len(m.repoList) {
			name = m.repoList[m.repoCursor].name
		}
		return "Remove Repo", []string{
			fmt.Sprintf("Remove repo %q?", name),
			"",
			"[ Yes ]  [ No ]",
			"",
			dimStyle.Render("y/Enter to confirm • n/Esc to cancel"),
		}, true

	case modeForkSelectRepo:
		lines := []string{fmt.Sprintf("Fork %q into which repo?", m.forkAddr), ""}
		for i, entry := range m.repoList {
			lines = append(lines, listLine(m, fmt.Sprintf("%s  %s", entry.name, entry.url), i == m.forkCursor))
		}
		lines = append(lines, "", dimStyle.Render("↑↓ select • Enter confirm • Esc cancel"))
		return "Fork Skill", lines, true

	case modeForkResolveChoice:
		skillName := filepath.Base(m.forkAddr)
		return "Fork: Unknown Provenance", []string{
			fmt.Sprintf("%q already exists in %q with unknown provenance.", skillName, m.forkTargetRepo),
			"This skill was not installed by skillpack.",
			"",
			"1  Override — replace existing with a fresh fork",
			fmt.Sprintf("2  Register — keep existing, record it as a fork of %s", m.forkAddr),
			"",
			dimStyle.Render("1/2 to choose • Esc to cancel"),
		}, true

	case modeAdoptSelectRepo:
		if m.unmanagedCursor >= len(m.unmanagedEntries) {
			return "", nil, false
		}
		entry := m.unmanagedEntries[m.unmanagedCursor]
		lines := []string{fmt.Sprintf("Adopt %q (%s) into which repo?", entry.skillName, entry.agentName), ""}
		for i, re := range m.repoList {
			lines = append(lines, listLine(m, fmt.Sprintf("%s  %s", re.name, re.url), i == m.adoptCursor))
		}
		lines = append(lines, "", dimStyle.Render("↑↓ select • Enter confirm • Esc cancel"))
		return "Adopt Skill", lines, true

	case modeRegisterForkInput:
		return "Register Fork Provenance", []string{
			fmt.Sprintf("Register fork provenance for %q", m.registerForkAddr),
			"Upstream skill address:",
			fmt.Sprintf("%s▌", m.registerForkInput),
			"",
			dimStyle.Render("Enter to confirm • Esc to cancel"),
		}, true

	case modeRelinkStaleInput:
		lines := []string{
			fmt.Sprintf("Relink stale skill: %q", m.relinkAddr),
			dimStyle.Render(fmt.Sprintf("Agent: %s", m.relinkAgentName)),
			"",
		}
		if m.relinkCandidateMode && len(m.relinkCandidates) > 0 {
			lines = append(lines, "Suggested replacements (Tab to type manually):", "")
			for i, c := range m.relinkCandidates {
				lines = append(lines, listLine(m, c, i == m.relinkCandidateCursor))
			}
		} else {
			if len(m.relinkCandidates) > 0 {
				lines = append(lines, "Enter replacement address (Tab to use candidates list):")
			} else {
				lines = append(lines, "Enter replacement address:")
			}
			lines = append(lines, fmt.Sprintf("%s▌", m.relinkInput))
		}
		lines = append(lines, "", dimStyle.Render("Enter to confirm • Esc to cancel"))
		return "Repair Skill", lines, true

	case modeRelinkBrokenChoice:
		return "Repair Broken Upstream", []string{
			fmt.Sprintf("Repair broken upstream pointer: %q", m.relinkAddr),
			dimStyle.Render(fmt.Sprintf("Agent: %s", m.relinkAgentName)),
			"",
			"1  Set new upstream address (--set-upstream)",
			"2  Clear upstream pointer (--clear-upstream)",
			"",
			dimStyle.Render("1/2 to choose • Esc to cancel"),
		}, true

	case modeRelinkBrokenSetInput:
		return "Repair Broken Upstream", []string{
			fmt.Sprintf("Set upstream for: %q", m.relinkAddr),
			dimStyle.Render(fmt.Sprintf("Agent: %s", m.relinkAgentName)),
			"",
			"New upstream skill address:",
			fmt.Sprintf("%s▌", m.relinkInput),
			"",
			dimStyle.Render("Enter to confirm • Esc to cancel"),
		}, true

	case modePackInstallAgents:
		lines := []string{fmt.Sprintf("Install pack %q for which agents?", m.packInstallAddr), ""}
		for i, name := range m.agents {
			check := "[ ]"
			if m.packAgentSel[i] {
				check = "[✓]"
			}
			lines = append(lines, listLine(m, check+" "+name, i == m.packAgentCursor))
		}
		lines = append(lines, "", dimStyle.Render("↑↓ move • Space toggle • a all • Enter install • Esc cancel"))
		return "Install Pack", lines, true

	case modePackConfirmRemove:
		addr := ""
		if m.packCursor < len(m.packRows) {
			addr = m.packRows[m.packCursor].packAddr
		}
		return "Remove Pack", []string{
			fmt.Sprintf("Remove pack %q?", addr),
			"",
			"[ Yes ]  [ No ]",
			"",
			dimStyle.Render("y/Enter to confirm • n/Esc to cancel"),
		}, true

	default:
		return "", nil, false
	}
}

// overlayDialog, when the active inputMode has dialog content, centers its
// box over body via whole-line occlusion — the same technique overlayDropdown
// uses, so it can't garble ANSI-styled panel content underneath.
func overlayDialog(m model, body string) string {
	title, bodyLines, ok := dialogContent(m)
	if !ok {
		return body
	}
	box := renderDialog(m.width, title, bodyLines)

	lines := strings.Split(body, "\n")
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}

	boxW := 0
	for _, l := range box {
		if w := lipgloss.Width(l); w > boxW {
			boxW = w
		}
	}
	top := (len(lines) - len(box)) / 2
	if top < 2 { // never cover the title/menu bar
		top = 2
	}
	if top+len(box) > len(lines) {
		top = len(lines) - len(box)
	}
	left := (m.width - boxW) / 2
	if left < 0 {
		left = 0
	}
	prefix := strings.Repeat(" ", left)

	for i, boxLine := range box {
		row := top + i
		if row < 0 || row >= len(lines) {
			continue
		}
		lines[row] = prefix + boxLine
	}
	return strings.Join(lines, "\n")
}
