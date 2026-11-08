package doctor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmaltais/skillpack/internal/gitops"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// The functions in this file WRITE to a repo cache and push to its remote.
// Diagnose never calls them; the caller invokes one only after the user has
// confirmed. Each restores the cache to its prior content if the commit or
// push fails, so a failed fix leaves nothing half-applied.

// DeleteCopy removes the skill at addr from its repo and pushes, but only when
// its content is identical to keepAddr's (so nothing is lost) and addr is not
// installed anywhere. token authenticates HTTPS pushes; SSH repos ignore it.
func DeleteCopy(st *state.State, addr, keepAddr, token string) error {
	if _, installed := st.InstalledSkills[addr]; installed {
		return fmt.Errorf("%s is installed; remove it from your agents first", addr)
	}
	info, rec, err := writable(st, addr, token)
	if err != nil {
		return err
	}
	keep, err := repo.FindSkill(keepAddr, st)
	if err != nil {
		return err
	}
	same, err := sameContent(info.FullPath, keep.FullPath)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("%s and %s differ; refusing to delete (overwrite one with the other instead)", addr, keepAddr)
	}
	return applyAndPush(info, rec, token,
		fmt.Sprintf("skillpack: remove duplicate %s (identical to %s)", addr, keepAddr),
		func() error { return os.RemoveAll(info.FullPath) })
}

// OverwriteCopy makes the skill at target match the one at source, then
// commits and pushes target's repo. target keeps its own .skillpack-fork
// provenance file; source's is not copied.
func OverwriteCopy(st *state.State, target, source, token string) error {
	info, rec, err := writable(st, target, token)
	if err != nil {
		return err
	}
	src, err := repo.FindSkill(source, st)
	if err != nil {
		return err
	}
	return applyAndPush(info, rec, token,
		fmt.Sprintf("skillpack: make %s match %s", target, source),
		func() error {
			stage, err := os.MkdirTemp("", "skillpack-stage-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(stage)
			if err := skill.CopyDir(src.FullPath, stage); err != nil {
				return err
			}
			_ = os.Remove(filepath.Join(stage, metadataFile))

			keepMeta, metaErr := os.ReadFile(filepath.Join(info.FullPath, metadataFile))
			if err := os.RemoveAll(info.FullPath); err != nil {
				return err
			}
			if err := skill.CopyDir(stage, info.FullPath); err != nil {
				return err
			}
			if metaErr == nil {
				return os.WriteFile(filepath.Join(info.FullPath, metadataFile), keepMeta, 0o600)
			}
			return nil
		})
}

// writable resolves addr and checks the user can push to its repo (SSH URL or
// a token), the same rule RegisterForkProvenance applies.
func writable(st *state.State, addr, token string) (*repo.SkillInfo, state.RepoRecord, error) {
	info, err := repo.FindSkill(addr, st)
	if err != nil {
		return nil, state.RepoRecord{}, err
	}
	rec := st.Repos[info.RepoName]
	if !gitops.IsSSHURL(rec.URL) && token == "" {
		return nil, rec, fmt.Errorf("no write access to repo %q (%s): use an SSH URL or store a token for it", info.RepoName, rec.URL)
	}
	return info, rec, nil
}

func sameContent(a, b string) (bool, error) {
	ha, err := skill.ComputeHash(a)
	if err != nil {
		return false, err
	}
	hb, err := skill.ComputeHash(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

// applyAndPush backs up the skill dir, runs mutate, then commits and pushes
// it. On any failure the dir is restored from the backup.
func applyAndPush(info *repo.SkillInfo, rec state.RepoRecord, token, message string, mutate func() error) error {
	backupRoot, err := os.MkdirTemp("", "skillpack-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backupRoot)
	backup := filepath.Join(backupRoot, "orig")
	if err := skill.CopyDir(info.FullPath, backup); err != nil {
		return fmt.Errorf("backing up %s: %w", info.Address, err)
	}
	restore := func() {
		_ = os.RemoveAll(info.FullPath)
		_ = skill.CopyDir(backup, info.FullPath)
	}
	if err := mutate(); err != nil {
		restore()
		return err
	}
	if _, err := gitops.CommitAndPush(rec.CachePath, info.RelPath, message, rec.URL, token); err != nil {
		restore()
		return fmt.Errorf("committing and pushing: %w", err)
	}
	return nil
}
