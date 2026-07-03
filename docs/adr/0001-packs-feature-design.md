# ADR-0001 — Packs Feature Design

**Status:** Accepted  
**Date:** 2026-07-03

---

## Context

Users want to bundle related skills so they can be shared and installed as a unit. A "pack" should work both as a distribution artifact (shareable with others) and as a local installation shortcut.

---

## Decisions

### 1. Packs are recipes, not bundles

A pack is a `pack.yaml` manifest that lists repos and skills. It does not embed skill file contents. Installing a pack clones/updates each listed repo and then copies skills into the agent's `skill_dir`, exactly as individual `skillpack install` calls would.

**Rationale:** Keeps packs lightweight and always in sync with the upstream skill content. Bundling would create a second copy problem and versioning complexity.

---

### 2. Pack recipe file format and discovery

A pack lives in a named directory containing a `pack.yaml`. Discovery follows the same recursive repo-walk as skill discovery — any directory inside a repo cache that contains a `pack.yaml` is a valid pack.

```
awesome-skills/
└── packs/
    └── go-dev/
        └── pack.yaml     # addressed as awesome-skills/packs/go-dev
```

`pack.yaml` minimum schema:

```yaml
name: go-dev                          # required
description: "Go development skills" # optional
repos:
  - name: awesome-skills
    url: https://github.com/example/awesome-skills
skills:
  - awesome-skills/coding/debugger
  - awesome-skills/coding/test-writer
```

**Rationale:** Consistent with `SKILL.md` discovery. No separate manifest registry needed. Packs can live alongside skills in any repo, or in a dedicated packs repo.

---

### 3. Pack addressing follows the skill address scheme

Pack address: `<repo-name>/<path/to/pack>`. Example: `awesome-skills/packs/go-dev`.

---

### 4. No version pinning in v1

Packs always resolve to the latest HEAD of each referenced repo at install time. Version pinning is deferred to a future version.

---

### 5. Packs are first-class state entities

An installed pack is tracked in `state.json` as a distinct record, separate from the individual skill install records it creates. This enables:

- `pack list` showing installed packs with completion status
- `pack remove` removing all skills installed by the pack from selected agents
- Detecting and displaying partial deployments

State schema addition:

```go
type InstalledPackRecord struct {
    PackAddress  string                         `json:"pack_address"`
    InstalledAt  time.Time                      `json:"installed_at"`
    Agents       []string                       `json:"agents"`
    Skills       map[string]PackSkillStatus     `json:"skills"`
    // key: skill address
}

type PackSkillStatus struct {
    Installed bool   `json:"installed"`
    Agent     string `json:"agent"`
    Error     string `json:"error,omitempty"` // set when install failed
}
```

---

### 6. Partial deployments are tracked, not auto-repaired

When a repo in a pack requires authentication that the user cannot provide, skillpack:

1. Prompts the user to configure credentials.
2. If the user cannot, continues installing skills from repos that are accessible.
3. Records the pack as a partial deployment in state.
4. Lists partial packs in `pack list` output and highlights them in the TUI.
5. Offers a "complete deployment" action for each partial pack in the TUI.

Skillpack never silently completes a partial deployment. The user must explicitly trigger completion.

---

### 7. `pack` is a top-level command with subcommands

```
skillpack pack install <address|url|filepath>  # deploy a pack
skillpack pack list                            # list installed packs (complete + partial)
skillpack pack create                          # open TUI to author + publish a new pack
skillpack pack remove  <address>               # remove skills installed by pack from selected agent(s)
skillpack pack update  <address>               # re-run install to pick up latest skill content
skillpack pack status  <address>               # show per-skill/per-agent deployment status
```

**Rationale:** Packs are a distinct enough concept to warrant their own namespace. Mixing with skill commands (flags/modifiers) would blur the model.

---

### 8. Pack authoring lives in the TUI

Pack creation is driven by an interactive TUI flow:

1. Name the pack (required) + description (optional).
2. Browse registered repos and select skills to include.
3. Review the generated `pack.yaml`.
4. Select which registered repo to publish the pack to.
5. Confirm — skillpack writes the file, commits, and pushes.

Hand-editing `pack.yaml` is also valid; the TUI is the guided path.

---

### 9. Multi-agent install: user selects agents at install time

`pack install` prompts the user to select which configured agents to deploy the pack to. Defaults to the default agent if run non-interactively with no flag.

---

### 10. Direct skill removal from a pack is allowed; pack becomes partial

If the user runs `skillpack remove` on a skill that was installed by a pack, the skill is removed and the pack is marked partial in state. The user is informed ("This skill is part of pack X — the pack is now partially deployed"). The pack is not auto-repaired.

---

## Consequences

- `state.json` gains a top-level `installed_packs` map.
- A new `packs/` section in the `internal/` tree (or extension of `internal/skill/`) handles pack install/remove/update logic.
- The TUI gains a pack creation flow and a partial-deployment status panel.
- `CONTEXT.md` gains Pack, Pack Recipe, Pack Address, Pack Deployment, Partial Pack Deployment, Pack Authoring, Pack Publication, and Installed Pack terms.
- No nesting of packs in v1.
- No version pinning in v1.
