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
// more Duplicate Sets, using each member's declared frontmatter name to keep
// disagreeing identities apart.
func detectSetsInGroup(basename string, group []repo.SkillInfo) ([]DuplicateSet, error) {
	names := make([]string, len(group))
	distinctNames := make(map[string]bool)
	for i, s := range group {
		n, err := parseSkillName(filepath.Join(s.FullPath, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("parsing SKILL.md frontmatter for %s: %w", s.Address, err)
		}
		names[i] = n
		if n != "" {
			distinctNames[n] = true
		}
	}

	var clusters [][]int
	if len(distinctNames) <= 1 {
		// At most one declared name in the whole group: no conflict is
		// possible, so unnamed members can safely join it too.
		all := make([]int, len(group))
		for i := range group {
			all[i] = i
		}
		clusters = [][]int{all}
	} else {
		// Two or more distinct declared names: each name is its own cluster.
		// Members with no declared name are ambiguous between the conflicting
		// identities, so they're excluded rather than bridging clusters together.
		byName := make(map[string][]int)
		for i, n := range names {
			if n != "" {
				byName[n] = append(byName[n], i)
			}
		}
		clusterNames := make([]string, 0, len(byName))
		for n := range byName {
			clusterNames = append(clusterNames, n)
		}
		sort.Strings(clusterNames)
		for _, n := range clusterNames {
			clusters = append(clusters, byName[n])
		}
	}

	var sets []DuplicateSet
	for _, idxs := range clusters {
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

func buildDuplicateSet(basename string, group []repo.SkillInfo, idxs []int) (DuplicateSet, error) {
	byAddr := make(map[string]repo.SkillInfo, len(idxs))
	members := make([]string, 0, len(idxs))
	for _, i := range idxs {
		byAddr[group[i].Address] = group[i]
		members = append(members, group[i].Address)
	}
	sort.Strings(members)

	// Compute each member's hash and fork upstream once, not once per pair
	// (ComputeHash walks the whole directory tree).
	hashes := make(map[string]string, len(members))
	forkUpstreams := make(map[string]string, len(members))
	for _, addr := range members {
		info := byAddr[addr]
		hash, err := hashSkill(addr, info.FullPath)
		if err != nil {
			return DuplicateSet{}, err
		}
		hashes[addr] = hash
		if meta, err := readForkMetadata(info.FullPath); err == nil && meta != nil {
			forkUpstreams[addr] = meta.UpstreamAddr
		}
	}

	var pairs []DuplicatePair
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			pairs = append(pairs, buildDuplicatePair(members[i], members[j], hashes, forkUpstreams))
		}
	}

	return DuplicateSet{Basename: basename, Members: members, Pairs: pairs}, nil
}

func buildDuplicatePair(addrA, addrB string, hashes, forkUpstreams map[string]string) DuplicatePair {
	confidence := ConfidenceDiverged
	if hashes[addrA] == hashes[addrB] {
		confidence = ConfidenceIdentical
	}

	linkStatus := LinkStatusUnlinked
	if forkUpstreams[addrA] == addrB || forkUpstreams[addrB] == addrA {
		linkStatus = LinkStatusLinked
	}

	return DuplicatePair{A: addrA, B: addrB, Confidence: confidence, LinkStatus: linkStatus}
}

// hashSkill wraps ComputeHash with error context naming the skill address.
func hashSkill(addr, fullPath string) (string, error) {
	hash, err := ComputeHash(fullPath)
	if err != nil {
		return "", fmt.Errorf("computing hash for %s: %w", addr, err)
	}
	return hash, nil
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

	// Normalise CRLF so the "\n---" closing-delimiter search also matches
	// SKILL.md files with Windows line endings.
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
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
