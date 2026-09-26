package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/repo"
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

		partial := pack.IsPartial(rec)
		status := green("complete")
		if partial {
			status = yellow("partial")
		}

		agents := strings.Join(rec.Agents, ", ")
		fmt.Printf("%-48s  [%s]  agents: %s\n", addr, status, agents)
	}
	return nil
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
	def, err := pack.Resolve(addr, app.St)
	if err != nil {
		return err
	}

	agents, err := selectAgentsForPack(agentName, allAgents, app.Cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Installing pack %q for agents: %s\n", def.Address, strings.Join(agents, ", "))

	res, err := pack.Install(app.Cfg, app.St, def, agents, pack.Options{Progress: printPackProgress})
	if err != nil {
		return err
	}

	if res.Partial() {
		fmt.Printf("\nPack %q installed with %s — some skills could not be deployed.\n", def.Address, yellow("partial"))
		fmt.Println("  Run `skillpack pack status` for details.")
	} else {
		fmt.Printf("\nPack %q installed %s.\n", def.Address, green("complete"))
	}
	return nil
}

// printPackProgress renders pack deployment events as CLI output.
func printPackProgress(e pack.Event) {
	switch e.Kind {
	case pack.RepoRegistering:
		fmt.Printf("  registering repo %q (%s) ...\n", e.Repo, e.URL)
	case pack.RepoFailed:
		fmt.Printf("  %s\n", yellow(fmt.Sprintf("warning: could not register repo %q: %v", e.Repo, e.Err)))
	case pack.SkillSkipped:
		fmt.Printf("  %s [%s]  skipped — repo %q unavailable: %v\n", e.Skill, e.Agent, e.Repo, e.Err)
	case pack.SkillInstalling:
		fmt.Printf("  installing %s for %s ...\n", e.Skill, e.Agent)
	case pack.SkillInstalled:
		fmt.Printf("    installed\n")
	case pack.SkillFailed:
		fmt.Printf("    %s\n", yellow("warning: "+e.Err.Error()))
	case pack.RepoPulling:
		fmt.Printf("  pulling repo %q ...\n", e.Repo)
	case pack.RepoPullFailed:
		fmt.Printf("  %s\n", yellow(fmt.Sprintf("warning: %v", e.Err)))
	case pack.RepoPullWarning:
		fmt.Printf("  %s\n", yellow(e.Message))
	case pack.SkillUpdating:
		fmt.Printf("  updating %s [%s] ...\n", e.Skill, e.Agent)
	case pack.SkillUpdated:
		fmt.Printf("    updated\n")
	case pack.SkillBlocked:
		fmt.Printf("  %s [%s]  %s\n", e.Skill, e.Agent, yellow("not updated — "+e.Err.Error()))
	case pack.SkillRemoving:
		fmt.Printf("  removing %s [%s] ...\n", e.Skill, e.Agent)
	case pack.SkillRemoved:
		fmt.Printf("    removed\n")
	case pack.SkillKept:
		fmt.Printf("    %s\n", yellow(e.Err.Error()))
	case pack.SkillNotInstalled:
		fmt.Printf("  skipping %s [%s]: %v\n", e.Skill, e.Agent, e.Err)
	}
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

		res, err := pack.Remove(app.Cfg, app.St, packAddr, agents, pack.RemoveOptions{Force: force, Progress: printPackProgress})
		if err != nil {
			return err
		}

		if res.PackRemoved {
			fmt.Printf("\nPack %q removed.\n", packAddr)
		} else {
			fmt.Printf("\nPack %q updated (removed from agents: %s; still installed for: %s).\n",
				packAddr, strings.Join(agents, ", "), strings.Join(res.RemainingAgents, ", "))
		}
		if res.Kept > 0 {
			fmt.Printf("%d skill(s) with local modifications were kept as ordinary installed skills.\n", res.Kept)
		}
		return nil
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

		fmt.Printf("Updating pack %q ...\n", packAddr)
		res, err := pack.Update(app.Cfg, app.St, packAddr, pack.Options{Progress: printPackProgress})
		if err != nil {
			return err
		}

		if changed := res.Installed + res.Updated; changed == 0 {
			fmt.Printf("\nPack %q is already up to date.\n", packAddr)
		} else {
			fmt.Printf("\nPack %q updated (%d skill(s) changed).\n", packAddr, changed)
		}
		if res.Blocked > 0 {
			fmt.Printf("%s %d skill(s) were not updated because of conflicts.\n", yellow("warning:"), res.Blocked)
		}
		if res.Partial() {
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

		partial := pack.IsPartial(rec)
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
	packCmd.AddCommand(packCreateCmd)
	packCmd.AddCommand(packEditCmd)
}
