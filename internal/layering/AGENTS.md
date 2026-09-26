# AGENTS.md — Layering Checks

## Purpose

Tests that enforce the mechanical Layering rules from `CODING_STANDARDS.md`: `cmd` never calls `state.Save`, and `internal/*` never prints.

## Ownership

| Concern | Owner |
|---------|-------|
| Source-scanning layering checks | `internal/layering/layering_test.go` |

## Local Contracts

- Each check scans non-test `.go` files for a pattern and fails on any match outside an allowlist.
- `internalPrintDebt` lists the files that still print; it only shrinks. Remove an entry once its output becomes part of a returned result.

## Work Guidance

- A new mechanical Layering rule becomes a test here, not a line in `CODING_STANDARDS.md`.

## Verification

- `go test ./internal/layering/...`

## Child DOX Index

None.
