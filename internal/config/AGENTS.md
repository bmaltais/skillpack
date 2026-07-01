# AGENTS.md — Configuration

## Purpose

Manages `~/.skillpack/config.yaml`: agent definitions, credential storage, and first-run agent auto-detection.

## Ownership

| Concern | Owner |
|---------|-------|
| Config struct & YAML load/save | `internal/config/config.go` |
| Agent detection from `DefaultAgents` list | `internal/config/config.go` (`DetectAgents`) |
| Path expansion (`~/` → absolute) | `internal/config/config.go` (`ExpandPath`) |
| Token resolution (config → env → env) | `internal/config/config.go` (`TokenForRepo`) |

## Local Contracts

- Config path: `~/.skillpack/config.yaml`. Never XDG, never per-project.
- Credentials are stored in plain text with mode `0600`. No encryption.
- `TokenForRepo` priority: `credentials[repo]` → `SKILLPACK_GIT_TOKEN` → `GITHUB_TOKEN` → `""`.
- Agent detection runs on every `Load()` — new agents found on disk are auto-added.
- `ExpandPath` converts `~/...` to absolute using `os.UserHomeDir()`. Never hardcode `~`.

## Work Guidance

- Adding a new known agent: extend `DefaultAgents` slice. The detection loop handles the rest.
- Adding a new credential source: update `TokenForRepo` and document the priority.
- Config schema changes require a migration note in `plan.md`.

## Verification

- `go test ./internal/config/...`

## Child DOX Index

None.