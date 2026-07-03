package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// ─── cobra command ────────────────────────────────────────────────────────────

var packCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Interactively author and publish a new pack",
	Long: `Launch an interactive TUI wizard that guides you through naming a pack,
selecting skills to include, previewing the generated pack.yaml, and
publishing it to a registered repo.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isInteractive() {
			return fmt.Errorf("pack create requires an interactive terminal")
		}
		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}
		return runPackCreateWizard(app.Cfg, app.St)
	},
}

// ─── wizard step type ─────────────────────────────────────────────────────────

type createStep int

const (
	createStepName    createStep = iota // text input: pack name
	createStepDesc                      // text input: description (optional)
	createStepSkills                    // multi-select skills
	createStepPath                      // text input: destination path in repo
	createStepRepo                      // select target repo
	createStepPreview                   // show pack.yaml preview + confirm
	createStepDone                      // success or error display
)

// ─── wizard model ─────────────────────────────────────────────────────────────

type packCreateModel struct {
	step createStep

	// --- text input state ---
	nameInput string
	descInput string
	pathInput string // e.g. "packs/go-dev"

	// --- skill multi-select ---
	allSkills    []repo.SkillInfo
	skillSel     map[int]bool // allSkills index → selected
	skillCursor  int          // cursor within visibleSkills
	skillFilter  string
	visibleSkills []int // allSkills indices that pass the current filter

	// --- repo selection ---
	repoList   []repoEntry
	repoCursor int

	// --- preview ---
	previewYAML string

	// --- done ---
	doneErr    error
	doneResult string

	// --- display ---
	width  int
	height int

	// --- context ---
	cfg *config.Config
	st  *state.State
}

func initialPackCreateModel(cfg *config.Config, st *state.State) packCreateModel {
	m := packCreateModel{
		step:     createStepName,
		skillSel: make(map[int]bool),
		cfg:      cfg,
		st:       st,
		width:    80,
		height:   24,
	}

	// Collect all skills from registered repos.
	skills, _ := repo.DiscoverAllSkills(st)
	sort.Slice(skills, func(i, j int) bool { return skills[i].Address < skills[j].Address })
	m.allSkills = skills
	m.rebuildVisible()

	// Collect all registered repos.
	for name, rec := range st.Repos {
		m.repoList = append(m.repoList, repoEntry{name: name, url: rec.URL})
	}
	sort.Slice(m.repoList, func(i, j int) bool { return m.repoList[i].name < m.repoList[j].name })

	return m
}

// rebuildVisible recomputes visibleSkills to match the current skillFilter.
func (m *packCreateModel) rebuildVisible() {
	m.visibleSkills = m.visibleSkills[:0]
	f := strings.ToLower(m.skillFilter)
	for i, s := range m.allSkills {
		if f == "" || strings.Contains(strings.ToLower(s.Address), f) {
			m.visibleSkills = append(m.visibleSkills, i)
		}
	}
	if m.skillCursor >= len(m.visibleSkills) {
		m.skillCursor = max(0, len(m.visibleSkills)-1)
	}
}

// buildPackFromWizard constructs a Pack and its canonical repo list from the
// wizard's current state. It returns an error when validation fails.
func (m *packCreateModel) buildPackFromWizard() (*packYAML, error) {
	name := strings.TrimSpace(m.nameInput)
	if name == "" {
		return nil, fmt.Errorf("pack name is required")
	}

	var skillAddrs []string
	reposSeen := make(map[string]bool)
	for i, sel := range m.skillSel {
		if !sel {
			continue
		}
		s := m.allSkills[i]
		skillAddrs = append(skillAddrs, s.Address)
		reposSeen[s.RepoName] = true
	}
	if len(skillAddrs) == 0 {
		return nil, fmt.Errorf("at least one skill must be selected")
	}
	sort.Strings(skillAddrs)

	// Build the repos section from the skills' parent repos.
	// Error rather than silently omit: a missing repo would produce an invalid pack.yaml.
	var repos []packRepoRef
	for rn := range reposSeen {
		rec, ok := m.st.Repos[rn]
		if !ok {
			return nil, fmt.Errorf("selected skills include repo %q which is not registered in state", rn)
		}
		repos = append(repos, packRepoRef{Name: rn, URL: rec.URL})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })

	return &packYAML{
		Name:        name,
		Description: strings.TrimSpace(m.descInput),
		Repos:       repos,
		Skills:      skillAddrs,
	}, nil
}

// packRepoRef mirrors pack.RepoRef but lives here so we don't import pack just for YAML.
type packRepoRef struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// packYAML is the marshallable form of a pack.
type packYAML struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Repos       []packRepoRef `yaml:"repos"`
	Skills      []string      `yaml:"skills"`
}

// renderYAML serialises p to a YAML string.
func renderYAML(p *packYAML) (string, error) {
	data, err := yaml.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// defaultPackPath returns the default destination path for the pack.yaml inside
// the target repo (e.g. "packs/go-dev").
func defaultPackPath(name string) string {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return "packs/" + slug
}

// ─── bubbletea Init / Update / View ──────────────────────────────────────────

func (m packCreateModel) Init() tea.Cmd { return nil }

// packCreateDoneMsg is fired when the commit+push goroutine finishes.
type packCreateDoneMsg struct {
	result string
	err    error
}

func (m packCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case packCreateDoneMsg:
		m.step = createStepDone
		m.doneErr = msg.err
		m.doneResult = msg.result
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case createStepName:
			next, val, cmd := m.updateTextInput(msg, m.nameInput, createStepDesc, -1)
			next.nameInput = val
			return next, cmd
		case createStepDesc:
			next, val, cmd := m.updateTextInput(msg, m.descInput, createStepSkills, createStepName)
			next.descInput = val
			return next, cmd
		case createStepSkills:
			return m.updateSkillSelect(msg)
		case createStepPath:
			next, val, cmd := m.updateTextInput(msg, m.pathInput, createStepRepo, createStepSkills)
			next.pathInput = val
			return next, cmd
		case createStepRepo:
			return m.updateRepoSelect(msg)
		case createStepPreview:
			return m.updatePreview(msg)
		case createStepDone:
			return m, tea.Quit
		}
	}
	return m, nil
}

// updateTextInput handles key events when the model is collecting a text field.
// fieldVal is the current value of the field being edited; the updated value is
// returned as the second element so callers can assign it back to the correct
// struct field. nextStep is the step to advance to on Enter; prevStep is
// returned to on Esc (pass -1 to disable Esc going back).
//
// Design note: packCreateModel uses value receivers throughout (required by
// Bubble Tea's tea.Model interface). Passing field *string and modifying through
// the pointer writes into the *caller's* copy but returns a *different* copy as
// the new model — the typed character is then lost. Passing by value and
// returning the new value avoids this.
func (m packCreateModel) updateTextInput(
	msg tea.KeyMsg, fieldVal string, nextStep, prevStep createStep,
) (packCreateModel, string, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, fieldVal, tea.Quit
	case tea.KeyEsc:
		if prevStep >= 0 {
			m.step = prevStep
		}
	case tea.KeyBackspace, tea.KeyDelete:
		// Trim the last rune (not byte) to correctly handle multi-byte UTF-8 characters.
		runes := []rune(fieldVal)
		if len(runes) > 0 {
			fieldVal = string(runes[:len(runes)-1])
		}
	case tea.KeyEnter:
		// Validate name when advancing from name step.
		if m.step == createStepName && strings.TrimSpace(fieldVal) == "" {
			return m, fieldVal, nil
		}
		if nextStep == createStepPath && strings.TrimSpace(m.pathInput) == "" {
			m.pathInput = defaultPackPath(m.nameInput)
		}
		m.step = nextStep
		// After advancing to path, seed the default if still empty.
		if m.step == createStepPath && strings.TrimSpace(m.pathInput) == "" {
			m.pathInput = defaultPackPath(m.nameInput)
		}
	case tea.KeyRunes:
		fieldVal += msg.String()
	case tea.KeySpace:
		fieldVal += " "
	}
	return m, fieldVal, nil
}

// updateSkillSelect handles key events during the skill multi-select step.
func (m packCreateModel) updateSkillSelect(msg tea.KeyMsg) (packCreateModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.step = createStepDesc
	case tea.KeyUp:
		if m.skillCursor > 0 {
			m.skillCursor--
		}
	case tea.KeyDown:
		if m.skillCursor < len(m.visibleSkills)-1 {
			m.skillCursor++
		}
	case tea.KeySpace:
		if m.skillCursor < len(m.visibleSkills) {
			idx := m.visibleSkills[m.skillCursor]
			m.skillSel[idx] = !m.skillSel[idx]
		}
	case tea.KeyEnter:
		// Require at least one skill selected.
		count := 0
		for _, sel := range m.skillSel {
			if sel {
				count++
			}
		}
		if count == 0 {
			return m, nil
		}
		// Advance to path step; seed default path if empty.
		if strings.TrimSpace(m.pathInput) == "" {
			m.pathInput = defaultPackPath(m.nameInput)
		}
		m.step = createStepPath
	case tea.KeyBackspace:
		if len(m.skillFilter) > 0 {
			m.skillFilter = m.skillFilter[:len(m.skillFilter)-1]
			m.rebuildVisible()
		}
	case tea.KeyRunes:
		m.skillFilter += msg.String()
		m.rebuildVisible()
	}
	return m, nil
}

// updateRepoSelect handles key events during repo selection.
func (m packCreateModel) updateRepoSelect(msg tea.KeyMsg) (packCreateModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.step = createStepPath
	case tea.KeyUp:
		if m.repoCursor > 0 {
			m.repoCursor--
		}
	case tea.KeyDown:
		if m.repoCursor < len(m.repoList)-1 {
			m.repoCursor++
		}
	case tea.KeyEnter:
		if len(m.repoList) == 0 {
			return m, nil
		}
		// Build preview YAML.
		p, err := m.buildPackFromWizard()
		if err != nil {
			m.doneErr = err
			m.step = createStepDone
			return m, nil
		}
		yml, err := renderYAML(p)
		if err != nil {
			m.doneErr = err
			m.step = createStepDone
			return m, nil
		}
		m.previewYAML = yml
		m.step = createStepPreview
	}
	return m, nil
}

// updatePreview handles key events during the pack.yaml preview + confirm step.
func (m packCreateModel) updatePreview(msg tea.KeyMsg) (packCreateModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.step = createStepRepo
	case tea.KeyEnter:
		// Kick off the async commit+push.
		return m, m.cmdCommitAndPush()
	}
	return m, nil
}

// cmdCommitAndPush creates the async Tea command that writes pack.yaml,
// commits, and pushes to the selected repo.
func (m packCreateModel) cmdCommitAndPush() tea.Cmd {
	cfg := m.cfg
	st := m.st
	selectedRepo := m.repoList[m.repoCursor]
	packPath := strings.TrimSpace(m.pathInput)
	if packPath == "" {
		packPath = defaultPackPath(m.nameInput)
	}
	previewYAML := m.previewYAML

	return func() tea.Msg {
		rec, ok := st.Repos[selectedRepo.name]
		if !ok {
			return packCreateDoneMsg{err: fmt.Errorf("repo %q not found in state", selectedRepo.name)}
		}

		// Validate repo is writable (has a remote URL).
		if rec.URL == "" {
			return packCreateDoneMsg{err: fmt.Errorf("repo %q has no remote URL — cannot push", selectedRepo.name)}
		}

		// Sanitize and validate packPath before any filesystem access.
		cleanPath, pathErr := sanitizePackPath(packPath)
		if pathErr != nil {
			return packCreateDoneMsg{err: pathErr}
		}

		// Write pack.yaml to the repo cache. cleanPath uses forward slashes;
		// convert to OS-native separators only for filesystem operations.
		packDir := filepath.Join(rec.CachePath, filepath.FromSlash(cleanPath))
		if err := os.MkdirAll(packDir, 0755); err != nil {
			return packCreateDoneMsg{err: fmt.Errorf("creating pack directory: %w", err)}
		}
		packFile := filepath.Join(packDir, "pack.yaml")
		if err := os.WriteFile(packFile, []byte(previewYAML), 0644); err != nil {
			return packCreateDoneMsg{err: fmt.Errorf("writing pack.yaml: %w", err)}
		}

		// Commit and push. Pass cleanPath (forward-slash) as the git rel-path.
		token := cfg.TokenForRepo(selectedRepo.name)
		name := strings.TrimSpace(m.nameInput)
		commitMsg := fmt.Sprintf("feat: add pack %q", name)
		result, err := gitops.CommitAndPush(rec.CachePath, cleanPath, commitMsg, rec.URL, token)
		if err != nil {
			return packCreateDoneMsg{err: fmt.Errorf("publishing pack: %w", err)}
		}

		packAddr := selectedRepo.name + "/" + cleanPath
		if !result.Committed {
			summary := fmt.Sprintf("Pack %q already up-to-date at %s (no changes to commit)", name, packAddr)
			return packCreateDoneMsg{result: summary}
		}
		summary := fmt.Sprintf("Pack %q published to %s (commit %s)", name, packAddr, result.CommitHash[:8])
		return packCreateDoneMsg{result: summary}
	}
}

// ─── View ─────────────────────────────────────────────────────────────────────

var (
	createTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	createPromptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	createHelpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	createInputStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	createSelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	createCheckStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	createErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	createOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	createCodeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
)

func (m packCreateModel) View() string {
	var b strings.Builder

	header := createTitleStyle.Render(" SkillPack — Create Pack")
	b.WriteString(header + "\n\n")

	switch m.step {
	case createStepName:
		b.WriteString(createPromptStyle.Render("Step 1/6 — Pack name") + "\n\n")
		b.WriteString("  Name: " + createInputStyle.Render(m.nameInput) + "█\n\n")
		b.WriteString(createHelpStyle.Render("  enter=next  ctrl+c=quit") + "\n")

	case createStepDesc:
		b.WriteString(createPromptStyle.Render("Step 2/6 — Description (optional)") + "\n\n")
		b.WriteString("  Name: " + createInputStyle.Render(m.nameInput) + "\n")
		cursor := "█"
		if m.descInput == "" {
			cursor = "█"
		}
		b.WriteString("  Desc: " + createInputStyle.Render(m.descInput) + cursor + "\n\n")
		b.WriteString(createHelpStyle.Render("  enter=next  esc=back  ctrl+c=quit") + "\n")

	case createStepSkills:
		b.WriteString(createPromptStyle.Render("Step 3/6 — Select skills") + "\n")
		count := 0
		for _, sel := range m.skillSel {
			if sel {
				count++
			}
		}
		b.WriteString(fmt.Sprintf("  %d selected", count))
		if len(m.allSkills) == 0 {
			b.WriteString("\n\n  " + createHelpStyle.Render("(no skills — register a repo first)") + "\n")
			break
		}
		b.WriteString("  filter: " + createInputStyle.Render(m.skillFilter) + "█\n\n")

		// Show a scrollable window of visible skills.
		listH := m.height - 10
		if listH < 5 {
			listH = 5
		}
		start := 0
		if m.skillCursor >= listH {
			start = m.skillCursor - listH + 1
		}
		end := start + listH
		if end > len(m.visibleSkills) {
			end = len(m.visibleSkills)
		}
		for i := start; i < end; i++ {
			idx := m.visibleSkills[i]
			s := m.allSkills[idx]
			check := "[ ]"
			if m.skillSel[idx] {
				check = createCheckStyle.Render("[✓]")
			}
			line := fmt.Sprintf("  %s %s", check, s.Address)
			if i == m.skillCursor {
				line = createSelectedStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
		if len(m.visibleSkills) == 0 {
			b.WriteString("  " + createHelpStyle.Render("(no skills match filter)") + "\n")
		}
		b.WriteString("\n" + createHelpStyle.Render("  ↑↓=navigate  space=toggle  type=filter  backspace=delete char  enter=next  esc=back") + "\n")

	case createStepPath:
		b.WriteString(createPromptStyle.Render("Step 4/6 — Pack directory path in repo") + "\n\n")
		b.WriteString("  Path: " + createInputStyle.Render(m.pathInput) + "█\n\n")
		b.WriteString(createHelpStyle.Render("  The pack.yaml will be written at <repo>/<path>/pack.yaml") + "\n")
		b.WriteString(createHelpStyle.Render("  enter=next  esc=back  ctrl+c=quit") + "\n")

	case createStepRepo:
		b.WriteString(createPromptStyle.Render("Step 5/6 — Select target repo") + "\n\n")
		if len(m.repoList) == 0 {
			b.WriteString("  " + createErrorStyle.Render("No repos registered. Run: skillpack repo add <name> <url>") + "\n")
			b.WriteString(createHelpStyle.Render("  esc=back  ctrl+c=quit") + "\n")
			break
		}
		for i, r := range m.repoList {
			line := fmt.Sprintf("  %s  %s", r.name, createHelpStyle.Render(r.url))
			if i == m.repoCursor {
				line = createSelectedStyle.Render(fmt.Sprintf("  %s  %s", r.name, r.url))
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + createHelpStyle.Render("  ↑↓=navigate  enter=select  esc=back  ctrl+c=quit") + "\n")

	case createStepPreview:
		b.WriteString(createPromptStyle.Render("Step 6/6 — Preview") + "\n\n")
		selectedRepo := ""
		if m.repoCursor < len(m.repoList) {
			selectedRepo = m.repoList[m.repoCursor].name
		}
		packPath := strings.TrimSpace(m.pathInput)
		if packPath == "" {
			packPath = defaultPackPath(m.nameInput)
		}
		b.WriteString(fmt.Sprintf("  Will write: %s/%s/pack.yaml\n\n", selectedRepo, packPath))
		for _, line := range strings.Split(strings.TrimSuffix(m.previewYAML, "\n"), "\n") {
			b.WriteString("  " + createCodeStyle.Render(line) + "\n")
		}
		b.WriteString("\n" + createHelpStyle.Render("  enter=publish  esc=back  ctrl+c=quit") + "\n")

	case createStepDone:
		if m.doneErr != nil {
			b.WriteString(createErrorStyle.Render("  ✗ "+m.doneErr.Error()) + "\n\n")
		} else {
			b.WriteString(createOKStyle.Render("  ✓ "+m.doneResult) + "\n\n")
			packPath := strings.TrimSpace(m.pathInput)
			if packPath == "" {
				packPath = defaultPackPath(m.nameInput)
			}
			if m.repoCursor < len(m.repoList) {
				packAddr := m.repoList[m.repoCursor].name + "/" + strings.TrimPrefix(packPath, "/")
				b.WriteString(fmt.Sprintf("  Browse with: skillpack pack list --available\n"))
				b.WriteString(fmt.Sprintf("  Install with: skillpack pack install %s\n", packAddr))
			}
		}
		b.WriteString("\n" + createHelpStyle.Render("  any key to exit") + "\n")
	}

	return b.String()
}

// ─── entry point ──────────────────────────────────────────────────────────────

func runPackCreateWizard(cfg *config.Config, st *state.State) error {
	m := initialPackCreateModel(cfg, st)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// sanitizePackPath cleans and validates a relative pack path for use inside a
// repo cache. It rejects absolute paths, ".." traversal components, and empty
// inputs. The returned path always uses forward slashes (suitable for git
// operations and CommitAndPush).
func sanitizePackPath(raw string) (string, error) {
	cleaned := path.Clean(filepath.ToSlash(strings.TrimSpace(raw)))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("pack path cannot be empty")
	}
	if path.IsAbs(cleaned) {
		return "", fmt.Errorf("pack path must be relative (got absolute path %q)", cleaned)
	}
	if strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("pack path must not escape the repo root (got %q)", cleaned)
	}
	return cleaned, nil
}

// ValidatePackCreate validates the inputs for a pack create operation without
// running the TUI. Used in tests.
func ValidatePackCreate(name string, skillCount int, repoCount int) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("pack name is required")
	}
	if skillCount == 0 {
		return fmt.Errorf("at least one skill must be selected")
	}
	if repoCount == 0 {
		return fmt.Errorf("target repo must be registered")
	}
	return nil
}
