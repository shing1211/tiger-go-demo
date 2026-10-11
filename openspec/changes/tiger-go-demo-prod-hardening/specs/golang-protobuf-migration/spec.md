# Spec Delta: golang-protobuf-migration

## Purpose

The `github.com/golang/protobuf` v1 package is deprecated and carries known CVEs. The replacement, `google.golang.org/protobuf`, is already a transitive dependency of the Tiger OpenAPI SDK. This capability records that the migration is complete and that no API changes are required in this project.

## ADDED Requirements

### Requirement: The project depends only on google.golang.org/protobuf

The `golang.org/x/protobuf` module SHALL NOT appear in `go.mod` after migration. The `google.golang.org/protobuf` module, which is already a transitive dependency through the SDK, SHALL be the sole protobuf dependency.

#### Scenario: golang/protobuf is removed from go.mod

- **WHEN** the migration is complete
- **THEN** `go list -m all` shows no `github.com/golang/protobuf` entry

#### Scenario: google.golang.org/protobuf satisfies all imports

- **WHEN** the migration is complete
- **THEN** every file that imports `github.com/golang/protobuf` is updated to import `google.golang.org/protobuf` instead, and `go build ./...` succeeds

### Requirement: No API changes are required in this project

The migration SHALL require no changes to source files beyond import path rewrites, because the `google.golang.org/protobuf` package is API-compatible with `github.com/golang/protobuf` for the features this project uses.

#### Scenario: All source files compile after import rewrite

- **WHEN** `github.com/golang/protobuf` is replaced with `google.golang.org/protobuf` in all import statements
- **THEN** `go build ./...` succeeds with no other changes to source files
