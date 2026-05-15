package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bernard/skillpack/internal/config"
)

var rootCmd = &cobra.Command{
	Use:   "skillpack",
	Short: "Manage AI agent skills across multiple agents",
	Long: `skillpack — install, update, publish and sync AI agent skills.

Skills live in git repositories and are installed as directories into
each agent's skill folder (e.g. ~/.claude/skills/, ~/.copilot/skills/).`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return ensureConfig()
	},
	SilenceUsage: true,
}

// Execute is the entry point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(syncCmd)
}

// ensureConfig runs the first-run wizard if no config file exists yet.
func ensureConfig() error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil // config already exists
	}
	cfg := &config.Config{Agents: make(map[string]config.AgentConfig)}
	return runWizard(cfg)
}

// runWizard detects installed agents and sets a default, then saves config.
func runWizard(cfg *config.Config) error {
	fmt.Println("Welcome to skillpack! Setting up your configuration...")
	fmt.Println()

	var detected []string
	for _, agent := range config.DefaultAgents {
		expanded, err := config.ExpandPath(agent.SkillDir)
		if err != nil {
			continue
		}
		if _, err := os.Stat(expanded); err == nil {
			detected = append(detected, agent.Name)
			cfg.Agents[agent.Name] = config.AgentConfig{SkillDir: agent.SkillDir}
			fmt.Printf("  detected: %s (%s)\n", agent.Name, agent.SkillDir)
		}
	}

	if len(detected) == 0 {
		fmt.Println("No known agents detected. Add agents manually to ~/.skillpack/config.yaml")
		return config.Save(cfg)
	}

	fmt.Println()
	fmt.Printf("Detected %d agent(s). Which should be the default?\n", len(detected))
	for i, name := range detected {
		fmt.Printf("  %d) %s\n", i+1, name)
	}

	idx := 0
	if isInteractive() {
		fmt.Print("\nEnter number [1]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			var n int
			if _, err := fmt.Sscanf(input, "%d", &n); err == nil && n >= 1 && n <= len(detected) {
				idx = n - 1
			}
		}
	}

	cfg.DefaultAgent = detected[idx]
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("\nDefault agent: %q — config saved to ~/.skillpack/config.yaml\n\n", cfg.DefaultAgent)
	return nil
}

// isInteractive returns true when stdin is an interactive terminal.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
