# TUI — DOS Shell-Styled Interactive Skill Manager

Run with: `skillpack tui`

The TUI is styled after the MS-DOS Shell: a title bar, a dropdown menu bar,
and a status bar pinned to the last row, wrapped around five panels.

## Layout

```
                         SkillPack v0.4.0 ── Skills
 File  View  Actions  Packs  Help

 Type to filter…

 SKILL                          claude-code  copilot   pi
 ─────────────────────────────  ───────────  ────────  ───────
▼ awesome-skills
     coding/debugger              [✓]          [ ]       [✓]
     coding/refactor              [ ]          [ ]       [ ]
▶ community-skills

 ──────────────────────────────────────────────────────────────
 6 skills in 2 repos  •  3 agents
 ↑↓ navigate  ←→ agents  Space/Enter toggle  f fork  R repair  F10=Menu
```

- **Title bar** (top, reverse-video): app name, version, active panel.
- **Menu bar**: `File`, `View`, `Actions`, `Packs`, `Help` — mnemonic
  letters in cyan. Open with **F10** or **Alt+**the mnemonic letter.
- **Status bar** (bottom, reverse-video, always pinned to the last row):
  the active panel's key hints on the left, `F10=Menu` on the right.

## Panels

Cycle with **Tab**, or jump directly:

| Key | Panel | Purpose |
|-----|-------|---------|
| F2 | **Skills** | Browse repos/skills, install/remove for each agent |
| F3 | **Status** | View installed skill states, update, sync |
| F4 | **Repos** | Add/remove skill repositories |
| F5 | **Unmanaged** | Adopt skills found in agent dirs but not tracked by skillpack |
| F6 | **Packs** | Browse, install, create, and edit packs |

## Menu Bar

**F10** (or **Alt+F/V/A/P/H**) opens the corresponding dropdown. Inside an
open menu: **←/→** switch menus, **↑/↓** move the highlight, **Enter** or a
mnemonic letter runs the highlighted/matched item (skipped if disabled for
the current panel/selection), **Esc** or **F10** closes without action.
Bare letters are never menu shortcuts — they stay reserved for
type-to-filter and existing single-letter shortcuts.

| Menu | Item | Key |
|------|------|-----|
| File | Add Repo | a |
| File | Remove Repo | d |
| File | Self-Update | U |
| File | Exit | q |
| View | Skills | F2 |
| View | Status | F3 |
| View | Repos | F4 |
| View | Unmanaged | F5 |
| View | Packs | F6 |
| View | Refresh Status | r |
| Actions | Toggle Install | Space |
| Actions | Fork | f |
| Actions | Repair | R |
| Actions | View SKILL.md | v |
| Actions | Update Skill | u |
| Actions | Sync All | S |
| Actions | Adopt | Enter (Unmanaged) |
| Packs | Create Pack | n |
| Packs | Edit Pack | e |
| Packs | Install Pack | i |
| Packs | Remove Pack | d |
| Help | Keys | F1 |
| Help | About | — |

Every menu item calls the same handler its keyboard shortcut does — the
menu is a second door onto existing behavior, not a separate
implementation. Items whose precondition isn't met (e.g. Fork without a
skill selected) render dim and are skipped by Enter, but stay navigable.

## Dialogs

Every prompt — text input, confirm, or list-select — renders as a centered
double-border box (`╔═╗║╚╝`) over the current panel:

```
                   ╔═════════════ Add Repo ═════════════╗
                   ║ Repo name: ▌                        ║
                   ║                                     ║
                   ║ Enter to confirm • Esc to cancel    ║
                   ╚═════════════════════════════════════╝
```

Text fields show a `▌` cursor; confirms show `[ Yes ]  [ No ]`; list-selects
(fork target repo, adopt target repo, relink candidates, pack agent
multi-select) show a `▶`-prefixed, reverse-video highlighted row.

## Help (F1)

Opens a scrollable dialog listing every menu binding above, generated
directly from the menu table so it can't drift from what the menus
actually run. **↑/↓** scroll, **Esc**/**F1**/**Enter** closes.

## Skills Panel

A collapsible tree of repos with agent columns forming a matrix.

### Controls

| Key | Action |
|-----|--------|
| ↑/↓ | Move between rows |
| ←/→ | Move between agent columns |
| Space/Enter | On repo: expand/collapse. On skill+agent: install/remove |
| f | Fork the selected skill into a writable repo |
| R | Repair a stale or broken-upstream skill |
| v | View SKILL.md of the selected skill |
| Type | Filter skills (incremental search) |
| Backspace | Delete filter character |
| Esc | Clear filter |
| Tab | Switch panel |
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

| Key | Action |
|-----|--------|
| ↑/↓ | Navigate between installed skills |
| u / Enter | Update the selected skill (if update available) |
| S | Sync all skills (pull updates + push local edits) |
| r | Refresh status (re-fetch repos, re-check all) |
| U | Self-update the skillpack binary |
| Tab | Switch panel |
| q / Ctrl+C | Quit |

## Repos Panel

Manage registered skill repositories without leaving the TUI.

| Key | Action |
|-----|--------|
| ↑/↓ | Navigate between repos |
| a | Add a repo (dialog prompts for name, then URL) |
| d / Delete | Remove selected repo (with confirmation dialog) |
| Tab | Switch panel |
| q / Ctrl+C | Quit |

## Unmanaged Panel

Skills found in an agent's skill directory that skillpack doesn't track yet.

| Key | Action |
|-----|--------|
| ↑/↓ | Navigate between items |
| Type | Filter (incremental search) |
| Backspace | Delete filter character |
| Esc | Clear filter |
| Enter | Adopt the selected skill into a registered repo |
| v | View SKILL.md of the selected skill |
| Tab | Switch panel |
| q / Ctrl+C | Quit |

## Packs Panel

Browse, install, create, and edit packs (bundles of skills published together).

| Key | Action |
|-----|--------|
| ↑/↓ | Navigate between packs |
| Enter | Toggle the detail overlay for the selected pack |
| n | Create a new pack (embedded wizard, chrome stays visible around it) |
| e | Edit the selected pack |
| i | Install the selected available pack (agent multi-select dialog) |
| c | Complete a partially-deployed pack |
| d | Remove the selected installed pack (confirmation dialog) |
| Tab | Switch panel |
| q / Ctrl+C | Quit |

## Update Banner

On startup, the TUI checks GitHub for a newer release. If one is found, a banner appears between the menu bar and the active panel:

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
- Chrome is ANSI-16 blue/white/cyan (Route B: chrome only — panel bodies keep the terminal's default background for lower fragility against the width/height arithmetic)
- Dropdowns and dialogs are centered by whole-line occlusion, never mid-line ANSI splicing, so they can't garble styled panel content underneath
- All changes are applied immediately (no preview mode)
- Async operations (status check, sync, self-update) show a ⌛ busy indicator
- State mutations in async commands use deep-copied state to avoid data races

## History

This redesign restyled the existing Bubble Tea TUI to an MS-DOS Shell look;
it did not replace the underlying implementation. A separate `tview`-based
DOS Shell prototype that existed in an unrelated stale clone is superseded
by this document and was never merged into this repo.
