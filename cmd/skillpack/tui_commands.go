package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
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
		rep, err := skill.Sync(cfg.TokenForRepo, stCopy)
		if err != nil {
			return syncDoneMsg{summary: fmt.Sprintf("✗ Sync error: %v", err), st: stCopy}
		}
		sum := rep.Summary

		if sum.Updated > 0 || sum.Published > 0 {
			_ = state.Save(stCopy)
		}

		parts := []string{}
		if sum.Updated > 0 {
			parts = append(parts, fmt.Sprintf("%d updated", sum.Updated))
		}
		if sum.Published > 0 {
			parts = append(parts, fmt.Sprintf("%d pushed", sum.Published))
		}
		if sum.Current > 0 {
			parts = append(parts, fmt.Sprintf("%d current", sum.Current))
		}
		if sum.Conflicts > 0 {
			parts = append(parts, fmt.Sprintf("%d conflict(s)", sum.Conflicts))
		}
		if sum.Errors > 0 {
			parts = append(parts, fmt.Sprintf("%d error(s)", sum.Errors))
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
func (m *model) cmdCompleteDeployment(packAddr string) tea.Cmd {
	cfg := m.cfg
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		res, err := pack.Complete(cfg, stCopy, packAddr, pack.Options{})
		if err != nil {
			return packCompleteDoneMsg{packAddr: packAddr, err: err}
		}

		var summary string
		switch {
		case res.Installed > 0 && res.Failed == 0:
			summary = fmt.Sprintf("✓ Pack %q complete — %d skill(s) installed", packAddr, res.Installed)
		case res.Installed > 0 && res.Failed > 0:
			summary = fmt.Sprintf("⚠ Pack %q still partial — %d installed, %d failed", packAddr, res.Installed, res.Failed)
		case res.Installed == 0 && res.Failed == 0:
			summary = fmt.Sprintf("✓ Pack %q already fully deployed", packAddr)
		default:
			summary = fmt.Sprintf("✗ Pack %q — all %d skill(s) failed to install", packAddr, res.Failed)
		}
		return packCompleteDoneMsg{packAddr: packAddr, st: stCopy, summary: summary}
	}
}

// cmdPackInstall installs every skill in the pack for the given agents. It stays
// quiet: the outcome is reported back to the Update loop via packInstallDoneMsg.
func (m *model) cmdPackInstall(packAddr string, agents []string) tea.Cmd {
	cfg := m.cfg
	stCopy := cloneState(m.st)
	return func() tea.Msg {
		def, err := pack.Resolve(packAddr, stCopy)
		if err != nil {
			return packInstallDoneMsg{packAddr: packAddr, err: err}
		}
		res, err := pack.Install(cfg, stCopy, def, agents, pack.Options{})
		if err != nil {
			return packInstallDoneMsg{packAddr: def.Address, err: err}
		}

		// Installed/Failed count skill×agent installs, not unique skills.
		var summary string
		if res.Failed > 0 {
			summary = fmt.Sprintf("⚠ Pack %q installed partial — %d install(s) succeeded, %d failed (Enter for details)", def.Address, res.Installed, res.Failed)
		} else {
			summary = fmt.Sprintf("✓ Pack %q installed complete — %d install(s) for %s", def.Address, res.Installed, strings.Join(agents, ", "))
		}
		return packInstallDoneMsg{packAddr: def.Address, st: stCopy, summary: summary}
	}
}
