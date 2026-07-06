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

Panels (Tab to cycle, or jump directly with F2-F6):
  F2  Skills      Browse and install/remove skills
  F3  Status      View installed skill states, update, sync
  F4  Repos       Add/remove skill repositories
  F5  Unmanaged   Adopt skills found in agent dirs but not tracked by skillpack
  F6  Packs       Browse, install, create, and edit packs

F10 (or Alt+F/V/A/P/H) opens the menu bar (File/View/Actions/Packs/Help);
F1 opens a scrollable key-binding reference sourced from the same menus.

Skills panel:
  ↑/↓         Move between items
  ←/→         Move between agent columns
  Space/Enter  Toggle install/remove or expand/collapse
  f           Fork a skill into your repo
  R           Repair a stale or broken-upstream skill
  v           View SKILL.md of the selected skill
  Type        Filter skills (incremental search)
  Backspace   Delete filter character
  Esc         Clear filter

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
  Type       Filter skills (incremental search)
  Backspace  Delete filter character
  Esc        Clear filter
  Enter      Adopt selected skill into a registered repo
  v          View SKILL.md of the selected skill

Packs panel:
  ↑/↓         Move between packs
  Enter       Toggle the detail overlay for the selected pack
  n           Create a new pack (embedded wizard)
  e           Edit the selected pack
  i           Install the selected available pack
  c           Complete a partially-deployed pack
  d           Remove the selected installed pack

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








































