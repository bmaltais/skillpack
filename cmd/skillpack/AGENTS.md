# AGENTS.md — CLI Layer

## Purpose

Owns the Cobra CLI surface: commands, flags, TUI, and re-exec plumbing. Translates user intent into domain calls.

## Ownership

| Concern | Owner |
|---------|-------|
| Command registration & flag wiring | `cmd/skillpack/root.go` |
| Install/remove/list/fork/publish/relink/repo/selfupdate/status/doctor/sync/pack/agent | `cmd/skillpack/*.go` |
| TUI (bubbletea model, views, handlers, commands) | `cmd/skillpack/tui*.go` |
| Cross-platform re-exec (macOS launchd, Windows) | `cmd/skillpack/reexec_*.go` |
| Color output | `cmd/skillpack/color.go` |

## Local Contracts

- All domain logic lives in `internal/*`. The CLI only calls into `config`, `state`, `repo`, `skill`, `pack`, and `gitops`. Pack install, complete, update and remove run through `internal/pack`; the CLI and TUI only render its result. Sync runs through `skill.RunSync`/`skill.Sync`; `cmd/skillpack/sync.go` renders the `SyncReport`.
- The CLI and TUI never call `state.Save`; internal operations own persistence (e.g. `InstalledSkill.RemoveDirect`, `repo.Rename`). Cmd tests build states with `memState` (in-memory store).
- The Packs panel is a pack × agent matrix like Skills: Space enables/disables the selected pack for the selected agent (`pack.AddAgent` / `pack.Remove` with one agent). Removal lives in the separate Remove Packs panel (F8): Space checks, `r` removes after a Yes/No confirm.
- Panel order lives once in `panelOrder` (tui_views.go): F2..F8, Tab and the always-visible panel strip all derive from it. Per-panel filters go through `activeFilter`/`filterChanged` (tui_update.go), never per-panel copies. Esc backs out and never quits; menu mnemonics are auto-assigned unique per menu (`mnemonicIndexes`), so don't hand-pick them.
- The Doctor panel (F7) lists `doctor.Finding`s; Enter or `f` applies the selected finding's fix by routing to the existing flow for its kind (relink dialogs, `cmdCompleteDeployment`, register-fork dialog, or the duplicate diff in the platform viewer). `doctorDirty` makes it rescan once the dialog or async work finishes. Diagnosis and the stale/broken classification live in `internal/doctor`; the TUI never re-implements them.
- Commands share `App{Cfg, St}` via Cobra context (set in `PersistentPreRunE`).
- `self-update` and `tui` bypass `ensureConfig()` — they run before config exists.
- Re-exec spawns a fresh binary process to avoid TUI signal-handling conflicts with the parent.

## Work Guidance

- New commands go in their own file under `cmd/skillpack/`. One file per command group.
- TUI changes touch `tui_model.go` (state), `tui_views.go` (render), `tui_handlers.go` (input), `tui_commands.go` (actions). Keep concerns separated.

## Verification

- `go build ./cmd/skillpack/` must succeed after every CLI change.
- `go vet ./...` must pass.
- TUI tests: `go test ./cmd/skillpack/...`

## Child DOX Index

None.