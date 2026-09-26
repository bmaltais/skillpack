# Coding Standards

Judgement-call conventions for this repo. `/code-review` reads this file as its Standards source. Mechanical rules live in CI (`.github/workflows/ci.yml`) and the pre-commit hook (`.githooks/`), not here. Architecture constraints live in `AGENTS.md`; domain terms in `CONTEXT.md`.

## Paths

- Skill addresses (`repo/dir/skill`) are slash-joined logical keys. Filesystem paths go through `filepath.Join()` and `os.UserHomeDir()`; convert with `filepath.ToSlash` when deriving an address from a path.

## Layering

- Orchestration that calls more than one domain function (install, update, remove, sync flows) lives in `internal/*`; `cmd/skillpack` resolves flags and renders the result, for the CLI and the TUI alike.
- `internal/*` never prints. Long operations report through an optional progress callback and return a structured result (see `internal/pack`).

## Maintenance

Add a rule when a review or correction establishes a convention no check can enforce. Turn any rule that a linter or test could enforce into a check instead. Keep each rule one line. Remove rules the code no longer follows.
