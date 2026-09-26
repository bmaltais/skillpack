package pack

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// EventKind identifies a step reported through Options.Progress.
type EventKind int

const (
	RepoRegistering   EventKind = iota // a missing repo is being registered
	RepoFailed                         // registering a repo failed (Err is set)
	SkillInstalling                    // a skill install is starting
	SkillInstalled                     // a skill install succeeded
	SkillFailed                        // a skill install failed (Err is set)
	SkillSkipped                       // a skill was skipped because its repo is unavailable (Err is set)
	RepoPulling                        // Update: a repo is being pulled
	RepoPullFailed                     // Update: pulling a repo failed (Err is set)
	RepoPullWarning                    // Update: pulling a repo produced a warning (Message is set)
	SkillUpdating                      // Update: a skill update is starting
	SkillUpdated                       // Update: a skill was updated
	SkillBlocked                       // Update: a skill was not updated (Err says why)
	SkillRemoving                      // Remove: a skill removal is starting
	SkillRemoved                       // Remove: a skill was removed
	SkillKept                          // Remove: a skill was kept because of local modifications
	SkillNotInstalled                  // Remove: a skill was not installed for the agent (Err is set)
)

// Event is one progress notification from a deployment operation.
type Event struct {
	Kind    EventKind
	Repo    string
	URL     string // RepoRegistering only
	Skill   string
	Agent   string
	Err     error
	Message string // RepoPullWarning only
}

// Options tunes a deployment operation.
type Options struct {
	// Progress, when non-nil, is called as the operation advances. Nil means quiet.
	Progress func(Event)
}

func (o Options) emit(e Event) {
	if o.Progress != nil {
		o.Progress(e)
	}
}

// OutcomeKind classifies what an operation did for one skill × agent pair.
type OutcomeKind int

const (
	OutcomeInstalled OutcomeKind = iota
	OutcomeUpdated
	OutcomeRemoved
	OutcomeFailed  // install, update or removal failed
	OutcomeBlocked // update not applied because of a Conflict
	OutcomeKept    // removal skipped because of a Local Modification
)

// Outcome is the result for one skill × agent pair.
type Outcome struct {
	Skill string
	Agent string
	Kind  OutcomeKind
	// Status is the pack-record status after the operation.
	Status state.PackSkillStatus
	// Message explains a Failed, Blocked or Kept outcome.
	Message string
}

// Result describes a finished deployment operation.
type Result struct {
	Address string
	// Record is the Installed Pack record as persisted to state.
	Record state.InstalledPackRecord
	// Outcomes lists every skill × agent attempted by this operation, in order.
	Outcomes []Outcome
	// Counts of skill × agent outcomes by kind (not unique skills).
	Installed int
	Updated   int
	Removed   int
	Failed    int
	Blocked   int
	Kept      int
	// Remove only: PackRemoved is true when the pack record was dropped;
	// otherwise RemainingAgents lists the agents it is still deployed for.
	PackRemoved     bool
	RemainingAgents []string
}

// Partial reports whether the persisted deployment is a Partial Pack Deployment.
func (r *Result) Partial() bool { return IsPartial(r.Record) }

// IsPartial reports whether any skill in rec is not installed.
func IsPartial(rec state.InstalledPackRecord) bool {
	for _, agentStatuses := range rec.Skills {
		for _, s := range agentStatuses {
			if !s.Installed {
				return true
			}
		}
	}
	return false
}

// Install deploys every skill of def for agents: it registers any missing repos,
// installs each skill, records the Pack Deployment in state and saves state.
// Per-skill failures never abort the operation; they are recorded as a Partial
// Pack Deployment. The returned error covers only record persistence.
func Install(cfg *config.Config, st *state.State, def *Definition, agents []string, opts Options) (*Result, error) {
	repoErrors := ensureRepos(def.Pack, cfg, st, opts)

	res := &Result{
		Address: def.Address,
		Record: state.InstalledPackRecord{
			PackAddress: def.Address,
			InstalledAt: time.Now(),
			Agents:      agents,
			Skills:      make(map[string]map[string]state.PackSkillStatus),
		},
	}
	for _, skillAddr := range def.Pack.Skills {
		res.Record.Skills[skillAddr] = make(map[string]state.PackSkillStatus)
		for _, ag := range agents {
			res.add(skillAddr, ag, deploySkill(cfg, st, skillAddr, ag, repoErrors, opts))
		}
	}

	if err := persist(st, res); err != nil {
		return nil, err
	}
	return res, nil
}

