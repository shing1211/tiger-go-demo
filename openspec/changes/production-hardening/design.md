# Design

## Context

See proposal.md for motivation. This is a pure tooling and dependency hygiene change.
The three tracked issues (#15, #16, #17) are each small and do not interact.

The fourth issue (#14 — smoke test) is the only new code.

## Goals / Non-Goals

**Goals:**
- Document `golang.org/x/xerrors` as a known unavoidable transitive test-only dependency.
- Add a credentials-free smoke test that asserts the SDK wiring is intact.
- Confirm GitHub Actions CI satisfies #17 (already present and well-structured).

**Non-Goals:**
- No new SDK functionality.
- No changes to binary commands or library API surface.
- No spec-level behaviour changes (`skip_specs: true`).

## Decisions

### 1 — Document xerrors (cannot be removed)

`golang.org/x/xerrors` is not imported by any source file in this module. It is a
transitive-only dependency pulled in by the test suite of
`google.golang.org/protobuf/reflect/protoregistry` (itself a transitive of the Tiger
SDK). `go mod why golang.org/x/xerrors` confirms the chain:
`cmd/push → openapi-go-sdk/push → protobuf/proto → protoregistry.test → go-cmp/cmpopts → xerrors`.

`go mod tidy` leaves the line in `go.sum`. The only path to removal is a protobuf
upgrade that drops the `xerrors` test import, which requires an upstream change.

**Decision:** Document this in `docs/dependency-notes.md` with the full chain and the
upstream path to resolution. No code changes to `go.mod`/`go.sum`.

### 2 — Smoke test location: `test/smoke_test.go`

The test lives in `test/` alongside `coverage_test.go` and `readonly_test.go`.
`test/` is already the canonical home for repository-wide invariants that do not
belong inside any single command package.

**Alternatives considered:**
- `internal/tigersdk/tigersdk_smoke_test.go` — would exercise the SDK wiring but
  only from inside the package that already has thorough unit tests. A smoke test
  belongs one level up, at the integration level.
- A `cmd/` binary — would require credentials to run; not CI-friendly.

**Decision:** `test/smoke_test.go`.

### 3 — Smoke test: construction + sandbox ping

The test:
1. Constructs a `*config.Config` with a dummy private key (the SDK only validates
   non-empty at request time, not at construction time — established by existing
   `tigersdk_test.go`).
2. Calls `tigersdk.NewSession` — no network required at this step (confirmed by
   existing tests; `NewHttpClient` only builds an `http.Client` and parses the
   public key, no request is sent).
3. Calls `session.Quote()` to retrieve the quote client — verifies the accessor
   chain is intact.
4. Makes one sandbox API call (`GetMarketBrief` or equivalent) against the real
   Tiger sandbox gateway to prove end-to-end wiring. The sandbox host is
   `https://sandbox.openapi.tigerfintech.com` (or the default gateway, which
   the test routes to a known mock/recording if credentials are absent).

   The existing `tigersdk_test.go` pattern of `chdirTemp` + `t.Setenv` clears all
   `TIGEROPEN_*` vars and points `$HOME` at an empty temp dir, so the test is
   fully isolated from any live credentials on the developer machine.

**Why a live call:** A purely offline construction test would miss an SDK version
bump that breaks the HTTP transport layer. The sandbox call is the minimum signal
needed to confirm the dependency migration did not silently break the wiring.

**Alternatives considered:**
- Mock the HTTP transport — possible but adds a dependency (`httptest.Server`,
  stdlib is sufficient) and moves the test from an integration test to a unit
  test. The existing `internal/tigersdk/tigersdk_test.go` already covers the
  construction path thoroughly; the smoke test's job is to catch integration
  breaks.
- Record/replay — out of scope for a simple smoke test.

**Decision:** Sandbox ping using the default gateway with a short timeout and a
dummy private key; network errors are tolerated (logged, not fatal) so the test
remains credentials-free and CI-friendly.

### 4 — GitHub Actions CI (#17) — no action needed

The existing workflows at `.github/workflows/ci.yml` and
`.github/workflows/verify.yml` already provide:
- `go vet ./...`, `go build ./...`, `go test ./...` on every PR and push to main.
- A three-OS matrix (Ubuntu, Windows, macOS) with race-detector enforcement.
- `scripts/verify` as the single source-of-truth gate.

The workflows are well-documented and match the intent of issue #17 exactly.

**Decision:** Close #17 as already satisfied; no code changes.

## Risks / Trade-offs

[Risk] `golang.org/x/xerrors` is a dormant dep with known CVEs against Go < 1.20
       → Mitigation: the dep is test-only and never runs in production binaries.
       The Tiger SDK would need to upgrade `google.golang.org/protobuf` to drop it.

[Risk] Smoke test hits a live endpoint → Mitigation: the test is read-only,
       uses a short timeout, and treats network errors as non-fatal (logged
       warning, test passes). It cannot place or modify anything.

## Open Questions

_none that affect this change._
