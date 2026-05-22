package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
  ↑/↓         Move between unmanaged skills
  Enter       Adopt selected skill into a registered repo
  v           View SKILL.md of the selected skill

All changes are applied immediately.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func init() {
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

// --- Panels ---

type panel int

const (
	panelSkills panel = iota
	panelRepos
	panelStatus
	panelUnmanaged
)

// --- Input mode ---

type inputMode int

const (
	modeNormal inputMode = iota
	modeAddRepoName
	modeAddRepoURL
	modeConfirmRemove
	modeForkSelectRepo
	modeForkResolveChoice
	modeAdoptSelectRepo
	modeRegisterForkInput
)

// --- Model ---

type model struct {
	// Data
	rows      []tuiRow
	agents    []string
	installed map[string]map[string]bool // addr → agent → installed

	// Repos panel data
	repoList []repoEntry

	// UI state
	activePanel panel
	cursorRow   int
	cursorCol   int // agent column index (skills panel)
	repoCursor  int // cursor for repos panel
	filter      string
	width       int
	height      int
	message     string

	// Input mode for repo add / fork
	inputMode   inputMode
	inputBuffer string
	newRepoName string
	forkAddr       string // skill address being forked
	forkCursor     int    // cursor for fork repo selection
	forkTargetRepo string // repo chosen in modeForkSelectRepo, kept for modeForkResolveChoice

	// Status info per skill+agent
	statusInfo   map[string]map[string]string // addr → agent → status text
	statusRows   []statusRow                  // rows for status panel
	statusCursor int                          // cursor for status panel
	busy         string                       // non-empty when an async operation is running

	// Update banner
	updateBanner    string // e.g. "v0.3.0" — shown as a banner when set
	bannerSelection int    // 0=Update, 1=Skip
	pendingMessage  string // message to show after next async completes

	// Fork candidate registration
	forkCandidates    map[string]string // addr → candidate upstream addr
	registerForkAddr  string            // skill addr being registered
	registerForkInput string            // current input buffer (editable upstream addr)

	// Unmanaged panel
	unmanagedEntries []unmanagedEntry
	unmanagedCursor  int
	adoptCursor      int // cursor for repo selection in adopt flow

	// Config/state refs
	cfg *config.Config
	st  *state.State
}

type repoEntry struct {
	name string
	url  string
}

type unmanagedEntry struct {
	agentName string
	skillName string
	localPath string
}

type statusRow struct {
	addr      string
	agentName string
	status    string // "ok", "update", "modified", "conflict", "error"
}

func initialModel(cfg *config.Config, st *state.State) model {
	m := model{
		installed:   make(map[string]map[string]bool),
		activePanel: panelSkills,
		width:       80,
		height:      24,
		cfg:         cfg,
		st:          st,
	}

	// Build sorted agent list
	for name := range cfg.Agents {
		m.agents = append(m.agents, name)
	}
	sort.Strings(m.agents)

	m.refreshSkills()
	m.refreshRepos()
	m.refreshUnmanaged()

	return m
}

func (m *model) refreshSkills() {
	m.rows = nil

	// Discover skills from registered repos
	allSkills, _ := repo.DiscoverAllSkills(m.st)

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
	m.installed = make(map[string]map[string]bool)
	for addr, agents := range m.st.InstalledSkills {
		m.installed[addr] = make(map[string]bool)
		for agentName := range agents {
			m.installed[addr][agentName] = true
		}
	}
}

func (m *model) refreshRepos() {
	m.repoList = nil
	for name, rec := range m.st.Repos {
		m.repoList = append(m.repoList, repoEntry{name: name, url: rec.URL})
	}
	sort.Slice(m.repoList, func(i, j int) bool { return m.repoList[i].name < m.repoList[j].name })
}

func (m *model) refreshUnmanaged() {
	m.unmanagedEntries = nil

	// Build set of all local paths that are already tracked in state
	knownPaths := make(map[string]bool)
	for _, agents := range m.st.InstalledSkills {
		for _, rec := range agents {
			if rec.LocalPath != "" {
				knownPaths[rec.LocalPath] = true
			}
		}
	}

	// Walk each configured agent's skill dir and find untracked skills
	var agentNames []string
	for name := range m.cfg.Agents {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)

	for _, agentName := range agentNames {
		agentCfg := m.cfg.Agents[agentName]
		expanded, err := config.ExpandPath(agentCfg.SkillDir)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(expanded)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			fullPath := filepath.Join(expanded, entry.Name())
			// Use os.Stat (follows symlinks) so symlinked directories are included
			info, statErr := os.Stat(fullPath)
			if statErr != nil || !info.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(fullPath, "SKILL.md")); err != nil {
				continue
			}
			if !knownPaths[fullPath] {
				m.unmanagedEntries = append(m.unmanagedEntries, unmanagedEntry{
					agentName: agentName,
					skillName: entry.Name(),
					localPath: fullPath,
				})
			}
		}
	}

	if m.unmanagedCursor >= len(m.unmanagedEntries) {
		m.unmanagedCursor = 0
	}
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

