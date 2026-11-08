package main

import (
	"fmt"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Core Dispatcher (extracted in Phase 4) ---
// The Update method is the heart of the Bubble Tea event loop.
// It handles all incoming messages (key events, async results from cmd* factories,
// window size changes, etc.) and decides what to render and what commands to run next.
// handleInputMode (the giant mode-specific key handler) was moved to tui_handlers.go in Phase 3.

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	// A Doctor fix finished (dialog closed, async work done): rescan so the
	// fixed finding disappears.
	if nm, ok := next.(model); ok && nm.doctorDirty && nm.inputMode == modeNormal && nm.busy == "" {
		nm.doctorDirty = false
		if err := nm.refreshDoctor(); err != nil {
			nm.message = fmt.Sprintf("✗ %v", err)
		}
		return nm, cmd
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Embedded pack wizard takes over key/window/result messages while active.
	// Other async results (status refresh, etc.) still fall through to the
	// main switch below.
	if m.packWizard != nil {
		switch msg.(type) {
		case tea.WindowSizeMsg, tea.KeyMsg, packCreateDoneMsg:
			return m.updateWizard(msg)
		}
	}

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
		m.rebuildStatusRows()
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

	case doctorWriteDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ %v", msg.err)
		} else {
			if msg.st != nil {
				*m.st = *msg.st
			}
			m.refreshSkills()
			m.message = "✓ " + msg.summary
		}
		return m, nil

	case registerForkDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ Register failed: %v", msg.err)
		} else {
			if msg.st != nil {
				*m.st = *msg.st
			}
			delete(m.forkCandidates, msg.addr)
			m.message = fmt.Sprintf("✓ Registered %s as fork of %s", msg.addr, msg.upstream)
		}
		return m, nil

	case relinkDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ Relink failed: %v", msg.err)
		} else {
			if msg.st != nil {
				*m.st = *msg.st
			}
			m.refreshSkills()
			m.message = fmt.Sprintf("✓ Relinked %s → %s (%s)", msg.oldAddr, msg.newAddr, msg.agent)
		}
		return m, nil

	case relinkUpstreamDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ Relink upstream failed: %v", msg.err)
		} else {
			if msg.st != nil {
				*m.st = *msg.st
			}
			m.refreshSkills()
			if msg.newUpstream == "" {
				m.message = fmt.Sprintf("✓ Cleared upstream pointer for %s (%s)", msg.addr, msg.agent)
			} else {
				m.message = fmt.Sprintf("✓ Set upstream %s → %s (%s)", msg.addr, msg.newUpstream, msg.agent)
			}
		}
		return m, nil

	case selfUpdateDoneMsg:
		m.busy = ""
		m.message = msg.summary
		m.updateBanner = "" // dismiss banner after update attempt
		if msg.needsRestart {
			m.restartPending = true
			return m, tea.Quit
		}
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

	case packInstallDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ Install failed: %v", msg.err)
		} else {
			if msg.st != nil {
				*m.st = *msg.st
			}
			m.refreshPacks()
			m.refreshSkills()
			m.message = msg.summary
		}
		return m, nil

	case packCompleteDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.message = fmt.Sprintf("✗ Complete deployment failed: %v", msg.err)
		} else {
			if msg.st != nil {
				*m.st = *msg.st
			}
			m.refreshPacks()
			m.refreshSkills()
			m.message = msg.summary
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

		// Dropdown menu, when open, consumes all keys.
		if m.menuOpen {
			return m.handleMenuKey(msg)
		}

		// Global menu-activation and F-key shortcuts, available from any panel.
		// Bare letters are deliberately excluded so type-to-filter (Skills,
		// Unmanaged) and existing single-letter shortcuts keep working.
		if cmd, handled := m.handleGlobalKey(msg); handled {
			return m, cmd
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyTab, tea.KeyShiftTab:
			step := 1
			if msg.Type == tea.KeyShiftTab {
				step = len(panelOrder) - 1
			}
			return m, m.switchPanel(panelOrder[(panelIndex(m.activePanel)+step)%len(panelOrder)])
		case tea.KeyEsc:
			// Esc only backs out (clear selection, close detail, clear filter);
			// it never quits - q and Ctrl+C do.
			f := m.activeFilter()
			switch {
			case f != nil && m.filterActive:
				*f = ""
				m.filterActive = false
				m.filterChanged()
			case m.activePanel == panelPacks && len(m.packMarks) > 0 && !m.packDetailOpen:
				m.packMarks = nil
			case m.activePanel == panelPacks && m.packDetailOpen:
				m.packDetailOpen = false
				m.message = ""
			case f != nil && *f != "":
				*f = ""
				m.filterChanged()
			}
		case tea.KeyUp:
			m.filterActive = false // arrows hand keyboard focus from the filter box to the list
			m.handleUp()
		case tea.KeyDown:
			m.filterActive = false
			m.handleDown()
		case tea.KeyLeft:
			if m.activePanel == panelSkills && m.cursorCol > 0 {
				m.cursorCol--
			}
			if m.activePanel == panelPacks && m.packCol > 0 && !m.filterActive {
				m.packCol--
			}
		case tea.KeyRight:
			if m.activePanel == panelSkills && m.cursorCol < len(m.agents)-1 {
				m.cursorCol++
			}
			if m.activePanel == panelPacks && m.packCol < len(m.agents)-1 && !m.filterActive {
				m.packCol++
			}
		case tea.KeyEnter:
			if m.activePanel == panelDoctor {
				return m, m.applyDoctorFix()
			}
			m.handleAction()
		case tea.KeySpace:
			if m.filterActive && m.activeFilter() != nil {
				m.appendFilter(" ")
				break
			}
			switch m.activePanel {
			case panelSkills:
				m.handleEnter()
			case panelPacks:
				if !m.packDetailOpen {
					return m, m.togglePackAgent()
				}
			}
		case tea.KeyDelete:
			if m.activePanel == panelRepos {
				m.startRemoveRepo()
			}
		case tea.KeyBackspace:
			if m.filterActive {
				m.backspaceFilter()
			}
		case tea.KeyRunes:
			ch := msg.String()
			if f := m.activeFilter(); f != nil {
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.appendFilter(ch)
					return m, nil
				}
			}
			switch ch {
			case "?":
				m.openHelp()
				return m, nil
			case ":":
				m.openPalette()
				return m, nil
			case "A":
				m.startAddAgent()
				return m, nil
			}
			switch m.activePanel {
			case panelSkills:
				// On Windows (ConPTY), space can arrive as a rune instead of
				// KeySpace. Treat it as the toggle action to match Linux/macOS.
				if ch == " " {
					m.handleEnter()
					return m, nil
				}
				switch ch {
				case "q":
					return m, tea.Quit
				case "f":
					m.startFork()
				case "R":
					m.startRepair()
				case "v":
					return m, m.startViewSkillMd()
				}
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
					return m, m.startSelfUpdate()
				}
			case panelRepos:
				switch ch {
				case "q":
					return m, tea.Quit
				case "a":
					m.startAddRepo()
				case "d":
					m.startRemoveRepo()
				}
			case panelUnmanaged:
				switch ch {
				case "q":
					return m, tea.Quit
				case "v":
					return m, m.startViewSkillMd()
				}
			case panelPacks:
				// Filter UI isn't shown while the detail overlay is open, so '/'
				// and typed runes fall through to the overlay's own shortcuts
				// instead of silently mutating packFilter behind it.
				// Windows ConPTY can deliver space as a rune.
				if ch == " " && !m.packDetailOpen {
					return m, m.togglePackAgent()
				}
				switch ch {
				case "q":
					return m, tea.Quit
				case "m":
					m.toggleMarkPack()
				case "d":
					m.startPackRemove()
				case "n":
					m.startPackCreate()
				case "e":
					m.startPackEdit()
				case "c":
					// Complete deployment for the selected partial pack
					if m.packCursor < len(m.packRows) && m.packRows[m.packCursor].isPartial {
						packAddr := m.packRows[m.packCursor].packAddr
						m.busy = "Completing deployment..."
						m.message = ""
						return m, m.cmdCompleteDeployment(packAddr)
					} else if m.packCursor < len(m.packRows) {
						if m.packRows[m.packCursor].installed {
							m.message = "Pack is already fully deployed"
						} else {
							m.message = "Pack is not installed — press Space to enable it for an agent"
						}
					}
				}
			case panelDoctor:
				switch ch {
				case "q":
					return m, tea.Quit
				case "f":
					return m, m.applyDoctorFix()
				case "r":
					if err := m.refreshDoctor(); err != nil {
						m.message = fmt.Sprintf("✗ %v", err)
					} else {
						m.message = "Rescanned"
					}
				}
			}
		}
	}
	return m, nil
}

