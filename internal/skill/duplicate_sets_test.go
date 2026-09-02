package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
)

// skillMD writes a SKILL.md at dir/relPath with the given frontmatter name
// (omitted when empty) and body, returning a repo.SkillInfo describing it.
func skillMD(t *testing.T, repoCache, repoName, relPath, name, body string) repo.SkillInfo {
	t.Helper()
	full := filepath.Join(repoCache, filepath.FromSlash(relPath))
	content := body
	if name != "" {
		content = "---\nname: " + name + "\n---\n" + body
	}
	writeFile(t, filepath.Join(full, "SKILL.md"), content)
	return repo.SkillInfo{
		Address:  repoName + "/" + relPath,
		RepoName: repoName,
		RelPath:  relPath,
		FullPath: full,
	}
}

func TestDetectDuplicateSets_NoDuplicates(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "triage", "", "# Triage"),
		skillMD(t, repoCacheB, "repo-b", "debugger", "", "# Debugger"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("expected no duplicate sets, got %v", sets)
	}
}

func TestDetectDuplicateSets_IdenticalContent(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "triage", "", "# Triage\nsame content"),
		skillMD(t, repoCacheB, "repo-b", "triage", "", "# Triage\nsame content"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 duplicate set, got %d: %v", len(sets), sets)
	}
	set := sets[0]
	if set.Basename != "triage" {
		t.Errorf("Basename = %q, want triage", set.Basename)
	}
	if len(set.Members) != 2 {
		t.Fatalf("expected 2 members, got %v", set.Members)
	}
	if len(set.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %v", set.Pairs)
	}
	pair := set.Pairs[0]
	if pair.Confidence != skill.ConfidenceIdentical {
		t.Errorf("Confidence = %q, want %q", pair.Confidence, skill.ConfidenceIdentical)
	}
	if pair.LinkStatus != skill.LinkStatusUnlinked {
		t.Errorf("LinkStatus = %q, want %q", pair.LinkStatus, skill.LinkStatusUnlinked)
	}
}

func TestDetectDuplicateSets_DivergedContent(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "triage", "", "# Triage v1"),
		skillMD(t, repoCacheB, "repo-b", "triage", "", "# Triage v2 (different)"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 duplicate set, got %d: %v", len(sets), sets)
	}
	pair := sets[0].Pairs[0]
	if pair.Confidence != skill.ConfidenceDiverged {
		t.Errorf("Confidence = %q, want %q", pair.Confidence, skill.ConfidenceDiverged)
	}
	if pair.LinkStatus != skill.LinkStatusUnlinked {
		t.Errorf("LinkStatus = %q, want %q", pair.LinkStatus, skill.LinkStatusUnlinked)
	}
}

func TestDetectDuplicateSets_LinkedFork_Identical(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	same := "# Triage\nshared content"
	a := skillMD(t, repoCacheA, "repo-a", "triage", "", same)
	skillMD(t, repoCacheB, "repo-b", "triage", "", same)
	writeFile(t, filepath.Join(a.FullPath, ".skillpack-fork"),
		`{"upstream_addr":"repo-b/triage","upstream_sha":"abc123"}`)

	skills := []repo.SkillInfo{
		a,
		{Address: "repo-b/triage", RepoName: "repo-b", RelPath: "triage", FullPath: filepath.Join(repoCacheB, "triage")},
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 duplicate set, got %d", len(sets))
	}
	pair := sets[0].Pairs[0]
	if pair.LinkStatus != skill.LinkStatusLinked {
		t.Errorf("LinkStatus = %q, want %q", pair.LinkStatus, skill.LinkStatusLinked)
	}
	if pair.Confidence != skill.ConfidenceIdentical {
		t.Errorf("Confidence = %q, want %q", pair.Confidence, skill.ConfidenceIdentical)
	}
}

func TestDetectDuplicateSets_LinkedFork_Diverged(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	a := skillMD(t, repoCacheA, "repo-a", "triage", "", "# Triage v1")
	skillMD(t, repoCacheB, "repo-b", "triage", "", "# Triage v2, edited since fork")
	writeFile(t, filepath.Join(a.FullPath, ".skillpack-fork"),
		`{"upstream_addr":"repo-b/triage","upstream_sha":"abc123"}`)

	skills := []repo.SkillInfo{
		a,
		{Address: "repo-b/triage", RepoName: "repo-b", RelPath: "triage", FullPath: filepath.Join(repoCacheB, "triage")},
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pair := sets[0].Pairs[0]
	// A "linked (fork)" pair that has diverged must still report as diverged —
	// "already tracked" must not quietly imply "assumed fine" (ADR-0003 decision 5).
	if pair.LinkStatus != skill.LinkStatusLinked {
		t.Errorf("LinkStatus = %q, want %q", pair.LinkStatus, skill.LinkStatusLinked)
	}
	if pair.Confidence != skill.ConfidenceDiverged {
		t.Errorf("Confidence = %q, want %q", pair.Confidence, skill.ConfidenceDiverged)
	}
}

func TestDetectDuplicateSets_FrontmatterNameDisagrees_Excluded(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "utils", "string-utils", "# String utils"),
		skillMD(t, repoCacheB, "repo-b", "utils", "date-utils", "# Date utils"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("expected disagreeing frontmatter names to be excluded, got %v", sets)
	}
}

