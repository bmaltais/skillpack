# ADR-0002 — Git-Host-Agnostic Repos (Azure DevOps Support)

**Status:** Accepted
**Date:** 2026-09-02 (amended same day — see Decision 7)

---

## Context

skillpack's git operations (clone, fetch, push) already go through `go-git`
directly — SSH-agent auth or HTTPS Basic Auth with a token — and never shell
out to the `gh` CLI. The only GitHub-specific surfaces were the `GITHUB_TOKEN`
env var fallback, a `gho_`-prefix token-expiry hint, and docs that assumed
`gh auth token` as the way to obtain a credential.

Adding "Azure DevOps support" was requested. The user was unavailable to
answer clarifying questions, so this ADR records the autonomous decisions
made and the reasoning, mirroring the existing "no per-agent adapter" rule
for Agents (see root `AGENTS.md`).

One real bug was found in the process: `NameFromURL` infers a repo name from
the URL's last two path segments (`owner/repo` → `owner-repo`). Azure DevOps
HTTPS URLs have the shape `https://dev.azure.com/{org}/{project}/_git/{repo}`
— the literal `_git` routing segment sat where "owner" was expected, producing
a nonsensical name like `_git-myrepo`.

---

## Decisions

### 1. No provider/host abstraction

skillpack does not detect or special-case git hosts (`github.com`,
`dev.azure.com`, `visualstudio.com`, self-hosted, etc.). A repo is just a
name + URL + optional token. This mirrors the existing Agent design
constraint ("Agents are config-only, no per-agent adapter files") — extending
it to Repos rather than introducing a parallel `internal/provider/` package.

**Rejected alternative:** host detection to pick tailored auth hints or a
provider-specific env var (e.g. `AZURE_DEVOPS_EXT_PAT`). Rejected because the
existing `SKILLPACK_GIT_TOKEN` / `credentials:` map are already
provider-agnostic and sufficient; a second env var per host would be an
ever-growing list with no functional benefit.

### 2. No `az` CLI dependency

Token acquisition stays manual: the user creates a PAT in Azure DevOps
(**User Settings → Personal Access Tokens**, scope `Code: Read` for
pull-only repos, `Code: Read & Write` to publish/fork) and pastes it into
`credentials:` in `config.yaml` or an env var — the same pattern already used
for GitHub.

**Rejected alternative:** shelling out to `az account get-access-token` to
auto-fetch a Microsoft Entra token. Rejected because it would add a new
runtime dependency (the `az` CLI) that most users won't have installed,
for a workflow (`gh auth token`) that today is also just a documented manual
step, not code skillpack executes itself.

### 3. HTTPS Basic Auth username stays `x-access-token` for all hosts

Confirmed via Azure DevOps documentation: "Git interactions require a
username, which can be anything except an empty string." The existing
hardcoded `x-access-token` username (a GitHub App token convention) works
identically against Azure DevOps's PAT-over-Basic-Auth — no host-specific
username branching needed.

### 4. Fixed: `_git` is stripped as a non-semantic path segment

`NameFromURL` now filters out any path segment literally equal to `_git`
before taking the last two segments for `owner-repo` inference. This is a
host-agnostic string rule (not an `if host == "dev.azure.com"` branch) — it
simply treats `_git` as a routing marker wherever it appears.

| URL | Inferred name |
|---|---|
| `https://dev.azure.com/myorg/myproject/_git/myrepo` | `myproject-myrepo` |
| `https://myorg.visualstudio.com/myproject/_git/myrepo` | `myproject-myrepo` |
| `git@ssh.dev.azure.com:v3/myorg/myproject/myrepo` | `myproject-myrepo` |

### 5. "Public" Azure DevOps repos need no special handling

Azure DevOps's anonymous-clone "public projects" behave exactly like a
public GitHub repo already does in skillpack: omit `--token`, `go-git`
performs an unauthenticated clone. No code path treats "public" as a
distinct case today for any host, so none was added for Azure DevOps either.

### 6. Fixed: embedded URL userinfo is stripped before use

Real-world failure: a URL copied from a browser address bar
(`https://<org>@dev.azure.com/<org>/<project>/_git/<repo>`) combined with a
separately supplied `--token` produced an HTTP 400 from Azure DevOps —
Microsoft's own docs warn about this exact pattern ("If you previously
added the origin using a username, remove it"). skillpack authenticates via
an explicit `Auth` transport, so an embedded username is always redundant.
`gitops.NormalizeURL` strips `user[:pass]@` from HTTPS/HTTP URLs (SSH URLs,
where `git@` is required syntax, are left alone) before the URL is cloned,
stored in state, or compared for name-collision checks.

### 7. Amendment: go-git cannot talk to Azure DevOps at all — system-git fallback required

Decision 1 above ("no provider/host abstraction") was wrong for one specific,
narrow case, discovered only after real-world testing against the user's own
ADO org: even with a clean URL and valid PAT, clone failed with
`unexpected client error ... status code: 400` on `git-upload-pack`.

Root cause: Azure DevOps's smart-HTTP git server requires the
`multi_ack`/`multi_ack_detailed` capability during fetch negotiation.
`go-git` has never implemented it — see
[go-git/go-git#64](https://github.com/go-git/go-git/issues/64), open since
2020 with 100+ comments and no fix. This is not an auth or URL bug; it is a
hard protocol incompatibility that affects **every** clone/fetch (and,
untested but not risked, push) against `dev.azure.com` / `*.visualstudio.com`,
regardless of credentials. Every other project that hit this (pulumi, flux,
nuclio, kpack, source-controller) worked around it the same way: detect the
ADO host and shell out to the system `git` binary instead of go-git for that
remote.

**Decision:** `gitops.IsAzureDevOpsURL` detects `dev.azure.com` /
`*.visualstudio.com` hosts. When true, `repo.Add` (clone), `repo.Update`
(fetch), and `gitops.CommitAndPush` (push) all route through
`gitops.SystemGit{Clone,Fetch,Push}`, which shell out to the system `git`
binary, instead of `go-git`. This is a **narrow, evidence-backed exception**
to "no host detection" — not a reversal of the broader philosophy. No new
config, env var, or user-facing flag was added; the routing is entirely
internal and transparent.

Auth for the system-git path is passed via an `Authorization: basic
<base64(:token)>` header injected through `git --config-env=http.extraheader=<ENV_VAR>`
(git 2.31+), with the header value carried in an env var rather than argv —
keeping the PAT out of process listings (`ps`), matching Microsoft's own
documented pattern for scripting ADO auth. SSH URLs and auth continue to work
as before (system git handles SSH via the user's own `ssh-agent`/config, same
as a manual `git clone` would).

**Consequence:** skillpack now has a runtime dependency on the `git` binary
being on `PATH` for Azure DevOps repos specifically (not for GitHub/GitLab/
other hosts, which remain pure-Go via go-git). This is considered acceptable:
every developer machine capable of running `skillpack repo add` already has
git installed to have cloned skillpack itself.

---

## Consequences

- Azure DevOps (and any other git host) repos work via
  `skillpack repo add <url> --token <PAT>`, exactly like GitHub, with no new
  flags, env vars, or dependencies.
- The only functional change required was the `_git` segment fix in
  `NameFromURL`.
- Auth-failure hints remain generic for non-GitHub hosts (no misleading
  `gh auth token` hint is ever shown for an Azure DevOps failure, since
  `FormatAuthHint` only fires on a `gho_`-prefixed token).
