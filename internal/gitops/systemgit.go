package gitops

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsAzureDevOpsURL reports whether rawURL points at Azure DevOps (dev.azure.com
// or the legacy <org>.visualstudio.com). go-git's pure-Go smart-HTTP client
// cannot talk to Azure DevOps at all: ADO's git server requires the
// multi_ack/multi_ack_detailed capability during fetch negotiation, which
// go-git has never implemented (https://github.com/go-git/go-git/issues/64,
// open since 2020, dozens of downstream projects hit it). Every clone/fetch
// against an ADO remote fails with "unexpected client error ... status code:
// 400" on git-upload-pack regardless of credentials. The universal workaround
// (used by pulumi, flux, nuclio, and others) is shelling out to the system
// `git` binary for these hosts instead of go-git.
func IsAzureDevOpsURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	return strings.Contains(lower, "dev.azure.com") || strings.Contains(lower, ".visualstudio.com")
}

// systemGitAuthEnvVar is the env var name used to pass a PAT to the git
// subprocess via --config-env, so the token value never appears in argv
// (and therefore never shows up in `ps`/process listings).
const systemGitAuthEnvVar = "SKILLPACK_GIT_AUTH_HEADER"

// gitCommand builds an *exec.Cmd for the system git binary. When token is
// non-empty, authentication is injected via a Basic Authorization header
// carried in an environment variable (git 2.31+ --config-env), not argv.
func gitCommand(dir, token string, args ...string) *exec.Cmd {
	env := os.Environ()
	if token != "" {
		headerValue := "Authorization: basic " + base64.StdEncoding.EncodeToString([]byte(":"+token))
		env = append(env, systemGitAuthEnvVar+"="+headerValue)
		args = append([]string{"--config-env=http.extraheader=" + systemGitAuthEnvVar}, args...)
	}
	cmd := exec.Command("git", args...) //nolint:gosec
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// SystemGitClone clones url into destPath using the system git binary.
// token is optional; pass "" for an anonymous clone of a public repo.
func SystemGitClone(url, destPath, token string) error {
	cmd := gitCommand("", token, "clone", "--", url, destPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", url, err)
	}
	return nil
}

// SystemGitFetch fetches and prunes "origin" in the repo at cachePath using
// the system git binary. token is optional; pass "" for an anonymous fetch.
func SystemGitFetch(cachePath, token string) error {
	cmd := gitCommand(cachePath, token, "fetch", "--prune", "origin")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	return nil
}

// SystemGitPush pushes the current branch to its same-named branch on
// "origin" in the repo at cachePath, using the system git binary.
func SystemGitPush(cachePath, token string) error {
	cmd := gitCommand(cachePath, token, "push", "origin", "HEAD")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}
