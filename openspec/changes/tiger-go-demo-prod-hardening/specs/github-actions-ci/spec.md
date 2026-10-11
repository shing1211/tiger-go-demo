# Spec Delta: github-actions-ci

## Purpose

A GitHub Actions workflow runs the full verification suite on every pull request and push to main, preventing regressions from reaching main and giving contributors an automated gate before requesting review.

## ADDED Requirements

### Requirement: CI runs the full verification suite on every push to main

On every push to the `main` branch, the CI SHALL run the following checks in sequence and SHALL report a failure if any step exits non-zero: `gofmt` check, `go vet`, `go test -race ./...`, SDK coverage assertion, and README/docs consistency check.

#### Scenario: A push to main runs the full suite

- **WHEN** a commit is pushed directly to main
- **THEN** CI runs gofmt, go vet, go test -race, the SDK coverage assertion, and the docs check

#### Scenario: The suite fails if any step fails

- **WHEN** any step in the suite exits non-zero
- **THEN** CI reports a failure and names the failing step

### Requirement: CI runs the full verification suite on every pull request

On every pull request targeting `main`, the CI SHALL run the same suite and SHALL block merge by reporting a failure if any step exits non-zero.

#### Scenario: A PR runs the full suite

- **WHEN** a pull request is opened or updated against main
- **THEN** CI runs gofmt, go vet, go test -race, the SDK coverage assertion, and the docs check

#### Scenario: A PR is blocked from merging by a failing step

- **WHEN** a pull request has a failing step
- **THEN** CI reports a failure and the PR cannot be merged until the suite passes

### Requirement: The workflow uses Go 1.24 from setup-go

The CI job SHALL use the `actions/setup-go` action with a pinned version of Go 1.24 or later, so the workflow reproduces the local development environment.

#### Scenario: The workflow specifies a Go version

- **WHEN** CI reads the workflow file
- **THEN** it pins a Go version through actions/setup-go and that version is 1.24 or later

### Requirement: The SDK coverage check runs as part of CI

The SDK coverage assertion (`make coverage-check`) SHALL run in CI and SHALL fail the run if the uncovered SDK methods do not match the allow-list exactly.

#### Scenario: A new SDK gap fails CI

- **WHEN** a PR introduces code that references a method not on the allow-list and not already covered
- **THEN** the SDK coverage check fails and CI reports the missing method and the allow-list line to add
