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
	RepoRegistering EventKind = iota // a missing repo is being registered
	RepoFailed                       // registering a repo failed (Err is set)
	SkillInstalling                  // a skill install is starting
	SkillInstalled                   // a skill install succeeded
	SkillFailed                      // a skill install failed (Err is set)
	SkillSkipped                     // a skill was skipped because its repo is unavailable (Err is set)
)

// Event is one progress notification from a deployment operation.
type Event struct {
	Kind  EventKind
	Repo  string
	URL   string // RepoRegistering only
	Skill string
	Agent string
	Err   error
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

// Outcome is the result for one skill × agent pair.
type Outcome struct {
	Skill  string
	Agent  string
	Status state.PackSkillStatus
}

// Result describes a finished deployment operation.
type Result struct {
	Address string
	// Record is the Installed Pack record as persisted to state.
	Record state.InstalledPackRecord
	// Outcomes lists every skill × agent attempted by this operation, in order.
	Outcomes []Outcome
	// Installed and Failed count skill × agent installs (not unique skills).
	Installed int
	Failed    int
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
	skillAddrs := make([]string, 0, len(rec.Skills))
	for skillAddr := range rec.Skills {
		skillAddrs = append(skillAddrs, skillAddr)
	}
	sort.Strings(skillAddrs)
	for _, skillAddr := range skillAddrs {
		agents := make([]string, 0, len(rec.Skills[skillAddr]))
		for ag := range rec.Skills[skillAddr] {
			agents = append(agents, ag)
		}
		sort.Strings(agents)
		for _, ag := range agents {
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

// add records one outcome in the result and its pack record.
func (r *Result) add(skillAddr, agent string, status state.PackSkillStatus) {
	if r.Record.Skills[skillAddr] == nil {
		r.Record.Skills[skillAddr] = make(map[string]state.PackSkillStatus)
	}
	r.Record.Skills[skillAddr][agent] = status
	r.Outcomes = append(r.Outcomes, Outcome{Skill: skillAddr, Agent: agent, Status: status})
	if status.Installed {
		r.Installed++
	} else {
		r.Failed++
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