// --- Async message types ---

type statusDoneMsg struct {
	info           map[string]map[string]string
	forkCandidates map[string]string // addr → first candidate upstream addr
}

type syncDoneMsg struct {
	summary string
	st      *state.State // updated state after sync
}

type selfUpdateDoneMsg struct {
	summary string
}

type updateCheckMsg struct {
	latestTag string // empty if up-to-date or error
	err       error
}

type viewerExitMsg struct {
	err error
}

// --- Bubble Tea interface ---

func (m model) Init() tea.Cmd {
	// Check for skillpack updates on startup (non-blocking)
	return cmdCheckForUpdate()
}

func cmdCheckForUpdate() tea.Cmd {
	return func() tea.Msg {
		current := strings.TrimPrefix(Version, "v")
		if current == "dev" {
			return updateCheckMsg{}
		}
		latest, err := fetchLatestTag()
		if err != nil {
			return updateCheckMsg{err: err}
		}
		latestClean := strings.TrimPrefix(latest, "v")
		if current == latestClean {
			return updateCheckMsg{}
		}
		return updateCheckMsg{latestTag: latest}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case statusDoneMsg:
		m.statusInfo = msg.info
		m.forkCandidates = msg.forkCandidates
		m.busy = ""
		if m.pendingMessage != "" {
			m.message = m.pendingMessage
			m.pendingMessage = ""
		} else {
			m.message = "Status refreshed"
		}
		// Build status rows
		m.statusRows = nil
		for addr, agents := range msg.info {
			for agentName, status := range agents {
				m.statusRows = append(m.statusRows, statusRow{addr: addr, agentName: agentName, status: status})
			}
		}
		sort.Slice(m.statusRows, func(i, j int) bool {
			if m.statusRows[i].addr != m.statusRows[j].addr {
				return m.statusRows[i].addr < m.statusRows[j].addr
			}
			return m.statusRows[i].agentName < m.statusRows[j].agentName
		})
		if m.statusCursor >= len(m.statusRows) {
			m.statusCursor = 0
		}
		return m, nil

	case syncDoneMsg:
		m.busy = ""
		m.message = msg.summary
		// Swap in the state that was mutated by the sync goroutine
		if msg.st != nil {
			*m.st = *msg.st
		}
		// Refresh installed state after sync
		m.installed = make(map[string]map[string]bool)
		for addr, agents := range m.st.InstalledSkills {
			m.installed[addr] = make(map[string]bool)
			for agentName := range agents {
				m.installed[addr][agentName] = true
			}
		}
		// Auto-refresh status after sync
		if m.activePanel == panelStatus {
			m.busy = "Refreshing status..."
			m.pendingMessage = msg.summary
			return m, m.cmdCheckStatus()
		}
		return m, nil

	case selfUpdateDoneMsg:
		m.busy = ""
		m.message = msg.summary
		m.updateBanner = "" // dismiss banner after update attempt
		return m, nil

	case updateCheckMsg:
		if msg.latestTag != "" {
			m.updateBanner = msg.latestTag
			m.bannerSelection = 0
		}
		return m, nil

	case viewerExitMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ Viewer error: %v", msg.err)
		}
		return m, nil

	case tea.KeyMsg:
		// Block input while busy
		if m.busy != "" {
			if msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle update banner if active
		if m.updateBanner != "" {
			switch msg.Type {
			case tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyLeft, tea.KeyRight:
				if m.bannerSelection == 0 {
					m.bannerSelection = 1
				} else {
					m.bannerSelection = 0
				}
			case tea.KeyEnter, tea.KeySpace:
				if m.bannerSelection == 0 {
					// Update
					m.busy = "Downloading update..."
					return m, cmdSelfUpdate()
				} else {
					// Skip
					m.updateBanner = ""
				}
			case tea.KeyEsc:
				m.updateBanner = ""
			case tea.KeyRunes:
				ch := msg.String()
				if ch == "q" {
					return m, tea.Quit
				}
			}
			return m, nil
		}

		// Handle input modes first
		if m.inputMode != modeNormal {
			return m.handleInputMode(msg)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyTab:
			switch m.activePanel {
			case panelSkills:
				m.activePanel = panelStatus
				// Auto-refresh status on first visit
				if len(m.statusRows) == 0 && len(m.st.InstalledSkills) > 0 {
					m.busy = "Checking status..."
					m.message = ""
					return m, m.cmdCheckStatus()
				}
			case panelStatus:
				m.activePanel = panelRepos
			case panelRepos:
				m.activePanel = panelUnmanaged
				m.refreshUnmanaged()
			case panelUnmanaged:
				m.activePanel = panelSkills
			}
			m.message = ""
		case tea.KeyEsc:
			if m.filter != "" {
				m.filter = ""
			} else {
				return m, tea.Quit
			}
		case tea.KeyUp:
			m.handleUp()
		case tea.KeyDown:
			m.handleDown()
		case tea.KeyLeft:
			if m.activePanel == panelSkills && m.cursorCol > 0 {
				m.cursorCol--
			}
		case tea.KeyRight:
			if m.activePanel == panelSkills && m.cursorCol < len(m.agents)-1 {
				m.cursorCol++
			}
		case tea.KeyEnter:
			m.handleAction()
		case tea.KeySpace:
			if m.activePanel == panelSkills {
				m.handleEnter()
			}
		case tea.KeyDelete:
			if m.activePanel == panelRepos {
				m.startRemoveRepo()
			}
		case tea.KeyBackspace:
			if m.activePanel == panelSkills && len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case tea.KeyRunes:
			ch := msg.String()
			switch m.activePanel {
			case panelSkills:
				if ch == "q" && m.filter == "" {
					return m, tea.Quit
				}
				if ch == "f" && m.filter == "" {
					m.startFork()
					return m, nil
				}
				if ch == "v" && m.filter == "" {
					if m.cursorRow >= 0 && m.cursorRow < len(m.rows) {
						row := m.rows[m.cursorRow]
						if row.kind == skillRow {
							cachePath := m.st.Repos[row.repoName].CachePath
							skillMd := filepath.Join(cachePath, row.relPath, "SKILL.md")
							return m, m.viewSkillMdAt(skillMd)
						}
					}
					return m, nil
				}
				// On Windows (ConPTY), space can arrive as a rune instead of
				// KeySpace. Treat it as the toggle action to match Linux/macOS.
				if ch == " " {
					m.handleEnter()
					return m, nil
				}
				m.filter += ch
			case panelStatus:
				switch ch {
				case "q":
					return m, tea.Quit
				case "r":
					// If the selected skill is a fork candidate, offer to register provenance.
					if m.statusCursor < len(m.statusRows) {
						row := m.statusRows[m.statusCursor]
						if upstream, ok := m.forkCandidates[row.addr]; ok {
							m.registerForkAddr = row.addr
							m.registerForkInput = upstream
							m.inputMode = modeRegisterForkInput
							m.message = ""
							return m, nil
						}
					}
					// Otherwise refresh status.
					m.busy = "Refreshing status..."
					m.message = ""
					return m, m.cmdCheckStatus()
				case "S":
					m.busy = "Syncing all..."
					m.message = ""
					return m, m.cmdSync()
				case "u":
					// Update the selected skill
					m.updateSelectedSkill()
				case "U":
					m.busy = "Checking for skillpack updates..."
					m.message = ""
					return m, cmdSelfUpdate()
				}
			case panelRepos:
				switch ch {
				case "q":
					return m, tea.Quit
				case "a":
					m.inputMode = modeAddRepoName
					m.inputBuffer = ""
					m.message = ""
				case "d":
					m.startRemoveRepo()
				}
			case panelUnmanaged:
				if ch == "q" {
					return m, tea.Quit
				}
				if ch == "v" {
					if m.unmanagedCursor >= 0 && m.unmanagedCursor < len(m.unmanagedEntries) {
						entry := m.unmanagedEntries[m.unmanagedCursor]
						skillMd := filepath.Join(entry.localPath, "SKILL.md")
						return m, m.viewSkillMdAt(skillMd)
					}
					return m, nil
				}
			}
		}
	}
	return m, nil
}

