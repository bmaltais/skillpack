package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

var packCmd = &cobra.Command{
	Use:   "pack",
	Short: "Manage skill packs",
}

// ─── pack list ───────────────────────────────────────────────────────────────

var packListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed packs (or browse available packs with --available)",
	Example: `  skillpack pack list
  skillpack pack list --available
  skillpack pack list --available --repo my-repo`,
	RunE: func(cmd *cobra.Command, args []string) error {
		available, _ := cmd.Flags().GetBool("available")
		repoFilter, _ := cmd.Flags().GetString("repo")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		if available {
			return packListAvailable(repoFilter, app.St)
		}
		return packListInstalled(app.St)
	},
}

// packListInstalled prints all installed packs with their completion status.
// Produces empty output when no packs are installed (scriptable).
func packListInstalled(st *state.State) error {
	if len(st.InstalledPacks) == 0 {
		return nil
	}

	addrs := make([]string, 0, len(st.InstalledPacks))
	for addr := range st.InstalledPacks {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	for _, addr := range addrs {
		rec := st.InstalledPacks[addr]

		partial := isPackPartial(rec)
		status := green("complete")
		if partial {
			status = yellow("partial")
		}

		agents := strings.Join(rec.Agents, ", ")
		fmt.Printf("%-48s  [%s]  agents: %s\n", addr, status, agents)
	}
	return nil
}

// isPackPartial returns true when any skill in the pack failed to install for any agent.
func isPackPartial(rec state.InstalledPackRecord) bool {
	for _, agentStatuses := range rec.Skills {
		for _, s := range agentStatuses {
			if !s.Installed {
				return true
			}
		}
	}
	return false
}

// packListAvailable lists all packs discoverable from registered repos.
func packListAvailable(repoFilter string, st *state.State) error {
	var packs []repo.PackInfo
	var err error

	if repoFilter != "" {
		packs, err = repo.DiscoverPacks(repoFilter, st)
	} else {
		packs, err = repo.DiscoverAllPacks(st)
	}
	if err != nil {
		return err
	}

	if len(packs) == 0 {
		fmt.Println("No packs found. Register a repo with: skillpack repo add <name> <url>")
		return nil
	}

	sort.Slice(packs, func(i, j int) bool { return packs[i].Address < packs[j].Address })

	// Group packs by their parent path (repo + category prefix).
	type group struct {
		prefix string
		items  []repo.PackInfo
	}
	var groups []group
	curPrefix := ""
	for _, p := range packs {
		prefix := path.Dir(p.Address) // e.g. "my-repo/packs"
		if prefix != curPrefix {
			groups = append(groups, group{prefix: prefix})
			curPrefix = prefix
		}
		groups[len(groups)-1].items = append(groups[len(groups)-1].items, p)
	}

	total := 0
	for _, g := range groups {
		fmt.Printf("%s/\n", bold(g.prefix))
		for _, p := range g.items {
			installed := ""
			if _, ok := st.InstalledPacks[p.Address]; ok {
				installed = "  " + green("[installed]")
			}
			// Read pack.yaml to surface the description.
			desc := ""
			if pk, err := pack.ParseFile(filepath.Join(p.FullPath, "pack.yaml")); err == nil && pk.Description != "" {
				desc = "  " + pk.Description
			}
			fmt.Printf("  %-48s%s%s\n", p.Address, desc, installed)
			total++
		}
	}
	fmt.Printf("\n%s available\n", bold(fmt.Sprintf("%d pack(s)", total)))
	return nil
}

// ─── pack install ─────────────────────────────────────────────────────────────

var packInstallCmd = &cobra.Command{
	Use:   "install <address|url|filepath>",
	Short: "Install all skills in a pack",
	Example: `  skillpack pack install my-repo/packs/go-dev
  skillpack pack install https://raw.githubusercontent.com/user/repo/main/packs/go-dev/pack.yaml
  skillpack pack install /path/to/pack.yaml
  skillpack pack install my-repo/packs/go-dev --agent claude-code
  skillpack pack install my-repo/packs/go-dev --all-agents`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		agentName, _ := cmd.Flags().GetString("agent")
		allAgents, _ := cmd.Flags().GetBool("all-agents")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		return runPackInstall(addr, agentName, allAgents, app)
	},
}

