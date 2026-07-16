# Auditable Events — skillpack CLI

**Controls:** AU-2 (Auditable Events) + AU-3 (Content of Audit Records) — ITSG-33 PBMM
**Status:** Implemented (`internal/audit`)

## Overview

skillpack emits structured audit events to **stderr** as newline-delimited JSON.
Each record satisfies ITSG-33 AU-3 required content fields:

| Field       | AU-3 Requirement          | Type   | Description                                        |
|-------------|--------------------------|--------|----------------------------------------------------|
| `timestamp` | Date and time of event    | string | RFC 3339 UTC timestamp                             |
| `event`     | Type of event             | string | Dotted event name (see table below)                |
| `actor`     | Subject identity          | string | `USER@hostname` of the process invoking the CLI    |
| `detail`    | Object / source-dest      | string | Human-readable target (skill address, agent name)  |
| `outcome`   | Outcome (success/failure) | string | `"success"` or `"failure"`                        |
| `error`     | Additional detail         | string | Error message — present only when outcome=failure  |

## Defined Auditable Events

| Event Name                | Trigger                                          | Auditable Per ITSG-33 Because…                     |
|---------------------------|--------------------------------------------------|-----------------------------------------------------|
| `skill.install`           | `skillpack install <repo>/<skill>`               | Software installation / supply-chain change         |
| `skill.remove`            | `skillpack remove <repo>/<skill>`                | Software removal / configuration change             |
| `skill.publish`           | `skillpack publish <skill\|dir> [--repo <r>]`    | Code/config change pushed to a remote repository    |
| `skill.update`            | `skillpack self-update`                          | Binary self-replacement — supply-chain integrity    |
| `config.credential.set`   | `skillpack repo add --token` / `repo rename`     | Credential creation or rotation — highest-risk op   |

## AU-12 Generation Points

Audit records are generated at the following code locations (AU-12 "configured generation"):

| Event | File | Notes |
|-------|------|-------|
| `skill.install` | `cmd/skillpack/install.go` | Per-agent target loop, success and failure |
| `skill.remove` | `cmd/skillpack/remove.go` | Per-agent target loop, success and failure |
| `skill.publish` | `cmd/skillpack/publish.go` | Both new-skill and existing-skill modes |
| `skill.update` | `cmd/skillpack/selfupdate.go` | On successful or failed binary replace |
| `config.credential.set` | `cmd/skillpack/repo.go` | Token add (new repo), token update (existing repo), token rekey (rename) |

The CI `audit-smoke` job in `.github/workflows/release.yml` runs `go test ./internal/audit/...` and asserts that at least one `"event"` field appears in test output, confirming the generation mechanism is active on every build.

## Log Destination

Events are written to **stderr**. Operators should capture stderr alongside stdout when
running skillpack in CI or automated contexts, and forward to their log aggregation
system (e.g. `2>> /var/log/skillpack/audit.log`).

## Example Record

```json
{"timestamp":"2026-07-15T14:22:01Z","event":"skill.install","actor":"alice@build-host","detail":"my-repo/tools/debugger → claude-code","outcome":"success"}
```

## Relationship to Other AU Controls

- **AU-12** (Audit Generation): events are generated at the key lifecycle points listed
  above; no additional configuration is required for the audit mechanism to activate.
