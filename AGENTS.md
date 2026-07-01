# AGENTS.md — Coding Agent Guide for SkillPack

This file is for AI coding agents (Claude Code, OpenCode, Codex, etc.) working on this codebase.
Read `CONTEXT.md` for the domain glossary. Read `plan.md` for the full design specification.

## Build & Test

```bash
go build ./cmd/skillpack/        # build the binary
go test ./...                    # run all tests
go vet ./...                     # static analysis
```

The binary entry point is `cmd/skillpack/`. There is no other binary in this repo.

## Key Files

| File | Purpose |
|------|---------|
| `CONTEXT.md` | Canonical domain glossary — read this first |
| `plan.md` | Full design spec with resolved decisions |
| `internal/config/config.go` | Config loading (`~/.skillpack/config.yaml`) |
| `internal/state/state.go` | State management (`~/.skillpack/state.json`) |
| `internal/repo/repo.go` | Repo management + skill discovery |
| `internal/skill/skill.go` | Install, remove, hash, conflict detection |

## Architecture Constraints — Do Not Violate

These decisions were made deliberately. Do not reverse them without discussing first.

1. **No per-agent adapter files.** Agents are config-only (`name` + `skill_dir`). Do not create `internal/agent/claude.go` or similar. If agent-specific behaviour is ever needed, add it as a named field in the config struct.

2. **No format conversion.** Install is a verbatim directory copy. Do not create or restore `pkg/convert/`. Skills are copied as-is regardless of agent.

3. **No `SKILL.md.sig` generation or verification.** Signing is out of scope for v1.

4. **State key structure is `skill-path → agent-name → record`.** Do not flatten it to a compound string key or a list.

5. **Single binary: `skillpack`.** Do not add additional binaries under `cmd/`.

6. **Everything lives under `~/.skillpack/`.** Do not write config or state to XDG paths or per-project directories.

## State Schema

```go
// ~/.skillpack/state.json
type State struct {
    Repos           map[string]RepoRecord                       `json:"repos"`
    InstalledSkills map[string]map[string]InstalledSkillRecord  `json:"installed_skills"`
    // key: skill address (e.g. "awesome-skills/coding/debugger")
    // inner key: agent name (e.g. "claude-code")
}

type RepoRecord struct {
    URL         string    `json:"url"`
    CachePath   string    `json:"cache_path"`
    LastUpdated time.Time `json:"last_updated"`
}

type InstalledSkillRecord struct {
    InstalledAtSHA string `json:"installed_at_sha"`
    InstalledHash  string `json:"installed_hash"`   // SHA-256 of installed dir contents
    LocalPath      string `json:"local_path"`
}
```

## Config Schema

```go
// ~/.skillpack/config.yaml
type Config struct {
    DefaultAgent string                 `yaml:"default_agent"`
    Agents       map[string]AgentConfig `yaml:"agents"`
}

type AgentConfig struct {
    SkillDir string `yaml:"skill_dir"`
}
```

## Default Agents (bundled in binary)

The first-run wizard uses this list to auto-detect installed agents:

| Agent name | Expected `skill_dir` | Verified |
|------------|----------------------|---------|
| `claude-code` | `~/.claude/skills` | ✓ |
| `copilot` | `~/.copilot/skills` | ✓ |
| `grok` | `~/.grok/skills` | ✓ |
| `hermes` | `~/.hermes/skills` | ✓ |
| `opencode` | `~/.config/opencode/skills` | TODO |
| `openclaw` | `~/.openclaw/skills` | TODO |
| `pi` | `~/.pi/agent/skills` | ✓ |

Detection logic: expand `~`, check if the directory exists on disk. If it does, offer to add it to config.

## Cross-Platform Rules

- Use `os.UserHomeDir()` to resolve home directory. **Never hardcode `~` or use `os.Expand` with `~`.**
- Use `filepath.Join()` for all path construction. Never concatenate paths with `/`.
- HTTPS auth on Windows relies on the system git credential store — go-git handles this transparently.
- SSH push on Windows is not supported in v1.

## Conflict Resolution Flags

When a skill has local modifications AND upstream changes, `update` and `sync` require one of:

- `--force-remote` — remote wins: overwrite installed files with cache, reset hash + SHA
- `--force-local` — local wins: copy installed files back to cache, commit, push, reset hash + SHA  
- `--merge` — three-way merge (base=`installed_at_sha`, ours=installed, theirs=cache HEAD); write conflict markers on failure

## Skill Discovery