func (m *model) handleInputMode(msg tea.KeyMsg) (model, tea.Cmd) {
	switch m.inputMode {
	case modeAddRepoName:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.inputBuffer = ""
			m.message = ""
		case tea.KeyEnter:
			name := strings.TrimSpace(m.inputBuffer)
			if name == "" {
				m.message = "✗ Repo name cannot be empty"
				m.inputMode = modeNormal
				m.inputBuffer = ""
			} else {
				m.newRepoName = name
				m.inputMode = modeAddRepoURL
				m.inputBuffer = ""
			}
		case tea.KeyBackspace:
			if len(m.inputBuffer) > 0 {
				m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
			}
		case tea.KeyRunes:
			m.inputBuffer += msg.String()
		case tea.KeySpace:
			m.inputBuffer += " "
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeAddRepoURL:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.inputBuffer = ""
			m.message = ""
		case tea.KeyEnter:
			url := strings.TrimSpace(m.inputBuffer)
			if url == "" {
				m.message = "✗ URL cannot be empty"
				m.inputMode = modeNormal
				m.inputBuffer = ""
			} else {
				m.doAddRepo(m.newRepoName, url)
				m.inputMode = modeNormal
				m.inputBuffer = ""
			}
		case tea.KeyBackspace:
			if len(m.inputBuffer) > 0 {
				m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
			}
		case tea.KeyRunes:
			m.inputBuffer += msg.String()
		case tea.KeySpace:
			m.inputBuffer += " "
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeConfirmRemove:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyRunes:
			ch := msg.String()
			if ch == "y" || ch == "Y" {
				m.doRemoveRepo()
			} else {
				m.inputMode = modeNormal
				m.message = ""
			}
		case tea.KeyEnter:
			m.doRemoveRepo()
		case tea.KeyCtrlC:
			return *m, tea.Quit
		default:
			m.inputMode = modeNormal
			m.message = ""
		}

	case modeForkSelectRepo:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyUp:
			if m.forkCursor > 0 {
				m.forkCursor--
			}
		case tea.KeyDown:
			if m.forkCursor < len(m.repoList)-1 {
				m.forkCursor++
			}
		case tea.KeyEnter:
			m.doFork()
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeForkResolveChoice:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyRunes:
			switch msg.String() {
			case "1":
				m.execFork(m.forkTargetRepo, skill.ForkModeOverride)
			case "2":
				m.execFork(m.forkTargetRepo, skill.ForkModeRegister)
			}
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeAdoptSelectRepo:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyUp:
			if m.adoptCursor > 0 {
				m.adoptCursor--
			}
		case tea.KeyDown:
			if m.adoptCursor < len(m.repoList)-1 {
				m.adoptCursor++
			}
		case tea.KeyEnter:
			m.doAdopt()
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeRegisterForkInput:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyEnter:
			m.doRegisterForkProvenance()
		case tea.KeyBackspace:
			if len(m.registerForkInput) > 0 {
				m.registerForkInput = m.registerForkInput[:len(m.registerForkInput)-1]
			}
		case tea.KeyRunes:
			m.registerForkInput += msg.String()
		case tea.KeySpace:
			m.registerForkInput += " "
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}
	}
	return *m, nil
}

