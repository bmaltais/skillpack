# AGENTS.md — Git Operations

## Purpose

Deep git operations abstraction: auth, commit, push, diff, file listing. Consolidates go-git ceremony behind a small interface.

## Ownership

| Concern | Owner |
|---------|-------|
| SSH/HTTPS auth resolution | `internal/gitops/gitops.go` (`Auth`) |
| SSH URL detection | `internal/gitops/gitops.go` (`IsSSHURL`) |
| Strip embedded URL userinfo | `internal/gitops/gitops.go` (`NormalizeURL`) |
| Azure DevOps host detection | `internal/gitops/systemgit.go` (`IsAzureDevOpsURL`) |
| System-git clone/fetch/push fallback (ADO only) | `internal/gitops/systemgit.go` (`SystemGitClone`, `SystemGitFetch`, `SystemGitPush`) |
| Commit + push (with rollback on failure) | `internal/gitops/gitops.go` (`CommitAndPush`) |
| HEAD SHA lookup | `internal/gitops/gitops.go` (`HeadSHA`) |
| Skill diff between commits | `internal/gitops/gitops.go` (`DiffSkillChanged`, `DiffSkillChangedFromHEAD`) |
| File listing at commit | `internal/gitops/gitops.go` (`ListFilesAtCommit`) |

## Local Contracts

- `Auth` returns SSH agent auth for `git@`/`ssh://` URLs, BasicAuth for HTTPS with token, nil otherwise.
- `NormalizeURL` strips `user[:pass]@` from HTTPS/HTTP URLs before they're cloned, stored, or compared — auth always goes through `Auth`'s separate token, so embedded userinfo is redundant and some hosts (Azure DevOps observed) reject it with a protocol-level 400. SSH URLs are untouched. Call it once at every entry point that accepts a raw user-supplied URL (`repo.Add`, the CLI's `repo add` command) — see [ADR-0002](../../docs/adr/0002-git-host-agnostic-repos.md).
- **go-git cannot talk to Azure DevOps at all** (missing `multi_ack` capability — [go-git/go-git#64](https://github.com/go-git/go-git/issues/64), unfixed since 2020). Every clone/fetch/push against `dev.azure.com`/`*.visualstudio.com` must route through `SystemGit{Clone,Fetch,Push}` instead of go-git's `PlainClone`/`Fetch`/`Push`. `repo.Add`, `repo.Update`, and `push()` in `gitops.go` all branch on `IsAzureDevOpsURL` — if you add a new git network operation, add the same branch. See [ADR-0002](../../docs/adr/0002-git-host-agnostic-repos.md) decision 7.
- System-git functions inject the PAT via `git --config-env=http.extraheader=<ENV_VAR>` with the header value in an env var, never argv — keeps the token out of `ps`. Do not pass tokens as command-line args to `exec.Command`.
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