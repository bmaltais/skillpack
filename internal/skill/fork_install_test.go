package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestInstall_LoadsForkProvenanceMetadata(t *testing.T) {
	setupHome(t)
	cacheDir := t.TempDir()
	skillRelPath := filepath.Join("skills", "engineering", "improve-codebase-architecture")
	writeFile(t, filepath.Join(cacheDir, skillRelPath, "SKILL.md"), "# Skill")
	writeFile(t, filepath.Join(cacheDir, skillRelPath, ".skillpack-fork"), `{"upstream_addr":"mattpocock-skills/skills/engineering/improve-codebase-architecture","upstream_sha":"abc123"}`)
	_, _ = initRepoWithCommit(t, cacheDir, "initial commit")

	installRoot := t.TempDir()
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"claude-code": {SkillDir: installRoot},
		},
	}
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"bmaltais-skills": {CachePath: cacheDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}

	addr := "bmaltais-skills/skills/engineering/improve-codebase-architecture"
	if err := Install(addr, "claude-code", cfg, st, false); err != nil {
		t.Fatalf("Install: %v", err)
	}

	rec := st.InstalledSkills[addr]["claude-code"]
	if rec.UpstreamAddr != "mattpocock-skills/skills/engineering/improve-codebase-architecture" {
		t.Fatalf("expected upstream addr from metadata, got %q", rec.UpstreamAddr)
	}
	if rec.UpstreamSHA != "abc123" {
		t.Fatalf("expected upstream sha from metadata, got %q", rec.UpstreamSHA)
	}
}