func (m *model) handleUp() {
	switch m.activePanel {
	case panelSkills:
		m.moveCursor(-1)
	case panelRepos:
		if m.repoCursor > 0 {
			m.repoCursor--
		}
	case panelStatus:
		if m.statusCursor > 0 {
			m.statusCursor--
		}
	case panelUnmanaged:
		if m.unmanagedCursor > 0 {
			m.unmanagedCursor--
		}
	}
}

func (m *model) handleDown() {
	switch m.activePanel {
	case panelSkills:
		m.moveCursor(1)
	case panelRepos:
		if m.repoCursor < len(m.repoList)-1 {
			m.repoCursor++
		}
	case panelStatus:
		if m.statusCursor < len(m.statusRows)-1 {
			m.statusCursor++
		}
	case panelUnmanaged:
		if m.unmanagedCursor < len(m.unmanagedEntries)-1 {
			m.unmanagedCursor++
		}
	}
}

func (m *model) handleAction() {
	switch m.activePanel {
	case panelSkills:
		m.handleEnter()
	case panelStatus:
		m.updateSelectedSkill()
	case panelUnmanaged:
		m.startAdopt()
	}
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

	// Skill row: toggle install/remove
	m.handleSkillToggle()
}

func (m *model) handleSkillToggle() {
	if m.cursorRow < 0 || m.cursorRow >= len(m.rows) {
		return
	}
	row := &m.rows[m.cursorRow]
	if row.kind != skillRow {
		return
	}

	agent := m.agents[m.cursorCol]
	addr := row.addr

	if m.installed[addr] == nil {
		m.installed[addr] = make(map[string]bool)
	}

	if m.installed[addr][agent] {
		// Remove
		is, err := skill.Open(addr, agent, m.cfg, m.st)
		if err != nil {
			m.message = fmt.Sprintf("✗ Remove failed: %v", err)
			return
		}
		if err := is.Remove(true); err != nil {
			m.message = fmt.Sprintf("✗ Remove failed: %v", err)
			return
		}
		m.installed[addr][agent] = false
		m.message = fmt.Sprintf("➖ Removed %s from %s", addr, agent)
	} else {
		// Install
		if err := skill.Install(addr, agent, m.cfg, m.st, false); err != nil {
			m.message = fmt.Sprintf("✗ Install failed: %v", err)
			return
		}
		if err := state.Save(m.st); err != nil {
			m.message = fmt.Sprintf("✗ Save failed: %v", err)
			return
		}
		m.installed[addr][agent] = true
		m.message = fmt.Sprintf("➕ Installed %s for %s", addr, agent)
	}
}

