# Design: tiger-go-demo production hardening

## Context

The `tiger-go-demo` project passes its full verification suite locally (`make verify`), but without a CI pipeline regressions can reach `main` between human runs. The dependency audit identified two concrete risks: `github.com/golang/protobuf v1.5.0` is deprecated with known CVEs, and `golang.org/x/xerrors` is pinned to a 2019 snapshot.

## Goals / Non-Goals

**Goals:**
- Gate every PR with `make verify` (gofmt, go vet, tests, SDK coverage, docs)
- Remove the deprecated protobuf dependency
- Update stale pinned dependencies
- Add a credentials-free smoke test that proves SDK wiring against the real Tiger gateway

**Non-Goals:**
- No live-API test with real credentials
- No changes to command behaviour or binary output
- No SDK coverage expansion (140/159 is unchanged)
- No service/containerisation changes (CLI demo, not a service)

## Decisions

### 1. GitHub Actions workflow at `.github/workflows/ci.yml`

The Makefile already defines `make verify` as the aggregate gate. The workflow runs exactly that target. No custom step list — a new step added to Makefile is automatically covered in CI.

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true
      - run: go version  # confirm Go 1.24
      - run: make verify
```

Using `ubuntu-latest` (currently 24.04) and Go 1.24. `cache: true` on setup-go handles module caching without extra steps.

### 2. `golang/protobuf` migration

`go get google.golang.org/protobuf@latest && go mod tidy && go mod download` — then rewrite all `github.com/golang/protobuf` imports to `google.golang.org/protobuf` in source files. The SDK already uses `google.golang.org/protobuf`, so the module is already in the graph. `go build ./...` confirms no further changes needed.

The two files to update (discovered via grep):
- `test//sdkcoverage_test.go` — imports `github.com/golang/protobuf/proto`
- `go.mod` — replace the direct dependency

After: `go list -m all | grep golang/protobuf` must return empty.

### 3. `golang.org/x/xerrors` update

```bash
go get golang.org/x/xerrors@latest && go mod tidy
```

The update target is `v0.0.0-20240903120638-7835f813f4da` (the latest tagged version). `go vet` and `go build` confirm no breakage.

### 4. Credentials-free smoke test

A new test file `test/smoke_test.go` (or in an existing test package) that:
- Creates a Tiger session with a deliberately invalid `TIGER_ID` and `TIGER_PRIVATE_KEY`
- Calls one read endpoint (e.g. `QuoteClient.GetMarketState`)
- Asserts the error is a Tiger API parameter error (`code=1000`), not a TLS/connection error
- Runs as part of `make test`, no credentials required

This is placed in `test/` (already in `go.test` configuration) so it is excluded from coverage percentage but included in the test run.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| Go version drift between CI and local | Pin `go-version: '1.24'` in setup-go; documented in README as minimum version |
| Smoke test becomes flaky if Tiger gateway changes error format | Test asserts error contains `"code=1000"` or `"param error"` — structural, not exact string |
| `golang/protobuf` import rewrite misses a file | `grep -r "golang/protobuf" . --include="*.go"` after rewrite as verification step |
| SDK coverage check slow in CI (15-20s) | Acceptable for a demo project; CI caching reduces module fetch time |
