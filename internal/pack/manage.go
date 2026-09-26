package pack

import (
	"fmt"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
	"github.com/bmaltais/skillpack/internal/state"
)

// RemoveOptions tunes Remove.
type RemoveOptions struct {
	// Force also removes skills that have Local Modifications. Without it
	// those skills are kept as ordinary Installed Skills.
	Force bool
	// Progress, when non-nil, is called as the operation advances.
	Progress func(Event)
}

// Update pulls every repo of the Installed Pack packAddr, installs skills that
// are missing and updates skills with upstream changes, then saves state.
//
// A skill in Conflict (upstream changed and locally modified) is never
// overwritten: it is reported as Blocked. A failed or blocked update leaves the
// skill's pack status untouched, so the deployment does not become partial.
func Update(cfg *config.Config, st *state.State, packAddr string, opts Options) (*Result, error) {
	rec, ok := st.InstalledPacks[packAddr]
	if !ok {
		return nil, fmt.Errorf("pack %q is not installed", packAddr)
	}
	def, err := Resolve(packAddr, st)
	if err != nil {
		return nil, err
	}

	repoErrors := pullRepos(def.Pack, cfg, st, opts)

	res := &Result{Address: packAddr, Record: rec}
	for _, skillAddr := range def.Pack.Skills {
		for _, ag := range rec.Agents {
			updateSkill(cfg, st, res, skillAddr, ag, repoErrors, opts)
		}
	}

	if err := persist(st, res); err != nil {
		return nil, err
	}
	return res, nil
}

// updateSkill installs or updates one skill for one agent and records the outcome.
func updateSkill(cfg *config.Config, st *state.State, res *Result, skillAddr, agent string, repoErrors map[string]error, opts Options) {
	repoName := repoNameFromAddr(skillAddr)
	repoErr, repoBad := repoErrors[repoName]

	is, openErr := skill.Open(skillAddr, agent, cfg, st)
	if openErr != nil {
		// Not installed for this agent: install it.
		res.add(skillAddr, agent, deploySkill(cfg, st, skillAddr, agent, repoErrors, opts))
		return
	}

	fail := func(err error) {
		opts.emit(Event{Kind: SkillFailed, Skill: skillAddr, Agent: agent, Err: err})
		res.report(Outcome{Skill: skillAddr, Agent: agent, Kind: OutcomeFailed, Message: err.Error()})
	}
	if repoBad {
		err := fmt.Errorf("repo unavailable: %w", repoErr)
		opts.emit(Event{Kind: SkillSkipped, Repo: repoName, Skill: skillAddr, Agent: agent, Err: repoErr})
		res.report(Outcome{Skill: skillAddr, Agent: agent, Kind: OutcomeFailed, Message: err.Error()})
		return
	}

	status, err := is.Status()
	if err != nil {
		fail(fmt.Errorf("checking %s [%s]: %w", skillAddr, agent, err))
		return
	}
	if !status.HasUpstream {
		return
	}
	if status.IsConflict {
		err := fmt.Errorf("local modifications and upstream changes conflict — resolve with `skillpack sync --merge`, `--force-remote` or `--force-local`")
		opts.emit(Event{Kind: SkillBlocked, Skill: skillAddr, Agent: agent, Err: err})
		res.report(Outcome{Skill: skillAddr, Agent: agent, Kind: OutcomeBlocked, Message: err.Error()})
		return
	}

	opts.emit(Event{Kind: SkillUpdating, Skill: skillAddr, Agent: agent})
	if err := is.Update(cfg.TokenForRepo(repoName)); err != nil {
		fail(err)
		return
	}
	opts.emit(Event{Kind: SkillUpdated, Skill: skillAddr, Agent: agent})
	installed := state.PackSkillStatus{Installed: true}
	res.setStatus(skillAddr, agent, installed)
	res.report(Outcome{Skill: skillAddr, Agent: agent, Kind: OutcomeUpdated, Status: installed})
}