func (m *model) startRemoveRepo() {
	if len(m.repoList) == 0 {
		m.message = "No repos to remove"
		return
	}
	if m.repoCursor >= len(m.repoList) {
		m.repoCursor = len(m.repoList) - 1
	}
	name := m.repoList[m.repoCursor].name
	m.message = fmt.Sprintf("Remove repo %q? (y/N)", name)
	m.inputMode = modeConfirmRemove
}

func (m *model) doAddRepo(name, url string) {
	token := m.cfg.TokenForRepo(name)
	if err := repo.Add(name, url, token, m.st); err != nil {
		m.message = fmt.Sprintf("✗ Add failed: %v", err)
		return
	}
	if err := state.Save(m.st); err != nil {
		m.message = fmt.Sprintf("✗ Save failed: %v", err)
		return
	}
	m.refreshRepos()
	m.refreshSkills()
	m.message = fmt.Sprintf("➕ Added repo %s", name)
}

func (m *model) doRemoveRepo() {
	if m.repoCursor >= len(m.repoList) {
		m.inputMode = modeNormal
		return
	}
	name := m.repoList[m.repoCursor].name
	if err := repo.Remove(name, m.st); err != nil {
		m.message = fmt.Sprintf("✗ Remove failed: %v", err)
		m.inputMode = modeNormal
		return
	}
	if err := state.Save(m.st); err != nil {
		m.message = fmt.Sprintf("✗ Save failed: %v", err)
		m.inputMode = modeNormal
		return
	}
	m.refreshRepos()
	m.refreshSkills()
	if m.repoCursor >= len(m.repoList) && m.repoCursor > 0 {
		m.repoCursor--
	}
	m.message = fmt.Sprintf("➖ Removed repo %s", name)
	m.inputMode = modeNormal
}

func (m *model) startAdopt() {
	if len(m.unmanagedEntries) == 0 {
		m.message = "No unmanaged skills to adopt"
		return
	}
	if m.unmanagedCursor >= len(m.unmanagedEntries) {
		return
	}
	if len(m.repoList) == 0 {
		m.message = "✗ No repos registered — add a writable repo first (Tab → Repos → a)"
		return
	}
	m.adoptCursor = 0
	m.inputMode = modeAdoptSelectRepo
	m.message = ""
}