// Complete retries every skill of the Installed Pack packAddr that is not
// installed. When the pack's recipe can still be resolved from packAddr, its
// missing repos are registered again first (the usual cause of a Partial Pack
// Deployment); otherwise only the installs are retried.
func Complete(cfg *config.Config, st *state.State, packAddr string, opts Options) (*Result, error) {
	rec, ok := st.InstalledPacks[packAddr]
	if !ok {
		return nil, fmt.Errorf("pack %q not found in state", packAddr)
	}

	var repoErrors map[string]error
	if def, err := Resolve(packAddr, st); err == nil {
		repoErrors = ensureRepos(def.Pack, cfg, st, opts)
	}

	res := &Result{Address: packAddr, Record: rec}
	for _, skillAddr := range sortedKeys(rec.Skills) {
		for _, ag := range sortedKeys(rec.Skills[skillAddr]) {
			if rec.Skills[skillAddr][ag].Installed {
				continue
			}
			res.add(skillAddr, ag, deploySkill(cfg, st, skillAddr, ag, repoErrors, opts))
		}
	}

	if err := persist(st, res); err != nil {
		return nil, err
	}
	return res, nil
}

// add records an install outcome in the result and its pack record.
func (r *Result) add(skillAddr, agent string, status state.PackSkillStatus) {
	r.setStatus(skillAddr, agent, status)
	kind := OutcomeInstalled
	if !status.Installed {
		kind = OutcomeFailed
	}
	r.report(Outcome{Skill: skillAddr, Agent: agent, Kind: kind, Status: status, Message: status.Error})
}

// setStatus writes one skill × agent status into the pack record.
func (r *Result) setStatus(skillAddr, agent string, status state.PackSkillStatus) {
	if r.Record.Skills[skillAddr] == nil {
		r.Record.Skills[skillAddr] = make(map[string]state.PackSkillStatus)
	}
	r.Record.Skills[skillAddr][agent] = status
}

// report appends an outcome and counts it. It does not touch the pack record.
func (r *Result) report(o Outcome) {
	r.Outcomes = append(r.Outcomes, o)
	switch o.Kind {
	case OutcomeInstalled:
		r.Installed++
	case OutcomeUpdated:
		r.Updated++
	case OutcomeRemoved:
		r.Removed++
	case OutcomeFailed:
		r.Failed++
	case OutcomeBlocked:
		r.Blocked++
	case OutcomeKept:
		r.Kept++
	}
}

func persist(st *state.State, res *Result) error {
	if err := st.RecordPackInstall(res.Address, res.Record); err != nil {
		return err
	}
	return state.Save(st)
}

// deploySkill installs one skill for one agent, unless its repo is unavailable.
func deploySkill(cfg *config.Config, st *state.State, skillAddr, agent string, repoErrors map[string]error, opts Options) state.PackSkillStatus {
	repoName := repoNameFromAddr(skillAddr)
	if repoErr, bad := repoErrors[repoName]; bad {
		opts.emit(Event{Kind: SkillSkipped, Repo: repoName, Skill: skillAddr, Agent: agent, Err: repoErr})
		return state.PackSkillStatus{Error: fmt.Sprintf("repo unavailable: %v", repoErr)}
	}
	opts.emit(Event{Kind: SkillInstalling, Skill: skillAddr, Agent: agent})
	if err := skill.Install(skillAddr, agent, cfg, st, false); err != nil {
		opts.emit(Event{Kind: SkillFailed, Skill: skillAddr, Agent: agent, Err: err})
		return state.PackSkillStatus{Error: err.Error()}
	}
	opts.emit(Event{Kind: SkillInstalled, Skill: skillAddr, Agent: agent})
	return state.PackSkillStatus{Installed: true}
}

// ensureRepos registers any repos listed in the pack that are not yet
// registered. Returns repoName → error for repos that could not be added.
func ensureRepos(pk *Pack, cfg *config.Config, st *state.State, opts Options) map[string]error {
	errs := make(map[string]error)
	for _, r := range pk.Repos {
		if _, exists := st.Repos[r.Name]; exists {
			continue
		}
		opts.emit(Event{Kind: RepoRegistering, Repo: r.Name, URL: r.URL})
		if _, err := repo.Add(r.Name, r.URL, cfg.TokenForRepo(r.Name), st); err != nil {
			opts.emit(Event{Kind: RepoFailed, Repo: r.Name, URL: r.URL, Err: err})
			errs[r.Name] = err
		}
	}
	return errs
}

// repoNameFromAddr returns the repo-name component of a skill address.
func repoNameFromAddr(addr string) string {
	name, _, _ := strings.Cut(addr, "/")
	return name
}

// sortedKeys returns the keys of m in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
