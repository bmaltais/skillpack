package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/state"
)

// publish pushes local edits of an installed skill back to the remote repo.
// Semantically equivalent to forceLocal: the local copy wins unconditionally.
func publish(addr, agentName, token string, st *state.State) error {
	return forceLocal(addr, agentName, token, st)
}

// PublishNew copies a local skill directory into the named repo, commits, and pushes.
// The skill is placed at <repo-root>/<basename-of-localDir>.
// The skill is NOT auto-installed; use `skillpack install` afterwards.
// Returns the new skill address (e.g. "my-repo/my-new-skill") on success.
func PublishNew(localDir, repoName, token string, st *state.State) (string, error) {
	if _, err := os.Stat(localDir); err != nil {
		return "", fmt.Errorf("local dir %q not found: %w", localDir, err)
	}
	if _, err := os.Stat(filepath.Join(localDir, "SKILL.md")); err != nil {
		return "", fmt.Errorf("%s does not contain a SKILL.md — not a valid skill directory", localDir)
	}

	repoRec, ok := st.Repos[repoName]
	if !ok {
		return "", fmt.Errorf("repo %q not registered — add it first with: skillpack repo add <name> <url>", repoName)
	}

	// Use the local dir's basename as the skill name in the repo.
	// filepath.Base(filepath.Clean(...)) handles both slash and backslash
	// separators correctly on all platforms.
	skillName := filepath.Base(filepath.Clean(localDir))
	if skillName == "" || skillName == "." {
		return "", fmt.Errorf("cannot determine skill name from path %q", localDir)
	}

	destPath := filepath.Join(repoRec.CachePath, skillName)
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf(
			"skill %q already exists in repo %q — use `skillpack publish %s/%s` to update it",
			skillName, repoName, repoName, skillName,
		)
	}

	if err := copyDir(localDir, destPath); err != nil {
		return "", fmt.Errorf("copying skill to repo cache: %w", err)
	}

	result, err := gitops.CommitAndPush(
		repoRec.CachePath,
		skillName,
		fmt.Sprintf("skillpack: add %s", skillName),
		repoRec.URL,
		token,
	)
	if err != nil {
		return "", err
	}
	if !result.Committed {
		return "", fmt.Errorf("no changes to commit for skill %q", skillName)
	}

	return repoName + "/" + skillName, nil
}