func (m *model) doAdopt() {
	if m.adoptCursor >= len(m.repoList) {
		m.inputMode = modeNormal
		return
	}
	if m.unmanagedCursor >= len(m.unmanagedEntries) {
		m.inputMode = modeNormal
		return
	}
	entry := m.unmanagedEntries[m.unmanagedCursor]
	targetRepo := m.repoList[m.adoptCursor].name
	token := m.cfg.TokenForRepo(targetRepo)

	newAddr, err := skill.PublishNew(entry.localPath, targetRepo, token, m.st)
	if err != nil {
		m.message = fmt.Sprintf("✗ Publish failed: %v", err)
		m.inputMode = modeNormal
		return
	}

	if err := skill.Install(newAddr, entry.agentName, m.cfg, m.st, false); err != nil {
		m.message = fmt.Sprintf("✗ Install failed: %v", err)
		m.inputMode = modeNormal
		return
	}

	if err := state.Save(m.st); err != nil {
		m.message = fmt.Sprintf("✗ Save failed: %v", err)
		m.inputMode = modeNormal
		return
	}

	m.refreshUnmanaged()
	m.refreshSkills()
	m.refreshRepos()

	m.message = fmt.Sprintf("✓ Adopted %s → %s (%s)", entry.skillName, newAddr, entry.agentName)
	m.inputMode = modeNormal
}

func (m *model) doRegisterForkProvenance() {
	addr := m.registerForkAddr
	upstream := strings.TrimSpace(m.registerForkInput)
	if upstream == "" {
		m.message = "✗ Upstream address cannot be empty"
		m.inputMode = modeNormal
		return
	}
	token := m.cfg.TokenForRepo(repoNameFromAddr(addr))
	if err := skill.RegisterForkProvenance(addr, upstream, token, m.st); err != nil {
		m.message = fmt.Sprintf("✗ Register failed: %v", err)
		m.inputMode = modeNormal
		return
	}
	// Remove from candidates map so the badge clears immediately.
	delete(m.forkCandidates, addr)
	m.message = fmt.Sprintf("✓ Registered %s as fork of %s", addr, upstream)
	m.inputMode = modeNormal
}

func (m *model) startFork() {
	if m.cursorRow < 0 || m.cursorRow >= len(m.rows) {
		return
	}
	row := m.rows[m.cursorRow]
	if row.kind != skillRow {
		m.message = "Select a skill to fork"
		return
	}

	// Must be installed for current agent
	agent := m.agents[m.cursorCol]
	if m.installed[row.addr] == nil || !m.installed[row.addr][agent] {
		m.message = fmt.Sprintf("✗ %s must be installed for %s before forking", row.addr, agent)
		return
	}

	if len(m.repoList) == 0 {
		m.message = "✗ No repos registered — add a writable repo first (Tab → a)"
		return
	}

	m.forkAddr = row.addr
	m.forkCursor = 0
	m.inputMode = modeForkSelectRepo
	m.message = ""
}

func (m *model) doFork() {
	if m.forkCursor >= len(m.repoList) {
		m.inputMode = modeNormal
		return
	}

	targetRepo := m.repoList[m.forkCursor].name

	// Check for unknown provenance before calling Fork — if destination already
	// exists in the repo cache but has no state entry, show the resolution overlay.
	skillName := filepath.Base(m.forkAddr)
	newAddr := targetRepo + "/" + skillName
	forkRec := m.st.Repos[targetRepo]
	forkDestPath := filepath.Join(forkRec.CachePath, skillName)
	_, destStatErr := os.Stat(forkDestPath)
	if destStatErr != nil && !os.IsNotExist(destStatErr) {
		m.message = fmt.Sprintf("✗ Fork: cannot check destination: %v", destStatErr)
		return
	}
	destExists := destStatErr == nil
	_, stateKnown := m.st.InstalledSkills[newAddr]
	if destExists && !stateKnown {
		m.forkTargetRepo = targetRepo
		m.inputMode = modeForkResolveChoice
		m.message = ""
		return
	}

	m.execFork(targetRepo, skill.ForkModeAuto)
}