A skill is any directory inside a repo clone that contains a `SKILL.md` file. Discovery = recursive walk of the repo cache dir, collect all paths containing `SKILL.md`. No manifest file required or supported.

# DOX framework

- DOX is highly performant AGENTS.md hierarchy installed here
- Agent must follow DOX instructions across any edits

## Core Contract

- AGENTS.md files are binding work contracts for their subtrees
- Work products, source materials, instructions, records, assets, and durable docs must stay understandable from the nearest applicable AGENTS.md plus every parent AGENTS.md above it

## Read Before Editing

1. Read the root AGENTS.md
2. Identify every file or folder you expect to touch
3. Walk from the repository root to each target path
4. Read every AGENTS.md found along each route
5. If a parent AGENTS.md lists a child AGENTS.md whose scope contains the path, read that child and continue from there
6. Use the nearest AGENTS.md as the local contract and parent docs for repo-wide rules
7. If docs conflict, the closer doc controls local work details, but no child doc may weaken DOX

Do not rely on memory. Re-read the applicable DOX chain in the current session before editing.

## Update After Editing

Every meaningful change requires a DOX pass before the task is done.

Update the closest owning AGENTS.md when a change affects:

- purpose, scope, ownership, or responsibilities
- durable structure, contracts, workflows, or operating rules
- required inputs, outputs, permissions, constraints, side effects, or artifacts
- user preferences about behavior, communication, process, organization, or quality
- AGENTS.md creation, deletion, move, rename, or index contents

Update parent docs when parent-level structure, ownership, workflow, or child index changes. Update child docs when parent changes alter local rules. Remove stale or contradictory text immediately. Small edits that do not change behavior or contracts may leave docs unchanged, but the DOX pass still must happen.

## Hierarchy

- Root AGENTS.md is the DOX rail: project-wide instructions, global preferences, durable workflow rules, and the top-level Child DOX Index
- Child AGENTS.md files own domain-specific instructions and their own Child DOX Index
- Each parent explains what its direct children cover and what stays owned by the parent
- The closer a doc is to the work, the more specific and practical it must be

## Child Doc Shape

- Create a child AGENTS.md when a folder becomes a durable boundary with its own purpose, rules, responsibilities, workflow, materials, or quality standards
- Work Guidance must reflect the current standards of the project or user instructions; if there are no specific standards or instructions yet, leave it empty
- Verification must reflect an existing check; if no verification framework exists yet, leave it empty and update it when one exists

Default section order:
- Purpose
- Ownership
- Local Contracts
- Work Guidance
- Verification
- Child DOX Index

## Style

- Keep docs concise, current, and operational
- Document stable contracts, not diary entries
- Put broad rules in parent docs and concrete details in child docs
- Prefer direct bullets with explicit names
- Do not duplicate rules across many files unless each scope needs a local version
- Delete stale notes instead of explaining history
- Trim obvious statements, repeated rules, misplaced detail, and warnings for risks that no longer exist

## Closeout

1. Re-check changed paths against the DOX chain
2. Update nearest owning docs and any affected parents or children
3. Refresh every affected Child DOX Index
4. Remove stale or contradictory text
5. Run existing verification when relevant
6. Report any docs intentionally left unchanged and why

## User Preferences

When the user requests a durable behavior change, record it here or in the relevant child AGENTS.md

## Child DOX Index

| Child | Scope |
|-------|-------|
| [`cmd/skillpack/AGENTS.md`](cmd/skillpack/AGENTS.md) | CLI layer: Cobra commands, TUI, re-exec, color output |
| [`internal/config/AGENTS.md`](internal/config/AGENTS.md) | Config loading, agent detection, credential storage, path expansion |
| [`internal/state/AGENTS.md`](internal/state/AGENTS.md) | State persistence: repo registry, installed skills, install snapshots |
| [`internal/repo/AGENTS.md`](internal/repo/AGENTS.md) | Repo management: clone, update, discover skills, cache |
| [`internal/skill/AGENTS.md`](internal/skill/AGENTS.md) | Skill lifecycle: install, remove, update, fork, sync, publish, relink |
| [`internal/gitops/AGENTS.md`](internal/gitops/AGENTS.md) | Git operations: auth, commit, push, diff, file listing |
| [`internal/testutil/AGENTS.md`](internal/testutil/AGENTS.md) | Test helpers: isolated temp HOME for tests |

Each child AGENTS.md lists its own children (if any) at the bottom. The repo child points to gitops as its only sub-domain.