# SkillPack — Agent Skill Management CLI

A CLI tool for managing AI agent skills across multiple agents on a local system. Point at a skill repository, install individual skills for specific agents, maintain locally-updated skills, push changes back to the central repo, add new local skills to the central repo, remove local skills, etc.

## Core Design

### Agent Configuration

Agents are fully config-driven — no per-agent code. Any agent can be added by the user by editing `~/.skillpack/config.yaml`. Install is a verbatim directory copy; no format conversion happens.

```yaml
# ~/.skillpack/config.yaml
default_agent: claude-code

agents:
  claude-code:
    skill_dir: ~/.claude/skills
  copilot:
    skill_dir: ~/.copilot/skills
  hermes:
    skill_dir: ~/.hermes/skills
  opencode:
    skill_dir: ~/.config/opencode/skills  # TODO: verify path
  openclaw:
    skill_dir: ~/.openclaw/skills         # TODO: verify path
  pi:
    skill_dir: ~/.pi/agent/skills
  my-custom-agent:
    skill_dir: ~/.myagent/skills
```

The first-run wizard auto-detects which agents are present by checking whether each `skill_dir` exists on disk. Only present agents are added to config; the rest are skipped silently.

On first run, if no config exists, the tool runs an interactive wizard to:
1. Detect installed agents (check if each known `skill_dir` exists on disk) and set `default_agent`
2. Offer to register the skillpack repo itself and install the `skillpack/skillpack` skill into all detected agents

### Skill Repository Format

Central skill repos follow a consistent structure. Skills are discovered by walking the repo for any directory containing a `SKILL.md` file — no manifest required. Repos may be flat or use category subdirectories; both work transparently.

```
skill-repo/
├── SKILL.md                      # flat skill at repo root level
├── coding/
│   └── debugger/
│       ├── SKILL.md              # categorised skill
│       └── references/
│           └── api-reference.md
└── writing/
    └── blogger/
        └── SKILL.md
```

### Skill Addressing

Skills are addressed by their repo-relative path: `<repo-name>/<path/to/skill>`.

Examples:
- `awesome-skills/debugger` (flat)
- `awesome-skills/coding/debugger` (categorised)

### Local File Layout

Everything lives under `~/.skillpack/`:

```
~/.skillpack/
├── config.yaml        # user-edited: agents, default_agent
├── state.json         # tool-managed: installed skills, repo metadata
└── repos/             # hidden git clones of registered repos
    ├── awesome-skills/
    └── copilot-skills/
```

### Local State

```json
{
  "repos": {
    "awesome-skills": {
      "url": "https://github.com/example/awesome-skills",
      "cache_path": "~/.skillpack/repos/awesome-skills",
      "last_updated": "2026-05-15T10:46:00Z"
    }
  },
  "installed_skills": {
    "awesome-skills/coding/debugger": {
      "claude-code": {
        "installed_at_sha": "a3f8c1d9...",
        "installed_hash": "sha256:9f4e2b...",
        "local_path": "~/.claude/skills/debugger"
      },
      "hermes-agent": {
        "installed_at_sha": "a3f8c1d9...",
        "installed_hash": "sha256:9f4e2b...",
        "local_path": "~/.copilot/skills/debugger"
      }
    }
  }
}
```

- `installed_at_sha` — git commit SHA of the repo cache at install time; used to detect upstream changes
- `installed_hash` — SHA-256 of the installed directory contents at install time; used to detect local user modifications

## Commands

### Repository Management

```
skillpack repo add <remote-url> [--name <name>]   # clone repo to cache, register it
skillpack repo list                                # show all registered repos
skillpack repo remove <repo-name>                  # remove repo (does not uninstall skills)
skillpack repo update [<repo-name>]                # git pull on cached clone(s)
```

### Skill Installation

```
skillpack install <repo>/<path/to/skill>                    # install for default agent
skillpack install --agent <agent> <repo>/<path/to/skill>    # install for specific agent
skillpack install --all-agents <repo>/<path/to/skill>       # install for all configured agents
skillpack install --skip-existing <repo>/<path/to/skill>    # no-op if already installed
```

### Skill Listing

```
skillpack list                          # installed skills (all agents)
skillpack list --agent <agent>          # installed skills for one agent
skillpack list --modified               # locally modified skills only
skillpack list --available              # all skills in all registered repos
skillpack list --available --repo <r>   # browse skills in one repo
```

### Skill Updates

