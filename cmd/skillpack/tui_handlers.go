package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/pack"
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

	case modeRelinkStaleInput:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyEnter:
			var newAddr string
			if m.relinkCandidateMode && len(m.relinkCandidates) > 0 {
				newAddr = m.relinkCandidates[m.relinkCandidateCursor]
			} else {
				newAddr = strings.TrimSpace(m.relinkInput)
			}
			if newAddr == "" {
				m.message = "✗ Replacement address cannot be empty"
				m.inputMode = modeNormal
				return *m, nil
			}
			m.inputMode = modeNormal
			m.busy = "Relinking..."
			return *m, m.cmdRelink(m.relinkAddr, newAddr, m.relinkAgentName)
		case tea.KeyUp:
			if m.relinkCandidateMode && m.relinkCandidateCursor > 0 {
				m.relinkCandidateCursor--
			}
		case tea.KeyDown:
			if m.relinkCandidateMode && m.relinkCandidateCursor < len(m.relinkCandidates)-1 {
				m.relinkCandidateCursor++
			}
		case tea.KeyTab:
			// Toggle between candidate list and free-text input
			if len(m.relinkCandidates) > 0 {
				m.relinkCandidateMode = !m.relinkCandidateMode
				if !m.relinkCandidateMode {
					m.relinkInput = ""
				}
			}
		case tea.KeyBackspace:
			if !m.relinkCandidateMode && len(m.relinkInput) > 0 {
				m.relinkInput = m.relinkInput[:len(m.relinkInput)-1]
			}
		case tea.KeyRunes:
			if !m.relinkCandidateMode {
				m.relinkInput += msg.String()
			}
		case tea.KeySpace:
			if !m.relinkCandidateMode {
				m.relinkInput += " "
			}
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeRelinkBrokenChoice:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyRunes:
			switch msg.String() {
			case "1":
				// Set upstream — prompt for new address
				m.relinkInput = ""
				m.inputMode = modeRelinkBrokenSetInput
			case "2":
				// Clear upstream
				m.inputMode = modeNormal
				m.busy = "Clearing upstream pointer..."
				return *m, m.cmdRelinkUpstream(m.relinkAddr, "", m.relinkAgentName)
			}
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modeRelinkBrokenSetInput:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyEnter:
			newUpstream := strings.TrimSpace(m.relinkInput)
			if newUpstream == "" {
				m.message = "✗ Upstream address cannot be empty"
				m.inputMode = modeNormal
				return *m, nil
			}
			m.inputMode = modeNormal
			m.busy = "Setting upstream pointer..."
			return *m, m.cmdRelinkUpstream(m.relinkAddr, newUpstream, m.relinkAgentName)
		case tea.KeyBackspace:
			if len(m.relinkInput) > 0 {
				m.relinkInput = m.relinkInput[:len(m.relinkInput)-1]
			}
		case tea.KeyRunes:
			m.relinkInput += msg.String()
		case tea.KeySpace:
			m.relinkInput += " "
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modePackInstallAgents:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyUp:
			if m.packAgentCursor > 0 {
				m.packAgentCursor--
			}
		case tea.KeyDown:
			if m.packAgentCursor < len(m.agents)-1 {
				m.packAgentCursor++
			}
		case tea.KeySpace:
			m.packAgentSel[m.packAgentCursor] = !m.packAgentSel[m.packAgentCursor]
		case tea.KeyRunes:
			if msg.String() == "a" {
				// Toggle all: select all unless everything is already selected.
				all := true
				for i := range m.agents {
					if !m.packAgentSel[i] {
						all = false
						break
					}
				}
				for i := range m.agents {
					m.packAgentSel[i] = !all
				}
			}
		case tea.KeyEnter:
			var agents []string
			for i, name := range m.agents {
				if m.packAgentSel[i] {
					agents = append(agents, name)
				}
			}
			if len(agents) == 0 {
				m.message = "Select at least one agent (Space to toggle)"
				return *m, nil
			}
			m.inputMode = modeNormal
			m.busy = fmt.Sprintf("Installing pack %s...", m.packInstallAddr)
			m.message = ""
			return *m, m.cmdPackInstall(m.packInstallAddr, agents)
		case tea.KeyCtrlC:
			return *m, tea.Quit
		}

	case modePackConfirmRemove:
		switch msg.Type {
		case tea.KeyEsc:
			m.inputMode = modeNormal
			m.message = ""
		case tea.KeyRunes:
			ch := msg.String()
			if ch == "y" || ch == "Y" {
				m.doPackRemove()
			} else {
				m.inputMode = modeNormal
				m.message = ""
			}
		case tea.KeyEnter:
			m.doPackRemove()
		case tea.KeyCtrlC:
			return *m, tea.Quit
		default:
			m.inputMode = modeNormal
			m.message = ""
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
	case panelPacks:
		if m.packCursor > 0 {
			m.packCursor--
			if m.packDetailOpen {
				m.packDetailOpen = false // close detail on navigation
			}
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
	case panelPacks:
		if m.packCursor < len(m.packRows)-1 {
			m.packCursor++
			if m.packDetailOpen {
				m.packDetailOpen = false // close detail on navigation
			}
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
	case panelPacks:
		// Toggle detail view
		if len(m.packRows) > 0 && m.packCursor < len(m.packRows) {
			m.packDetailOpen = !m.packDetailOpen
			m.message = ""
			// Load the pack definition once on open for available packs;
			// rendering must not touch the disk on every frame.
			m.packDetailDef, m.packDetailErr = nil, nil
			if m.packDetailOpen && !m.packRows[m.packCursor].installed {
				m.loadPackDetail(m.packRows[m.packCursor].packAddr)
			}
		}
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

// startAddRepo begins the add-repo input flow. Shared by the 'a' key in the
// Repos panel and the File→Add Repo menu item.
func (m *model) startAddRepo() {
	m.inputMode = modeAddRepoName
	m.inputBuffer = ""
	m.message = ""
}

// startSelfUpdate kicks off a self-update check+download. Shared by the 'U'
// key in the Status panel and the File→Self-Update menu item.
func (m *model) startSelfUpdate() tea.Cmd {
	m.busy = "Checking for skillpack updates..."
	m.message = ""
	return cmdSelfUpdate()
}

// switchPanel activates panel p, applying the same per-panel refresh Tab
// applies when cycling onto it. Shared by F2-F6 and the View menu.
func (m *model) switchPanel(p panel) tea.Cmd {
	m.activePanel = p
	m.message = ""
	switch p {
	case panelUnmanaged:
		m.refreshUnmanaged()
	case panelPacks:
		m.refreshPacks()
	case panelStatus:
		if len(m.statusRows) == 0 && len(m.st.InstalledSkills) > 0 {
			m.busy = "Checking status..."
			return m.cmdCheckStatus()
		}
	}
	return nil
}

// startViewSkillMd opens the SKILL.md viewer for the row under the cursor in
// the Skills or Unmanaged panel. Shared by the 'v' key and the Actions→View
// SKILL.md menu item.
func (m *model) startViewSkillMd() tea.Cmd {
	switch m.activePanel {
	case panelSkills:
		if m.cursorRow >= 0 && m.cursorRow < len(m.rows) {
			row := m.rows[m.cursorRow]
			if row.kind == skillRow {
				cachePath := m.st.Repos[row.repoName].CachePath
				skillMd := filepath.Join(cachePath, row.relPath, "SKILL.md")
				return m.viewSkillMdAt(skillMd)
			}
		}
	case panelUnmanaged:
		if m.unmanagedCursor >= 0 && m.unmanagedCursor < len(m.unmanagedEntries) {
			entry := m.unmanagedEntries[m.unmanagedCursor]
			skillMd := filepath.Join(entry.localPath, "SKILL.md")
			return m.viewSkillMdAt(skillMd)
		}
	}
	return nil
}

// startPackCreate opens the embedded pack-creation wizard. Shared by the 'n'
// key and the Packs→Create Pack menu item.
func (m *model) startPackCreate() {
	w := initialPackCreateModel(m.cfg, m.st)
	w.embedded = true
	w.width, w.height = m.width, m.height
	m.packWizard = &w
	m.message = ""
}

// startPackEdit opens the embedded pack-edit wizard for the selected pack.
// Shared by the 'e' key and the Packs→Edit Pack menu item.
func (m *model) startPackEdit() {
	if m.packCursor >= len(m.packRows) {
		return
	}
	packAddr := m.packRows[m.packCursor].packAddr
	w, err := initialPackEditModel(packAddr, m.cfg, m.st)
	if err != nil {
		m.message = fmt.Sprintf("✗ Edit failed: %v", err)
		return
	}
	w.embedded = true
	w.width, w.height = m.width, m.height
	m.packWizard = &w
	m.message = ""
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
	recovered, err := repo.Add(name, url, token, m.st)
	if err != nil {
		m.message = fmt.Sprintf("✗ Add failed: %v", err)
		return
	}
	if err := state.Save(m.st); err != nil {
		m.message = fmt.Sprintf("✗ Save failed: %v", err)
		return
	}
	m.refreshRepos()
	m.refreshSkills()
	if recovered {
		m.message = fmt.Sprintf("➕ Re-registered repo %s from existing cache", name)
	} else {
		m.message = fmt.Sprintf("➕ Added repo %s", name)
	}
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

func (m *model) startRepair() {
	if m.cursorRow < 0 || m.cursorRow >= len(m.rows) {
		return
	}
	row := m.rows[m.cursorRow]
	if row.kind != skillRow {
		m.message = "Select a skill to repair"
		return
	}
	if row.problem == problemNone {
		m.message = "No repair needed for this skill"
		return
	}

	agent := m.agents[m.cursorCol]
	m.relinkAddr = row.addr
	m.relinkAgentName = agent
	m.relinkInput = ""

	switch row.problem {
	case problemStale:
		candidates := skill.SuggestReplacements(row.addr, m.st)
		m.relinkCandidates = candidates
		m.relinkCandidateCursor = 0
		m.relinkCandidateMode = len(candidates) > 0
		m.inputMode = modeRelinkStaleInput
		m.message = ""
	case problemBrokenUpstream:
		m.inputMode = modeRelinkBrokenChoice
		m.message = ""
	}
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

// loadPackDetail parses the pack.yaml for packAddr into packDetailDef so the
// detail overlay can render without per-frame disk I/O.
func (m *model) loadPackDetail(packAddr string) {
	info, err := repo.FindPack(packAddr, m.st)
	if err != nil {
		m.packDetailErr = err
		return
	}
	m.packDetailDef, m.packDetailErr = pack.ParseFile(filepath.Join(info.FullPath, "pack.yaml"))
}

// startPackInstall opens the agent-selection overlay for the selected pack.
func (m *model) startPackInstall() {
	if len(m.packRows) == 0 || m.packCursor >= len(m.packRows) {
		return
	}
	row := m.packRows[m.packCursor]
	if row.installed {
		// Re-installing would replace the pack record and lose per-skill
		// status; partial packs are completed with 'c' instead.
		if row.isPartial {
			m.message = "Pack is already installed — press c to complete the partial deployment"
		} else {
			m.message = "Pack is already installed"
		}
		return
	}
	if len(m.agents) == 0 {
		m.message = "✗ No agents configured — add one to ~/.skillpack/config.yaml"
		return
	}
	m.packInstallAddr = row.packAddr
	m.packAgentCursor = 0
	m.packAgentSel = make(map[int]bool)
	// Preselect the default agent (or the only one).
	for i, name := range m.agents {
		if name == m.cfg.DefaultAgent || len(m.agents) == 1 {
			m.packAgentSel[i] = true
			m.packAgentCursor = i
			break
		}
	}
	m.inputMode = modePackInstallAgents
	m.message = ""
}

func (m *model) startPackRemove() {
	if len(m.packRows) == 0 {
		m.message = "No packs to remove"
		return
	}
	if m.packCursor >= len(m.packRows) {
		return
	}
	if !m.packRows[m.packCursor].installed {
		m.message = "Pack is not installed — press i to install"
		return
	}
	packAddr := m.packRows[m.packCursor].packAddr
	m.message = fmt.Sprintf("Remove pack %q? (y/N)", packAddr)
	m.inputMode = modePackConfirmRemove
}

func (m *model) doPackRemove() {
	if m.packCursor >= len(m.packRows) {
		m.inputMode = modeNormal
		return
	}
	packAddr := m.packRows[m.packCursor].packAddr
	rec, ok := m.st.InstalledPacks[packAddr]
	if !ok {
		m.message = fmt.Sprintf("✗ Pack %q not found in state", packAddr)
		m.inputMode = modeNormal
		return
	}

	// Remove all skills for all agents in the pack; count failures for user feedback.
	removeFailures := 0
	for skillAddr, agStatuses := range rec.Skills {
		for ag, agStatus := range agStatuses {
			if !agStatus.Installed {
				continue
			}
			is, err := skill.Open(skillAddr, ag, m.cfg, m.st)
			if err != nil {
				continue // skill not installed — nothing to remove
			}
			if err := is.Remove(true); err != nil {
				removeFailures++
			}
		}
	}

	if err := m.st.RecordPackRemove(packAddr); err != nil {
		m.message = fmt.Sprintf("✗ Remove failed: %v", err)
		m.inputMode = modeNormal
		return
	}
	if err := state.Save(m.st); err != nil {
		m.message = fmt.Sprintf("✗ Save failed: %v", err)
		m.inputMode = modeNormal
		return
	}

	m.refreshPacks()
	m.refreshSkills()
	m.packDetailOpen = false
	if m.packCursor >= len(m.packRows) && m.packCursor > 0 {
		m.packCursor--
	}
	if removeFailures > 0 {
		m.message = fmt.Sprintf("⚠ Removed pack %s (%d skill file(s) could not be deleted — check manually)", packAddr, removeFailures)
	} else {
		m.message = fmt.Sprintf("➖ Removed pack %s", packAddr)
	}
	m.inputMode = modeNormal
}