func TestDetectDuplicateSets_FrontmatterNameAgrees_StillMatches(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "utils", "shared-utils", "# Utils v1"),
		skillMD(t, repoCacheB, "repo-b", "utils", "shared-utils", "# Utils v2"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 duplicate set, got %d", len(sets))
	}
}

func TestDetectDuplicateSets_MissingFrontmatterDoesNotBlockMatch(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "triage", "triage", "# Triage"),
		skillMD(t, repoCacheB, "repo-b", "triage", "", "# Triage, no frontmatter"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 duplicate set (missing frontmatter should not block matching), got %d", len(sets))
	}
}

func TestDetectDuplicateSets_SameRepoOnly_NotReported(t *testing.T) {
	repoCache := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCache, "repo-a", "triage", "", "# Triage"),
		skillMD(t, repoCache, "repo-a", "nested/triage", "", "# Triage nested"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("expected same-repo-only matches to be excluded (Duplicate Set requires ≥2 repos), got %v", sets)
	}
}

func TestDetectDuplicateSets_ThreeMembers(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	repoCacheC := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "triage", "", "# Triage v1"),
		skillMD(t, repoCacheB, "repo-b", "triage", "", "# Triage v1"),
		skillMD(t, repoCacheC, "repo-c", "triage", "", "# Triage v2 (different)"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected 1 duplicate set, got %d: %v", len(sets), sets)
	}
	set := sets[0]
	if len(set.Members) != 3 {
		t.Fatalf("expected 3 members, got %v", set.Members)
	}
	if len(set.Pairs) != 3 {
		t.Fatalf("expected 3 pairs (3 choose 2), got %d: %v", len(set.Pairs), set.Pairs)
	}
	var identical, diverged int
	for _, p := range set.Pairs {
		switch p.Confidence {
		case skill.ConfidenceIdentical:
			identical++
		case skill.ConfidenceDiverged:
			diverged++
		}
	}
	if identical != 1 || diverged != 2 {
		t.Errorf("expected 1 identical + 2 diverged pairs, got %d identical, %d diverged", identical, diverged)
	}
}

func TestDetectDuplicateSets_NoSkills_EmptyResult(t *testing.T) {
	sets, err := skill.DetectDuplicateSets(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("expected empty result, got %v", sets)
	}
}

func TestDetectDuplicateSets_SortedOutput(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "zeta", "", "# Zeta"),
		skillMD(t, repoCacheB, "repo-b", "zeta", "", "# Zeta"),
		skillMD(t, repoCacheA, "repo-a", "alpha", "", "# Alpha"),
		skillMD(t, repoCacheB, "repo-b", "alpha", "", "# Alpha"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("expected 2 duplicate sets, got %d", len(sets))
	}
	if sets[0].Basename != "alpha" || sets[1].Basename != "zeta" {
		t.Errorf("expected sets sorted by basename, got %q, %q", sets[0].Basename, sets[1].Basename)
	}
}

// Ensure the package compiles against os for path existence checks used by
// the SKILL.md-less directory case (mirrors fork_candidates_test.go's
// TestDetectForkCandidates_NoSKILLmd style coverage for the doctor path).
func TestDetectDuplicateSets_DirWithoutSKILLmd_NotDiscovered(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoCacheB, "triage"), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "triage", "", "# Triage"),
		// repo-b/triage has no SKILL.md; a real caller (repo.DiscoverAllSkills)
		// would never include it, so it must never appear in the input here.
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("expected no duplicate sets with only one real skill, got %v", sets)
	}
}

// An unnamed member must never bridge two conflicting explicit names together
// via transitive matching (basename "utils": A=string-utils, B=<no name>,
// C=date-utils must NOT all end up in one set just because B is compatible
// with both individually).
func TestDetectDuplicateSets_UnnamedMemberDoesNotBridgeConflictingNames(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	repoCacheC := t.TempDir()
	skills := []repo.SkillInfo{
		skillMD(t, repoCacheA, "repo-a", "utils", "string-utils", "# String utils"),
		skillMD(t, repoCacheB, "repo-b", "utils", "", "# Utils, no frontmatter"),
		skillMD(t, repoCacheC, "repo-c", "utils", "date-utils", "# Date utils"),
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, set := range sets {
		if len(set.Members) > 2 {
			t.Fatalf("expected no set to bridge string-utils and date-utils via the unnamed member, got %v", set)
		}
		for _, m := range set.Members {
			if m == "repo-a/utils" {
				for _, other := range set.Members {
					if other == "repo-c/utils" {
						t.Fatalf("string-utils and date-utils must not share a set, got %v", set.Members)
					}
				}
			}
		}
	}
}

func TestDetectDuplicateSets_CRLFFrontmatter(t *testing.T) {
	repoCacheA := t.TempDir()
	repoCacheB := t.TempDir()
	full := filepath.Join(repoCacheA, "triage")
	writeFile(t, filepath.Join(full, "SKILL.md"), "---\r\nname: triage\r\n---\r\n# Triage")
	a := repo.SkillInfo{Address: "repo-a/triage", RepoName: "repo-a", RelPath: "triage", FullPath: full}
	b := skillMD(t, repoCacheB, "repo-b", "triage", "triage", "# Triage")

	skills := []repo.SkillInfo{a, b}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("expected CRLF frontmatter to still be parsed and matched, got %d sets", len(sets))
	}
}