```
skillpack update                                           # check + update all installed skills
skillpack update <repo>/<path/to/skill>                    # update specific skill
skillpack update --dry-run                                 # show what would change

# Conflict resolution flags (required when skill has local modifications):
skillpack update --force-remote <repo>/<path/to/skill>     # remote wins: overwrite local
skillpack update --force-local  <repo>/<path/to/skill>     # local wins: push local to remote, mark up-to-date
skillpack update --merge        <repo>/<path/to/skill>     # three-way merge; conflict markers written on failure
```

### Skill Removal

```
skillpack remove <repo>/<path/to/skill>                    # uninstall for default agent
skillpack remove --agent <agent> <repo>/<path/to/skill>    # uninstall for specific agent
skillpack remove --all-agents <repo>/<path/to/skill>       # uninstall everywhere
skillpack remove --force <repo>/<path/to/skill>            # remove even if locally modified
```

### Skill Publishing

```
skillpack publish <repo>/<path/to/skill>                   # push local edits to remote (auto commit msg)
skillpack publish ./my-new-skill --repo <repo-name>        # add a new local skill to a repo
skillpack publish --dry-run <repo>/<path/to/skill>         # show what would be committed
```

### Sync

```
skillpack sync             # two-way reconciliation across all installed skills:
                           #   1. git pull all registered repos
                           #   2. update unmodified skills that have upstream changes
                           #   3. publish locally-modified skills back to remote
                           #   4. warn + skip skills modified both locally and upstream
skillpack sync --dry-run   # show what would change without applying
```

On conflicts during sync (modified locally AND upstream changed): warn, skip, report at end. User resolves with `update --force-remote|--force-local|--merge`.

When a skill is installed for multiple agents and copies have diverged, the default agent's copy is used as source of truth for publish/sync.

## Conflict Resolution: `--merge`

v1: write standard three-way conflict markers to installed files (base=`installed_at_sha`, ours=local, theirs=remote HEAD). User resolves manually.

v2 (planned): `--merge --llm` delegates resolution to the agent configured to use the skill — it understands the domain and can make intelligent merge decisions.

## Implementation Phases

### Phase 1: Core Infrastructure

- [x] Config loading + first-run wizard (`~/.skillpack/config.yaml`; auto-detect agents from known defaults)
- [x] State management (`~/.skillpack/state.json`)
- [x] `repo add` / `repo list` / `repo remove` / `repo update` (clone to `~/.skillpack/repos/`)
- [x] `install` for default agent (verbatim copy, record SHA + hash in state)
- [x] `remove` for default agent

### Phase 2: Multi-Agent + Listing ✓

- [x] `install --agent` / `install --all-agents`
- [x] `remove --agent` / `remove --all-agents`
- [x] `list` (installed, with `--agent`, `--modified`, `--available` flags)
- [x] Unit tests: config, repo discovery, skill hash + modification detection

### Phase 3: Updates + Conflict Detection ✓

- [x] `update` — compare `installed_at_sha` against current cache HEAD to detect upstream changes
- [x] Abort with clear error when skill is modified; require `--force-remote|--force-local|--merge`
- [x] `update --force-remote` / `--force-local` / `--merge`

### Phase 4: Publishing + Sync ✓

- [x] `publish` (copy from agent dir → cache, git commit auto-message, git push to main)
- [x] `publish ./new-skill --repo <name>` for new skills
- [x] `sync` (two-way reconciliation)
- [x] `--dry-run` on `update`, `publish`, `sync`

### Phase 5: Polish ✓

- [x] Color output / formatting
- [x] Help text and examples
- [x] `list --available` with category grouping
- [x] Write `skillpack/SKILL.md` — the self-describing skill for AI agents
- [x] First-run wizard offers to register the skillpack repo and install `skillpack/skillpack` automatically

## Tech Stack

- **Language:** Go (single binary, no dependency issues)
- **Git integration:** `go-git` (clone, pull, commit, push; auth via SSH agent / system credential store)
- **State storage:** JSON file (`~/.skillpack/state.json`)
- **Config:** YAML (`~/.skillpack/config.yaml`)
- **CLI framework:** `cobra`

## File Structure

```
skillpack/
├── cmd/
│   └── skillpack/
│       ├── root.go        # cobra root, first-run wizard
│       ├── repo.go        # repo subcommands
│       ├── install.go
│       ├── list.go
│       ├── update.go
│       ├── remove.go
│       ├── publish.go
│       └── sync.go
├── internal/
│   ├── config/
│   │   └── config.go      # load/save ~/.skillpack/config.yaml
│   ├── state/
│   │   └── state.go       # load/save ~/.skillpack/state.json
│   ├── repo/
│   │   └── repo.go        # repo management, skill discovery (walk for SKILL.md)
│   └── skill/
│       └── skill.go       # install, remove, hash, conflict detection
├── skillpack/
│   └── SKILL.md           # the skillpack skill (address: skillpack/skillpack)
├── go.mod
└── plan.md
```

