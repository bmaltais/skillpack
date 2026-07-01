# AGENTS.md — State Persistence

## Purpose

Manages `~/.skillpack/state.json`: tracks registered repos, installed skills per agent, and install-time snapshots (SHA + hash).

## Ownership

| Concern | Owner |
|---------|-------|
| State struct & JSON load/save | `internal/state/state.go` |
| Install/remove/rename/hash mutation methods | `internal/state/state.go` |

## Local Contracts

- State path: `~/.skillpack/state.json`. Never XDG, never per-project.
- Key structure: `skill address → agent name → record`. Never flatten to compound string key.
- `InstalledSkillRecord` fields:
  - `InstalledAtSHA` — repo HEAD at install time
  - `InstalledHash` — SHA-256 of installed dir contents
  - `LocalPath` — absolute path in agent's skill dir
  - `UpstreamAddr` — original address before forking (non-empty for forks)
  - `UpstreamSHA` — upstream HEAD at fork time (for three-way merge base)
- `RecordRemove` deletes the agent entry; removes the address map if empty.
- `RecordRenameAddr` moves all agent entries from oldAddr to newAddr.

## Work Guidance

- New state fields: add to `InstalledSkillRecord`, update JSON tags, ensure backward compat (zero-value default).
- Validation: `validateMutation` rejects empty addr/agentName. Call before mutations.

## Verification

- `go test ./internal/state/...`

## Child DOX Index

None.