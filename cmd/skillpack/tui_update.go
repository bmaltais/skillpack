package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Core Dispatcher (extracted in Phase 4) ---
// The Update method is the heart of the Bubble Tea event loop.
// It handles all incoming messages (key events, async results from cmd* factories,
// window size changes, etc.) and decides what to render and what commands to run next.
// handleInputMode (the giant mode-specific key handler) was moved to tui_handlers.go in Phase 3.

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				m.activePanel = panelPacks
				m.refreshPacks()
			case panelPacks:
				m.activePanel = panelDoctor
				m.doctorScroll = 0
			case panelDoctor:
				m.activePanel = panelSkills
			}
			m.message = ""
			m.filterActive = false
			if m.activePanel == panelDoctor {
				if err := m.refreshDoctor(); err != nil {
					m.message = fmt.Sprintf("✗ %v", err)
				}
			}
		case tea.KeyEsc:
			switch {
			case m.activePanel == panelUnmanaged:
				m.unmanagedFilter = ""
				m.filterActive = false
			case m.activePanel == panelPacks && m.packDetailOpen:
				m.packDetailOpen = false
				m.message = ""
			case m.activePanel == panelPacks && (m.packFilter != "" || m.filterActive):
				m.packFilter = ""
				m.filterActive = false
				m.refreshPacks()
			case m.activePanel == panelStatus && (m.statusFilter != "" || m.filterActive):
				m.statusFilter = ""
				m.filterActive = false
				m.rebuildStatusRows()
			case m.activePanel == panelRepos && (m.repoFilter != "" || m.filterActive):
				m.repoFilter = ""
				m.filterActive = false
				m.refreshRepos()
			case m.activePanel == panelDoctor && (m.doctorFilter != "" || m.filterActive):
				m.doctorFilter = ""
				m.filterActive = false
				m.doctorScroll = 0
			case m.filter != "" || m.filterActive:
				m.filter = ""
				m.filterActive = false
			default:
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
				if m.filterActive {
					m.filter += " "
				} else {
					m.handleEnter()
				}
			}
			if m.activePanel == panelUnmanaged && m.filterActive {
				m.unmanagedFilter += " "
				m.refreshUnmanaged()
			}
			if m.activePanel == panelStatus && m.filterActive {
				m.statusFilter += " "
				m.rebuildStatusRows()
			}
			if m.activePanel == panelRepos && m.filterActive {
				m.repoFilter += " "
				m.refreshRepos()
			}
			if m.activePanel == panelPacks && m.filterActive {
				m.packFilter += " "
				m.refreshPacks()
			}
			if m.activePanel == panelDoctor && m.filterActive {
				m.doctorFilter += " "
				m.doctorScroll = 0
			}
		case tea.KeyDelete:
			if m.activePanel == panelRepos {
				m.startRemoveRepo()
			}
		case tea.KeyBackspace:
			if !m.filterActive {
				break
			}
			if m.activePanel == panelSkills && len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
			if m.activePanel == panelUnmanaged && len(m.unmanagedFilter) > 0 {
				m.unmanagedFilter = m.unmanagedFilter[:len(m.unmanagedFilter)-1]
				m.refreshUnmanaged()
			}
			if m.activePanel == panelStatus && len(m.statusFilter) > 0 {
				m.statusFilter = m.statusFilter[:len(m.statusFilter)-1]
				m.rebuildStatusRows()
			}
			if m.activePanel == panelRepos && len(m.repoFilter) > 0 {
				m.repoFilter = m.repoFilter[:len(m.repoFilter)-1]
				m.refreshRepos()
			}
			if m.activePanel == panelPacks && len(m.packFilter) > 0 {
				m.packFilter = m.packFilter[:len(m.packFilter)-1]
				m.refreshPacks()
			}
			if m.activePanel == panelDoctor && len(m.doctorFilter) > 0 {
				m.doctorFilter = m.doctorFilter[:len(m.doctorFilter)-1]
				m.doctorScroll = 0
			}
		case tea.KeyRunes:
			ch := msg.String()
			switch m.activePanel {
			case panelSkills:
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.filter += ch
					return m, nil
				}
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
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.statusFilter += ch
					m.rebuildStatusRows()
					return m, nil
				}
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
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.repoFilter += ch
					m.refreshRepos()
					return m, nil
				}
				switch ch {
				case "q":
					return m, tea.Quit
				case "a":
					m.startAddRepo()
				case "d":
					m.startRemoveRepo()
				}
			case panelUnmanaged:
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.unmanagedFilter += ch
					m.refreshUnmanaged()
					return m, nil
				}
				switch ch {
				case "q":
					return m, tea.Quit
				case "v":
					return m, m.startViewSkillMd()
				}
			case panelPacks:
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.packFilter += ch
					m.refreshPacks()
					return m, nil
				}
				switch ch {
				case "q":
					return m, tea.Quit
				case "n":
					m.startPackCreate()
				case "e":
					m.startPackEdit()
				case "i":
					// Install the selected available pack
					m.startPackInstall()
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
							m.message = "Pack is not installed — press i to install"
						}
					}
				case "d", "D":
					// Remove pack (with confirmation)
					m.startPackRemove()
				}
			case panelDoctor:
				if ch == "/" && !m.filterActive {
					m.filterActive = true
					return m, nil
				}
				if m.filterActive {
					m.doctorFilter += ch
					m.doctorScroll = 0
					return m, nil
				}
				switch ch {
				case "q":
					return m, tea.Quit
				case "r":
					m.doctorScroll = 0
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
