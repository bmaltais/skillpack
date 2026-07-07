package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DOS Shell palette (ANSI-16, for authenticity on real terminals).
// Route B (chosen at end of Move 1): blue reverse-video applies only to
// chrome (title bar, menu bar, status bar) — panel bodies keep the
// terminal's default background. Full-background painting was rejected as
// too fragile against the existing per-panel width/height arithmetic.
var (
	chromeBg     = lipgloss.Color("4")  // blue
	chromeFg     = lipgloss.Color("15") // white
	chromeAccent = lipgloss.Color("14") // cyan — mnemonic letters

	chromeBarStyle    = lipgloss.NewStyle().Foreground(chromeFg).Background(chromeBg).Bold(true)
	chromeAccentStyle = lipgloss.NewStyle().Foreground(chromeAccent).Background(chromeBg).Bold(true)
)

// padLine pads s with spaces to width visible columns, so that rendering it
// through a background-colored style fills the full row. Must be called
// before styling — appending plain spaces after Render leaves them
// unstyled.
func padLine(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
