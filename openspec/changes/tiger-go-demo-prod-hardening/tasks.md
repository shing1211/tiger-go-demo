# Tasks: tiger-go-demo production hardening

## 1. Create GitHub Actions CI workflow

- [ ] 1.1 Create `.github/workflows/ci.yml` with `on: [push, pull_request]` targeting `main`
- [ ] 1.2 Add `actions/checkout@v4` and `actions/setup-go@v5` with `go-version: '1.24'` and `cache: true`
- [ ] 1.3 Add `run: go version` to confirm Go 1.24 is active
- [ ] 1.4 Add `run: make verify` as the single verification step
- [ ] 1.5 Verify: `go build ./... && go vet ./... && go test -race ./... && make coverage-check && ./scripts/check-docs` passes locally

## 2. Migrate golang/protobuf

- [ ] 2.1 Run `go get google.golang.org/protobuf@latest && go mod tidy`
- [ ] 2.2 Find all files importing `github.com/golang/protobuf`: `grep -r "github.com/golang/protobuf" . --include="*.go"`
- [ ] 2.3 Rewrite imports from `github.com/golang/protobuf` to `google.golang.org/protobuf` in all files found
- [ ] 2.4 Verify `go list -m all | grep golang/protobuf` returns empty
- [ ] 2.5 Verify `go build ./...` succeeds

## 3. Update golang.org/x/xerrors

- [ ] 3.1 Run `go get golang.org/x/xerrors@latest && go mod tidy`
- [ ] 3.2 Verify `go build ./... && go vet ./...` succeed with the updated version

## 4. Add credentials-free smoke test

- [ ] 4.1 Create `test/smoke_test.go` with a test that calls Tiger with invalid credentials
- [ ] 4.2 Assert the error is a Tiger parameter error (`code=1000` or `"param error"`), not a TLS/connection error
- [ ] 4.3 Verify `go test ./test/...` passes with no TIGER_* env vars set
- [ ] 4.4 Verify `make test` includes the smoke test

## 5. Update CONTRIBUTING.md

- [ ] 5.1 Add OpenSpec mention to CONTRIBUTING.md (change goes through OpenSpec workflow)
- [ ] 5.2 Verify `openspec validate --change tiger-go-demo-prod-hardening` passes

## 6. Commit, push, and open PR

- [ ] 6.1 Create branch `feat/production-hardening` from `main`
- [ ] 6.2 Commit all changes with conventional commit message referencing the issue
- [ ] 6.3 Push branch and open PR against `main`
- [ ] 6.4 Verify CI passes on the PR branch
- [ ] 6.5 Request review (self-review first, then Mary)
- [ ] 6.6 After merge: `openspec archive tiger-go-demo-prod-hardening`
