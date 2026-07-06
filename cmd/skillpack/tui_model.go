package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// --- Data types (moved in Phase 2) ---

type rowKind int

const (
	repoRow rowKind = iota
	skillRow
)

// skillProblem describes the health state of an installed skill row.
type skillProblem int

const (
	problemNone           skillProblem = iota
	problemStale                       // skill's own path no longer exists upstream (SyncStaleAddress)
	problemBrokenUpstream              // fork with an UpstreamAddr that no longer exists
)

type tuiRow struct {
	kind     rowKind
	repoName string
	addr     string       // full skill address (only for skillRow)
	relPath  string       // repo-relative path (only for skillRow)
	expanded bool         // only for repoRow
	problem  skillProblem // only for skillRow
}

// --- Panels ---

type panel int

const (
	panelSkills panel = iota
	panelRepos
	panelStatus
	panelUnmanaged
	panelPacks
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
	modeRelinkStaleInput     // stale repair: user types (or selects) a replacement address
	modeRelinkBrokenChoice   // broken-upstream repair: choose set-upstream (1) or clear (2)
	modeRelinkBrokenSetInput // broken-upstream repair: user types new upstream address
	modePackConfirmRemove    // packs panel: confirm pack removal
	modePackInstallAgents    // packs panel: select agents to install a pack for
	modeHelp                 // Help→Keys / F1: scrollable key-binding reference
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

	// DOS Shell menu bar state (F10 / Alt+letter to open, wired in Move 4)
	menuOpen      bool
	menuIndex     int // which top-level menu in appMenus is open
	menuItemIndex int // cursor within the open menu's items
	helpScroll    int // scroll offset for the F1 help dialog

	// Input mode for repo add / fork
	inputMode      inputMode
	inputBuffer    string
	newRepoName    string
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

	// Relink flow state (stale and broken-upstream repair)
	relinkAddr            string   // skill address being relinked
	relinkAgentName       string   // agent for the relink operation
	relinkCandidates      []string // SuggestReplacements candidates (stale flow)
	relinkCandidateCursor int      // cursor within relinkCandidates list
	relinkInput           string   // current input buffer for address entry
	relinkCandidateMode   bool     // true = candidate list shown; false = free-text input

	// Unmanaged panel
	unmanagedEntries []unmanagedEntry
	unmanagedCursor  int
	unmanagedFilter  string // incremental filter for the unmanaged panel
	adoptCursor      int    // cursor for repo selection in adopt flow

	// Packs panel
	packRows       []packRow
	packCursor     int
	packDetailOpen bool       // true when showing per-skill detail overlay for selected pack
	packDetailDef  *pack.Pack // parsed pack.yaml for an available pack's detail overlay, loaded once on open
	packDetailErr  error      // load error for the detail overlay, if any

	// packWizard, when non-nil, is the embedded create/edit wizard child model.
	// It receives all key/window messages and is rendered in place of the
	// active panel until it finishes or is cancelled.
	packWizard *packCreateModel

	// Pack install flow (agent multi-select overlay)
	packInstallAddr string       // pack address being installed
	packAgentCursor int          // cursor within m.agents
	packAgentSel    map[int]bool // m.agents index → selected

	// Config/state refs
	cfg *config.Config
	st  *state.State

	// restartPending is set after a successful self-update that replaced the binary.
	// runTUI detects it after the program exits and re-execs the new binary.
	restartPending bool
}

type repoEntry struct {
	name string
	url  string
}

// packRow holds display data for one row in the packs panel. The panel lists
// both installed packs (from state) and available packs discovered in
// registered repo caches.
type packRow struct {
	packAddr  string
	installed bool
	isPartial bool     // only meaningful when installed
	agents    []string // only set when installed
	desc      string   // pack.yaml description (available packs)
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
	m.refreshPacks()

	return m
}

