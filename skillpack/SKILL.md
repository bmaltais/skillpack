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

```bash
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

After forking, `skillpack update` detects upstream changes as conflicts.
Use `skillpack update --merge <addr>` (or `--merge --llm`) to pull them in.

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

```bash
mkdir ~/my-new-skill && echo "# My Skill" > ~/my-new-skill/SKILL.md
skillpack publish ~/my-new-skill --repo my-skills
skillpack install my-skills/my-new-skill
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
