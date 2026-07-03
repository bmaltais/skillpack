package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for browsing and installing/removing skills",
	Long: `Launch an interactive terminal UI to browse skills across repos
and toggle installation for each configured agent.

Panels (Tab to switch):
  Skills      Browse and install/remove skills
  Status      View installed skill states, update, sync
  Repos       Add/remove skill repositories
  Unmanaged   Adopt skills found in agent dirs but not tracked by skillpack

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








