func (m *model) refreshSkills() {
	m.rows = nil

	// Discover skills from registered repos
	allSkills, _ := repo.DiscoverAllSkills(m.st)

	// Build set of discovered addresses so we can detect stale installed skills
	discoveredAddrs := make(map[string]bool, len(allSkills))
	for _, s := range allSkills {
		discoveredAddrs[s.Address] = true
	}

	// Add stale installed skills: in InstalledSkills but no longer discoverable
	for addr := range m.st.InstalledSkills {
		if discoveredAddrs[addr] {
			continue
		}
		repoName := ""
		relPath := addr
		if parts := strings.SplitN(addr, "/", 2); len(parts) == 2 {
			repoName = parts[0]
			relPath = parts[1]
		}
		allSkills = append(allSkills, repo.SkillInfo{
			Address:  addr,
			RepoName: repoName,
			RelPath:  relPath,
		})
	}

	// Compute skill problems for all installed skills
	skillProblems := computeSkillProblems(m.st, discoveredAddrs)

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
			m.rows = append(m.rows, tuiRow{
				kind:     skillRow,
				repoName: rn,
				addr:     s.Address,
				relPath:  s.RelPath,
				problem:  skillProblems[s.Address],
			})
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

func (m *model) refreshPacks() {
	m.packRows = nil

	// Discover available packs from registered repo caches.
	rows := make(map[string]packRow)
	if available, err := repo.DiscoverAllPacks(m.st); err == nil {
		for _, p := range available {
			desc := ""
			if pk, perr := pack.ParseFile(filepath.Join(p.FullPath, "pack.yaml")); perr == nil {
				desc = pk.Description
			}
			rows[p.Address] = packRow{packAddr: p.Address, desc: desc}
		}
	}

	// Overlay installed packs (also covers packs installed from URL/local path
	// that are not discoverable in any repo cache).
	for addr, rec := range m.st.InstalledPacks {
		agents := make([]string, len(rec.Agents))
		copy(agents, rec.Agents)
		partial := false
		for _, agentStatuses := range rec.Skills {
			for _, s := range agentStatuses {
				if !s.Installed {
					partial = true
				}
			}
		}
		row := rows[addr] // keeps desc when discoverable
		row.packAddr = addr
		row.installed = true
		row.isPartial = partial
		row.agents = agents
		rows[addr] = row
	}

	addrs := make([]string, 0, len(rows))
	for addr := range rows {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	for _, addr := range addrs {
		m.packRows = append(m.packRows, rows[addr])
	}

	if m.packCursor >= len(m.packRows) {
		m.packCursor = 0
	}
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
			if !knownPaths[fullPath] && (m.unmanagedFilter == "" || unmanagedMatches(entry.Name(), agentName, fullPath, m.unmanagedFilter)) {
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

	// Sort by skill name for consistent presentation
	sort.SliceStable(m.unmanagedEntries, func(i, j int) bool {
		if m.unmanagedEntries[i].skillName != m.unmanagedEntries[j].skillName {
			return m.unmanagedEntries[i].skillName < m.unmanagedEntries[j].skillName
		}
		return m.unmanagedEntries[i].agentName < m.unmanagedEntries[j].agentName
	})
}

// unmanagedMatches returns true if the given skill entry matches the filter
// substring (case-insensitive) against skill name, agent name, or local path.
func unmanagedMatches(skillName, agentName, localPath, filter string) bool {
	lower := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(skillName), lower) ||
		strings.Contains(strings.ToLower(agentName), lower) ||
		strings.Contains(strings.ToLower(localPath), lower)
}

// computeSkillProblems classifies installed skills into health states.
// discoveredAddrs is the set of addresses found by repo.DiscoverAllSkills;
// addresses absent from this set are classified as problemStale.
// For forks (UpstreamAddr set), if the upstream skill cache path no longer
// contains a SKILL.md the skill is classified as problemBrokenUpstream.
// The function performs only local filesystem stat calls — no git or network I/O.
func computeSkillProblems(st *state.State, discoveredAddrs map[string]bool) map[string]skillProblem {
	problems := make(map[string]skillProblem)
	for addr, agents := range st.InstalledSkills {
		// If already flagged (e.g. from a previous loop iteration), skip.
		if problems[addr] != problemNone {
			continue
		}
		if !discoveredAddrs[addr] {
			// Installed but not discoverable → stale address.
			problems[addr] = problemStale
			continue
		}
		// Check for broken upstream pointer on forks.
		for _, rec := range agents {
			if rec.UpstreamAddr == "" {
				continue
			}
			upstreamParts := strings.SplitN(rec.UpstreamAddr, "/", 2)
			if len(upstreamParts) != 2 {
				continue
			}
			upstreamRepo, ok := st.Repos[upstreamParts[0]]
			if !ok {
				// Upstream repo not registered — not necessarily broken, just unknown.
				continue
			}
			// Only flag broken if the upstream repo is a real git clone
			// (avoids false positives on empty test fixtures).
			if _, gitErr := os.Stat(filepath.Join(upstreamRepo.CachePath, ".git")); gitErr != nil {
				continue
			}
			upstreamSkillPath := filepath.Join(upstreamRepo.CachePath, filepath.FromSlash(upstreamParts[1]))
			if _, statErr := os.Stat(filepath.Join(upstreamSkillPath, "SKILL.md")); os.IsNotExist(statErr) {
				problems[addr] = problemBrokenUpstream
			}
			break // one agent record is enough to determine fork status
		}
	}
	return problems
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

// --- Async message types (moved in Phase 2) ---

type statusDoneMsg struct {
	info           map[string]map[string]string
	forkCandidates map[string]string // addr → first candidate upstream addr
}

type syncDoneMsg struct {
	summary string
	st      *state.State // updated state after sync
}

type registerForkDoneMsg struct {
	addr     string
	upstream string
	st       *state.State // updated state on success; nil on error
	err      error
}

type relinkDoneMsg struct {
	oldAddr string
	newAddr string
	agent   string
	st      *state.State // updated state on success; nil on error
	err     error
}

type relinkUpstreamDoneMsg struct {
	addr        string
	newUpstream string // "" means clear
	agent       string
	st          *state.State // updated state on success; nil on error
	err         error
}

// packCompleteDoneMsg is sent when a "complete deployment" finishes.
type packCompleteDoneMsg struct {
	packAddr string
	st       *state.State // updated state on success
	summary  string
	err      error
}

// packInstallDoneMsg is sent when an async pack install finishes.
type packInstallDoneMsg struct {
	packAddr string
	st       *state.State // updated state on success
	summary  string
	err      error
}

type selfUpdateDoneMsg struct {
	summary      string
	needsRestart bool
}

type updateCheckMsg struct {
	latestTag string // empty if up-to-date or error
	err       error
}

type viewerExitMsg struct {
	err error
}

// --- Entry point (moved in Phase 2) ---

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
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := finalModel.(model); ok && fm.restartPending {
		if execErr := reexecSelf(); execErr != nil {
			fmt.Fprintf(os.Stderr, "skillpack: restart failed: %v — please restart manually\n", execErr)
		}
	}
	return nil
}

// cloneState creates a deep copy of State to avoid data races
// between async commands and UI rendering.
func cloneState(src *state.State) *state.State {
	dst := &state.State{
		Repos:           make(map[string]state.RepoRecord, len(src.Repos)),
		InstalledSkills: make(map[string]map[string]state.InstalledSkillRecord, len(src.InstalledSkills)),
		InstalledPacks:  make(map[string]state.InstalledPackRecord, len(src.InstalledPacks)),
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
	for packAddr, rec := range src.InstalledPacks {
		newRec := state.InstalledPackRecord{
			PackAddress: rec.PackAddress,
			InstalledAt: rec.InstalledAt,
			Agents:      append([]string{}, rec.Agents...),
			Skills:      make(map[string]map[string]state.PackSkillStatus, len(rec.Skills)),
		}
		for skillAddr, agStatuses := range rec.Skills {
			newRec.Skills[skillAddr] = make(map[string]state.PackSkillStatus, len(agStatuses))
			for agName, s := range agStatuses {
				newRec.Skills[skillAddr][agName] = s
			}
		}
		dst.InstalledPacks[packAddr] = newRec
	}
	return dst
}
