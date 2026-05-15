<p align="center">
  <img src="assets/SkillPack.svg" alt="SkillPack" width="200"/>
</p>

<h1 align="center">skillpack</h1>

<p align="center">
  Install, update, publish, and sync AI agent skills across multiple agents —<br/>
  Claude Code, GitHub Copilot, Hermes, Pi, and more.
</p>

<p align="center">
  <a href="#installation">Installation</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#commands">Commands</a> ·
  <a href="#concepts">Concepts</a> ·
  <a href="#configuration">Configuration</a>
</p>

---

## What is skillpack?

Skills are directories containing a `SKILL.md` file that teach AI agents how to perform specific tasks — debugging, writing plans, creating PRs, and so on. They live in git repositories and are installed into each agent's skill folder (e.g. `~/.claude/skills/`, `~/.copilot/skills/`).

**skillpack** is the package manager for skills: one command to install them, keep them up to date, push local edits back to the repo, and sync everything in one go.

## Installation

### Pre-built binary (Linux & macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/bmaltais/skillpack/main/install.sh | sh
```

The script auto-detects your OS and architecture, installs to `~/.local/bin/skillpack` (no `sudo` required), creates the directory if needed, and prints a `PATH` hint if the install directory is not already on your `PATH`.

**System-wide install** (requires write access to `/usr/local/bin`):

```bash
SKILLPACK_INSTALL_DIR=/usr/local/bin sudo sh -c \
  'curl -fsSL https://raw.githubusercontent.com/bmaltais/skillpack/main/install.sh | sh'
```

**Custom location:**

```bash
SKILLPACK_INSTALL_DIR=~/bin curl -fsSL \
  https://raw.githubusercontent.com/bmaltais/skillpack/main/install.sh | sh
```

### From source (requires Go 1.21+)

```bash
git clone https://github.com/bmaltais/skillpack.git
cd skillpack
go install ./cmd/skillpack/
```

## Quick Start

```bash
# 1. Register a skill repository
skillpack repo add https://github.com/example/my-skills.git

# 1b. Private repo? Pass a token — saved to config for future use
skillpack repo add https://github.com/example/private-skills.git --token ghp_xxx

# 2. Browse available skills
skillpack list --available

# 3. Install a skill
skillpack install my-skills/coding/debugger

# 4. Keep everything up to date
skillpack sync
```

On first run, skillpack detects your installed AI agents and asks which should be the default.

## Commands

### Repos

| Command | Description |
|---------|-------------|
| `skillpack repo add <url>` | Register and clone a skill repo (name inferred as `owner-repo`) |
| `skillpack repo add <url> --token <pat>` | Same, with a saved personal access token |
| `skillpack repo list` | List registered repos |
| `skillpack repo remove <name>` | Unregister a repo (cache kept on disk) |
| `skillpack repo update [<name>]` | `git pull` one or all repos |
| `skillpack repo rename <old> <new>` | Rename a repo (updates state, cache, and installed skill addresses) |

### Skills

| Command | Description |
|---------|-------------|
| `skillpack install <addr>` | Install a skill for the default agent |
| `skillpack install <addr> --agent <name>` | Install for a specific agent |
| `skillpack install <addr> --all-agents` | Install for every configured agent |
| `skillpack remove <addr>` | Remove an installed skill |
| `skillpack list` | List installed skills |
| `skillpack list --available` | Browse all skills in registered repos |
| `skillpack list --modified` | Show locally-edited skills |

### Updates

| Command | Description |
|---------|-------------|
| `skillpack update` | Check and apply upstream updates |
| `skillpack update <addr> --force-remote` | Upstream wins (overwrites local edits) |
| `skillpack update <addr> --force-local` | Local wins (pushes to remote) |
| `skillpack update <addr> --merge` | File-level three-way merge |
| `skillpack update <addr> --merge --llm` | Three-way merge with LLM-assisted conflict resolution |
| `skillpack update <addr> --merge --llm <agent>` | Same, using a specific LLM agent |
| `skillpack update --dry-run` | Preview only |

### Publishing

| Command | Description |
|---------|-------------|
| `skillpack publish <addr>` | Push local edits back to the remote repo |
| `skillpack publish ./my-skill --repo <name>` | Add a new local skill to a repo |
| `skillpack publish --dry-run` | Preview only |

### Forking

| Command | Description |
|---------|-------------|
| `skillpack fork <addr> <my-repo>` | Copy an installed skill into your own repo, tracking upstream origin |
| `skillpack fork <addr> <my-repo> --agent <name>` | Fork from a specific agent's installed copy |

### Sync

```bash
skillpack sync           # pull repos, update clean skills, publish local edits
skillpack sync --dry-run # preview what would change
```

Sync logic per installed skill:

| State | Action |
|-------|--------|
| No local edits, upstream changed | Auto-update |
| Local edits, no upstream change | Auto-publish to remote |
| Both local edits and upstream change | Skip — resolve with `update --force-*` |

## Concepts

| Term | Meaning |
|------|---------|
| **Skill** | Directory containing `SKILL.md`, installed into an agent's skill folder |
| **Skill Address** | `<repo-name>/<path/in/repo>` — e.g. `my-skills/coding/debugger` |
| **Repo** | A registered git repository, cached at `~/.skillpack/repos/<name>` |
| **Agent** | An AI tool with a skill directory (e.g. Claude Code → `~/.claude/skills/`) |
| **Conflict** | A skill with both local edits and upstream changes |

## Configuration

Config lives at `~/.skillpack/config.yaml` and state at `~/.skillpack/state.json`.

```yaml
default_agent: claude-code
agents:
  claude-code:
    skill_dir: ~/.claude/skills
  copilot:
    skill_dir: ~/.copilot/skills
  hermes:
    skill_dir: ~/.hermes/skills
# Optional: per-repo tokens for private HTTPS repos
credentials:
  my-private-repo: ghp_yourtoken
```

Tokens can also be provided via environment variables (useful in CI):
```bash
export SKILLPACK_GIT_TOKEN=ghp_xxx   # takes priority over GITHUB_TOKEN
export GITHUB_TOKEN=ghp_xxx          # fallback (set automatically by GitHub Actions)
```

Token lookup order: `credentials` in config → `SKILLPACK_GIT_TOKEN` → `GITHUB_TOKEN`.

Default agents detected automatically on first run: `claude-code`, `copilot`, `hermes`, `pi`, `opencode`, `openclaw`.

## Other

```bash
skillpack --version    # print the installed version
skillpack self-update  # check for a newer release and print the upgrade command
skillpack --help       # command reference
```

## Contributing Skills

```bash
# 1. Create your skill directory with a SKILL.md
mkdir ~/my-new-skill
echo "# My Skill\n\nDoes something useful." > ~/my-new-skill/SKILL.md

# 2. Publish it to a registered repo
skillpack publish ~/my-new-skill --repo my-skills

# 3. Install it locally
skillpack install my-skills/my-new-skill
```

## License

MIT