func TestFork_ExistingDestinationWithSameUpstream_ReforksInPlace(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	forkCache := t.TempDir()
	skillName := "improve-codebase-architecture"
	writeFile(t, filepath.Join(forkCache, skillName, "SKILL.md"), "# Existing fork content")
	forkRepo := initRepoOnly(t, forkCache)
	_, _ = commitAll(t, forkRepo, "existing fork commit")
	remoteDir := attachBareOrigin(t, forkRepo, forkCache)

	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# Local installed content")
	hash, err := ComputeHash(installedDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	newAddr := "bmaltais-skills/" + skillName
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache, URL: "file://" + remoteDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
			newAddr: {
				"claude-code": {
					UpstreamAddr: "source-skills/skills/engineering/improve-codebase-architecture",
					UpstreamSHA:  "old-sha",
				},
			},
		},
	}

	gotAddr, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeAuto, st)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if gotAddr != newAddr {
		t.Fatalf("expected new addr %q, got %q", newAddr, gotAddr)
	}
	if _, ok := st.InstalledSkills[addr]; ok {
		t.Fatalf("expected original addr %q to be removed from state", addr)
	}

	rec := st.InstalledSkills[newAddr]["claude-code"]
	if rec.UpstreamAddr != addr {
		t.Fatalf("expected upstream addr %q, got %q", addr, rec.UpstreamAddr)
	}
	if rec.UpstreamSHA == "" || rec.UpstreamSHA == "old-sha" {
		t.Fatalf("expected refreshed upstream sha, got %q", rec.UpstreamSHA)
	}

	content, err := os.ReadFile(filepath.Join(forkCache, skillName, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading forked SKILL.md: %v", err)
	}
	if string(content) != "# Local installed content" {
		t.Fatalf("expected fork cache to be overwritten from local install, got %q", string(content))
	}

	metaRaw, err := os.ReadFile(filepath.Join(forkCache, skillName, ".skillpack-fork"))
	if err != nil {
		t.Fatalf("reading .skillpack-fork: %v", err)
	}
	var meta struct {
		UpstreamAddr string `json:"upstream_addr"`
		UpstreamSHA  string `json:"upstream_sha"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.UpstreamAddr != addr {
		t.Fatalf("metadata upstream_addr: expected %q, got %q", addr, meta.UpstreamAddr)
	}
	if meta.UpstreamSHA != rec.UpstreamSHA {
		t.Fatalf("metadata upstream_sha: expected %q, got %q", rec.UpstreamSHA, meta.UpstreamSHA)
	}
}

func TestFork_MovesAllInstalledAgentsToForkAddress(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	forkCache := t.TempDir()
	skillName := "improve-codebase-architecture"
	writeFile(t, filepath.Join(forkCache, "README.md"), "fork repo")
	forkRepo := initRepoOnly(t, forkCache)
	_, _ = commitAll(t, forkRepo, "initial fork repo commit")
	remoteDir := attachBareOrigin(t, forkRepo, forkCache)

	claudeInstalledDir := t.TempDir()
	writeFile(t, filepath.Join(claudeInstalledDir, "SKILL.md"), "# Claude local installed content")
	claudeHash, err := ComputeHash(claudeInstalledDir)
	if err != nil {
		t.Fatalf("ComputeHash(claude): %v", err)
	}
	copilotInstalledDir := t.TempDir()
	writeFile(t, filepath.Join(copilotInstalledDir, "SKILL.md"), "# Copilot local installed content")
	copilotHash, err := ComputeHash(copilotInstalledDir)
	if err != nil {
		t.Fatalf("ComputeHash(copilot): %v", err)
	}

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	newAddr := "bmaltais-skills/" + skillName
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache, URL: "file://" + remoteDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: claudeHash,
					LocalPath:     claudeInstalledDir,
				},
				"copilot": {
					InstalledHash: copilotHash,
					LocalPath:     copilotInstalledDir,
				},
			},
		},
	}

	gotAddr, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeAuto, st)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if gotAddr != newAddr {
		t.Fatalf("expected new addr %q, got %q", newAddr, gotAddr)
	}
	if _, ok := st.InstalledSkills[addr]; ok {
		t.Fatalf("expected original addr %q to be removed from state", addr)
	}

	forkedAgents := st.InstalledSkills[newAddr]
	claudeRec, ok := forkedAgents["claude-code"]
	if !ok {
		t.Fatalf("expected claude-code to be moved under %q", newAddr)
	}
	copilotRec, ok := forkedAgents["copilot"]
	if !ok {
		t.Fatalf("expected copilot to be moved under %q", newAddr)
	}
	if claudeRec.LocalPath != claudeInstalledDir {
		t.Fatalf("claude local path mismatch: got %q want %q", claudeRec.LocalPath, claudeInstalledDir)
	}
	if copilotRec.LocalPath != copilotInstalledDir {
		t.Fatalf("copilot local path mismatch: got %q want %q", copilotRec.LocalPath, copilotInstalledDir)
	}
	if claudeRec.UpstreamAddr != addr || copilotRec.UpstreamAddr != addr {
		t.Fatalf("expected both agents to track upstream %q; got claude=%q copilot=%q", addr, claudeRec.UpstreamAddr, copilotRec.UpstreamAddr)
	}
	if claudeRec.UpstreamSHA == "" || copilotRec.UpstreamSHA == "" {
		t.Fatalf("expected upstream SHA for both agents; got claude=%q copilot=%q", claudeRec.UpstreamSHA, copilotRec.UpstreamSHA)
	}

	claudeMetaRaw, err := os.ReadFile(filepath.Join(claudeInstalledDir, ".skillpack-fork"))
	if err != nil {
		t.Fatalf("reading claude .skillpack-fork: %v", err)
	}
	copilotMetaRaw, err := os.ReadFile(filepath.Join(copilotInstalledDir, ".skillpack-fork"))
	if err != nil {
		t.Fatalf("reading copilot .skillpack-fork: %v", err)
	}
	var claudeMeta struct {
		UpstreamAddr string `json:"upstream_addr"`
	}
	if err := json.Unmarshal(claudeMetaRaw, &claudeMeta); err != nil {
		t.Fatalf("unmarshal claude metadata: %v", err)
	}
	var copilotMeta struct {
		UpstreamAddr string `json:"upstream_addr"`
	}
	if err := json.Unmarshal(copilotMetaRaw, &copilotMeta); err != nil {
		t.Fatalf("unmarshal copilot metadata: %v", err)
	}
	if claudeMeta.UpstreamAddr != addr || copilotMeta.UpstreamAddr != addr {
		t.Fatalf("expected both metadata files to track upstream %q; got claude=%q copilot=%q", addr, claudeMeta.UpstreamAddr, copilotMeta.UpstreamAddr)
	}
}

func TestFork_ExistingDestinationWithMatchingUpstream_AllowsRemainingAgentMigration(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	forkCache := t.TempDir()
	skillName := "improve-codebase-architecture"
	writeFile(t, filepath.Join(forkCache, skillName, "SKILL.md"), "# Existing fork content")
	forkRepo := initRepoOnly(t, forkCache)
	_, _ = commitAll(t, forkRepo, "existing fork commit")
	remoteDir := attachBareOrigin(t, forkRepo, forkCache)

	copilotInstalledDir := t.TempDir()
	writeFile(t, filepath.Join(copilotInstalledDir, "SKILL.md"), "# Copilot local installed content")
	copilotHash, err := ComputeHash(copilotInstalledDir)
	if err != nil {
		t.Fatalf("ComputeHash(copilot): %v", err)
	}

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	newAddr := "bmaltais-skills/" + skillName
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache, URL: "file://" + remoteDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"copilot": {
					InstalledHash: copilotHash,
					LocalPath:     copilotInstalledDir,
				},
			},
			newAddr: {
				"claude-code": {
					UpstreamAddr: addr,
					UpstreamSHA:  "old-sha",
				},
			},
		},
	}

	if _, err := fork(addr, "bmaltais-skills", "copilot", "", ForkModeAuto, st); err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if _, ok := st.InstalledSkills[addr]; ok {
		t.Fatalf("expected original addr %q to be removed from state", addr)
	}

	if _, ok := st.InstalledSkills[newAddr]["claude-code"]; !ok {
		t.Fatalf("expected existing forked agent state for claude-code to be preserved at %q", newAddr)
	}
	if _, ok := st.InstalledSkills[newAddr]["copilot"]; !ok {
		t.Fatalf("expected remaining source agent copilot to migrate to %q", newAddr)
	}
}

func TestFork_ExistingDestinationWithDifferentUpstream_ReturnsError(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# Local installed content")
	hash, _ := ComputeHash(installedDir)

	forkCache := t.TempDir()
	writeFile(t, filepath.Join(forkCache, "improve-codebase-architecture", "SKILL.md"), "# Existing")

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	newAddr := "bmaltais-skills/improve-codebase-architecture"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
			newAddr: {
				"claude-code": {
					UpstreamAddr: "other/skill",
				},
			},
		},
	}

	_, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeAuto, st)
	if err == nil {
		t.Fatal("expected error for conflicting upstream")
	}
	if !strings.Contains(err.Error(), "tracked as a fork of") {
		t.Fatalf("expected clear conflicting-upstream error, got: %v", err)
	}
}

func TestFork_ExistingDestinationWithoutState_ReturnsError(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# Local installed content")
	hash, _ := ComputeHash(installedDir)

	forkCache := t.TempDir()
	writeFile(t, filepath.Join(forkCache, "improve-codebase-architecture", "SKILL.md"), "# Existing")

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
		},
	}

	_, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeAuto, st)
	if err == nil {
		t.Fatal("expected error for unknown-provenance existing destination skill")
	}
	if !strings.Contains(err.Error(), "unknown fork provenance") {
		t.Fatalf("expected unknown-provenance error, got: %v", err)
	}
}

func TestFork_ForkModeOverride_ReplacesExistingDestination(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	forkCache := t.TempDir()
	skillName := "improve-codebase-architecture"
	writeFile(t, filepath.Join(forkCache, skillName, "SKILL.md"), "# Old existing content")
	forkRepo := initRepoOnly(t, forkCache)
	_, _ = commitAll(t, forkRepo, "existing fork commit")
	remoteDir := attachBareOrigin(t, forkRepo, forkCache)

	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# Local installed content")
	hash, err := ComputeHash(installedDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	newAddr := "bmaltais-skills/" + skillName
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache, URL: "file://" + remoteDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
			// No state entry for newAddr — unknown provenance, user chose override
		},
	}

	gotAddr, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeOverride, st)
	if err != nil {
		t.Fatalf("Fork with ForkModeOverride: %v", err)
	}
	if gotAddr != newAddr {
		t.Fatalf("expected new addr %q, got %q", newAddr, gotAddr)
	}

	// Destination dir should contain the installed content, not the old content
	content, err := os.ReadFile(filepath.Join(forkCache, skillName, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading forked SKILL.md: %v", err)
	}
	if string(content) != "# Local installed content" {
		t.Fatalf("expected override to replace dest with installed content, got %q", string(content))
	}

	// Fork metadata must be written
	if _, err := os.Stat(filepath.Join(forkCache, skillName, ".skillpack-fork")); err != nil {
		t.Fatalf("expected .skillpack-fork to be written: %v", err)
	}

	// State must track the new address
	if _, ok := st.InstalledSkills[newAddr]; !ok {
		t.Fatalf("expected state entry for new addr %q after override fork", newAddr)
	}
}

func TestFork_ForkModeRegister_KeepsExistingDestinationContents(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	forkCache := t.TempDir()
	skillName := "improve-codebase-architecture"
	writeFile(t, filepath.Join(forkCache, skillName, "SKILL.md"), "# Existing destination content")
	forkRepo := initRepoOnly(t, forkCache)
	_, _ = commitAll(t, forkRepo, "existing fork commit")
	remoteDir := attachBareOrigin(t, forkRepo, forkCache)

	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# Local installed content")
	hash, err := ComputeHash(installedDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	newAddr := "bmaltais-skills/" + skillName
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache, URL: "file://" + remoteDir},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
			// No state entry for newAddr — unknown provenance, user chose register
		},
	}

	gotAddr, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeRegister, st)
	if err != nil {
		t.Fatalf("Fork with ForkModeRegister: %v", err)
	}
	if gotAddr != newAddr {
		t.Fatalf("expected new addr %q, got %q", newAddr, gotAddr)
	}

	// Destination dir content must be unchanged
	content, err := os.ReadFile(filepath.Join(forkCache, skillName, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading forked SKILL.md: %v", err)
	}
	if string(content) != "# Existing destination content" {
		t.Fatalf("expected register to preserve existing dest content, got %q", string(content))
	}

	// Fork metadata must be written
	if _, err := os.Stat(filepath.Join(forkCache, skillName, ".skillpack-fork")); err != nil {
		t.Fatalf("expected .skillpack-fork to be written: %v", err)
	}

	// State must track the new address
	if _, ok := st.InstalledSkills[newAddr]; !ok {
		t.Fatalf("expected state entry for new addr %q after register fork", newAddr)
	}
}

func TestFork_ForkModeRegister_NonExistentDestination_ReturnsError(t *testing.T) {
	setupHome(t)
	upstreamCache := t.TempDir()
	writeFile(t, filepath.Join(upstreamCache, "skills", "engineering", "improve-codebase-architecture", "SKILL.md"), "# Upstream")
	_, _ = initRepoWithCommit(t, upstreamCache, "upstream commit")

	installedDir := t.TempDir()
	writeFile(t, filepath.Join(installedDir, "SKILL.md"), "# Local installed content")
	hash, _ := ComputeHash(installedDir)

	forkCache := t.TempDir() // destination dir does NOT exist inside forkCache

	addr := "source-skills/skills/engineering/improve-codebase-architecture"
	st := &state.State{
		Repos: map[string]state.RepoRecord{
			"source-skills":   {CachePath: upstreamCache},
			"bmaltais-skills": {CachePath: forkCache},
		},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{
			addr: {
				"claude-code": {
					InstalledHash: hash,
					LocalPath:     installedDir,
				},
			},
		},
	}

	_, err := fork(addr, "bmaltais-skills", "claude-code", "", ForkModeRegister, st)
	if err == nil {
		t.Fatal("expected error when register mode used with non-existent destination")
	}
	if !strings.Contains(err.Error(), "register mode requires") {
		t.Fatalf("expected 'register mode requires' error, got: %v", err)
	}
}

func initRepoOnly(t *testing.T, dir string) *gogit.Repository {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit(%s): %v", dir, err)
	}
	return r
}

func initRepoWithCommit(t *testing.T, dir, message string) (*gogit.Repository, string) {
	t.Helper()
	r := initRepoOnly(t, dir)
	commit, wt := commitAll(t, r, message)
	if _, err := wt.Status(); err != nil {
		t.Fatalf("status after commit: %v", err)
	}
	return r, commit.String()
}

func commitAll(t *testing.T, r *gogit.Repository, message string) (plumbing.Hash, *gogit.Worktree) {
	t.Helper()
	wt, err := r.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("git add .: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}
	commit, err := wt.Commit(message, &gogit.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return commit, wt
}

func attachBareOrigin(t *testing.T, cacheRepo *gogit.Repository, cacheDir string) string {
	t.Helper()
	remoteDir := t.TempDir()
	if _, err := gogit.PlainClone(remoteDir, true, &gogit.CloneOptions{URL: "file://" + cacheDir}); err != nil {
		t.Fatalf("create bare remote: %v", err)
	}
	cfg, err := cacheRepo.Config()
	if err != nil {
		t.Fatalf("read cache config: %v", err)
	}
	cfg.Remotes["origin"] = &gitconfig.RemoteConfig{
		Name:  "origin",
		URLs:  []string{"file://" + remoteDir},
		Fetch: []gitconfig.RefSpec{"refs/heads/*:refs/remotes/origin/*"},
	}
	if err := cacheRepo.SetConfig(cfg); err != nil {
		t.Fatalf("set remote config: %v", err)
	}
	return remoteDir
}
