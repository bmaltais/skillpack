# AGENTS.md — Doctor

## Purpose

Diagnoses a skillpack setup and describes, per problem, the fix a caller can offer. Powers the TUI Doctor panel (selectable findings with fixes); `SkillProblems` is also what the Skills panel uses to flag stale and broken-upstream rows. The `skillpack doctor` CLI command stays a read-only Duplicate Set report.

## Ownership

| Concern | Owner |
|---------|-------|
| `Diagnose`, `Finding`, `Kind` | `internal/doctor/doctor.go` |
| `Diff` (line diff of two skills' files in the repo caches) | `internal/doctor/diff.go` |
| Tests (temp-dir repo caches, in-memory `state.State`) | `internal/doctor/doctor_test.go` |

## Local Contracts

- `Diagnose` is read-only: no writes, no network, no git, no `state.Save`. It composes `repo.DiscoverAllSkills`, `skill.DetectForkCandidates`, `skill.DetectDuplicateSets`, and `pack.IsPartial`.
- A `Finding` is plain data (`Kind`, `Subject`, `Related`, `Agent`, `Detail`, `Fix`). `Fix` is the label of what the caller will do; empty means informational. The package never applies a fix: the caller routes each `Kind` to the existing operation (`skill.Relink`/`RelinkUpstream`, `pack.Complete`, `skill.RegisterForkProvenance`).
- Adding a `Kind` means adding its rank in `Diagnose`'s ordering and a test; keep a fix non-destructive (ADR-0004).
- Unregistered or non-clone upstream repos are unknown, never broken.
- Imports `pack` and `skill`; neither may import `doctor`.

## Verification

- `go test ./internal/doctor/...`

## Child DOX Index

None.
