// Package doctor diagnoses the health of a skillpack setup and describes, for
// each problem, the fix the caller can offer. Diagnose is read-only: it never
// writes files or state. Applying a fix is the caller's job and goes through
// the existing operation for that kind (relink, pack complete, fork
// registration).
package doctor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmaltais/skillpack/internal/pack"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// Kind classifies a Finding and decides which fix applies.
type Kind string

const (
	// StaleSkill: installed, but its address no longer exists in any repo
	// cache. Fix: relink to a replacement address.
	StaleSkill Kind = "stale-skill"
	// BrokenUpstream: a fork whose UpstreamAddr no longer exists. Fix: set a
	// new upstream or clear it.
	BrokenUpstream Kind = "broken-upstream"
	// PartialPack: an installed pack with skills that failed to install.
	// Fix: complete the deployment.
	PartialPack Kind = "partial-pack"
	// ForkCandidate: an installed skill with no fork provenance that matches a
	// skill in another repo. Fix: register it as a fork of Related.
	ForkCandidate Kind = "fork-candidate"
	// Duplicate: one pair inside a Duplicate Set. Diverged pairs offer a diff;
	// identical pairs are informational (no Fix).
	Duplicate Kind = "duplicate"
)

// Finding is one problem and the fix on offer. All fields are plain data so
// the TUI never holds domain types.
type Finding struct {
	Kind    Kind
	Subject string // skill address, pack address, or duplicate member A
	Related string // fork-candidate upstream, or duplicate member B
	Agent   string // an agent the skill is installed for (stale / broken)
	Detail  string // what is wrong, one line
	Fix     string // what Enter will do; empty means informational only
}

// Diagnose collects every finding for st. canWrite (may be nil) restricts
// fork candidates to skills whose repo the user can push to, as the Status
// panel does. Findings are ordered by kind, then subject.
func Diagnose(st *state.State, canWrite func(string) bool) ([]Finding, error) {
	skills, err := repo.DiscoverAllSkills(st)
	if err != nil {
		return nil, err
	}
	discovered := make(map[string]bool, len(skills))
	for _, s := range skills {
		discovered[s.Address] = true
	}

	var out []Finding
	out = append(out, skillFindings(st, discovered)...)
	out = append(out, packFindings(st)...)

	cands, err := skill.DetectForkCandidates(st, canWrite)
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		out = append(out, Finding{
			Kind: ForkCandidate, Subject: c.Addr, Related: c.CandidateUpstream,
			Detail: "no fork provenance, but " + c.CandidateUpstream + " looks like its origin",
			Fix:    "Register as a fork of " + c.CandidateUpstream,
		})
	}

	sets, err := skill.DetectDuplicateSets(skills)
	if err != nil {
		return nil, err
	}
	for _, set := range sets {
		for _, p := range set.Pairs {
			f := Finding{
				Kind: Duplicate, Subject: p.A, Related: p.B,
				Detail: p.A + " and " + p.B + ": " + p.Confidence + ", " + p.LinkStatus,
			}
			if p.Confidence == skill.ConfidenceDiverged {
				f.Fix = "View diff between the copies"
			}
			out = append(out, f)
		}
	}

	rank := map[Kind]int{StaleSkill: 0, BrokenUpstream: 1, PartialPack: 2, ForkCandidate: 3, Duplicate: 4}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Kind] != rank[out[j].Kind] {
			return rank[out[i].Kind] < rank[out[j].Kind]
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Related < out[j].Related
	})
	return out, nil
}

// skillFindings reports stale and broken-upstream installed skills using only
// local stat calls (no git or network).
func skillFindings(st *state.State, discovered map[string]bool) []Finding {
	var out []Finding
	for addr, agents := range st.InstalledSkills {
		agent := firstAgent(agents)
		if !discovered[addr] {
			out = append(out, Finding{
				Kind: StaleSkill, Subject: addr, Agent: agent,
				Detail: "installed, but no longer exists in any registered repo",
				Fix:    "Relink to a replacement address",
			})
			continue
		}
		for _, rec := range agents {
			if rec.UpstreamAddr == "" {
				continue
			}
			if upstreamMissing(st, rec.UpstreamAddr) {
				out = append(out, Finding{
					Kind: BrokenUpstream, Subject: addr, Related: rec.UpstreamAddr, Agent: agent,
					Detail: "fork upstream " + rec.UpstreamAddr + " no longer exists",
					Fix:    "Set a new upstream, or clear it",
				})
			}
			break // one agent record is enough to determine fork status
		}
	}
	return out
}

// upstreamMissing reports whether upstream names a skill that is gone from a
// registered, real git clone. Unregistered repos and non-clones are unknown,
// not broken, so they never flag.
func upstreamMissing(st *state.State, upstream string) bool {
	parts := strings.SplitN(upstream, "/", 2)
	if len(parts) != 2 {
		return false
	}
	rec, ok := st.Repos[parts[0]]
	if !ok {
		return false
	}
	if _, err := os.Stat(filepath.Join(rec.CachePath, ".git")); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(rec.CachePath, filepath.FromSlash(parts[1]), "SKILL.md"))
	return os.IsNotExist(err)
}

func packFindings(st *state.State) []Finding {
	var out []Finding
	for addr, rec := range st.InstalledPacks {
		if pack.IsPartial(rec) {
			out = append(out, Finding{
				Kind: PartialPack, Subject: addr,
				Detail: "some skills failed to install",
				Fix:    "Complete the deployment",
			})
		}
	}
	return out
}

func firstAgent(agents map[string]state.InstalledSkillRecord) string {
	names := make([]string, 0, len(agents))
	for n := range agents {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}
