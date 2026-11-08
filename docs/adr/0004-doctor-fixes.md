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
3. **Fixes that write to a repo are guarded, not banned.** Added later:
   for a duplicate pair, Doctor can overwrite one copy with the other, or
   delete one of two *identical* copies. Guardrails: the user can push to the
   target repo; the user confirms (default No); deleting requires identical
   content and an uninstalled target; a failed commit or push restores the
   files. This is one-shot, user-chosen, per pair. It is not automatic
   propagation, which ADR-0003 deferred and which remains out of scope.
4. **`Diagnose` stays read-only and lives apart from `skill` and `pack`**
   (it imports both; `pack` already imports `skill`).
5. **The `skillpack doctor` CLI command is unchanged**: still a read-only
   Duplicate Set report.

## Consequences

- `doctor.OverwriteCopy` and `doctor.DeleteCopy` are the only Doctor code
  that writes a repo cache; `Diagnose` and `Diff` stay read-only.
- New package `internal/doctor`; the TUI Doctor panel becomes a selectable
  list (later phases).
- The stale/broken-upstream classification lives only in
  `doctor.SkillProblems`; the Skills panel's `computeSkillProblems` wraps it.
- Duplicate pairs offer a line diff (`doctor.Diff`) opened in the platform
  viewer; the other kinds reuse the relink, pack-complete and
  fork-registration flows.
