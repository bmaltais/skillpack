# AGENTS.md — Git Operations

## Purpose

Deep git operations abstraction: auth, commit, push, diff, file listing. Consolidates go-git ceremony behind a small interface.

## Ownership

| Concern | Owner |
|---------|-------|
| SSH/HTTPS auth resolution | `internal/gitops/gitops.go` (`Auth`) |
| SSH URL detection | `internal/gitops/gitops.go` (`IsSSHURL`) |
| Commit + push (with rollback on failure) | `internal/gitops/gitops.go` (`CommitAndPush`) |
| HEAD SHA lookup | `internal/gitops/gitops.go` (`HeadSHA`) |
| Skill diff between commits | `internal/gitops/gitops.go` (`DiffSkillChanged`, `DiffSkillChangedFromHEAD`) |
| File listing at commit | `internal/gitops/gitops.go` (`ListFilesAtCommit`) |

## Local Contracts

- `Auth` returns SSH agent auth for `git@`/`ssh://` URLs, BasicAuth for HTTPS with token, nil otherwise.
- `CommitAndPush` stages only files under `skillRelPath`. No empty commits.
- `CommitAndPush` rolls back the local commit if push fails (preserves cache HEAD SHA).
- `DefaultSignature` uses `skillpack <skillpack@local>` — not user identity.
- `DiffSkillChanged` compares tree objects directly — no worktree required.
- `pathUnderPrefix` checks exact match or `prefix/` prefix. Empty inputs return false.

## Work Guidance

- New git operations: add to `internal/gitops/gitops.go`. Keep the interface small.
- Auth: call `Auth(url, token)` — never build transport auth inline.
- Error handling: `CommitAndPush` returns `CommitResult{Committed: false}` when no changes staged.

## Verification

- `go test ./internal/gitops/...`

## Child DOX Index

None.