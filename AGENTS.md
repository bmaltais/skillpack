# AGENTS.md — Coding Agent Guide for SkillPack

This file is for AI coding agents (Claude Code, OpenCode, Codex, etc.) working on this codebase.
Read `CONTEXT.md` for the domain glossary. Read `plan.md` for the full design specification.

## Build & Test

```bash
go build ./cmd/skillpack/        # build the binary
go test ./...                    # run all tests
go vet ./...                     # static analysis
```

The binary entry point is `cmd/skillpack/`. There is no other binary in this repo.

## Key Files

| File | Purpose |
|------|---------|
| `CONTEXT.md` | Canonical domain glossary — read this first |
| `plan.md` | Full design spec with resolved decisions |
| `internal/config/config.go` | Config loading (`~/.skillpack/config.yaml`) |
| `internal/state/state.go` | State management (`~/.skillpack/state.json`) |
| `internal/repo/repo.go` | Repo management + skill discovery |
| `internal/skill/skill.go` | Install, remove, hash, conflict detection |

## Architecture Constraints — Do Not Violate

These decisions were made deliberately. Do not reverse them without discussing first.

1. **No per-agent adapter files.** Agents are config-only (`name` + `skill_dir`). Do not create `internal/agent/claude.go` or similar. If agent-specific behaviour is ever needed, add it as a named field in the config struct.

2. **No format conversion.** Install is a verbatim directory copy. Do not create or restore `pkg/convert/`. Skills are copied as-is regardless of agent.

3. **No `SKILL.md.sig` generation or verification.** Signing is out of scope for v1.

4. **State key structure is `skill-path → agent-name → record`.** Do not flatten it to a compound string key or a list.

5. **Single binary: `skillpack`.** Do not add additional binaries under `cmd/`.

6. **Everything lives under `~/.skillpack/`.** Do not write config or state to XDG paths or per-project directories.

## State Schema

```go
// ~/.skillpack/state.json
type State struct {
    Repos           map[string]RepoRecord                       `json:"repos"`
    InstalledSkills map[string]map[string]InstalledSkillRecord  `json:"installed_skills"`
    // key: skill address (e.g. "awesome-skills/coding/debugger")
    // inner key: agent name (e.g. "claude-code")
}

type RepoRecord struct {
    URL         string    `json:"url"`
    CachePath   string    `json:"cache_path"`
    LastUpdated time.Time `json:"last_updated"`
}

type InstalledSkillRecord struct {
    InstalledAtSHA string `json:"installed_at_sha"`
    InstalledHash  string `json:"installed_hash"`   // SHA-256 of installed dir contents
    LocalPath      string `json:"local_path"`
}
```

## Config Schema

```go
// ~/.skillpack/config.yaml
type Config struct {
    DefaultAgent string                 `yaml:"default_agent"`
    Agents       map[string]AgentConfig `yaml:"agents"`
}

type AgentConfig struct {
    SkillDir string `yaml:"skill_dir"`
}
```

## Default Agents (bundled in binary)

The first-run wizard uses this list to auto-detect installed agents:

| Agent name | Expected `skill_dir` | Verified |
|------------|----------------------|---------|
| `claude-code` | `~/.claude/skills` | ✓ |
| `copilot` | `~/.copilot/skills` | ✓ |
| `hermes` | `~/.hermes/skills` | ✓ |
| `opencode` | `~/.config/opencode/skills` | TODO |
| `openclaw` | `~/.openclaw/skills` | TODO |
| `pi` | `~/.pi/agent/skills` | ✓ |

Detection logic: expand `~`, check if the directory exists on disk. If it does, offer to add it to config.

## Conflict Resolution Flags

When a skill has local modifications AND upstream changes, `update` and `sync` require one of:

- `--force-remote` — remote wins: overwrite installed files with cache, reset hash + SHA
- `--force-local` — local wins: copy installed files back to cache, commit, push, reset hash + SHA  
- `--merge` — three-way merge (base=`installed_at_sha`, ours=installed, theirs=cache HEAD); write conflict markers on failure

## Skill Discovery

A skill is any directory inside a repo clone that contains a `SKILL.md` file. Discovery = recursive walk of the repo cache dir, collect all paths containing `SKILL.md`. No manifest file required or supported.
