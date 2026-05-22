# Design: `analyse-session-tools` skill

**Date:** 2026-05-22  
**Status:** Approved  
**Type:** pi skill (SKILL.md)

## Problem

LLMs default to familiar single-purpose tools (`read`, `edit`, `bash`) even when compound tools
would collapse 2–3 turns into 1. The `compound-tools` extension already covers the most common
patterns, but new patterns emerge as usage grows. Currently there is no way to discover them
without manually reviewing session history.

## Goal

A skill that analyses the **current session's** tool call history, surfaces all detected
multi-step patterns (no frequency threshold — show everything), and emits a ranked report plus
ready-to-paste TypeScript scaffolds for any patterns not already covered by compound tools.

## Scope

- **Input:** current session JSONL only (not historical / cross-project)
- **Output:** markdown report + TypeScript stubs written to a temp file
- **Action:** report and scaffold only — no auto-patching of `compound-tools.ts`

## Architecture

### 1. Session file location

Derived at runtime:

```bash
cwd_slug=$(pwd | sed 's|/|-|g' | sed 's|^-||')
ls ~/.pi/agent/sessions/--${cwd_slug}--/*.jsonl | sort | tail -1
```

### 2. Analysis script

A self-contained Python 3 script (`/tmp/analyse-session-tools.py`) generated and executed by
the skill. It:

1. Reads the session JSONL line by line
2. Collects all `toolCall` blocks from assistant messages, in order, with their arguments
3. Builds a flat sequence: `[(turn_idx, tool_name, args), ...]`
4. Runs two detection passes (see below)
5. Prints a structured markdown report to stdout

### 3. Pass 1 — Tool-name pair/triple detection

Sliding window (size 2 and 3) over the flat tool call sequence.

For each window, record:
- `pattern`: tuple of tool names e.g. `("read", "edit")`
- `same_resource`: whether both calls reference the same file path (extracted from `path` arg)
- `count`: occurrences across the session
- `turns_consumed`: number of LLM round-trips used

### 4. Pass 2 — Bash sub-pattern classification

Re-scan every `bash` call, classify by command content:

| Trigger strings | Classification |
|---|---|
| `rg`, `grep`, `ag`, `ripgrep` | `bash(search)` |
| `find`, `ls`, `fd`, `tree` | `bash(find)` |
| `git add`, `git commit`, `git push`, `git stash` | `bash(git-write)` |
| `git log`, `git diff`, `git status`, `git show` | `bash(git-read)` |
| `mkdir`, `touch`, `cp`, `mv`, `rm` | `bash(fs-op)` |
| `go `, `npm `, `pytest`, `make`, `cargo`, `yarn` | `bash(build/test)` |
| anything else | `bash(other)` |

Re-run pair/triple detection with classified names so `bash(search) → read` is distinct from
`bash(build) → read`.

### 5. Already-covered detection

Before emitting a scaffold, check against the known compound tools:

| Pattern | Covered by |
|---|---|
| `read → edit` | `read_and_patch` |
| `write → bash` | `create_and_run` |
| `bash(find) → read` | `find_and_read` |
| `bash(search) → read` | `search_and_read` |

Covered patterns are included in the report (so the user can see adoption) but marked
**✓ already covered** and get no scaffold.

## Output format

```
## Session Tool Pattern Analysis

Session: <file>
Turns: N  |  Tool calls: M  |  Unique patterns: K

---

### Section 1 — Tool-name pairs (all occurrences)

| Pattern           | Count | Same resource | Turns spent | Status        |
|-------------------|-------|---------------|-------------|---------------|
| read → edit       |   4   | ✓             | 8           | ✓ covered     |
| bash → read       |   3   | ~             | 6           | ✓ covered     |
| bash → bash       |   2   | —             | 4           | ⚠ new pattern |

---

### Section 2 — Bash sub-patterns

| Pattern                         | Count | Notes                    | Status        |
|---------------------------------|-------|--------------------------|---------------|
| bash(search) → read             |   3   | rg then read file        | ✓ covered     |
| bash(git-read) → bash(git-read) |   2   | git log + git diff       | ⚠ new pattern |

---

### Section 3 — Scaffolds for new patterns

Scaffolds are written to: /tmp/compound-tools-suggestions.ts

#### `git_inspect` — bash(git-read) → bash(git-read)
> Appeared 2 times. Collapses 2 turns into 1.

[TypeScript stub inline here]
```

## Scaffold format

Each stub is a minimal `pi.registerTool()` block:

```typescript
// SUGGESTED: git_inspect
// Pattern: bash(git-read) → bash(git-read) — seen 2 times this session
// TODO: implement execute() body
pi.registerTool({
  name: "git_inspect",
  label: "Git Inspect",
  description:
    "Run two git read operations (e.g. log + diff) and return combined output — one call.",
  promptSnippet: "Run git log + git diff in one call. Replaces bash(git-read) → bash(git-read).",
  promptGuidelines: [
    "Use git_inspect when you need to run multiple git read commands back-to-back.",
  ],
  parameters: Type.Object({
    // TODO: define parameters
  }),
  async execute(_toolCallId, params, _signal, _onUpdate, _ctx) {
    // TODO: implement
    throw new Error("not implemented");
  },
});
```

## Skill invocation

The skill is triggered by `/analyse-session-tools` or by asking the agent to
"analyse this session's tool usage".

Steps the agent executes:
1. Derive session file path from cwd
2. Generate and run the Python analysis script via `create_and_run`
3. Print the report inline in the conversation
4. Write scaffolds to `/tmp/compound-tools-suggestions.ts`
5. Tell the user the scaffold path and how to add it to `compound-tools.ts`

## Out of scope (v1)

- Cross-session or cross-project analysis
- Automatic patching of `compound-tools.ts`
- Similarity clustering (e.g. grouping `bash(git-read) → bash(git-read) → bash(git-read)` triples)
- Cost / token analysis (only turn count is reported)