// updateWizard routes a message to the embedded pack wizard child model.
// It closes the wizard when the done screen is dismissed or when Esc is
// pressed on the first step (nowhere left to go back to).
func (m model) updateWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.packWizard.step == createStepDone {
			if m.packWizard.doneErr != nil {
				m.message = fmt.Sprintf("✗ %v", m.packWizard.doneErr)
			} else {
				m.message = "✓ " + m.packWizard.doneResult
			}
			m.packWizard = nil
			m.refreshPacks()
			return m, nil
		}
		if key.Type == tea.KeyEsc && m.packWizard.step == createStepName {
			m.packWizard = nil
			m.message = ""
			return m, nil
		}
	}
	wiz, cmd := m.packWizard.Update(msg)
	if w, ok := wiz.(packCreateModel); ok {
		m.packWizard = &w
	}
	return m, cmd
}

// panelIndex returns p's position in panelOrder.
func panelIndex(p panel) int {
	for i, q := range panelOrder {
		if q == p {
			return i
		}
	}
	return 0
}

// activeFilter returns the filter text of the active panel, or nil when the
// panel has no filter (Remove Packs, or Packs while its detail overlay hides
// the filter box).
func (m *model) activeFilter() *string {
	switch m.activePanel {
	case panelSkills:
		return &m.filter
	case panelStatus:
		return &m.statusFilter
	case panelRepos:
		return &m.repoFilter
	case panelUnmanaged:
		return &m.unmanagedFilter
	case panelPacks:
		if !m.packDetailOpen {
			return &m.packFilter
		}
	case panelDoctor:
		return &m.doctorFilter
	}
	return nil
}

// filterChanged re-derives the active panel's view after its filter text
// changed.
func (m *model) filterChanged() {
	switch m.activePanel {
	case panelStatus:
		m.rebuildStatusRows()
	case panelRepos:
		m.refreshRepos()
	case panelUnmanaged:
		m.refreshUnmanaged()
	case panelPacks:
		m.refreshPacks()
	case panelDoctor:
		m.doctorCursor = 0
	}
}

func (m *model) appendFilter(s string) {
	*m.activeFilter() += s
	m.filterChanged()
}

// backspaceFilter drops the last rune (not byte) of the active filter.
func (m *model) backspaceFilter() {
	f := m.activeFilter()
	if f == nil || *f == "" {
		return
	}
	_, n := utf8.DecodeLastRuneInString(*f)
	*f = (*f)[:len(*f)-n]
	m.filterChanged()
}
