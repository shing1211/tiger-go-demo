# Tasks

## 1. Dependency documentation

- [ ] 1.1 Create `docs/dependency-notes.md` documenting `golang.org/x/xerrors` as a known
  unavoidable transitive test-only dependency. Include: the `go mod why` chain, the
  upstream path to resolution (protobuf upgrade), and the mitigation (test-only, never
  runs in production binaries).
  — verification: file exists at `docs/dependency-notes.md`; `go build ./...` and
  `go test ./...` both pass.

## 2. Smoke test

- [ ] 2.1 Create `test/smoke_test.go` — follows the `chdirTemp` isolation pattern from
  `internal/tigersdk/tigersdk_test.go` (clears all `TIGEROPEN_*` vars, `$HOME` at empty temp dir).
  Constructs a `*config.Config` with a dummy private key, builds a `tigersdk.Session`,
  retrieves `session.Quote()`, and makes one read-only API call with a 5-second
  timeout. Network errors are logged as warnings and do not fail the test — the test
  goal is wiring verification, not live connectivity.
  — verification: `go test -run TestSmoke ./test/` passes.

## 3. Verify full suite

- [ ] 3.1 Run `go vet ./...` and confirm clean — verification: no output.
- [ ] 3.2 Run `go build ./...` — verification: exit code 0.
- [ ] 3.3 Run `go test ./...` — verification: all tests pass.
- [ ] 3.4 Confirm GitHub Actions CI satisfies issue #17 — verification: `.github/workflows/ci.yml`
  and `.github/workflows/verify.yml` already exist and exercise `go vet`, `go build`, `go test`,
  and `scripts/verify` on every PR and push. No changes needed.

## 4. Commit and PR

- [ ] 4.1 Stage and commit: `docs/dependency-notes.md`, `test/smoke_test.go`, and
  `openspec/changes/production-hardening/` on branch `production-hardening`.
  — verification: `git log --oneline` shows the commit on the feature branch.
- [ ] 4.2 Push branch to origin.
  — verification: `git push -u origin production-hardening` succeeds.
- [ ] 4.3 Open PR against `main` — assign Mary (`openclaw-bot-mary`) as reviewer.
  — verification: PR created at `https://github.com/shing1211/tiger-go-demo/pulls`.
- [ ] 4.4 After Mary merges, close issues #13, #14, #15, #16, #17 with a comment linking the
  merged commit.

## Workflow follow-up

- Reviewer Mary merges the PR.
- Issues #13 (meta), #14 (smoke test), #15 (xerrors), #16 (protobuf), #17 (CI) are closed post-merge.
