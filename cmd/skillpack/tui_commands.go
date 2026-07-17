package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// --- Async Command Factories (extracted in Phase 4) ---
// These functions create tea.Cmd values that perform I/O-heavy work
// (git operations, status checks, sync, self-update, LLM fork registration, etc.)
// in background goroutines and send *Msg results back to the Update loop.
// They deliberately take deep copies of state (via cloneState) to avoid races.

func (m *model) doAddAgent(name, skillDir string) {
	if err := config.AddAgent(m.cfg, name, skillDir); err != nil {
		m.message = fmt.Sprintf("✗ Add agent failed: %v", err)
		return
	}
	m.refreshAgents()
	m.message = fmt.Sprintf("➕ Added agent %s → %s", name, skillDir)
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

func (m *model) cmdRegisterForkProvenance(addr, upstream string) tea.Cmd {
	cfg := m.cfg
	token := cfg.TokenForRepo(repoNameFromAddr(addr))
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		err := skill.RegisterForkProvenance(addr, upstream, token, stCopy)
		if err != nil {
			return registerForkDoneMsg{addr: addr, upstream: upstream, err: err}
		}
		return registerForkDoneMsg{addr: addr, upstream: upstream, st: stCopy}
	}
}

func (m *model) cmdRelink(oldAddr, newAddr, agentName string) tea.Cmd {
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		err := skill.Relink(oldAddr, newAddr, agentName, false, stCopy)
		if err != nil {
			return relinkDoneMsg{oldAddr: oldAddr, newAddr: newAddr, agent: agentName, err: err}
		}
		return relinkDoneMsg{oldAddr: oldAddr, newAddr: newAddr, agent: agentName, st: stCopy}
	}
}

func (m *model) cmdRelinkUpstream(addr, newUpstreamAddr, agentName string) tea.Cmd {
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		err := skill.RelinkUpstream(addr, newUpstreamAddr, agentName, stCopy)
		if err != nil {
			return relinkUpstreamDoneMsg{addr: addr, newUpstream: newUpstreamAddr, agent: agentName, err: err}
		}
		return relinkUpstreamDoneMsg{addr: addr, newUpstream: newUpstreamAddr, agent: agentName, st: stCopy}
	}
}

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
			_, _ = repo.Update(name, cfg.TokenForRepo(name), stCopy)
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

		// Detect skills with missing fork provenance. Only flag skills in repos
		// the user can write to so upstream read-only skills are excluded.
		canWrite := func(name string) bool {
			rec, ok := stCopy.Repos[name]
			if !ok {
				return false
			}
			return strings.HasPrefix(rec.URL, "git@") || strings.HasPrefix(rec.URL, "ssh://") || cfg.TokenForRepo(name) != ""
		}
		return statusDoneMsg{info: info, forkCandidates: skill.ForkCandidateMap(stCopy, canWrite)}
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
			parts = append(parts, fmt.Sprintf("%d pushed", published))
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

		return selfUpdateDoneMsg{
			summary:      fmt.Sprintf("✓ Updated: v%s → %s", current, latest),
			needsRestart: true,
		}
	}
}

// cmdCompleteDeployment installs all missing skills in a partial pack.
// It mirrors the logic of packUpdateCmd but targets only skills marked not-installed.
func (m *model) cmdCompleteDeployment(packAddr string) tea.Cmd {
	cfg := m.cfg
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		rec, ok := stCopy.InstalledPacks[packAddr]
		if !ok {
			return packCompleteDoneMsg{
				packAddr: packAddr,
				err:      fmt.Errorf("pack %q not found in state", packAddr),
			}
		}

		installed := 0
		failed := 0
		for skillAddr, agStatuses := range rec.Skills {
			for ag, agStatus := range agStatuses {
				if agStatus.Installed {
					continue // already installed
				}
				installErr := skill.Install(skillAddr, ag, cfg, stCopy, false)
				if installErr != nil {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{
						Installed: false,
						Error:     installErr.Error(),
					}
					failed++
				} else {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{Installed: true}
					installed++
				}
			}
		}

		if err := stCopy.RecordPackInstall(packAddr, rec); err != nil {
			return packCompleteDoneMsg{packAddr: packAddr, err: err}
		}
		if err := state.Save(stCopy); err != nil {
			return packCompleteDoneMsg{packAddr: packAddr, err: err}
		}

		var summary string
		switch {
		case installed > 0 && failed == 0:
			summary = fmt.Sprintf("✓ Pack %q complete — %d skill(s) installed", packAddr, installed)
		case installed > 0 && failed > 0:
			summary = fmt.Sprintf("⚠ Pack %q still partial — %d installed, %d failed", packAddr, installed, failed)
		case installed == 0 && failed == 0:
			summary = fmt.Sprintf("✓ Pack %q already fully deployed", packAddr)
		default:
			summary = fmt.Sprintf("✗ Pack %q — all %d skill(s) failed to install", packAddr, failed)
		}
		return packCompleteDoneMsg{packAddr: packAddr, st: stCopy, summary: summary}
	}
}

// cmdPackInstall installs every skill in the pack for the given agents.
// It mirrors runPackInstall (pack.go) but stays quiet — progress is reported
// back to the Update loop via packInstallDoneMsg instead of stdout.
func (m *model) cmdPackInstall(packAddr string, agents []string) tea.Cmd {
	cfg := m.cfg
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		pk, canonAddr, err := loadPackDefinition(packAddr, cfg, stCopy)
		if err != nil {
			return packInstallDoneMsg{packAddr: packAddr, err: err}
		}

		// Register any repos the pack references that aren't registered yet.
		repoErrors := make(map[string]error)
		for _, r := range pk.Repos {
			if _, exists := stCopy.Repos[r.Name]; exists {
				continue
			}
			if _, addErr := repo.Add(r.Name, r.URL, cfg.TokenForRepo(r.Name), stCopy); addErr != nil {
				repoErrors[r.Name] = addErr
			}
		}

		rec := state.InstalledPackRecord{
			PackAddress: canonAddr,
			InstalledAt: time.Now(),
			Agents:      agents,
			Skills:      make(map[string]map[string]state.PackSkillStatus),
		}

		installed, failed := 0, 0
		for _, skillAddr := range pk.Skills {
			rec.Skills[skillAddr] = make(map[string]state.PackSkillStatus)
			repoName := repoNameFromAddr(skillAddr)
			for _, ag := range agents {
				if repoErr, bad := repoErrors[repoName]; bad {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{
						Installed: false,
						Error:     fmt.Sprintf("repo unavailable: %v", repoErr),
					}
					failed++
					continue
				}
				if installErr := skill.Install(skillAddr, ag, cfg, stCopy, false); installErr != nil {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{Installed: false, Error: installErr.Error()}
					failed++
				} else {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{Installed: true}
					installed++
				}
			}
		}

		if err := stCopy.RecordPackInstall(canonAddr, rec); err != nil {
			return packInstallDoneMsg{packAddr: canonAddr, err: err}
		}
		if err := state.Save(stCopy); err != nil {
			return packInstallDoneMsg{packAddr: canonAddr, err: err}
		}

		// installed/failed count skill×agent installs, not unique skills.
		var summary string
		if failed > 0 {
			summary = fmt.Sprintf("⚠ Pack %q installed partial — %d install(s) succeeded, %d failed (Enter for details)", canonAddr, installed, failed)
		} else {
			summary = fmt.Sprintf("✓ Pack %q installed complete — %d install(s) for %s", canonAddr, installed, strings.Join(agents, ", "))
		}
		return packInstallDoneMsg{packAddr: canonAddr, st: stCopy, summary: summary}
	}
}
