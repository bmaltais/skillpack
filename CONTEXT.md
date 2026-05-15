# SkillPack — Ubiquitous Language

## Skill

A directory containing a `SKILL.md` file, optionally with supporting files (`references/`, `templates/`, `scripts/`). The `SKILL.md` is the primary instruction file consumed by an AI agent. A skill encodes a specific capability or workflow.

## Skill Repo

A git repository that contains one or more skills. May be flat (skills at the root level) or categorised (skills grouped under category subdirectories). Discovered by walking the repo for directories that contain a `SKILL.md` file.

## Skill Address

The canonical identifier for a skill: `<repo-name>/<repo-relative/path/to/skill>`. Mirrors the path on disk inside the repo clone. Examples: `awesome-skills/debugger`, `awesome-skills/coding/debugger`.

## Agent

An AI agent tool that consumes skills (e.g. claude-code, hermes-agent). Defined entirely by configuration — a name and a `skill_dir` path on disk. No code adapter exists per agent.

## Default Agent

The agent targeted by commands when no `--agent` flag is provided. Set in `~/.skillpack/config.yaml`. Chosen interactively on first run.

## Installed Skill

A skill that has been copied from a repo cache into an agent's `skill_dir`. Tracked in state with `installed_at_sha` and `installed_hash`.

## Installed Hash (`installed_hash`)

A SHA-256 digest of all file contents in the installed skill directory, computed at install time. Used to detect whether the user has locally modified an installed skill.

## Installed SHA (`installed_at_sha`)

The git commit SHA of the repo cache at the time the skill was installed. Used to detect whether the upstream repo has new changes for that skill since installation.

## Local Modification

A skill is locally modified when its current content hash differs from `installed_hash`. Locally modified skills are blocked from silent updates; the user must explicitly choose a conflict resolution strategy.

## Upstream Change

A skill has an upstream change when the skill's content in the repo cache at current HEAD differs from the content at `installed_at_sha`.

## Conflict

A skill has a conflict when it has both a local modification and an upstream change simultaneously. Requires explicit resolution: `--force-remote`, `--force-local`, or `--merge`.

## Repo Cache

A hidden git clone of a registered skill repo, stored at `~/.skillpack/repos/<repo-name>/`. All read and write operations on the remote repo go through this clone.

## Publish

The act of copying a locally-modified skill from an agent's `skill_dir` back into the repo cache, committing with an auto-generated message, and pushing to the remote's main branch.

## Sync

A two-way reconciliation command. Pulls all registered repos, applies upstream changes to unmodified installed skills, publishes locally-modified skills to their remotes, and warns on conflicts.
