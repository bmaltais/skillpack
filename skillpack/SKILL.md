---
name: skillpack
description: >
  Manage AI agent skills — install, update, publish, and sync skill directories
  across multiple AI agents (Claude Code, Copilot, Hermes, Pi, etc.).
categories: [software-development, ai-tools, cli]
agents: [pi, hermes, claude, copilot]
version: 1.1.0
---

# skillpack

CLI for managing AI agent skills stored in git repositories.

## Core Concepts

| Term | Meaning |
|------|---------|
| **Skill** | A directory containing a `SKILL.md` file, installed into an agent's skill folder |
| **Skill Address** | `<repo-name>/<rel/path/in/repo>` — e.g. `awesome-skills/coding/debugger` |
| **Repo** | A git repository registered in skillpack; cloned to `~/.skillpack/repos/<name>` |
| **Agent** | An AI tool with a skill directory (e.g. `~/.claude/skills/`) |
| **Installed Hash** | SHA-256 of a skill's installed files — used to detect local edits |
| **Conflict** | A skill that has both local modifications AND upstream changes |

## Setup

Config: `~/.skillpack/config.yaml`  
State:  `~/.skillpack/state.json`

```yaml
# ~/.skillpack/config.yaml
default_agent: claude-code
agents:
  claude-code:
    skill_dir: ~/.claude/skills
  copilot:
    skill_dir: ~/.copilot/skills
# Optional: per-repo tokens for private HTTPS repos
credentials:
  my-private-repo: ghp_yourtoken
```

Token lookup order: `credentials` in config → `SKILLPACK_GIT_TOKEN` env var → `GITHUB_TOKEN` env var.

## Commands

### Repo Management

```bash
skillpack repo add <url>              # register + clone (name inferred as owner-repo)
skillpack repo add <url> --name <n>   # explicit name
skillpack repo add <url> --token <t>  # private HTTPS repo — token saved to config
skillpack repo list                   # list registered repos
skillpack repo remove <name>          # unregister (cache kept on disk)
skillpack repo update [<name>]        # git pull one or all repos
skillpack repo rename <old> <new>     # rename a repo (updates state, cache dir, installed skill addresses)
```

### Installing Skills

> **IMPORTANT — skill addresses are full repo-relative paths, not just skill names.**
> `skillpack install my-repo/diagnose` will fail if the skill lives at `my-repo/skills/engineering/diagnose`.
> Always run `skillpack list --available --repo <name>` first to get the exact address.

```bash
skillpack list --available --repo my-repo              # discover exact addresses BEFORE installing
skillpack install <repo>/<path/to/skill>               # default agent
skillpack install <addr> --agent claude-code           # specific agent
skillpack install <addr> --all-agents                  # every configured agent
skillpack install <addr> --skip-existing               # no-op if already installed
```

### Removing Skills

```bash
skillpack remove <repo>/<path/to/skill>
skillpack remove <addr> --agent claude-code
skillpack remove <addr> --all-agents
skillpack remove <addr> --force        # remove even if locally modified
```

### Listing Skills

```bash
skillpack list                         # installed skills (all agents)
skillpack list --agent claude-code     # one agent
skillpack list --modified              # only locally-modified skills
skillpack list --available             # all skills in all registered repos
skillpack list --available --repo <r>  # browse one repo (grouped by category)
```

### Checking for Updates

```bash
skillpack update                              # check + update all installed skills
skillpack update <addr>                       # specific skill
skillpack update --dry-run                    # preview only

# Conflict resolution (required when skill has local edits AND upstream changes):
skillpack update --force-remote <addr>        # upstream wins — overwrites local
skillpack update --force-local  <addr>        # local wins — pushes to remote
skillpack update --merge        <addr>        # file-level 3-way merge
```

### Publishing Skills

```bash
skillpack publish <repo>/<path/to/skill>      # push local edits to remote
skillpack publish <addr> --agent claude-code  # specific agent's copy
skillpack publish ./my-new-skill --repo <r>   # add a brand-new skill to a repo
skillpack publish <addr> --dry-run            # preview only
```

### Syncing Everything

```bash
skillpack sync           # pull all repos, then update + publish all installed skills
skillpack sync --dry-run # preview what would change
```

### Forking a Skill

```bash
skillpack fork <addr> <my-repo>              # copy skill into your own repo, track upstream origin
skillpack fork <addr> <my-repo> --agent <n>  # fork from a specific agent's installed copy
```

If `<my-repo>/<skill-name>` already exists, `fork` handles it gracefully:
- same tracked upstream → re-fork in place (overwrite cache copy + refresh upstream SHA)
- different tracked upstream → clear collision error with conflicting upstream
- exists on disk but no tracked fork provenance → clear unknown-provenance error

Forked skills carry provenance metadata in `.skillpack-fork`:

```json
{
  "upstream_addr": "source-repo/path/to/skill",
  "upstream_sha": "<upstream HEAD SHA at fork time>"
}
```

On install, this metadata is imported into state so `update` and `sync` can track upstream drift automatically.

After forking, `skillpack update` detects upstream changes as conflicts.
Use `skillpack update --merge <addr>` (or `--merge --llm`) to pull them in.

### Retroactively Adding Missing Fork Metadata

Skills forked before `.skillpack-fork` tracking was introduced (or forked manually)
will be missing the metadata file in the repo cache. They still display correctly
if state has `UpstreamAddr` set, but anyone who installs from your repo won't
inherit the provenance.

