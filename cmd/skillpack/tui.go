package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for browsing and installing/removing skills",
	Long: `Launch an interactive terminal UI (DOS Shell-styled chrome: title bar,
menu bar, bottom status bar) to browse skills across repos and toggle
installation for each configured agent.

Panels (a strip under the menu bar lists them; Tab/Shift+Tab cycle, F2-F7 jump):
  F2  Skills      Browse and install/remove skills
  F3  Status      View installed skill states, update, sync
  F4  Repos       Add/remove skill repositories
  F5  Unmanaged   Adopt skills found in agent dirs but not tracked by skillpack
  F6  Packs       Browse packs, install them per agent (Space), create, and edit
  F7  Doctor      Find problems (stale skills, broken upstreams, partial packs, duplicates); Enter applies a fix, asking first when it writes to a repo

F10 (or Alt+F/V/A/P/H) opens the menu bar (File/View/Actions/Packs/Help);
F1 or ? opens a scrollable key-binding reference sourced from the same menus.
Ctrl+P (or :) opens a command palette: type to fuzzy-search every menu action and
jump to any skill, repo or pack. A adds an agent from any panel.
Esc only backs out (clears a filter, closes a dialog); q or Ctrl+C quits.

Skills panel:
  ↑/↓         Move between items
  ←/→         Move between agent columns
  Space/Enter  Toggle install/remove or expand/collapse
  f           Fork a skill into your repo
  R           Repair a stale or broken-upstream skill
  v           View SKILL.md of the selected skill
  /           Enter filter mode (incremental search)
  Backspace   Delete filter character (while filtering)
  Esc         Exit filter mode and clear the filter

Status panel:
  ↑/↓         Move between skills
  u/Enter     Update the selected skill
  S           Sync all skills
  r           Refresh status
  U           Self-update skillpack binary

Repos panel:
  ↑/↓         Move between repos
  a           Add a repo
  d/Delete    Remove selected repo

Unmanaged panel:
  ↑/↓         Move between items
  /           Enter filter mode (incremental search)
  Backspace   Delete filter character (while filtering)
  Esc         Exit filter mode and clear the filter
  Enter       Adopt selected skill into a registered repo
  v           View SKILL.md of the selected skill

Packs panel:
  ↑/↓ ←/→     Move between packs and agents (pack × agent matrix)
  Space       Install the selected pack on the selected agent (or remove it from that agent)
  Enter       Toggle the detail overlay for the selected pack
  n           Create a new pack (embedded wizard)
  e           Edit the selected pack
  c           Complete a partially-deployed pack
  m           Mark the selected installed pack for removal
  d           Remove the marked packs (or the selected one), after a confirm

All changes are applied immediately.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

// Init satisfies the tea.Model interface. It is kept here (in the thin
// coordinator file) so that model (defined in tui_model.go) implements tea.Model
// for tea.NewProgram. It simply starts the non-blocking update check.
func (m model) Init() tea.Cmd {
	return cmdCheckForUpdate()
}
