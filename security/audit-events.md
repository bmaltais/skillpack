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

| Event Name      | Trigger                                         | Auditable Per ITSG-33 Because…                   |
|-----------------|------------------------------------------------|--------------------------------------------------|
| `skill.install` | `skillpack install <repo>/<skill>`             | Software installation / supply-chain change      |
| `skill.remove`  | `skillpack remove <repo>/<skill>`              | Software removal / configuration change          |
| `skill.publish` | `skillpack publish <skill\|dir> [--repo <r>]`  | Code/config change pushed to a remote repository |

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