## Platform Support

| Platform | Status | Notes |
|----------|--------|-------|
| Linux (amd64, arm64) | v1 | Primary development platform |
| Windows (amd64) | v1 | HTTPS auth only; SSH push not supported in v1 |
| macOS (amd64, arm64) | future | Same as Linux; trivial to add |

### Cross-Platform Rules

- **Always use `os.UserHomeDir()`** to resolve the home directory. Never shell-expand `~` directly.
- **Always use `filepath.Join()`** for path construction. Never concatenate with `/`.
- **HTTPS auth on Windows:** rely on the system git credential store (populated by `git credential-manager`, `gh auth`, etc.). go-git uses this transparently.
- **SSH push on Windows:** not supported in v1. Document the limitation. Users needing SSH can set `GIT_SSH_COMMAND`.

## CI / Release (GitHub Actions + GoReleaser)

`.github/workflows/release.yml` — triggered on git tag push (`v*`):

1. Run `go test ./...` and `go vet ./...`
2. GoReleaser cross-compiles and produces archives for:
   - `linux/amd64`, `linux/arm64`
   - `windows/amd64`
   - `darwin/amd64`, `darwin/arm64`
3. Publishes binaries + checksums to GitHub Releases automatically

A separate `.github/workflows/ci.yml` runs tests + vet on every push and PR.

## Packs

A pack bundles a curated set of skills into a shareable, installable recipe. See `docs/adr/0001-packs-feature-design.md` for full design decisions.

### Pack Recipe Format

A `pack.yaml` in a named directory, discovered by the same recursive repo walk as skills:

```
awesome-skills/
└── packs/
    └── go-dev/
        └── pack.yaml    # address: awesome-skills/packs/go-dev
```

```yaml
name: go-dev
description: "Go development skills"  # optional
repos:
  - name: awesome-skills
    url: https://github.com/example/awesome-skills
skills:
  - awesome-skills/coding/debugger
  - awesome-skills/coding/test-writer
```

### Pack Commands

```
skillpack pack install <address|url|filepath>  # deploy a pack; prompts for agent selection
skillpack pack list                            # list installed packs; partial deployments flagged
skillpack pack create                          # TUI flow to author + publish a pack
skillpack pack remove  <address>               # remove skills installed by pack from selected agent(s)
skillpack pack update  <address>               # re-run install to pick up latest skill content
skillpack pack status  <address>               # show per-skill/per-agent deployment status
```

### Partial Deployments

When a repo requires auth the user cannot provide:
1. Prompt for credentials; if unavailable, continue with accessible repos.
2. Mark the pack as partially deployed in state.
3. Surface in `pack list` and TUI with a "complete deployment" action.
4. Never auto-repair — the user must explicitly trigger completion.

A pack also becomes partial if the user directly removes one of its skills via `skillpack remove`. The user is notified and the pack is flagged in state.

### Pack State Schema

```go
// added to ~/.skillpack/state.json
type InstalledPackRecord struct {
    PackAddress string                     `json:"pack_address"`
    InstalledAt time.Time                  `json:"installed_at"`
    Agents      []string                   `json:"agents"`
    Skills      map[string]PackSkillStatus `json:"skills"` // key: skill address
}

type PackSkillStatus struct {
    Installed bool   `json:"installed"`
    Agent     string `json:"agent"`
    Error     string `json:"error,omitempty"`
}
```

### Pack Authoring (TUI flow)

1. Name + optional description.
2. Browse registered repos, select skills.
3. Review generated `pack.yaml`.
4. Select target repo to publish to.
5. Confirm — skillpack writes, commits, pushes.

### Phase 6: Packs

- [ ] `pack.yaml` discovery in repo walk
- [ ] `pack install` with multi-agent selection + partial deployment tracking
- [ ] `pack list` with partial/complete status
- [ ] `pack remove` (removes member skills from selected agents)
- [ ] `pack update`
- [ ] `pack status`
- [ ] `pack create` TUI flow
- [ ] Partial deployment completion action in TUI
- [ ] State: `installed_packs` top-level map in `state.json`
- [ ] Direct `skillpack remove` on a pack member marks pack partial

---

## Out of Scope for v1

- `SKILL.md.sig` integrity/signing
- LLM-assisted merge (`--merge --llm`)
- Version pinning (`install @<sha>`)
- GitHub/GitLab PR workflow on publish (push directly to main only)
- SSH push on Windows
- macOS release builds (cross-compile target exists but untested)
- Project-level skills (`.claude/skills/` inside a repo) — global user skills only
