# TUI Prototypes — Skill Manager

Three layout prototypes for an interactive skill installer/remover.
Run with: `skillpack tui` (cycles layouts with Tab).

## Common Controls (all layouts)

| Key | Action |
|-----|--------|
| ↑/↓ | Move cursor up/down through skill/repo rows |
| ←/→ | Move cursor left/right through agent columns |
| Enter | On repo row: expand/collapse skills. On skill+agent cell: toggle install/remove |
| Tab | Cycle between layout prototypes |
| Type chars | Filter skills (incremental search) |
| Backspace | Delete filter character |
| Esc | Clear filter |
| q / Ctrl+C | Quit |

---

## Prototype A: Tree + Grid

A collapsible tree on the left with agent columns forming a matrix on the right.
The repo rows act as section headers you expand/collapse.

```
┌─ SkillPack TUI ──────────────────────────────────────────────────────────┐
│ Filter: _                                                                 │
│                                                                           │
│  SKILL                          claude-code  copilot   pi       hermes    │
│  ─────────────────────────────  ───────────  ────────  ───────  ────────  │
│ ▼ awesome-skills                                                          │
│     coding/debugger              [✓]          [ ]       [✓]      [ ]      │
│     coding/refactor              [ ]          [ ]       [ ]      [ ]      │
│   ▶ writing/blogger              [ ]          [✓]       [ ]      [ ]      │
│ ▶ my-private-repo                                                         │
│ ▼ community-skills                                                        │
│     linter                       [✓]          [✓]       [✓]      [✓]      │
│     docker-compose               [ ]          [ ]       [ ]      [ ]      │
│                                                                           │
│                                                                           │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│ ↑↓ navigate  ←→ agents  Enter expand/install  Tab layout  q quit         │
│ 12 skills across 3 repos  •  4 agents configured                         │
└───────────────────────────────────────────────────────────────────────────┘
```

**Cursor highlight:** The active cell (row + agent column) is highlighted.
When the cursor is on a repo row, only the repo name is highlighted (no agent action).

---

## Prototype B: Compact Matrix (No tree nesting)

Flat list of all skills (repo prefix shown inline). Repo grouping via subtle
separator lines. More compact — good for many skills.

```
┌─ SkillPack TUI ──────────────────────────────────────────────────────────┐
│ 🔍 debug                                                                  │
│                                                                           │
│  SKILL                              claude  copilot  pi    hermes         │
│  ──────────────────────────────────  ──────  ───────  ────  ──────        │
│  awesome-skills/coding/debugger      [✓]     [ ]      [✓]   [ ]          │
│  community-skills/debug-helper       [ ]     [ ]      [ ]   [ ]          │
│                                                                           │
│                                                                           │
│                                                                           │
│                                                                           │
│                                                                           │
│                                                                           │
│                                                                           │
├───────────────────────────────────────────────────────────────────────────┤
│ Filter: "debug" (2 matches)  ↑↓←→ navigate  Enter toggle  Tab layout     │
└───────────────────────────────────────────────────────────────────────────┘
```

**Key difference:** No expand/collapse — all skills shown flat with their full
address. The filter is always visible and active. Typing narrows the list
instantly. Great when you know what you're looking for.

---

## Prototype C: Split Pane (Tree left, Detail right)

Left pane shows the repo/skill tree. Right pane shows the selected skill's
agent install status as a vertical list — better for many agents.

```
┌─ SkillPack TUI ──────────────────────────────────────────────────────────┐
│ Filter: _                                                                 │
│                                                                           │
│  REPOS / SKILLS            │  coding/debugger                             │
│  ──────────────────────────│  ───────────────────────────────────         │
│ ▼ awesome-skills           │   claude-code     [✓] installed              │
│    coding/debugger    ←────│   copilot         [ ]                        │
│    coding/refactor         │   pi              [✓] installed              │
│    writing/blogger         │   hermes          [ ]                        │
│ ▶ my-private-repo         │   opencode        [ ]                        │
│ ▼ community-skills        │   openclaw        [ ]                        │
│    linter                  │                                              │
│    docker-compose          │                                              │
│                            │                                              │
│                            │                                              │
├────────────────────────────┴──────────────────────────────────────────────┤
│ ↑↓ skills  ←→ or Tab pane  Enter install/remove  q quit                  │
│ Selected: awesome-skills/coding/debugger                                  │
└───────────────────────────────────────────────────────────────────────────┘
```

**Key difference:** The agent list is vertical on the right, so it scales to
many agents without horizontal scrolling. Navigation: ↑↓ moves in whichever
pane has focus, ←/→ or Tab switches panes.

---

## Comparison

| Aspect | A: Tree+Grid | B: Compact Matrix | C: Split Pane |
|--------|-------------|-------------------|---------------|
| Best for | Few agents (3-4), visual overview | Quick search, flat browsing | Many agents (5+), detail view |
| Repo grouping | Collapsible tree | Flat with full address | Collapsible tree |
| Agent display | Horizontal columns | Horizontal columns | Vertical list |
| Filter UX | Top bar, auto-expands matches | Always active, narrows list | Top bar |
| Scalability | Limited by terminal width | Limited by terminal width | Scales well both ways |

---

## Recommended Starting Point

**Prototype A (Tree+Grid)** is the most natural fit for the described workflow:
- Repos on the left, expand/collapse with Enter
- Agent columns to the right
- Arrow keys to navigate the matrix
- Enter to toggle install/remove
- Type to filter

It matches the original description most closely. Prototypes B and C are
alternatives if A feels too busy or doesn't scale well.

---

## Implementation Notes

- The TUI is **read-only by default** (preview mode). Pass `--live` to actually
  install/remove skills when Enter is pressed.
- Uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) +
  [Lip Gloss](https://github.com/charmbracelet/lipgloss) for rendering.
- Command: `skillpack tui` (new cobra subcommand).
- Loads real data from config + state + repo discovery. Falls back to sample
  data if no repos are registered.
