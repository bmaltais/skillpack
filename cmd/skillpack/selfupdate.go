package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubReleaseURL  = "https://api.github.com/repos/bmaltais/skillpack/releases/latest"
	githubDownloadFmt = "https://github.com/bmaltais/skillpack/releases/download/%s/skillpack-%s-%s%s"
	installOneLiner   = "curl -fsSL https://raw.githubusercontent.com/bmaltais/skillpack/main/install.sh \\\n  -o /tmp/skillpack-install.sh && sh /tmp/skillpack-install.sh"
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Download and install the latest version of skillpack",
	Long: `Fetches the latest release from GitHub and replaces the running binary.`,
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

		fmt.Print("Downloading update... ")
		if err := downloadAndReplace(latest); err != nil {
			fmt.Println()
			fmt.Println(yellow("Automatic update failed: " + err.Error()))
			fmt.Println()
			fmt.Println("To upgrade manually, run:")
			fmt.Printf("\n    %s\n\n", bold(installOneLiner))
			return nil
		}
		fmt.Println("done.")
		fmt.Println()
		fmt.Printf("%s skillpack updated to %s\n", green("✓"), bold(latest))
		fmt.Println()
		return nil
	},
}

// downloadAndReplace downloads the latest release binary for the current
// OS/arch and replaces the running executable.
func downloadAndReplace(tag string) error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Windows releases are uploaded with a .exe suffix.
	exeSuffix := ""
	if goos == "windows" {
		exeSuffix = ".exe"
	}

	url := fmt.Sprintf(githubDownloadFmt, tag, goos, goarch, exeSuffix)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	// Download to a temp file beside the current binary.
	tmpPath := execPath + ".new"
	if err := downloadFile(url, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Make it executable (no-op on Windows).
	if goos != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("could not chmod downloaded binary: %w", err)
		}
	}

	// On Windows, os.Rename cannot overwrite the running executable directly.
	// Rename the current binary to .old first, then move the new one into place.
	if goos == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath) // clean up any leftover from a previous update
		if err := os.Rename(execPath, oldPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("could not rename current binary: %w", err)
		}
		if err := os.Rename(tmpPath, execPath); err != nil {
			// Try to restore the original binary before returning the error.
			_ = os.Rename(oldPath, execPath)
			_ = os.Remove(tmpPath)
			return fmt.Errorf("could not replace binary: %w", err)
		}
		// Best-effort cleanup of the old binary; ignore errors.
		_ = os.Remove(oldPath)
		return nil
	}

	// Unix: atomic replace via rename.
	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("could not replace binary (try with sudo): %w", err)
	}

	return nil
}

// downloadFile fetches url and writes it to dest.
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "skillpack/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("error writing download: %w", err)
	}
	return nil
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