> **Preferred approach — use the helper script:**
> ```bash
> skillpack/scripts/retroforkt <my-repo> <skill-name> <upstream-addr>
> # e.g.
> skillpack/scripts/retroforkt bmaltais-skills triage mattpocock-skills/skills/engineering/triage
> ```
> The script handles all four steps (write file, commit+push, copy to agents, patch state.json)
> atomically. Use the manual steps below only if the script is not available.

> **Why not `skillpack fork` again?**
> `skillpack fork` blocks multi-hop forks: once a skill is already tracked in
> state as a fork (has `UpstreamAddr`), running `fork` on it will fail with
> *"multi-hop forks are not supported"*. Manual metadata injection is the only
> path.

**Detect affected skills** (skills in state with an upstream but no file on disk):

```bash
find ~/.skillpack/repos -mindepth 2 -maxdepth 2 -name "SKILL.md" \
  | while read f; do
      dir=$(dirname "$f")
      skill=$(basename "$dir")
      repo=$(basename "$(dirname "$dir")")
      [ ! -f "$dir/.skillpack-fork" ] && echo "$repo/$skill"
    done
```

Cross-reference the output against `skillpack status` — any skill listed there
as `[fork of ...]` that also appears above is missing the file.

To fix, write the file directly into the repo cache and commit+push:

```bash
# 1. Get the upstream HEAD SHA
UPSTREAM_SHA=$(cd ~/.skillpack/repos/<upstream-repo> && git rev-parse HEAD)

# 2. Write the metadata file
cat > ~/.skillpack/repos/<my-repo>/<skill-name>/.skillpack-fork << EOF
{
  "upstream_addr": "<upstream-repo>/path/to/skill",
  "upstream_sha": "$UPSTREAM_SHA"
}
EOF

# 3. Commit and push
cd ~/.skillpack/repos/<my-repo>
git add <skill-name>/.skillpack-fork
git commit -m "skillpack: add fork provenance metadata for <skill-name>"
git push
```

Also copy the file into each agent's installed copy so local state stays consistent:

```bash
FORK_META=~/.skillpack/repos/<my-repo>/<skill-name>/.skillpack-fork
for dir in ~/.copilot/skills ~/.claude/skills ~/.hermes/skills ~/.pi/agent/skills; do
  target="$dir/<skill-name>"
  [ -d "$target" ] && cp "$FORK_META" "$target/.skillpack-fork" && echo "wrote $target/.skillpack-fork"
done
```

After this, anyone installing from your repo will automatically get the provenance.

**If `skillpack status` still doesn't show `[fork of ...]`**, state has no `UpstreamAddr`
(skill was added to the repo manually, never via `skillpack fork`). Patch it directly:

```bash
python3 << 'PYEOF'
import json
STATE = "/home/YOUR_USER/.skillpack/state.json"  # adjust path
with open(STATE) as f: s = json.load(f)
for agent in s["installed_skills"].get("<my-repo>/<skill-name>", {}):
    s["installed_skills"]["<my-repo>/<skill-name>"][agent]["upstream_addr"] = "<upstream-repo>/path/to/skill"
    s["installed_skills"]["<my-repo>/<skill-name>"][agent]["upstream_sha"] = "<upstream-sha>"
with open(STATE, "w") as f: json.dump(s, f, indent=2); f.write("\n")
PYEOF
```

### Self-Update

```bash
skillpack self-update   # check for a newer release and print the upgrade command
```

Sync logic per installed skill:
- No local edits + upstream changed → **auto-update**
- Local edits + no upstream change → **auto-publish**
- Both local edits AND upstream changed → **skip, report conflict**

## Conflict Workflow

When `sync` or `update` reports a CONFLICT:

```bash
skillpack update --force-remote <addr>        # discard local edits, take upstream
skillpack update --force-local  <addr>        # push local version, mark as upstream
skillpack update --merge        <addr>        # 3-way merge; writes conflict markers on failure
skillpack update --merge --llm  <addr>        # 3-way merge + LLM-assisted conflict resolution
skillpack update --merge --llm <agent> <addr> # use a specific LLM agent to resolve
```

After `--merge` with conflicts, resolve `<<<<<<< ours` / `>>>>>>> theirs` blocks
in the installed files, then run `skillpack publish <addr>` to push the result.

## Common Workflows

### First time setup

```bash
skillpack repo add https://github.com/example/my-skills.git
skillpack list --available
skillpack install my-skills/coding/debugger
```

### Daily driver

```bash
skillpack sync   # pulls updates, publishes your local edits
```

### Contributing a new skill

> **RULE: After publishing a brand-new skill, install it for the agent whose skill dir it came from.**
> A publish only pushes to the repo — it does not register the skill in state.
> Determine the owning agent by matching the skill's local path against each agent's `skill_dir` in config.

```bash
# Example: skill was created in ~/.copilot/skills/ → owning agent is "copilot"
skillpack publish ~/.copilot/skills/my-new-skill --repo my-skills
skillpack install my-skills/my-new-skill --agent copilot   # REQUIRED — use the agent that owns the source dir

# If the skill was created outside any agent dir (e.g. ~/my-new-skill), ask the user which agent to install for.
```

### Check version

```bash
skillpack --version
```

### Editing an installed skill and pushing back

```bash
# Edit files in the skill's installed dir (e.g. ~/.claude/skills/debugger/)
skillpack publish my-skills/coding/debugger   # pushes to git
```
