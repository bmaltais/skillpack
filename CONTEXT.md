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

---

## Pack

A named, curated collection of skills bundled as a recipe. A pack declares the repos and skills that belong together for a given purpose (e.g. "go-dev", "writing-tools"). Installing a pack clones/updates all referenced repos, then installs all listed skills in one operation. A pack is a distribution unit (shareable) and an installation shortcut (one command deploys many skills).

## Pack Recipe

The `pack.yaml` file that defines a pack. Contains the pack's name, an optional description, and an explicit list of repos (with URLs) and skills (with their addresses) that make up the pack. The recipe is the only artifact — packs do not bundle skill file contents.

## Pack Address

The canonical identifier for a pack: `<repo-name>/<repo-relative/path/to/pack>`. Discovery follows the same recursive walk as skill discovery — any directory inside a repo cache that contains a `pack.yaml` is a pack. Example: `awesome-skills/packs/go-dev`.

## Pack Deployment

The act of installing all skills declared in a pack recipe for one or more selected agents. A pack deployment is a first-class entity tracked in state, distinct from the individual skill installs it produces.

## Partial Pack Deployment

A pack deployment where one or more skills failed to install — typically because a referenced repo required authentication that was not available. Tracked in state. Listed as partial in CLI output and the TUI. The user can complete a partial deployment once credentials are configured; skillpack never completes it automatically.

## Pack Authoring

The process of assembling a new pack using the TUI. The TUI guides the user through naming the pack, selecting skills from registered repos, and choosing which repo to publish the recipe to. The resulting `pack.yaml` is saved locally and optionally published.

## Pack Publication

The act of committing and pushing a `pack.yaml` to a selected skill repo, making the pack discoverable by other users of that repo.

## Installed Pack

A pack that has been deployed on the local machine. Tracked in state with the pack address, the agents it was deployed for, the list of skills it installed per agent, and whether the deployment is complete or partial.
