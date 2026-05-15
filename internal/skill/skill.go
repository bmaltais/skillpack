package skill

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bernard/skillpack/internal/config"
	"github.com/bernard/skillpack/internal/repo"
	"github.com/bernard/skillpack/internal/state"
)

// Install copies a skill from the repo cache into an agent's skill dir and records it in state.
func Install(addr, agentName string, cfg *config.Config, st *state.State, skipExisting bool) error {
	agentCfg, ok := cfg.Agents[agentName]
	if !ok {
		return fmt.Errorf("agent %q not found in config", agentName)
	}
	skillDir, err := config.ExpandPath(agentCfg.SkillDir)
	if err != nil {
		return err
	}

	skillInfo, err := repo.FindSkill(addr, st)
	if err != nil {
		return err
	}

	// Install target: <agent.skill_dir>/<skill-name>
	// skill-name is the final path component so category structure is not preserved in the agent dir.
	skillName := filepath.Base(skillInfo.FullPath)
	targetDir := filepath.Join(skillDir, skillName)

	if skipExisting {
		if agents, ok := st.InstalledSkills[addr]; ok {
			if _, ok := agents[agentName]; ok {
				fmt.Printf("  skipping %s (already installed for %s)\n", addr, agentName)
				return nil
			}
		}
	}

	if err := os.MkdirAll(skillDir, 0700); err != nil {
		return fmt.Errorf("creating agent skill dir: %w", err)
	}
	if err := copyDir(skillInfo.FullPath, targetDir); err != nil {
		return fmt.Errorf("copying skill files: %w", err)
	}

	hash, err := ComputeHash(targetDir)
	if err != nil {
		return fmt.Errorf("computing installed hash: %w", err)
	}
	sha, err := repo.HeadSHA(skillInfo.RepoName, st)
	if err != nil {
		return fmt.Errorf("getting repo HEAD SHA: %w", err)
	}

	if st.InstalledSkills[addr] == nil {
		st.InstalledSkills[addr] = make(map[string]state.InstalledSkillRecord)
	}
	st.InstalledSkills[addr][agentName] = state.InstalledSkillRecord{
		InstalledAtSHA: sha,
		InstalledHash:  hash,
		LocalPath:      targetDir,
	}
	return nil
}

// Remove deletes an installed skill from an agent's skill dir.
func Remove(addr, agentName string, cfg *config.Config, st *state.State, force bool) error {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	if !force {
		modified, err := IsModified(rec)
		if err != nil {
			return err
		}
		if modified {
			return fmt.Errorf("skill %q has local modifications — use --force to remove anyway", addr)
		}
	}

	if err := os.RemoveAll(rec.LocalPath); err != nil {
		return fmt.Errorf("removing skill directory: %w", err)
	}

	delete(st.InstalledSkills[addr], agentName)
	if len(st.InstalledSkills[addr]) == 0 {
		delete(st.InstalledSkills, addr)
	}
	return nil
}

// IsModified returns true if the installed skill directory has changed since installation.
func IsModified(rec state.InstalledSkillRecord) (bool, error) {
	if _, err := os.Stat(rec.LocalPath); os.IsNotExist(err) {
		return false, nil
	}
	current, err := ComputeHash(rec.LocalPath)
	if err != nil {
		return false, err
	}
	return current != rec.InstalledHash, nil
}

// ComputeHash returns a deterministic SHA-256 digest of all file contents in dir,
// sorted by relative path to ensure stability across platforms.
func ComputeHash(dir string) (string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		// Normalise path separators so hashes are consistent across platforms.
		fmt.Fprintf(h, "%s\n", strings.ReplaceAll(rel, "\\", "/"))
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write(data)
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// copyDir recursively copies the src directory tree to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
