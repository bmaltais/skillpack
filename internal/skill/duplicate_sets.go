package skill

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmaltais/skillpack/internal/repo"
	"gopkg.in/yaml.v3"
)

// Confidence and link-status labels, matching the CONTEXT.md "Duplicate Set" vocabulary.
const (
	ConfidenceIdentical = "identical"
	ConfidenceDiverged  = "diverged"

	LinkStatusLinked   = "linked (fork)"
	LinkStatusUnlinked = "unlinked"
)

// DuplicatePair annotates one pair of members within a Duplicate Set.
type DuplicatePair struct {
	A, B       string // skill addresses; A sorts before B
	Confidence string // ConfidenceIdentical or ConfidenceDiverged
	LinkStatus string // LinkStatusLinked or LinkStatusUnlinked
}

// DuplicateSet is a group of skill addresses across ≥2 registered repos that
// appear to be the same skill. See ADR-0003 and the "Duplicate Set" entry in
// CONTEXT.md.
type DuplicateSet struct {
	// Basename is the shared directory name that identifies this set. It is
	// recomputed on every run rather than persisted (see ADR-0003 decision 2).
	Basename string
	Members  []string        // skill addresses, sorted
	Pairs    []DuplicatePair // every pairwise combination of Members, sorted
}

// DetectDuplicateSets scans skills (typically produced by
// repo.DiscoverAllSkills) and groups those that appear to be copies of the
// same skill living in two or more different repos.
//
// Matching is basename-first, then cross-checked against each skill's
// SKILL.md frontmatter `name:` field: members whose declared name disagrees
// with another member's are not merged into the same set, cutting noise from
// coincidentally same-named-but-unrelated directories (e.g. two unrelated
// "utils" folders). A missing/unparsable name does not constrain matching.
//
// Pure and read-only: it makes no writes, no network calls, and depends on
// nothing beyond the supplied skill list.
func DetectDuplicateSets(skills []repo.SkillInfo) ([]DuplicateSet, error) {
	byBasename := make(map[string][]repo.SkillInfo)
	for _, s := range skills {
		base := pathpkg.Base(s.RelPath)
		byBasename[base] = append(byBasename[base], s)
	}

	basenames := make([]string, 0, len(byBasename))
	for b := range byBasename {
		basenames = append(basenames, b)
	}
	sort.Strings(basenames)

	var sets []DuplicateSet
	for _, base := range basenames {
		group := byBasename[base]
		if len(group) < 2 {
			continue
		}
		groupSets, err := detectSetsInGroup(base, group)
		if err != nil {
			return nil, err
		}
		sets = append(sets, groupSets...)
	}
	return sets, nil
}

// detectSetsInGroup splits a basename-matched group of skills into one or
// more Duplicate Sets, using frontmatter name compatibility (connected
// components) to keep disagreeing members apart.
func detectSetsInGroup(basename string, group []repo.SkillInfo) ([]DuplicateSet, error) {
	names := make([]string, len(group))
	for i, s := range group {
		n, err := parseSkillName(filepath.Join(s.FullPath, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("parsing SKILL.md frontmatter for %s: %w", s.Address, err)
		}
		names[i] = n
	}

	parent := make([]int, len(group))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	union := func(i, j int) {
		ri, rj := find(i), find(j)
		if ri != rj {
			parent[ri] = rj
		}
	}
	// Union-find groups members into connected components so a chain of
	// name-compatible members stays together even if not every pair agrees
	// directly (compatibility is transitive via a shared/missing name).
	for i := 0; i < len(group); i++ {
		for j := i + 1; j < len(group); j++ {
			if namesCompatible(names[i], names[j]) {
				union(i, j)
			}
		}
	}

	components := make(map[int][]int)
	for i := range group {
		components[find(i)] = append(components[find(i)], i)
	}
	roots := make([]int, 0, len(components))
	for r := range components {
		roots = append(roots, r)
	}
	sort.Ints(roots)

	var sets []DuplicateSet
	for _, r := range roots {
		idxs := components[r]
		if len(idxs) < 2 {
			continue
		}
		repoNames := make(map[string]bool)
		for _, i := range idxs {
			repoNames[group[i].RepoName] = true
		}
		if len(repoNames) < 2 {
			continue // same-repo duplicates only; a Duplicate Set requires ≥2 repos
		}
		set, err := buildDuplicateSet(basename, group, idxs)
		if err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	return sets, nil
}

// namesCompatible reports whether two SKILL.md frontmatter `name:` values are
// consistent with the skills sharing an identity. A missing name (empty
// string) does not constrain matching, so directories without frontmatter
// still match on basename alone.
func namesCompatible(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	return a == b
}

func buildDuplicateSet(basename string, group []repo.SkillInfo, idxs []int) (DuplicateSet, error) {
	byAddr := make(map[string]repo.SkillInfo, len(idxs))
	members := make([]string, 0, len(idxs))
	for _, i := range idxs {
		byAddr[group[i].Address] = group[i]
		members = append(members, group[i].Address)
	}
	sort.Strings(members)

	var pairs []DuplicatePair
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			pair, err := buildDuplicatePair(members[i], byAddr[members[i]], members[j], byAddr[members[j]])
			if err != nil {
				return DuplicateSet{}, err
			}
			pairs = append(pairs, pair)
		}
	}

	return DuplicateSet{Basename: basename, Members: members, Pairs: pairs}, nil
}

func buildDuplicatePair(addrA string, a repo.SkillInfo, addrB string, b repo.SkillInfo) (DuplicatePair, error) {
	hashA, err := hashSkill(addrA, a.FullPath)
	if err != nil {
		return DuplicatePair{}, err
	}
	hashB, err := hashSkill(addrB, b.FullPath)
	if err != nil {
		return DuplicatePair{}, err
	}

	confidence := ConfidenceDiverged
	if hashA == hashB {
		confidence = ConfidenceIdentical
	}

	linkStatus := LinkStatusUnlinked
	if forkLinksTo(a.FullPath, addrB) || forkLinksTo(b.FullPath, addrA) {
		linkStatus = LinkStatusLinked
	}

	return DuplicatePair{A: addrA, B: addrB, Confidence: confidence, LinkStatus: linkStatus}, nil
}

// hashSkill wraps ComputeHash with error context naming the skill address.
func hashSkill(addr, fullPath string) (string, error) {
	hash, err := ComputeHash(fullPath)
	if err != nil {
		return "", fmt.Errorf("computing hash for %s: %w", addr, err)
	}
	return hash, nil
}

// forkLinksTo reports whether the .skillpack-fork metadata at skillDir
// records upstreamAddr as its upstream origin.
func forkLinksTo(skillDir, upstreamAddr string) bool {
	meta, err := readForkMetadata(skillDir)
	if err != nil || meta == nil {
		return false
	}
	return meta.UpstreamAddr == upstreamAddr
}

// parseSkillName extracts the `name:` field from a SKILL.md file's YAML
// frontmatter block (the content between the first two "---" lines).
// Returns "" (no error) when the file is missing, has no frontmatter, no
// name field, or the frontmatter fails to parse — matching degrades
// gracefully to basename-only rather than failing the whole detection run
// over one malformed file.
func parseSkillName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	content := string(data)
	const delim = "---"
	if !strings.HasPrefix(content, delim) {
		return "", nil
	}
	rest := content[len(delim):]
	end := strings.Index(rest, "\n"+delim)
	if end == -1 {
		return "", nil
	}
	frontmatter := rest[:end]

	var meta struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return "", nil
	}
	return strings.TrimSpace(meta.Name), nil
}
