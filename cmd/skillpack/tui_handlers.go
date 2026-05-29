package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// --- Action Handlers (extracted in Phase 3) ---
// These methods implement the direct user-initiated actions (toggling install,
// forking, adopting, adding/removing repos, updating skills, etc.).
// They mutate model state and call into the skill/repo layers.
// The large Update switch and async cmd* factories remain in tui.go for Phase 4.

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
			upstream := strings.TrimSpace(m.registerForkInput)
			if upstream == "" {
				m.message = "✗ Upstream address cannot be empty"
				m.inputMode = modeNormal
				return *m, nil
			}
			m.inputMode = modeNormal
			m.busy = "Registering fork provenance..."
			return *m, m.cmdRegisterForkProvenance(m.registerForkAddr, upstream)
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