// runPackInstall handles the pack install command logic.
func runPackInstall(addr, agentName string, allAgents bool, app *App) error {
	// 1. Load the pack definition.
	pk, packAddr, err := loadPackDefinition(addr, app.Cfg, app.St)
	if err != nil {
		return err
	}

	// 2. Determine target agents.
	agents, err := selectAgentsForPack(agentName, allAgents, app.Cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Installing pack %q for agents: %s\n", packAddr, strings.Join(agents, ", "))

	// 3. Ensure all repos referenced by the pack are registered, registering missing ones.
	repoErrors := ensurePackRepos(pk, app.Cfg, app.St)

	// 4. Build the pack record.
	rec := state.InstalledPackRecord{
		PackAddress: packAddr,
		InstalledAt: time.Now(),
		Agents:      agents,
		Skills:      make(map[string]map[string]state.PackSkillStatus),
	}

	// 5. Install each skill for each agent.
	for _, skillAddr := range pk.Skills {
		rec.Skills[skillAddr] = make(map[string]state.PackSkillStatus)

		// Check whether this skill's repo had a registration error.
		repoName := repoNameFromAddr(skillAddr)
		if repoErr, failed := repoErrors[repoName]; failed {
			for _, ag := range agents {
				rec.Skills[skillAddr][ag] = state.PackSkillStatus{
					Installed: false,
					Error:     fmt.Sprintf("repo unavailable: %v", repoErr),
				}
				fmt.Printf("  %s [%s]  skipped — repo %q unavailable: %v\n", skillAddr, ag, repoName, repoErr)
			}
			continue
		}

		for _, ag := range agents {
			fmt.Printf("  installing %s for %s ...\n", skillAddr, ag)
			installErr := skill.Install(skillAddr, ag, app.Cfg, app.St, false)
			if installErr != nil {
				rec.Skills[skillAddr][ag] = state.PackSkillStatus{
					Installed: false,
					Error:     installErr.Error(),
				}
				fmt.Printf("    %s\n", yellow("warning: "+installErr.Error()))
			} else {
				rec.Skills[skillAddr][ag] = state.PackSkillStatus{Installed: true}
				fmt.Printf("    installed\n")
			}
		}
	}

	// 6. Save the pack record.
	if err := app.St.RecordPackInstall(packAddr, rec); err != nil {
		return err
	}
	if err := state.Save(app.St); err != nil {
		return err
	}

	if isPackPartial(rec) {
		fmt.Printf("\nPack %q installed with %s — some skills could not be deployed.\n", packAddr, yellow("partial"))
		fmt.Println("  Run `skillpack pack status` for details.")
	} else {
		fmt.Printf("\nPack %q installed %s.\n", packAddr, green("complete"))
	}
	return nil
}

// loadPackDefinition resolves addr to a Pack and a canonical pack address.
// addr may be:
//   - a registered pack address (e.g. "my-repo/packs/go-dev")
//   - an HTTP/HTTPS URL to a raw pack.yaml file
//   - a local filepath to a pack.yaml file or directory containing one
func loadPackDefinition(addr string, cfg *config.Config, st *state.State) (*pack.Pack, string, error) {
	switch {
	case strings.HasPrefix(addr, "https://"):
		// Fetch raw pack.yaml content. Only HTTPS is accepted (see fetchURL).
		data, err := fetchURL(addr)
		if err != nil {
			return nil, "", fmt.Errorf("fetching pack.yaml from %s: %w", addr, err)
		}
		pk, err := pack.Parse(data)
		if err != nil {
			return nil, "", err
		}
		return pk, packAddrFromName(pk.Name), nil

	case isLocalPath(addr):
		// Local filepath.
		packFile := addr
		expanded, err := config.ExpandPath(addr)
		if err != nil {
			return nil, "", fmt.Errorf("expanding path %q: %w", addr, err)
		}
		info, statErr := os.Stat(expanded)
		if statErr != nil {
			return nil, "", fmt.Errorf("accessing %q: %w", expanded, statErr)
		}
		if info.IsDir() {
			packFile = filepath.Join(expanded, "pack.yaml")
		} else {
			packFile = expanded
		}
		pk, err := pack.ParseFile(packFile)
		if err != nil {
			return nil, "", err
		}
		return pk, packAddrFromName(pk.Name), nil

	default:
		// Registered pack address.
		packInfo, err := repo.FindPack(addr, st)
		if err != nil {
			return nil, "", err
		}
		pk, err := pack.ParseFile(filepath.Join(packInfo.FullPath, "pack.yaml"))
		if err != nil {
			return nil, "", err
		}
		return pk, addr, nil
	}
}

// isLocalPath returns true when addr looks like a filesystem path (cross-platform).
func isLocalPath(addr string) bool {
	return filepath.IsAbs(addr) ||
		strings.HasPrefix(addr, "./") ||
		strings.HasPrefix(addr, "../") ||
		strings.HasPrefix(addr, "~/")
}

// packAddrFromName builds a synthetic pack address from a pack name for URL/filepath installs.
func packAddrFromName(name string) string {
	// Normalise: lowercase, replace spaces with hyphens.
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// fetchURL downloads the content at rawURL and returns the bytes.
// Only HTTPS URLs are accepted to prevent MITM tampering of downloaded pack.yaml.
func fetchURL(rawURL string) ([]byte, error) {
	if !strings.HasPrefix(rawURL, "https://") {
		return nil, fmt.Errorf("only HTTPS URLs are supported for pack.yaml downloads (got %q)", rawURL)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(rawURL) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

// ensurePackRepos registers any repos listed in the pack that are not yet
// registered. Returns a map of repoName → error for repos that could not be added.
func ensurePackRepos(pk *pack.Pack, cfg *config.Config, st *state.State) map[string]error {
	errors := make(map[string]error)
	for _, r := range pk.Repos {
		if _, exists := st.Repos[r.Name]; exists {
			continue
		}
		fmt.Printf("  registering repo %q (%s) ...\n", r.Name, r.URL)
		token := cfg.TokenForRepo(r.Name)
		_, err := repo.Add(r.Name, r.URL, token, st)
		if err != nil {
			fmt.Printf("  %s\n", yellow(fmt.Sprintf("warning: could not register repo %q: %v", r.Name, err)))
			errors[r.Name] = err
		}
	}
	return errors
}

// selectAgentsForPack returns agents to install a pack for.
// In non-interactive mode it falls back to resolveAgents (default agent).
// In interactive mode, it prompts the user.
func selectAgentsForPack(agentName string, allAgents bool, cfg *config.Config) ([]string, error) {
	// Explicit flags take priority.
	if agentName != "" || allAgents {
		return resolveAgents(agentName, allAgents, cfg)
	}

	// Non-interactive: use default agent.
	if !isInteractive() {
		return resolveAgents("", false, cfg)
	}

	// Interactive: prompt for agent selection.
	names := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no agents configured; add one to ~/.skillpack/config.yaml")
	}
	if len(names) == 1 {
		fmt.Printf("  deploying to agent: %s\n", names[0])
		return names, nil
	}

	fmt.Println("Which agents should this pack be installed for?")
	for i, name := range names {
		fmt.Printf("  %d) %s\n", i+1, name)
	}
	fmt.Printf("  a) all\n")
	defaultIdx := 0
	for i, name := range names {
		if name == cfg.DefaultAgent {
			defaultIdx = i
			break
		}
	}
	fmt.Printf("Select [%d]: ", defaultIdx+1)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return []string{names[defaultIdx]}, nil
	}
	if strings.ToLower(input) == "a" {
		return names, nil
	}
	var n int
	if _, err := fmt.Sscanf(input, "%d", &n); err == nil && n >= 1 && n <= len(names) {
		return []string{names[n-1]}, nil
	}
	return []string{names[defaultIdx]}, nil
}

// ─── pack remove ──────────────────────────────────────────────────────────────

var packRemoveCmd = &cobra.Command{
	Use:   "remove <address>",
	Short: "Remove all skills installed by a pack",
	Example: `  skillpack pack remove my-repo/packs/go-dev
  skillpack pack remove my-repo/packs/go-dev --agent claude-code
  skillpack pack remove my-repo/packs/go-dev --all-agents`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		packAddr := args[0]
		agentName, _ := cmd.Flags().GetString("agent")
		allAgents, _ := cmd.Flags().GetBool("all-agents")
		force, _ := cmd.Flags().GetBool("force")

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		rec, ok := app.St.InstalledPacks[packAddr]
		if !ok {
			return fmt.Errorf("pack %q is not installed", packAddr)
		}

		agents, err := resolvePackAgents(agentName, allAgents, rec.Agents, app.Cfg)
		if err != nil {
			return err
		}

		fmt.Printf("Removing pack %q from agents: %s\n", packAddr, strings.Join(agents, ", "))

		for _, skillAddr := range skillsInPack(rec) {
			for _, ag := range agents {
				// Only remove if actually installed for this agent.
				if agSt, ok := rec.Skills[skillAddr]; ok {
					if agStatus, ok := agSt[ag]; !ok || !agStatus.Installed {
						continue
					}
				}
				is, err := skill.Open(skillAddr, ag, app.Cfg, app.St)
				if err != nil {
					fmt.Printf("  skipping %s [%s]: %v\n", skillAddr, ag, err)
					continue
				}
				fmt.Printf("  removing %s [%s] ...\n", skillAddr, ag)
				if err := is.Remove(force); err != nil {
					fmt.Printf("    %s\n", yellow("warning: "+err.Error()))
				} else {
					fmt.Printf("    removed\n")
				}
			}
		}

		// Remove the pack record entirely when all agents are removed;
		// update the agents list when only some are removed.
		remainingAgents := removeStrings(rec.Agents, agents)
		if len(remainingAgents) == 0 {
			if err := app.St.RecordPackRemove(packAddr); err != nil {
				return err
			}
			fmt.Printf("\nPack %q removed.\n", packAddr)
		} else {
			rec.Agents = remainingAgents
			// Prune skill statuses for removed agents.
			for skillAddr, agStatuses := range rec.Skills {
				for _, ag := range agents {
					delete(agStatuses, ag)
				}
				if len(agStatuses) == 0 {
					delete(rec.Skills, skillAddr)
				}
			}
			if err := app.St.RecordPackInstall(packAddr, rec); err != nil {
				return err
			}
			fmt.Printf("\nPack %q updated (removed from agents: %s; still installed for: %s).\n",
				packAddr, strings.Join(agents, ", "), strings.Join(remainingAgents, ", "))
		}
		return state.Save(app.St)
	},
}

// resolvePackAgents returns the agents to act on for a pack command.
// Falls back to the pack's installed agents when --all-agents is set with no other filter.
func resolvePackAgents(agentName string, allAgents bool, packAgents []string, cfg *config.Config) ([]string, error) {
	if allAgents {
		if len(packAgents) == 0 {
			return nil, fmt.Errorf("pack is not installed for any agent")
		}
		sorted := append([]string{}, packAgents...)
		sort.Strings(sorted)
		return sorted, nil
	}
	return resolveAgents(agentName, false, cfg)
}

// skillsInPack returns a sorted list of skill addresses in a pack record.
func skillsInPack(rec state.InstalledPackRecord) []string {
	addrs := make([]string, 0, len(rec.Skills))
	for addr := range rec.Skills {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	return addrs
}

// removeStrings returns a copy of slice with all elements in remove deleted.
func removeStrings(slice, remove []string) []string {
	removed := make(map[string]bool, len(remove))
	for _, s := range remove {
		removed[s] = true
	}
	var result []string
	for _, s := range slice {
		if !removed[s] {
			result = append(result, s)
		}
	}
	return result
}

// ─── pack update ──────────────────────────────────────────────────────────────

var packUpdateCmd = &cobra.Command{
	Use:   "update <address>",
	Short: "Pull latest repo content and reinstall changed skills in a pack",
	Example: `  skillpack pack update my-repo/packs/go-dev`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		packAddr := args[0]

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		rec, ok := app.St.InstalledPacks[packAddr]
		if !ok {
			return fmt.Errorf("pack %q is not installed", packAddr)
		}

		// Load the pack definition from the repo cache.
		pk, _, err := loadPackDefinition(packAddr, app.Cfg, app.St)
		if err != nil {
			return err
		}

		// Pull all repos referenced by the pack.
		fmt.Printf("Updating pack %q ...\n", packAddr)
		repoErrors := make(map[string]error)
		seen := make(map[string]bool)
		for _, r := range pk.Repos {
			if seen[r.Name] {
				continue
			}
			seen[r.Name] = true
			token := app.Cfg.TokenForRepo(r.Name)
			fmt.Printf("  pulling repo %q ...\n", r.Name)
			if warning, err := repo.Update(r.Name, token, app.St); err != nil {
				fmt.Printf("  %s\n", yellow(fmt.Sprintf("warning: %v", err)))
				repoErrors[r.Name] = err
			} else if warning != "" {
				fmt.Printf("  %s\n", yellow(warning))
			}
		}

		// Reinstall skills that have changed.
		changed := 0
		for _, skillAddr := range pk.Skills {
			repoName := repoNameFromAddr(skillAddr)
			// Ensure the per-skill inner map exists before any write.
			if rec.Skills[skillAddr] == nil {
				rec.Skills[skillAddr] = make(map[string]state.PackSkillStatus)
			}
			for _, ag := range rec.Agents {
				if repoErr, failed := repoErrors[repoName]; failed {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{
						Installed: false,
						Error:     fmt.Sprintf("repo unavailable: %v", repoErr),
					}
					continue
				}

				is, openErr := skill.Open(skillAddr, ag, app.Cfg, app.St)
				if openErr != nil {
					// Skill not installed for this agent — try to install it.
					fmt.Printf("  installing missing %s [%s] ...\n", skillAddr, ag)
					installErr := skill.Install(skillAddr, ag, app.Cfg, app.St, false)
					if installErr != nil {
						rec.Skills[skillAddr][ag] = state.PackSkillStatus{
							Installed: false,
							Error:     installErr.Error(),
						}
						fmt.Printf("    %s\n", yellow("warning: "+installErr.Error()))
					} else {
						rec.Skills[skillAddr][ag] = state.PackSkillStatus{Installed: true}
						fmt.Printf("    installed\n")
						changed++
					}
					continue
				}

				status, err := is.Status()
				if err != nil {
					fmt.Printf("  %s checking %s [%s]: %v\n", yellow("warning:"), skillAddr, ag, err)
					continue
				}
				if !status.HasUpstream {
					continue
				}
				fmt.Printf("  updating %s [%s] ...\n", skillAddr, ag)
				token := app.Cfg.TokenForRepo(repoName)
				if updateErr := is.Update(token); updateErr != nil {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{
						Installed: false,
						Error:     updateErr.Error(),
					}
					fmt.Printf("    %s\n", yellow("warning: "+updateErr.Error()))
				} else {
					rec.Skills[skillAddr][ag] = state.PackSkillStatus{Installed: true}
					fmt.Printf("    updated\n")
					changed++
				}
			}
		}

		if err := app.St.RecordPackInstall(packAddr, rec); err != nil {
			return err
		}
		if err := state.Save(app.St); err != nil {
			return err
		}

		if changed == 0 {
			fmt.Printf("\nPack %q is already up to date.\n", packAddr)
		} else {
			fmt.Printf("\nPack %q updated (%d skill(s) changed).\n", packAddr, changed)
		}
		if isPackPartial(rec) {
			fmt.Printf("Pack is %s — run `skillpack pack status %s` for details.\n", yellow("partial"), packAddr)
		}
		return nil
	},
}

// ─── pack status ──────────────────────────────────────────────────────────────

var packStatusCmd = &cobra.Command{
	Use:   "status <address>",
	Short: "Show per-skill, per-agent install status for a pack",
	Example: `  skillpack pack status my-repo/packs/go-dev`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		packAddr := args[0]

		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		rec, ok := app.St.InstalledPacks[packAddr]
		if !ok {
			return fmt.Errorf("pack %q is not installed", packAddr)
		}

		partial := isPackPartial(rec)
		overallStatus := green("complete")
		if partial {
			overallStatus = yellow("partial")
		}
		fmt.Printf("Pack: %s  [%s]\n", bold(packAddr), overallStatus)
		fmt.Printf("Installed: %s\n", rec.InstalledAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Agents:    %s\n\n", strings.Join(rec.Agents, ", "))

		skills := skillsInPack(rec)
		if len(skills) == 0 {
			fmt.Println("  (no skills recorded)")
			return nil
		}

		// Column widths.
		skillW := 5
		agentW := 5
		for _, skillAddr := range skills {
			if len(skillAddr) > skillW {
				skillW = len(skillAddr)
			}
			for ag := range rec.Skills[skillAddr] {
				if len(ag) > agentW {
					agentW = len(ag)
				}
			}
		}

		fmt.Printf("  %-*s  %-*s  %s\n", skillW, "SKILL", agentW, "AGENT", "STATUS")
		fmt.Printf("  %s  %s  %s\n", strings.Repeat("-", skillW), strings.Repeat("-", agentW), "------")

		for _, skillAddr := range skills {
			agStatuses := rec.Skills[skillAddr]
			agents := make([]string, 0, len(agStatuses))
			for ag := range agStatuses {
				agents = append(agents, ag)
			}
			sort.Strings(agents)
			for _, ag := range agents {
				s := agStatuses[ag]
				status := green("installed")
				if !s.Installed {
					if s.Error != "" {
						status = red("error: " + s.Error)
					} else {
						status = yellow("missing")
					}
				}
				fmt.Printf("  %-*s  %-*s  %s\n", skillW, skillAddr, agentW, ag, status)
			}
		}
		return nil
	},
}

// ─── init ─────────────────────────────────────────────────────────────────────

func init() {
	packListCmd.Flags().Bool("available", false, "Browse packs available in registered repos")
	packListCmd.Flags().String("repo", "", "Filter available packs by repo name (used with --available)")

	packInstallCmd.Flags().String("agent", "", "Target agent (default: configured default_agent)")
	packInstallCmd.Flags().Bool("all-agents", false, "Install for all configured agents")

	packRemoveCmd.Flags().String("agent", "", "Target agent (default: configured default_agent)")
	packRemoveCmd.Flags().Bool("all-agents", false, "Remove from all agents the pack is installed for")
	packRemoveCmd.Flags().Bool("force", false, "Remove even if skills have local modifications")

	packCmd.AddCommand(packListCmd)
	packCmd.AddCommand(packInstallCmd)
	packCmd.AddCommand(packRemoveCmd)
	packCmd.AddCommand(packUpdateCmd)
	packCmd.AddCommand(packStatusCmd)
}
