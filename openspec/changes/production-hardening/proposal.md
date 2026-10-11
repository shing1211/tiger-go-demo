# Proposal

## Why

The project has three dependency hygiene issues and one testing gap that reduce confidence in daily development. `golang.org/x/xerrors` is a transitive-only dependency at 2019 vintage — its package was folded into the standard `errors` module in Go 1.20 and the old import is dormant — however, it is pulled in as a test-only transitive of `google.golang.org/protobuf` and cannot be removed without a SDK or protobuf upgrade. The SDK smoke test is missing, so there is no credentials-free way to assert the library wiring is not broken. GitHub Actions CI already exists and is well-structured; it does not need changes.

## What Changes

- **Document `golang.org/x/xerrors`** as a known unavoidable transitive test dep from `google.golang.org/protobuf`. Not removable without upstream changes.
- **Add a credentials-free smoke test** (`test/smoke_test.go`) that constructs the SDK client, exercises read-only API paths (no credentials required), and asserts the response shape is well-formed. No Tiger account needed.
- **Close issues** #14, #15, #16 (which are already done or no-ops), and confirm #17 is satisfied by existing CI.

## Capabilities

This is a pure tooling, dependency, and test change. No capability-level behaviour changes.

### New Capabilities

_none_

### Modified Capabilities

_none_

## Impact

- `docs/dependency-notes.md`: new file documenting `golang.org/x/xerrors` as a known
  unavoidable transitive test-only dependency.
- `test/smoke_test.go`: new file, credentials-free SDK wiring test.
- `go.mod`/`go.sum`: no changes (xerrors is not removable without upstream SDK changes).
- GitHub Actions workflows (`.github/workflows/`): no changes — already satisfy issue #17.
