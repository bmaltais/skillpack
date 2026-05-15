package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubReleaseURL    = "https://api.github.com/repos/bmaltais/skillpack/releases/latest"
	goInstallCommand    = "go install github.com/bmaltais/skillpack/cmd/skillpack@latest"
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Check for a newer version of skillpack and print upgrade instructions",
	Long: `Fetches the latest release tag from GitHub and compares it to the
running version. If a newer release exists, prints the install command
so you can upgrade at your discretion.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		current := strings.TrimPrefix(Version, "v")
		if current == "dev" {
			fmt.Println(yellow("Running a development build — skipping version check."))
			return nil
		}

		fmt.Print("Checking for updates... ")
		latest, err := fetchLatestTag()
		if err != nil {
			fmt.Println()
			return fmt.Errorf("could not reach GitHub releases: %w", err)
		}
		fmt.Println("done.")
		fmt.Println()

		latestClean := strings.TrimPrefix(latest, "v")

		fmt.Printf("  Current version : %s\n", bold("v"+current))
		fmt.Printf("  Latest release  : %s\n", bold(latest))
		fmt.Println()

		if current == latestClean {
			fmt.Println(green("Already up to date."))
			return nil
		}

		fmt.Println(yellow("A newer version is available!"))
		fmt.Println()
		fmt.Println("To upgrade, run:")
		fmt.Println()
		fmt.Printf("    %s\n", bold(goInstallCommand))
		fmt.Println()
		return nil
	},
}

// fetchLatestTag queries the GitHub Releases API and returns the tag name of
// the most recent release.
func fetchLatestTag() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, githubReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "skillpack/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("GitHub API response contained no tag_name")
	}
	return payload.TagName, nil
}

// colorize wraps s in an ANSI escape sequence when colors are enabled.
// Removed — use ansiWrap helpers from color.go (bold, green, yellow, etc.)
