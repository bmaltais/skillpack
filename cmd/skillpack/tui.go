package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for browsing and installing/removing skills",
	Long: `Launch an interactive terminal UI to browse skills across repos
and toggle installation for each configured agent.

Navigation:
  ↑/↓       Move between skills and repos
  ←/→       Move between agent columns
  Enter     On repo: expand/collapse. On skill+agent: install/remove
  Type      Filter skills (incremental search)
  Backspace Delete filter character
  Esc       Clear filter
  q         Quit

By default the TUI is in preview mode (no changes written to disk).
Pass --live to actually install/remove skills on Enter.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		live, _ := cmd.Flags().GetBool("live")
		return runTUI(live)
	},
}

func init() {
	tuiCmd.Flags().Bool("live", false, "Actually install/remove skills (default: preview only)")
	rootCmd.AddCommand(tuiCmd)
}

// --- Data types ---

type rowKind int

const (
	repoRow rowKind = iota
	skillRow
)

type tuiRow struct {
	kind     rowKind
	repoName string
	addr     string // full skill address (only for skillRow)
	relPath  string // repo-relative path (only for skillRow)
	expanded bool   // only for repoRow
}

// --- Model ---

type model struct {
	// Data
	rows      []tuiRow
	agents    []string
	installed map[string]map[string]bool // addr → agent → installed

	// UI state
	cursorRow int
	cursorCol int // agent column index
	filter    string
	width     int
	height    int
	live      bool
	message   string

	// Config/state refs for live mode
	cfg *config.Config
	st  *state.State
}

func initialModel(cfg *config.Config, st *state.State, live bool) model {
	m := model{
		installed: make(map[string]map[string]bool),
		width:     80,
		height:    24,
		live:      live,
		cfg:       cfg,
		st:        st,
	}

	// Build sorted agent list
	for name := range cfg.Agents {
		m.agents = append(m.agents, name)
	}
	sort.Strings(m.agents)

	// Fallback for preview if no agents configured
	if len(m.agents) == 0 {
		m.agents = []string{"claude-code", "copilot", "pi", "hermes"}
	}

	// Discover skills from registered repos
	allSkills, _ := repo.DiscoverAllSkills(st)

	// Group by repo
	repoSkills := make(map[string][]repo.SkillInfo)
	for _, s := range allSkills {
		repoSkills[s.RepoName] = append(repoSkills[s.RepoName], s)
	}

	var repoNames []string
	for name := range repoSkills {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)

	// Sample data if no repos registered
	if len(repoNames) == 0 {
		repoNames, repoSkills = sampleData()
	}

	// Build rows
	for _, rn := range repoNames {
		m.rows = append(m.rows, tuiRow{kind: repoRow, repoName: rn, expanded: true})
		skills := repoSkills[rn]
		sort.Slice(skills, func(i, j int) bool { return skills[i].Address < skills[j].Address })
		for _, s := range skills {
			m.rows = append(m.rows, tuiRow{kind: skillRow, repoName: rn, addr: s.Address, relPath: s.RelPath})
		}
	}

	// Populate installed state from state.json
	for addr, agents := range st.InstalledSkills {
		m.installed[addr] = make(map[string]bool)
		for agentName := range agents {
			m.installed[addr][agentName] = true
		}
	}

	return m
}

func sampleData() ([]string, map[string][]repo.SkillInfo) {
	repoNames := []string{"awesome-skills", "community-skills"}
	skills := map[string][]repo.SkillInfo{
		"awesome-skills": {
			{Address: "awesome-skills/coding/debugger", RepoName: "awesome-skills", RelPath: "coding/debugger"},
			{Address: "awesome-skills/coding/refactor", RepoName: "awesome-skills", RelPath: "coding/refactor"},
			{Address: "awesome-skills/writing/blogger", RepoName: "awesome-skills", RelPath: "writing/blogger"},
		},
		"community-skills": {
			{Address: "community-skills/linter", RepoName: "community-skills", RelPath: "linter"},
			{Address: "community-skills/docker-compose", RepoName: "community-skills", RelPath: "docker-compose"},
			{Address: "community-skills/debug-helper", RepoName: "community-skills", RelPath: "debug-helper"},
		},
	}
	return repoNames, skills
}

// --- Bubble Tea interface ---

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			if m.filter != "" {
				m.filter = ""
			} else {
				return m, tea.Quit
			}
		case tea.KeyUp:
			m.moveCursor(-1)
		case tea.KeyDown:
			m.moveCursor(1)
		case tea.KeyLeft:
			if m.cursorCol > 0 {
				m.cursorCol--
			}
		case tea.KeyRight:
			if m.cursorCol < len(m.agents)-1 {
				m.cursorCol++
			}
		case tea.KeyEnter:
			m.handleEnter()
		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case tea.KeyRunes:
			ch := msg.String()
			if ch == "q" && m.filter == "" {
				return m, tea.Quit
			}
			m.filter += ch
		}
	}
	return m, nil
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

func (m *model) handleEnter() {
	if m.cursorRow < 0 || m.cursorRow >= len(m.rows) {
		return
	}
	row := &m.rows[m.cursorRow]

	// Repo row: toggle expand/collapse
	if row.kind == repoRow {
		row.expanded = !row.expanded
		return
	}

	// Skill row: toggle install/remove for selected agent
	agent := m.agents[m.cursorCol]
	addr := row.addr

	if m.installed[addr] == nil {
		m.installed[addr] = make(map[string]bool)
	}

	if m.installed[addr][agent] {
		// Remove
		if m.live {
			if err := skill.Remove(addr, agent, m.cfg, m.st, true); err != nil {
				m.message = fmt.Sprintf("✗ Remove failed: %v", err)
				return
			}
			if err := state.Save(m.st); err != nil {
				m.message = fmt.Sprintf("✗ Save failed: %v", err)
				return
			}
		}
		m.installed[addr][agent] = false
		m.message = fmt.Sprintf("➖ Removed %s from %s", addr, agent)
	} else {
		// Install
		if m.live {
			if err := skill.Install(addr, agent, m.cfg, m.st, false); err != nil {
				m.message = fmt.Sprintf("✗ Install failed: %v", err)
				return
			}
			if err := state.Save(m.st); err != nil {
				m.message = fmt.Sprintf("✗ Save failed: %v", err)
				return
			}
		}
		m.installed[addr][agent] = true
		m.message = fmt.Sprintf("➕ Installed %s for %s", addr, agent)
	}

	if !m.live {
		m.message += "  (preview — use --live to apply)"
	}
}

// --- View ---

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	cellSelStyle  = lipgloss.NewStyle().Background(lipgloss.Color("62")).Bold(true)
	repoStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	checkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	emptyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	filterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	msgStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m model) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render(" SkillPack "))
	if m.live {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(" LIVE"))
	} else {
		b.WriteString(dimStyle.Render(" preview"))
	}
	b.WriteString("\n")

	// Filter
	if m.filter != "" {
		b.WriteString(filterStyle.Render(fmt.Sprintf(" Filter: %s▌", m.filter)))
	} else {
		b.WriteString(dimStyle.Render(" Type to filter…"))
	}
	b.WriteString("\n\n")

	// Column widths
	nameColW := 34
	agentColW := 12

	// Compute dynamic name column width based on longest visible skill
	vis := m.visibleRows()
	for _, idx := range vis {
		row := m.rows[idx]
		if row.kind == skillRow {
			w := len(row.relPath) + 6 // indent + padding
			if w > nameColW {
				nameColW = w
			}
		}
	}
	if nameColW > 50 {
		nameColW = 50
	}

	// Header row
	header := fmt.Sprintf(" %-*s", nameColW, "SKILL")
	for _, a := range m.agents {
		name := a
		if len(name) > agentColW-2 {
			name = name[:agentColW-2]
		}
		header += fmt.Sprintf(" %-*s", agentColW-1, name)
	}
	b.WriteString(dimStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" " + strings.Repeat("─", tuiMin(m.width-2, len(header)))))
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
			// Skill name
			label := fmt.Sprintf("     %s", row.relPath)
			if len(label) > nameColW {
				label = label[:nameColW-1] + "…"
			}
			nameStr := fmt.Sprintf("%-*s", nameColW+1, label)

			if isSelected {
				b.WriteString(selectedStyle.Render(nameStr))
			} else {
				b.WriteString(nameStr)
			}

			// Agent cells
			for j, agent := range m.agents {
				isInstalled := m.installed[row.addr] != nil && m.installed[row.addr][agent]
				var cell string
				if isInstalled {
					cell = " [✓] "
				} else {
					cell = " [ ] "
				}
				padded := fmt.Sprintf("%-*s", agentColW, cell)

				if isSelected && j == m.cursorCol {
					b.WriteString(cellSelStyle.Render(padded))
				} else if isInstalled {
					b.WriteString(checkStyle.Render(padded))
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
	b.WriteString(dimStyle.Render(" " + strings.Repeat("─", tuiMin(m.width-2, 74))))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" ↑↓ navigate  ←→ agents  Enter expand/toggle  Esc clear  q quit"))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
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

	return b.String()
}

// --- Run ---

func runTUI(live bool) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{Agents: make(map[string]config.AgentConfig)}
	}
	st, err := state.Load()
	if err != nil {
		st = &state.State{
			Repos:           make(map[string]state.RepoRecord),
			InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord),
		}
	}

	m := initialModel(cfg, st, live)

	// Validate: warn if no repos and not using sample
	if len(st.Repos) == 0 {
		fmt.Fprintln(os.Stderr, "No repos registered — showing sample data. Use 'skillpack repo add' first.")
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func tuiMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
