package main

import (
	"fmt"
	"path/filepath"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Core Dispatcher (extracted in Phase 4) ---
// The Update method is the heart of the Bubble Tea event loop.
// It handles all incoming messages (key events, async results from cmd* factories,
// window size changes, etc.) and decides what to render and what commands to run next.
// handleInputMode (the giant mode-specific key handler) was moved to tui_handlers.go in Phase 3.

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
