package state_test

import (
	"testing"

	"github.com/bmaltais/skillpack/internal/state"
)

func emptyState() *state.State {
	return &state.State{
		Repos:           map[string]state.RepoRecord{},
		InstalledSkills: map[string]map[string]state.InstalledSkillRecord{},
	}
}

func rec(sha, hash, path string) state.InstalledSkillRecord {
	return state.InstalledSkillRecord{InstalledAtSHA: sha, InstalledHash: hash, LocalPath: path}
}

// ─── RecordInstall ────────────────────────────────────────────────────────────

func TestRecordInstall_CreatesEntry(t *testing.T) {
	st := emptyState()
	if err := st.RecordInstall("repo/skill", "copilot", rec("sha1", "h1", "/path")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := st.InstalledSkills["repo/skill"]["copilot"]
	if !ok {
		t.Fatal("record not created")
	}
	if got.InstalledAtSHA != "sha1" || got.InstalledHash != "h1" {
		t.Errorf("wrong record: %+v", got)
	}
}

func TestRecordInstall_ReplacesExistingEntry(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("sha1", "h1", "/path"))
	_ = st.RecordInstall("repo/skill", "copilot", rec("sha2", "h2", "/path"))
	got := st.InstalledSkills["repo/skill"]["copilot"]
	if got.InstalledAtSHA != "sha2" {
		t.Errorf("want sha2, got %q", got.InstalledAtSHA)
	}
}

func TestRecordInstall_EmptyAddrError(t *testing.T) {
	st := emptyState()
	if err := st.RecordInstall("", "copilot", rec("sha1", "h1", "/path")); err == nil {
		t.Error("want error for empty addr, got nil")
	}
}

func TestRecordInstall_EmptyAgentNameError(t *testing.T) {
	st := emptyState()
	if err := st.RecordInstall("repo/skill", "", rec("sha1", "h1", "/path")); err == nil {
		t.Error("want error for empty agentName, got nil")
	}
}

// ─── RecordRemove ─────────────────────────────────────────────────────────────

func TestRecordRemove_DeletesEntry(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("sha1", "h1", "/path"))
	if err := st.RecordRemove("repo/skill", "copilot"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := st.InstalledSkills["repo/skill"]; ok {
		t.Error("want addr entry deleted when no agents remain")
	}
}

func TestRecordRemove_KeepsAddrWhenOtherAgentsRemain(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("sha1", "h1", "/path"))
	_ = st.RecordInstall("repo/skill", "claude", rec("sha1", "h1", "/path2"))
	_ = st.RecordRemove("repo/skill", "copilot")
	if _, ok := st.InstalledSkills["repo/skill"]["claude"]; !ok {
		t.Error("claude entry should survive when copilot is removed")
	}
}

func TestRecordRemove_EmptyAddrError(t *testing.T) {
	st := emptyState()
	if err := st.RecordRemove("", "copilot"); err == nil {
		t.Error("want error for empty addr, got nil")
	}
}

// ─── RecordHash ───────────────────────────────────────────────────────────────

func TestRecordHash_UpdatesHash(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("sha1", "old-hash", "/path"))
	if err := st.RecordHash("repo/skill", "copilot", "new-hash"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := st.InstalledSkills["repo/skill"]["copilot"].InstalledHash; got != "new-hash" {
		t.Errorf("want new-hash, got %q", got)
	}
}

func TestRecordHash_PreservesOtherFields(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("sha1", "old-hash", "/path"))
	_ = st.RecordHash("repo/skill", "copilot", "new-hash")
	if got := st.InstalledSkills["repo/skill"]["copilot"].InstalledAtSHA; got != "sha1" {
		t.Errorf("InstalledAtSHA should not change, got %q", got)
	}
}

func TestRecordHash_ErrorWhenNotInstalled(t *testing.T) {
	st := emptyState()
	if err := st.RecordHash("repo/skill", "copilot", "new-hash"); err == nil {
		t.Error("want error for missing record, got nil")
	}
}

// ─── RecordSHA ────────────────────────────────────────────────────────────────

func TestRecordSHA_UpdatesSHA(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("old-sha", "h1", "/path"))
	if err := st.RecordSHA("repo/skill", "copilot", "new-sha"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := st.InstalledSkills["repo/skill"]["copilot"].InstalledAtSHA; got != "new-sha" {
		t.Errorf("want new-sha, got %q", got)
	}
}

func TestRecordSHA_PreservesOtherFields(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("repo/skill", "copilot", rec("old-sha", "h1", "/path"))
	_ = st.RecordSHA("repo/skill", "copilot", "new-sha")
	if got := st.InstalledSkills["repo/skill"]["copilot"].InstalledHash; got != "h1" {
		t.Errorf("InstalledHash should not change, got %q", got)
	}
}

func TestRecordSHA_ErrorWhenNotInstalled(t *testing.T) {
	st := emptyState()
	if err := st.RecordSHA("repo/skill", "copilot", "new-sha"); err == nil {
		t.Error("want error for missing record, got nil")
	}
}

// ─── RecordRenameAddr ─────────────────────────────────────────────────────────

func TestRecordRenameAddr_RenamesEntry(t *testing.T) {
	st := emptyState()
	_ = st.RecordInstall("old-repo/skill", "copilot", rec("sha1", "h1", "/path"))
	if err := st.RecordRenameAddr("old-repo/skill", "new-repo/skill"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := st.InstalledSkills["old-repo/skill"]; ok {
		t.Error("old addr should be removed")
	}
	if _, ok := st.InstalledSkills["new-repo/skill"]["copilot"]; !ok {
		t.Error("new addr should have the record")
	}
}

func TestRecordRenameAddr_NoopWhenMissing(t *testing.T) {
	st := emptyState()
	if err := st.RecordRenameAddr("nonexistent/skill", "new/skill"); err != nil {
		t.Fatalf("unexpected error for missing addr: %v", err)
	}
}

func TestRecordRenameAddr_EmptyOldAddrError(t *testing.T) {
	st := emptyState()
	if err := st.RecordRenameAddr("", "new/skill"); err == nil {
		t.Error("want error for empty oldAddr, got nil")
	}
}

func TestRecordRenameAddr_EmptyNewAddrError(t *testing.T) {
	st := emptyState()
	if err := st.RecordRenameAddr("old/skill", ""); err == nil {
		t.Error("want error for empty newAddr, got nil")
	}
}
