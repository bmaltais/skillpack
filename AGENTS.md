# AGENTS.md — Coding Agent Guide for SkillPack

This file is for AI coding agents (Claude Code, OpenCode, Codex, etc.) working on this codebase.
Read `CONTEXT.md` for the domain glossary. Read `CODING_STANDARDS.md` for code conventions. Read `plan.md` for the full design specification.

## Agent skills

### Issue tracker

Issues live as GitHub issues in `bmaltais/skillpack`, managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Canonical triage labels kept as-is (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Build & Test

```bash
go build ./cmd/skillpack/        # build the binary
go test ./...                    # run all tests
go vet ./...                     # static analysis
make check                       # vet + test (what CI runs)
make hooks                       # enable the pre-commit hook (once per clone)
```

The binary entry point is `cmd/skillpack/`. There is no other binary in this repo.

## Git Workflow

- **Never commit directly to `main`.** Always create a branch first, even for small or docs-only changes.
- Branch naming follows the existing convention: `feat/issue-<N>-<slug>` for features, `fix/issue-<N>-<slug>` for bug fixes (see `git branch -a` for examples). Use the GitHub issue number when one exists.
- After committing to the branch, push it with `git push -u origin <branch>`, then open a PR with `gh pr create --base main --head <branch> --title "..." --body "Closes #<N>..."`.
- If a change is accidentally committed to `main` before pushing, fix it before pushing: create a branch at that commit (`git branch <name> <sha>`), reset `main` back to `origin/main` (`git reset --hard origin/main`), then check out the new branch and push it.
- Do not merge your own PR unless the user explicitly asks you to.

## Key Files

| File | Purpose |
|------|---------|
| `docs/adr/` | Architecture Decision Records |
| `docs/adr/0001-packs-feature-design.md` | Packs feature design decisions |
| `CONTEXT.md` | Canonical domain glossary — read this first |
| `CODING_STANDARDS.md` | Code conventions; the Standards source for `/code-review` |
| `plan.md` | Full design spec with resolved decisions |
| `internal/config/config.go` | Config schema, `DefaultAgents`, loading (`~/.skillpack/config.yaml`) |
| `internal/state/state.go` | State schema and management (`~/.skillpack/state.json`) |
| `internal/repo/repo.go` | Repo management + skill discovery |
| `internal/skill/skill.go` | Install, remove, hash, conflict detection |
| `internal/pack/pack.go` | Pack schema: `pack.yaml` parsing and validation (deployment lives in `deploy.go`, resolution in `resolve.go`) |

## Architecture Constraints

Deliberate decisions. Discuss with the user before reversing one.

1. **Agents are config-only** (`name` + `skill_dir`). Agent-specific behaviour becomes a named field in the config struct.
2. **Install is a verbatim directory copy**, regardless of agent. Format conversion stays out of scope.
3. **Signing is out of scope for v1**: no `SKILL.md.sig` generation or verification.
4. **State key structure is `skill-path → agent-name → record`** (nested maps).
5. **Single binary**: `skillpack` under `cmd/skillpack/`.
6. **Everything lives under `~/.skillpack/`**: config and state, no XDG or per-project paths.

## Cross-Platform Rules

- Resolve home with `os.UserHomeDir()`; build filesystem paths with `filepath.Join()` (see `CODING_STANDARDS.md`).
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

DOX is the AGENTS.md hierarchy installed here; follow it across any edits. Hierarchy, child doc shape, style, and closeout: `docs/agents/dox.md`.

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

Re-read the applicable DOX chain in the current session before editing.

## Update After Editing

Every meaningful change requires a DOX pass before the task is done.

Update the closest owning AGENTS.md when a change affects:

- purpose, scope, ownership, or responsibilities
- durable structure, contracts, workflows, or operating rules
- required inputs, outputs, permissions, constraints, side effects, or artifacts
- user preferences about behavior, communication, process, organization, or quality
- AGENTS.md creation, deletion, move, rename, or index contents

Update parent docs when parent-level structure, ownership, workflow, or child index changes. Update child docs when parent changes alter local rules. Remove stale or contradictory text immediately. Small edits that do not change behavior or contracts may leave docs unchanged, but the DOX pass still must happen.

## User Preferences

When the user requests a durable behavior change, record it here or in the relevant child AGENTS.md.

## Coding Standards Upkeep

When a review or user correction establishes a convention no check enforces, add it to `CODING_STANDARDS.md` in the same branch.

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
| [`internal/pack/AGENTS.md`](internal/pack/AGENTS.md) | Pack Deployment: `pack.yaml` schema, recipe resolution, install/complete |

Each child AGENTS.md lists its own children (if any) at the bottom. The repo child points to gitops as its only sub-domain.