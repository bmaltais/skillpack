# AGENTS.md — Repository Management

## Purpose

Clones, updates, and discovers skills in git repos. Owns the repo cache at `~/.skillpack/repos/<name>/`.

## Ownership

| Concern | Owner |
|---------|-------|
| Clone/register/remove repos | `internal/repo/repo.go` (`Add`, `Remove`) |
| Fetch + hard-reset to origin/HEAD | `internal/repo/repo.go` (`Update`) |
| Skill discovery (walk cache, find SKILL.md) | `internal/repo/repo.go` (`DiscoverSkills`, `DiscoverAllSkills`) |
| Address → skill resolution | `internal/repo/repo.go` (`FindSkill`) |
| HEAD SHA lookup | `internal/repo/repo.go` (`HeadSHA`) |
| Repo name inference from URL | `internal/repo/repo.go` (`NameFromURL`) |
| Cache rename | `internal/repo/repo.go` (`RenameCache`) |
| Git auth (SSH/HTTPS) | `internal/repo/auth_test.go` (delegated to `gitops.Auth`) |

## Local Contracts

- Cache lives at `~/.skillpack/repos/<repo-name>/`. One dir per repo.
- Cache is a read-only mirror: never edited by users. Updates use `git fetch` + `git reset --hard`.
- `Update` recovers from stale credentials by retrying anonymously for public repos.
- `resolveRemoteHEAD` tries `refs/remotes/origin/HEAD` first, then derives from checked-out branch tracking ref. Never hardcode "main"/"master".
- `DiscoverSkills` skips `.git` dirs only. Other hidden dirs (e.g. `.agents/`) may contain skills.
- `NameFromURL` infers `<owner>-<repo>` from HTTPS or SSH URLs. Single-segment URLs become the name as-is.

## Work Guidance

- New git operations: delegate to `internal/gitops` for auth, commit, push, diff.
- Auth resolution: call `gitops.Auth(url, token)` — never build auth inline.
- Token flow: CLI resolves token → passes to `repo.Add/Update`. `gitops.Auth` handles the rest.

## Verification

- `go test ./internal/repo/...`

## Child DOX Index

- `internal/gitops/` — Git operations abstraction (auth, commit, push, diff)