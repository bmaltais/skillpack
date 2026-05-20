# TUI — Interactive Skill Manager

Run with: `skillpack tui`

## Layout

The TUI has three panels, accessible via **Tab**:

| Panel | Purpose |
|-------|---------|
| **Skills** | Browse repos/skills, install/remove for each agent |
| **Status** | View installed skill states, update, sync |
| **Repos** | Add/remove skill repositories |

## Skills Panel

A collapsible tree of repos with agent columns forming a matrix.

```
 SkillPack  [Skills]   Status    Repos

 Type to filter…

 SKILL                          claude-code  copilot   pi
 ─────────────────────────────  ───────────  ────────  ───────
▼ awesome-skills
     coding/debugger              [✓]          [ ]       [✓]
     coding/refactor              [ ]          [ ]       [ ]
▶ community-skills

 ──────────────────────────────────────────────────────────────
 ↑↓ navigate  ←→ agents  Space/Enter toggle  f fork  Tab switch  q quit
 6 skills in 2 repos  •  3 agents
```

### Controls

| Key | Action |
|-----|--------|
| ↑/↓ | Move between rows |
| ←/→ | Move between agent columns |
| Space/Enter | On repo: expand/collapse. On skill+agent: install/remove |
| f | Fork the selected skill into a writable repo |
| Type | Filter skills (incremental search) |
| Backspace | Delete filter character |
| Esc | Clear filter |
| Tab | Switch to Status panel |
| q / Ctrl+C | Quit |

### Status Indicators (after status check)

| Cell | Meaning |
|------|---------|
| `[✓]` | Installed, up-to-date |
| `[↑]` | Update available (cyan) |
| `[~]` | Locally modified (yellow) |
| `[!]` | Conflict (red) |
| `[ ]` | Not installed |

## Status Panel

Shows every installed skill with its current state. Auto-refreshes on first visit.

```
 SkillPack   Skills   [Status]   Repos

 SKILL                         AGENT        STATUS
 ─────────────────────────────────────────────────────
 awesome-skills/debugger       claude-code  ✓ up-to-date
 awesome-skills/debugger       copilot      ↑ update available
 community-skills/linter       pi           ~ locally modified

 ──────────────────────────────────────────────────────
 ↑↓ navigate  u update selected  S sync all  r refresh  U self-update  Tab switch  q quit
 2 up-to-date  •  1 update available  •  1 modified
```

### Controls

| Key | Action |
|-----|--------|
| ↑/↓ | Navigate between installed skills |
| u / Enter | Update the selected skill (if update available) |
| S | Sync all skills (pull updates + push local edits) |
| r | Refresh status (re-fetch repos, re-check all) |
| U | Self-update the skillpack binary |
| Tab | Switch to Repos panel |
| q / Ctrl+C | Quit |

## Repos Panel

Manage registered skill repositories without leaving the TUI.

```
 SkillPack   Skills    Status   [Repos]

 NAME                  URL
 ──────────────────────────────────────────────────────
 awesome-skills        https://github.com/user/awesome-skills.git
 community-skills      git@github.com:org/community-skills.git

 ──────────────────────────────────────────────────────
 ↑↓ navigate  a add  d remove  Tab skills  q quit
 2 repo(s) registered
```

### Controls

| Key | Action |
|-----|--------|
| ↑/↓ | Navigate between repos |
| a | Add a repo (prompts for name, then URL) |
| d / Delete | Remove selected repo (with confirmation) |
| Tab | Switch to Skills panel |
| q / Ctrl+C | Quit |

## Update Banner

On startup, the TUI checks GitHub for a newer release. If one is found, a yellow banner appears:

```
 ⚠ Update available: v0.2.0 → v0.3.0   [Update]  Skip
```

| Key | Action |
|-----|--------|
| ←/→ | Switch between Update and Skip buttons |
| Space/Enter | Activate selected button |
| Esc | Dismiss (same as Skip) |

## Implementation

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- All changes are applied immediately (no preview mode)
- Async operations (status check, sync, self-update) show a ⌛ busy indicator
- State mutations in async commands use deep-copied state to avoid data races
