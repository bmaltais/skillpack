# AGENTS.md — Pack Deployment

## Purpose

Owns everything about Packs: the `pack.yaml` schema, resolving a Pack Recipe from an address, and deploying it (install, complete a partial deployment). The CLI and TUI both call this package; neither re-implements the flow.

## Ownership

| Concern | Owner |
|---------|-------|
| `pack.yaml` schema, parse, validate | `internal/pack/pack.go` |
| Resolve registered address / HTTPS URL / local path to a `Definition` | `internal/pack/resolve.go` |
| Install, Complete, `Result`, `Options.Progress` events, `IsPartial` | `internal/pack/deploy.go` |

## Local Contracts

- Per-skill failures never abort an operation; they are recorded as a Partial Pack Deployment. The returned error covers only record persistence.
- Each operation saves state once, at the end. Callers must not `state.Save` again for the same operation.
- No printing. Progress goes through `Options.Progress` (nil means quiet); summary wording belongs to the caller.
- `Complete` re-registers missing repos only when the recipe resolves from the pack address (registered packs); URL/filepath packs have synthetic addresses and only retry installs.
- `Resolve` accepts only `https://` URLs for remote recipes.
- Agent selection and flag handling stay in `cmd/skillpack`; the module takes an explicit agent list.

## Work Guidance

- Import direction: `pack` sits above `config`, `repo`, `skill`, `state`; the compiler rejects the reverse (import cycle).
- Tests use the temp-HOME `TestMain` and local git fixtures (see `deploy_test.go`); no network.

## Verification

- `go test ./internal/pack/...`

## Child DOX Index

None.