func (m *model) execFork(targetRepo string, mode skill.ForkMode) {
	agent := m.agents[m.cursorCol]
	token := m.cfg.TokenForRepo(targetRepo)

	is, err := skill.Open(m.forkAddr, agent, m.cfg, m.st)
	if err != nil {
		m.message = fmt.Sprintf("✗ Fork failed: %v", err)
		m.inputMode = modeNormal
		return
	}
	newAddr, err := is.Fork(targetRepo, token, mode)
	if err != nil {
		m.message = fmt.Sprintf("✗ Fork failed: %v", err)
		m.inputMode = modeNormal
		return
	}

	m.refreshSkills()

	m.message = fmt.Sprintf("🍴 Forked %s → %s", m.forkAddr, newAddr)
	m.inputMode = modeNormal
}

func (m *model) updateSelectedSkill() {
	if len(m.statusRows) == 0 || m.statusCursor >= len(m.statusRows) {
		m.message = "No skill selected"
		return
	}
	row := m.statusRows[m.statusCursor]
	if row.status != "update" {
		m.message = fmt.Sprintf("%s/%s: no update available (status: %s)", row.addr, row.agentName, row.status)
		return
	}

	token := m.cfg.TokenForRepo(repoNameFromAddr(row.addr))
	is, err := skill.Open(row.addr, row.agentName, m.cfg, m.st)
	if err != nil {
		m.message = fmt.Sprintf("✗ Update failed: %v", err)
		return
	}
	if err := is.Update(token); err != nil {
		m.message = fmt.Sprintf("✗ Update failed: %v", err)
		return
	}

	// Update the status row
	m.statusRows[m.statusCursor].status = "ok"
	if m.statusInfo != nil && m.statusInfo[row.addr] != nil {
		m.statusInfo[row.addr][row.agentName] = "ok"
	}
	// Refresh installed
	m.installed = make(map[string]map[string]bool)
	for addr, agents := range m.st.InstalledSkills {
		m.installed[addr] = make(map[string]bool)
		for agentName := range agents {
			m.installed[addr][agentName] = true
		}
	}
	m.message = fmt.Sprintf("✓ Updated %s for %s", row.addr, row.agentName)
}

// --- Async commands ---

// viewSkillMdAt stats path and either sets an error message or returns a
// command to open the file in the platform default viewer.
func (m *model) viewSkillMdAt(path string) tea.Cmd {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.message = "✗ SKILL.md not found"
		} else {
			m.message = fmt.Sprintf("✗ %v", err)
		}
		return nil
	}
	m.message = ""
	return cmdViewSkillMd(path)
}

// cmdViewSkillMd opens path in the platform default viewer by suspending the
// TUI until the launcher process exits. On macOS, "open -W" is used so the TUI
// stays suspended until the viewer application itself closes. On Linux and
// Windows, xdg-open/start return as soon as the viewer is launched, so the TUI
// resumes promptly after the handoff.
func cmdViewSkillMd(path string) tea.Cmd {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-W", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return viewerExitMsg{err: err}
	})
}

func (m *model) cmdCheckStatus() tea.Cmd {
	cfg := m.cfg
	// Deep-copy state to avoid data races with UI reads
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		// Fetch repos first
		for name := range stCopy.Repos {
			_ = repo.Update(name, cfg.TokenForRepo(name), stCopy)
		}

		info := make(map[string]map[string]string)
		for addr, agents := range stCopy.InstalledSkills {
			info[addr] = make(map[string]string)
			for agentName := range agents {
				is, openErr := skill.Open(addr, agentName, cfg, stCopy)
				if openErr != nil {
					info[addr][agentName] = "error"
					continue
				}
				r, err := is.Status()
				if err != nil {
					info[addr][agentName] = "error"
					continue
				}
				switch {
				case r.IsConflict:
					info[addr][agentName] = "conflict"
				case r.IsModified:
					info[addr][agentName] = "modified"
				case r.HasUpstream:
					info[addr][agentName] = "update"
				default:
					info[addr][agentName] = "ok"
				}
			}
		}

		// Detect skills with missing fork provenance.
		return statusDoneMsg{info: info, forkCandidates: skill.ForkCandidateMap(stCopy)}
	}
}

