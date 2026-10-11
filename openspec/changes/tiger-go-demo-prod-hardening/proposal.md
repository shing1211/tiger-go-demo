# Proposal: tiger-go-demo production hardening

## Why

The `tiger-go-demo` CLI demo has a well-designed safety architecture (write-gate, credential isolation, secret redaction) but lacks the tooling to maintain it reliably. Two specific gaps prevent it from functioning as a safe, maintainable production tool: a deprecated `golang/protobuf` dependency carrying known CVEs, and the absence of any CI pipeline to catch regressions before they reach `main`.

## What Changes

1. **Add GitHub Actions CI** — run `make verify` (fmt check, go vet, tests with race detector, SDK coverage assertion, docs check) on every PR and push to `main`.
2. **Migrate `golang/protobuf` to `google.golang.org/protobuf`** — the deprecated `v1.5.0` carries known CVEs; the SDK already depends on `google.golang.org/protobuf v1.34+`, so the migration path is clear and the update is a one-line `go get` plus `go mod tidy`.
3. **Update pinned `golang.org/x/xerrors`** — currently pinned to a 2019 snapshot; update to the latest tagged version to pick up any subsequent fixes.
4. **Add live-API smoke test** (opt-in, secrets-free) — a credentials-free test that proves the SDK wiring, request signing, and HTTP transport are intact by exercising the real Tiger gateway with deliberately invalid credentials and asserting the expected rejection response.

## Capabilities

### New Capabilities

- `github-actions-ci`: A GitHub Actions workflow that runs the full verification suite (`make verify`) on every PR and on every push to `main`, gating merge on a clean run.
- `golang-protobuf-migration`: The `golang/protobuf` v1 package is replaced by `google.golang.org/protobuf`, which is already a transitive dependency of the SDK. The migration is a one-line upgrade with no API changes to this project.

### Modified Capabilities

- `verification-honesty`: The existing capability records that no authenticated live response has ever been observed. A smoke test (credentials-free, uses only invalid credentials to prove wiring) extends the verifiable coverage without requiring real credentials. The spec's "NOT proven" boundary is updated to distinguish "never attempted" from "attempted with invalid credentials and correctly refused."

## Impact

- **New files:** `.github/workflows/ci.yml`
- **Modified files:** `go.mod`, `go.sum`
- **No behavior changes** to any command or binary
- **Spec changes:** `openspec/specs/verification-honesty/` (delta) and new `openspec/specs/github-actions-ci/` and `openspec/specs/golang-protobuf-migration/`
- **SDK coverage:** unchanged (140/159 methods)
