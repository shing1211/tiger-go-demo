# Dependency Notes

This document records dependency decisions that are not changeable without upstream
SDK changes.

## `golang.org/x/xerrors`

**Status:** Unavoidable transitive test-only dependency.

`golang.org/x/xerrors` is not imported by any source file in this module. It is
present in `go.sum` because it is required by the test suite of
`google.golang.org/protobuf/reflect/protoregistry` — a transitive dependency of the
Tiger SDK.

### Why it cannot be removed

```
$ go mod why golang.org/x/xerrors

github.com/shing1211/tiger-go-demo/cmd/push
github.com/tigerfintech/openapi-go-sdk/push
google.golang.org/protobuf/proto
google.golang.org/protobuf/reflect/protoregistry
google.golang.org/protobuf/reflect/protoregistry.test
github.com/google/go-cmp/cmp/cmpopts
golang.org/x/xerrors
```

The chain is: `cmd/push` → `openapi-go-sdk/push` → `protobuf/proto` →
`protoregistry.test` → `go-cmp/cmpopts` → `xerrors`.

`go mod tidy` does not remove it because `protoregistry.test` is a Go test
package that still references `xerrors`.

### Risk

`golang.org/x/xerrors` carried CVE-2022-41717 (a 2022 denial-of-service in
`golang.org/x/text` that propagated through `xerrors` in some configurations).
The CVE affects `golang.org/x/text <= v0.3.7` and is not directly a problem in
`xerrors` itself, but the package is dormant and unmaintained. It runs only in
test contexts, never in production binaries.

### Path to resolution

The `xerrors` import in `protoregistry.test` must be removed upstream in
`google.golang.org/protobuf`, or the Tiger SDK must update to a version of
`protobuf` that no longer has this test-only reference. This change is not
available to this project directly.

### Mitigation

`golang.org/x/xerrors` is loaded only when running test packages in the
`google.golang.org/protobuf/reflect/protoregistry` module. It never runs in
production binaries. The CI gate (`go test ./...`) exercises it, but it cannot
authenticate, place orders, or modify account state.