// pullRepos updates every repo listed in the pack. Returns repoName → error
// for repos that could not be pulled.
func pullRepos(pk *Pack, cfg *config.Config, st *state.State, opts Options) map[string]error {
	errs := make(map[string]error)
	seen := make(map[string]bool)
	for _, r := range pk.Repos {
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		opts.emit(Event{Kind: RepoPulling, Repo: r.Name})
		warning, err := repo.Update(r.Name, cfg.TokenForRepo(r.Name), st)
		switch {
		case err != nil:
			opts.emit(Event{Kind: RepoPullFailed, Repo: r.Name, Err: err})
			errs[r.Name] = err
		case warning != "":
			opts.emit(Event{Kind: RepoPullWarning, Repo: r.Name, Message: warning})
		}
	}
	return errs
}

// Remove removes the skills a pack installed for agents (all of the pack's
// agents when agents is empty), then drops the pack record, or prunes the
// removed agents from it when the pack stays deployed for others. State is
// saved once.
//
// Skills with Local Modifications are kept unless opts.Force is set; a kept
// skill simply becomes an ordinary Installed Skill and is reported as Kept.
func Remove(cfg *config.Config, st *state.State, packAddr string, agents []string, opts RemoveOptions) (*Result, error) {
	rec, ok := st.InstalledPacks[packAddr]
	if !ok {
		return nil, fmt.Errorf("pack %q is not installed", packAddr)
	}
	if len(agents) == 0 {
		agents = rec.Agents
	}

	res := &Result{Address: packAddr, Record: rec}
	emit := Options{Progress: opts.Progress}.emit
	for _, skillAddr := range sortedKeys(rec.Skills) {
		for _, ag := range agents {
			// Only remove what the pack actually installed for this agent.
			if s, ok := rec.Skills[skillAddr][ag]; !ok || !s.Installed {
				continue
			}
			is, err := skill.Open(skillAddr, ag, cfg, st)
			if err != nil {
				emit(Event{Kind: SkillNotInstalled, Skill: skillAddr, Agent: ag, Err: err})
				continue
			}
			if !opts.Force {
				if modified, err := is.IsModified(); err == nil && modified {
					msg := "local modifications — kept (use --force to remove anyway)"
					emit(Event{Kind: SkillKept, Skill: skillAddr, Agent: ag, Err: fmt.Errorf("%s", msg)})
					res.report(Outcome{Skill: skillAddr, Agent: ag, Kind: OutcomeKept, Message: msg})
					continue
				}
			}
			emit(Event{Kind: SkillRemoving, Skill: skillAddr, Agent: ag})
			if err := is.Remove(true); err != nil {
				emit(Event{Kind: SkillFailed, Skill: skillAddr, Agent: ag, Err: err})
				res.report(Outcome{Skill: skillAddr, Agent: ag, Kind: OutcomeFailed, Message: err.Error()})
				continue
			}
			emit(Event{Kind: SkillRemoved, Skill: skillAddr, Agent: ag})
			res.report(Outcome{Skill: skillAddr, Agent: ag, Kind: OutcomeRemoved})
		}
	}

	res.RemainingAgents = removeStrings(rec.Agents, agents)
	if len(res.RemainingAgents) == 0 {
		res.PackRemoved = true
		if err := st.RecordPackRemove(packAddr); err != nil {
			return nil, err
		}
	} else {
		pruned := rec
		pruned.Agents = res.RemainingAgents
		pruned.Skills = make(map[string]map[string]state.PackSkillStatus)
		for skillAddr, statuses := range rec.Skills {
			kept := make(map[string]state.PackSkillStatus)
			for ag, s := range statuses {
				if !contains(agents, ag) {
					kept[ag] = s
				}
			}
			if len(kept) > 0 {
				pruned.Skills[skillAddr] = kept
			}
		}
		res.Record = pruned
		if err := st.RecordPackInstall(packAddr, pruned); err != nil {
			return nil, err
		}
	}
	if err := state.Save(st); err != nil {
		return nil, err
	}
	return res, nil
}

// removeStrings returns a copy of slice with all elements in remove deleted.
func removeStrings(slice, remove []string) []string {
	var result []string
	for _, s := range slice {
		if !contains(remove, s) {
			result = append(result, s)
		}
	}
	return result
}

func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
