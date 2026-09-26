# Coding Standards

Judgement-call conventions for this repo. `/code-review` reads this file as its Standards source. Mechanical rules live in CI (`.github/workflows/ci.yml`) and the pre-commit hook (`.githooks/`), not here. Architecture constraints live in `AGENTS.md`; domain terms in `CONTEXT.md`.

## Paths

- Skill addresses (`repo/dir/skill`) are slash-joined logical keys. Filesystem paths go through `filepath.Join()` and `os.UserHomeDir()`; convert with `filepath.ToSlash` when deriving an address from a path.

## Maintenance

Add a rule when a review or correction establishes a convention no check can enforce. Turn any rule that a linter or test could enforce into a check instead. Keep each rule one line. Remove rules the code no longer follows.
