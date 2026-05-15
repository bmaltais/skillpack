# SkillPack — Agent Skill Management CLI

A CLI tool for managing AI agent skills across multiple agents on a local system. Point at a skill repository, install individual skills for specific agents, maintain locally-updated skills, push changes back to the central repo, add new local skills to the central repo, remove local skills, etc.

## Core Design

### Agent Abstraction

Each agent has its own skill directory and format. The tool doesn't convert between formats — it tracks which skills are installed for which agents.

```yaml
# skill-repo config
agents:
  claude-code:
    skill_dir: ~/.claude/skills
    format: skilm-md  # SKILL.md files
    extension: .md
  hermes-agent:
    skill_dir: ~/.hermes/skills
    format: hermes-skill  # SKILL.md + references/ + templates/
    extension: .md
  openclaw:
    skill_dir: ~/.openclaw/skills
    format: openclaw-yaml  # YAML-based skill definitions
    extension: .yaml
```

### Skill Repository Format

Central skill repos follow a consistent structure:

```
skill-repo/
├── SKILL.md              # Primary instruction file (<500 lines)
├── references/           # Deep-dive reference files
│   ├── api-reference.md
│   └── examples.md
├── templates/            # Reusable templates
│   └── config-template.yaml
├── scripts/              # Helper scripts
│   └── validate.sh
└── SKILL.md.sig          # Signature file (for integrity)
```

### Local State

Track installed skills, repo mappings, and modification status in a local database (SQLite or JSON):

```json
{
  "repos": {
    "awesome-copilot": {
      "url": "https://github.com/awesome-copilot",
      "branch": "main",
      "version": "v1.2.0",
      "last_updated": "2026-05-15T10:46:00Z"
    }
  },
  "installed_skills": {
    "awesome-copilot/documentation-writer": {
      "repo": "awesome-copilot",
      "agent": "claude-code",
      "version": "v1.2.0",
      "local_path": "~/.claude/skills/documentation-writer",
      "modified": false
    },
    "awesome-copilot/documentation-writer": {
      "repo": "awesome-copilot",
      "agent": "hermes-agent",
      "version": "v1.2.0",
      "local_path": "~/.hermes/skills/documentation-writer",
      "modified": false
    }
  }
}
```

## Commands

### Repository Management

```
skill-repo add <remote-url> [--name <name>] [--branch <branch>]
skill-repo list                              # show all skill repos
skill-repo remove <repo-name>                # remove a skill repo (does not remove installed skills)
skill-repo update <repo-name>                # update repo reference (fetch latest)
```

### Skill Installation

```
skill install <repo>/<skill>                 # install skill for all configured agents
skill install --agent <agent> <repo>/<skill> # install skill for specific agent
skill install <repo>/<skill>@<version>       # version pin
skill install --skip-existing <repo>/<skill> # skip if already installed
```

### Skill Listing

```
skill list                                   # list all installed skills
skill list --agent <agent>                   # list skills for specific agent
skill list --local                           # show locally-modified skills
skill list --repo <repo-name>                # list skills from specific repo
```

### Skill Updates

```
skill update                                 # check for updates on all installed skills
skill update <repo>/<skill>                  # update specific skill
skill update --dry-run                       # show what would update without changing
```

### Skill Removal

```
skill remove <repo>/<skill>                  # uninstall skill for all agents
skill remove --agent <agent> <repo>/<skill>  # uninstall for specific agent
skill remove --force <repo>/<skill>          # force remove even if locally modified
```

### Skill Publishing

```
skill publish <repo>/<skill>                 # push local skill changes to repo
skill publish <new-skill-path>               # add a brand new local skill to a repo
skill publish --tag <version> <repo>/<skill> # publish with version tag
```

## Implementation Phases

### Phase 1: Core Infrastructure (Weekend 1)

- [ ] Initialize Go project structure
- [ ] Implement config loading (YAML config for agent definitions)
- [ ] Implement local state management (JSON-based state file)
- [ ] Implement repo management (add, list, remove)
- [ ] Implement basic skill install/remove for a single agent

### Phase 2: Multi-Agent Support (Weekend 2)

- [ ] Add agent adapter abstraction
- [ ] Implement skill install for multiple agents
- [ ] Implement skill list with agent filtering
- [ ] Implement skill remove with agent filtering

### Phase 3: Update & Conflict Detection (Weekend 3)

- [ ] Implement skill update (compare local SKILL.md frontmatter version against remote)
- [ ] Implement conflict detection (detect locally-modified skills)
- [ ] Add warning when updating a modified skill

### Phase 4: Publishing (Weekend 4)

- [ ] Implement skill publish (git add, commit, push)
- [ ] Add version tagging support
- [ ] Add conflict warning when publishing modified skills

### Phase 5: Polish & UX (Weekend 5)

- [ ] Add dry-run mode for update/publish
- [ ] Add interactive prompts for conflict resolution
- [ ] Add color output / formatting
- [ ] Add help text and examples

## Tech Stack

- **Language:** Go (single binary, no dependency issues)
- **Git integration:** `go-git` (programmatic access to repos)
- **State storage:** JSON file (simple, no external dependencies)
- **Config:** YAML (human-readable, easy to extend)
- **CLI framework:** `cobra` (standard Go CLI framework)

## File Structure

```
skill-repo/
├── cmd/
│   └── skill-repo/
│       ├── root.go          # cobra root command
│       ├── repo.go          # repo subcommands
│       ├── install.go       # install subcommand
│       ├── list.go          # list subcommand
│       ├── update.go        # update subcommand
│       ├── remove.go        # remove subcommand
│       └── publish.go       # publish subcommand
├── internal/
│   ├── config/
│   │   ├── config.go        # config loading and validation
│   │   └── config.yaml      # default config template
│   ├── repo/
│   │   └── repo.go          # repo management (add, list, remove)
│   ├── agent/
│   │   ├── agent.go         # agent abstraction
│   │   └── claude.go        # Claude Code adapter
│   │   ├── hermes.go        # Hermes Agent adapter
│   │   └── openclaw.go      # OpenClaw adapter
│   └── skill/
│       └── skill.go         # skill management (install, update, remove)
├── pkg/
│   ├── state/
│   │   └── state.go         # local state management
│   └── convert/
│       └── convert.go       # (future) skill format conversion
├── go.mod
└── plan.md
```