func (m *model) cmdSync() tea.Cmd {
	cfg := m.cfg
	// Deep-copy state to avoid data races with UI reads
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		results, conflicts, err := skill.Sync(false, cfg.TokenForRepo, stCopy)
		if err != nil {
			return syncDoneMsg{summary: fmt.Sprintf("✗ Sync error: %v", err), st: stCopy}
		}

		var updated, published, current, errCount int
		for _, r := range results {
			switch {
			case r.Err != nil:
				errCount++
			case r.Action == skill.SyncUpdated:
				updated++
			case r.Action == skill.SyncPublished:
				published++
			case r.Action == skill.SyncAlreadyCurrent:
				current++
			}
		}

		if updated > 0 || published > 0 {
			_ = state.Save(stCopy)
		}

		parts := []string{}
		if updated > 0 {
			parts = append(parts, fmt.Sprintf("%d updated", updated))
		}
		if published > 0 {
			parts = append(parts, fmt.Sprintf("%d published", published))
		}
		if current > 0 {
			parts = append(parts, fmt.Sprintf("%d current", current))
		}
		if len(conflicts) > 0 {
			parts = append(parts, fmt.Sprintf("%d conflict(s)", len(conflicts)))
		}
		if errCount > 0 {
			parts = append(parts, fmt.Sprintf("%d error(s)", errCount))
		}
		summary := "✓ Sync: " + strings.Join(parts, ", ")
		if len(parts) == 0 {
			summary = "✓ Nothing to sync"
		}
		return syncDoneMsg{summary: summary, st: stCopy}
	}
}

func cmdSelfUpdate() tea.Cmd {
	return func() tea.Msg {
		current := strings.TrimPrefix(Version, "v")
		if current == "dev" {
			return selfUpdateDoneMsg{summary: "Running dev build — skipping update"}
		}

		latest, err := fetchLatestTag()
		if err != nil {
			return selfUpdateDoneMsg{summary: fmt.Sprintf("✗ Could not check: %v", err)}
		}

		latestClean := strings.TrimPrefix(latest, "v")
		if current == latestClean {
			return selfUpdateDoneMsg{summary: fmt.Sprintf("✓ Already up to date (v%s)", current)}
		}

		// Perform the update
		if err := downloadAndReplace(latest); err != nil {
			return selfUpdateDoneMsg{summary: fmt.Sprintf("✗ Update failed: %v", err)}
		}

		return selfUpdateDoneMsg{summary: fmt.Sprintf("✓ Updated: v%s → %s (restart to use new version)", current, latest)}
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

// --- View ---

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
)

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
			// Skill name
			upstream := m.upstreamAddr(row.addr)
			isFork := upstream != ""
			const forkGlyph = " ⑂"
			const forkGlyphW = 2 // visible columns: space + ⑂
			label := fmt.Sprintf("     %s", row.relPath)
			if isFork {
				// Reserve room for the glyph; truncate label if needed
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
			} else {
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
	b.WriteString(helpStyle.Render(" ↑↓ navigate  ←→ agents  Space/Enter toggle  f fork  v view  Tab switch  q quit"))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		// Show fork provenance when cursor is on a forked skill
		if m.cursorRow >= 0 && m.cursorRow < len(m.rows) {
			selRow := m.rows[m.cursorRow]
			if selRow.kind == skillRow {
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

// upstreamAddr returns the upstream skill address for a forked skill, or "" if
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

	displayed := 0
	for i, entry := range m.repoList {
		if displayed >= maxRows {
			break
		}
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

		displayed := 0
		for i, entry := range m.unmanagedEntries {
			if displayed >= maxRows {
				break
			}
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
	b.WriteString(helpStyle.Render(" ↑↓ navigate  Enter adopt into repo  v view  Tab switch  q quit"))
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(msgStyle.Render(" " + m.message))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d unmanaged skill(s) found", len(m.unmanagedEntries))))
	}
}

// --- Run ---

func runTUI() error {
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

	m := initialModel(cfg, st)

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// safeRepeat returns a string of n repetitions of s, or empty if n <= 0.
func safeRepeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}

// cloneState creates a deep copy of State to avoid data races
// between async commands and UI rendering.
func cloneState(src *state.State) *state.State {
	dst := &state.State{
		Repos:           make(map[string]state.RepoRecord, len(src.Repos)),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord, len(src.InstalledSkills)),
	}
	for k, v := range src.Repos {
		dst.Repos[k] = v
	}
	for addr, agents := range src.InstalledSkills {
		dst.InstalledSkills[addr] = make(map[string]state.InstalledSkillRecord, len(agents))
		for agent, rec := range agents {
			dst.InstalledSkills[addr][agent] = rec
		}
	}
	return dst
}
