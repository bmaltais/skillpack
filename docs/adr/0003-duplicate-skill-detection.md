# ADR-0003 — Duplicate Skill Detection (`skillpack doctor`)

**Status:** Accepted
**Date:** 2026-09-02

---

## Context

Users end up with copies of the same skill living in multiple registered
repos with no tracked relationship between them — sometimes copy-pasted
independently, sometimes an old fork whose provenance was lost. Today,
`DetectForkCandidates` only surfaces this for skills that are currently
*installed* (via the `[fork?]` hint in `status`/TUI), and only by directory
basename, with no signal about whether the copies still agree.

This ADR resolves how skillpack detects and reports these duplicates. It
deliberately does **not** decide how (or whether) to keep duplicates in sync
— that is out of scope until a real need for it is demonstrated.

---

## Decisions

### 1. Detection and reporting only — no propagation in v1

`skillpack doctor` reports duplication; it does not push edits between
copies, offer an "auto-sync" action, or write anything to disk or state.
Propagation (keeping copies in sync automatically) is a materially bigger
feature — trigger model, conflict handling, blast radius of pushing to a
repo the user didn't mean to touch — and is deferred until detection alone
has been used long enough to know it's actually wanted.

### 2. New concept: Duplicate Set, identified by basename — no persisted ID

A **Duplicate Set** is a set of skill addresses across ≥2 registered repos
that appear to be the same skill. Its identity is the directory basename,
computed fresh on every run — not a new persisted ID assigned at first
detection.

**Rationale:** the basename is already the matching key `findSkillInRepo`
uses today, requires no new state schema, and is trivially deterministic. A
persisted ID would only earn its cost once a future propagation feature
needs to track something about the group that can't be derived from the
name alone (e.g. a per-group auto-sync flag) — that bridge is crossed if
and when propagation is actually designed, keyed off this same basename.

**Rejected alternative:** assigning a stable UUID per set at detection time.
Rejected as speculative: it adds a state-schema change today to support a
feature (propagation) that isn't approved yet.

### 3. Matching: basename + `SKILL.md` frontmatter `name:` cross-check

Detection scans *all* registered repos' caches (not just installed skills)
for directories sharing a basename, then cross-checks each candidate's
`SKILL.md` frontmatter `name:` field to raise confidence and cut noise from
generically-named directories (e.g. two unrelated `utils` folders).

**Rejected alternative:** basename-only matching (today's `ForkCandidate`
behaviour). Sufficient for the installed-skills-only scope it was built for,
but scanning every repo cache is a much larger surface where generic names
collide more often; the frontmatter field is data already parsed for
discovery, so the cross-check is nearly free.

### 4. Confidence label is informational, not a filter

Each Duplicate Set member pair is annotated `identical` (content hash
matches) or `diverged` (same identity, different content) via the existing
`ComputeHash`. This is a label, not a filter — a diverged pair is still
reported, since divergence is exactly the kind of thing a user investigating
duplication wants to see, not have hidden.

### 5. Link status is a separate, orthogonal label

Pairs that already carry `.skillpack-fork` provenance are labeled
`linked (fork)`; all others are `unlinked`. Both appear in the same report
rather than two separate commands or a filtered-out "already handled" set —
a `linked` pair that has diverged is exactly as actionable as an `unlinked`
one, so hiding it would throw away information the report exists to surface.

### 6. New top-level command: `skillpack doctor`

Duplicate-set detection does not extend `status`. `status` is scoped to
local install state (installed skills, agents, packs); this check is scoped
to repo caches regardless of install state — a different shape of data.
`doctor` is introduced as a new top-level command, leaving room for future
non-install health checks (e.g. broken pack references, orphaned fork
metadata) without overloading `status`'s existing, clean scope.

### 7. Any future propagation reuses the existing single-upstream Fork model

If propagation is built later, it extends today's `Fork`
(`UpstreamAddr`/`UpstreamSHA`, one parent per fork — multi-hop forks are
already rejected) as hub-and-spoke: each participating repo is a single-hop
fork of one canonical source. A full mesh (any copy can be a source of
truth, propagating to all siblings) was considered and rejected — it
multiplies conflict-resolution cases for no demonstrated benefit over
reusing the model that already exists.

---

## Consequences

- `CONTEXT.md` gains the **Duplicate Set** term (and its `identical`/
  `diverged` confidence and `linked`/`unlinked` status labels).
- A new `skillpack doctor` command is added; `internal/skill` gains
  duplicate-set detection logic (likely alongside or reusing parts of
  `fork_candidates.go`, extended to scan all repo caches rather than just
  installed skills).
- No `state.json` schema change. No new config keys.
- Propagation (auto-sync between copies) remains explicitly out of scope
  until a future ADR decides otherwise.
