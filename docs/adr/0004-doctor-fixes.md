# ADR-0004 — Doctor offers user-confirmed fixes

**Status:** Accepted
**Date:** 2026-11-08
**Amends:** ADR-0003 decision 1 (detection and reporting only)

## Context

The TUI Doctor panel showed only Duplicate Sets, with nothing to act on. A
health check that cannot fix what it finds sends the user off to find the
right command. Most problems it can see already have a fix in skillpack
(relink, complete a pack, register a fork).

## Decisions

1. **Doctor diagnoses more than duplicates.** `internal/doctor.Diagnose`
   returns typed findings: stale installed skill, broken fork upstream,
   partial pack deployment, fork candidate, and Duplicate Set pairs.
2. **Each finding names its fix, and applying it is a user-confirmed action
   in the TUI.** The fix reuses the existing operation for that kind; Doctor
   adds no new write path.
3. **Fixes are non-destructive.** No deleting a duplicate copy and no
   syncing diverged copies. ADR-0003's deferral of propagation stands.
4. **`Diagnose` stays read-only and lives apart from `skill` and `pack`**
   (it imports both; `pack` already imports `skill`).
5. **The `skillpack doctor` CLI command is unchanged**: still a read-only
   Duplicate Set report.

## Consequences

- New package `internal/doctor`; the TUI Doctor panel becomes a selectable
  list (later phases).
- The stale/broken-upstream check now exists in `doctor` and in the TUI's
  `computeSkillProblems`; the TUI copy is removed when the panel adopts
  `Diagnose`.
