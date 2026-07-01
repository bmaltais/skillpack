# AGENTS.md — Test Utilities

## Purpose

Shared test helpers. Provides `RunWithTempHome` to isolate tests from the developer's real `~/.skillpack`.

## Ownership

| Concern | Owner |
|---------|-------|
| Temp HOME setup | `internal/testutil/testmain.go` (`RunWithTempHome`) |

## Local Contracts

- `RunWithTempHome` sets `HOME` to a fresh `/tmp` dir, runs the test suite, then removes the temp dir.
- Every `TestMain` should use: `func TestMain(m *testing.M) { os.Exit(testutil.RunWithTempHome(m)) }`
- Prevents tests from writing to the developer's real config/state.

## Work Guidance

- New test helpers: add to this package. Keep it small.
- Never import production code that has side effects (network, file I/O to real paths).

## Verification

- `go test ./internal/testutil/...`

## Child DOX Index

None.