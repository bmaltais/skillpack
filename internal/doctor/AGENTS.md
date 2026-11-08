# AGENTS.md — Doctor

## Purpose

Diagnoses a skillpack setup and describes, per problem, the fix a caller can offer. Powers the TUI Doctor panel (selectable findings with fixes); `SkillProblems` is also what the Skills panel uses to flag stale and broken-upstream rows. The `skillpack doctor` CLI command stays a read-only Duplicate Set report.

## Ownership

| Concern | Owner |
|---------|-------|
| `Diagnose`, `Finding`, `Kind` | `internal/doctor/doctor.go` |
| `Diff` (line diff of two skills' files in the repo caches) | `internal/doctor/diff.go` |
| Repo writes: `DeleteCopy`, `OverwriteCopy` (back up, mutate, commit and push, restore on failure) | `internal/doctor/actions.go` |
| Tests (temp-dir repo caches, in-memory `state.State`) | `internal/doctor/doctor_test.go` |

## Local Contracts

- `Diagnose` and `Diff` are read-only: no writes, no network, no git, no `state.Save`. Only `actions.go` writes, and only when the caller invokes it after user confirmation. It composes `repo.DiscoverAllSkills`, `skill.DetectForkCandidates`, `skill.DetectDuplicateSets`, and `pack.IsPartial`.
- A `Finding` is plain data (`Kind`, `Subject`, `Related`, `Agent`, `Detail`, `Fix`). `Fix` is the label of what the caller will do; empty means informational. `Diagnose` never applies a fix: the caller routes each `Kind` to the existing operation (`skill.Relink`/`RelinkUpstream`, `pack.Complete`, `skill.RegisterForkProvenance`, or `actions.go` for duplicate pairs).
- Adding a `Kind` means adding its rank in `Diagnose`'s ordering and a test. Fixes that write follow the ADR-0004 guardrails: write access required, nothing lost without an identical surviving copy, pushed failures restored.
- `DeleteCopy` refuses installed copies and copies whose content differs from the kept one. `OverwriteCopy` keeps the target's own `.skillpack-fork`.
- Unregistered or non-clone upstream repos are unknown, never broken.
- Imports `pack` and `skill`; neither may import `doctor`.

## Verification

- `go test ./internal/doctor/...`

## Child DOX Index

None.
