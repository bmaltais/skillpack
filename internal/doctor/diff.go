package doctor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/state"
)

// metadataFile is skillpack's own provenance file; it is not skill content.
const metadataFile = ".skillpack-fork"

// contextLines is how many unchanged lines to keep around each change.
const contextLines = 2

// Diff returns a human-readable, line-based diff between two skills in the
// repo caches: files only in one copy, and for files in both, the changed
// lines. Read-only. Returns "no differences" text when the copies match.
func Diff(st *state.State, addrA, addrB string) (string, error) {
	a, err := repo.FindSkill(addrA, st)
	if err != nil {
		return "", err
	}
	b, err := repo.FindSkill(addrB, st)
	if err != nil {
		return "", err
	}
	return diffDirs(addrA, a.FullPath, addrB, b.FullPath)
}

func diffDirs(nameA, dirA, nameB, dirB string) (string, error) {
	filesA, err := listFiles(dirA)
	if err != nil {
		return "", err
	}
	filesB, err := listFiles(dirB)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	var all []string
	for _, f := range append(append([]string{}, filesA...), filesB...) {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	sort.Strings(all)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", nameA, nameB)
	changed := false
	for _, rel := range all {
		ca, errA := os.ReadFile(filepath.Join(dirA, rel))
		cb, errB := os.ReadFile(filepath.Join(dirB, rel))
		switch {
		case errA != nil && errB != nil:
			continue
		case errA != nil:
			fmt.Fprintf(&sb, "\nOnly in %s: %s\n", nameB, rel)
		case errB != nil:
			fmt.Fprintf(&sb, "\nOnly in %s: %s\n", nameA, rel)
		case string(ca) != string(cb):
			fmt.Fprintf(&sb, "\n@@ %s\n%s", rel, lineDiff(string(ca), string(cb)))
		default:
			continue
		}
		changed = true
	}
	if !changed {
		sb.WriteString("\nno differences\n")
	}
	return sb.String(), nil
}

func listFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == metadataFile {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

// lineDiff renders "- removed", "+ added" and "  context" lines, collapsing
// long unchanged runs to contextLines at each edge.
func lineDiff(a, b string) string {
	dmp := diffmatchpatch.New()
	ca, cb, lines := dmp.DiffLinesToChars(a, b)
	diffs := dmp.DiffCharsToLines(dmp.DiffMain(ca, cb, false), lines)

	var sb strings.Builder
	emit := func(prefix, text string) {
		for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			sb.WriteString(prefix + l + "\n")
		}
	}
	for i, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			emit("+ ", d.Text)
		case diffmatchpatch.DiffDelete:
			emit("- ", d.Text)
		default:
			ls := strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n")
			head, tail := contextLines, contextLines
			if i == 0 {
				head = 0
			}
			if i == len(diffs)-1 {
				tail = 0
			}
			if len(ls) <= head+tail+1 {
				emit("  ", d.Text)
				continue
			}
			for _, l := range ls[:head] {
				sb.WriteString("  " + l + "\n")
			}
			sb.WriteString("  …\n")
			for _, l := range ls[len(ls)-tail:] {
				sb.WriteString("  " + l + "\n")
			}
		}
	}
	return sb.String()
}
