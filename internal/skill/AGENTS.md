# AGENTS.md — Skill Lifecycle

## Purpose

Installs, removes, updates, forks, syncs, publishes, and reconciles skills. The central business logic layer.

## Ownership

| Concern | Owner |
|---------|-------|
| Install (copy + hash + record) | `internal/skill/skill.go` (`Install`) |
| Remove (with modification check) | `internal/skill/skill.go` (`remove`) |
| Hash computation (deterministic SHA-256) | `internal/skill/skill.go` (`ComputeHash`) |
| Directory copy (symlink-safe) | `internal/skill/skill.go` (`copyDir`) |
| Update plan generation | `internal/skill/update_plan.go` |
| Update execution | `internal/skill/update.go` |
| Fork candidate discovery | `internal/skill/fork_candidates.go` |
| Fork creation & install | `internal/skill/fork.go`, `internal/skill/fork_install.go` |
| Fork metadata (provenance) | `internal/skill/fork_metadata.go` |
| Fork missing upstream detection | `internal/skill/fork_missing_upstream_test.go` |
| Sync (reconcile installed ↔ cache) | `internal/skill/sync.go`, `internal/skill/sync_reconcile_test.go` |
| Sync integration tests | `internal/skill/sync_integration_test.go` |
| Publish (push to upstream) | `internal/skill/publish.go` |
| Relink (re-register skill) | `internal/skill/relink.go` |
| Resolve (address → info) | `internal/skill/resolve.go` |
| LLM-assisted operations | `internal/skill/llm.go` |

## Local Contracts

- Install is a verbatim directory copy. No format conversion.
- Installed skill name = `filepath.Base(skillInfo.FullPath)`. Category structure is NOT preserved in the agent dir.
- `ComputeHash` sorts files by relative path, skips `.fork.json`, handles symlinks-to-directories gracefully.
- `remove` checks for local modifications unless `--force`. Modified skills refuse removal.
- Conflict resolution flags (`--force-remote`, `--force-local`, `--merge`) apply to `update` and `sync`.
- Fork metadata lives in `.fork.json` at the skill root. Skipped during hash.

## Work Guidance

- New skill operations: add to `internal/skill/`. One file per concern.
- State mutations go through `state.State` methods — never write to JSON directly.
- Git operations (commit, push, diff) delegate to `internal/gitops`.
- LLM calls are optional helpers in `llm.go`. Not required for core flows.

## Verification

- `go test ./internal/skill/...`

## Child DOX Index

None.