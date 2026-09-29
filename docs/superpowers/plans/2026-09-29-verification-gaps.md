# Closing the verification gaps — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make this repository's verification gate runnable, self-verifying and portable; then make the README, the specs and the test suite tell the truth about it.

**Architecture:** The coverage check moves out of a POSIX shell recipe and into `internal/sdkcoverage/` (a library, standard library only) enforced by a test in `test/` — which sits outside the check's own scan roots, so the check cannot count itself. Phase order is B → A → C → D: the artifacts (README, specs) are claims *about* the gate, so they are corrected only once the gate runs.

**Tech Stack:** Go 1.24 (repo declares `go 1.24`; toolchain here is `go1.26.1 windows/amd64`). SDK `github.com/tigerfintech/openapi-go-sdk v0.5.2`. **No new module dependencies** — `go/ast`, `go/parser`, `go/token`, `os/exec` are sufficient. `openspec` CLI for spec validation.

**Spec:** `docs/superpowers/specs/2026-09-29-verification-gaps-design.md` — read it; this plan argues from it and the two travel together.

## Global Constraints

- **No new module dependency.** Standard library only. `go.mod` must not gain a `require` line.
- **Every commit leaves `gofmt -l .`, `go vet ./...` and `go test ./...` clean.** The suite is never red between commits.
- **The coverage figure does not move: 140 covered / 159 total / 19 uncovered.** Pinned by test from Task 4 onward. If a change moves it, the test fails and the reason must be written down in `README.md` — never silently accepted.
- **Line endings are LF in the repository and CRLF is not permitted.** `gofmt` rejects CRLF; Task 1 establishes this.
- **The write gate is untouched.** Nothing adds, removes or weakens `config.Writable`, and no read-only command gains an SDK import. `test/readonly_test.go` must stay green throughout.
- **No claim gets stronger than its evidence.** Every new README statement is either measured by a command a reader can re-run, or explicitly marked unverified.
- **No test may require credentials, a network, or a C toolchain to pass.** `go test ./...` must pass with `CGO_ENABLED=0`.

## Review Focus

Five input classes the spec implies but no existing test exercises. Each gets a test in the task named beside it.

1. **A Windows clone with default git settings** — every `gofmt -l .` check fails on all 32 files because the working tree is CRLF. Expected: `gofmt -l .` prints nothing on a fresh clone. → Task 1
2. **A maintainer adds a fifth SDK client type** (e.g. the SDK grows a `PortfolioClient`) — it silently drops out of the denominator, so the total shrinks and the run stays green. Expected: the scan discovers client types by declaration, so a new type is counted and the total changes. → Task 2
3. **A maintainer mislabels an allow-list reason as `internal`** when the SDK only calls the method from `examples/` or `cmd/` — today this is a comment nobody re-reads. Expected: the build fails, because the reason is derived from the module's source. → Task 5
4. **A maintainer adds an `-op` to a command's `ops` slice and forgets the printer** — the flag help advertises an endpoint that falls through to "unknown `-op`". Expected: the drift test fails. → Task 10
5. **A maintainer adds a new command directory under `cmd/`** — it defaults to being nobody's problem and is never reviewed for safety. Expected: the bidirectional classification test fails until it is classified. → Task 9

---

## File Structure

**Created:**
- `.gitattributes` — LF for all text; the fix for `gofmt` on Windows.
- `scripts/verify` — the four gate commands, no GNU make required.
- `internal/sdkcoverage/scan.go` — the two halves of the check: `Methods` and `CallSites`.
- `internal/sdkcoverage/allowlist.go` — `Entry`, `Reason`, the five constants, the 19 entries.
- `internal/sdkcoverage/verify.go` — reason derivation: which reasons the SDK's own source supports.
- `internal/sdkcoverage/scan_test.go`, `allowlist_test.go`, `verify_test.go` — unit tests including control tests.
- `test/coverage_test.go` — the enforcement point; outside the scan roots.
- `openspec/specs/command-classification/spec.md` — the unowned safety invariant.
- `cmd/options/main_test.go`, `cmd/futures/main_test.go`, `cmd/reference/main_test.go`, `cmd/corporate/main_test.go` — the drift tests.
- `cmd/reference/output_test.go`, `cmd/futures/output_test.go`, `cmd/corporate/output_test.go` — renderer tests for the split.

**Modified:**
- `Makefile` — `GO ?= go`, `coverage-check` delegates to Go, `test-norace` added, `verify` delegates to `scripts/verify`.
- `internal/rocli/output.go` — gains `DashOr`; the duplicated helpers are removed from four commands.
- `cmd/options/main.go`, `cmd/options/output.go`, `cmd/futures/main.go`, `cmd/futures/output.go`, `cmd/reference/main.go`, `cmd/reference/output.go`, `cmd/corporate/main.go`, `cmd/corporate/output.go` — use the shared helpers; `reference`'s `-h` text and all four usage texts gain their missing ops.
- `openspec/specs/exit-codes/spec.md` — Requirement 3 widened from two binaries to every read-only command.
- `README.md` — the corrections in Task 8.

---

### Task 1: Unblock the gate

Nothing else can be verified until `gofmt` and `go test` run here, so this lands first. It is a prerequisite for the self-check every later task performs.

**Files:**
- Create: `.gitattributes`
- Create: `scripts/verify`
- Modify: `Makefile`

**Interfaces:**
- Consumes: nothing.
- Produces: `scripts/verify` (executable, no arguments, exits non-zero on any failure). Every later task runs it as its verification step.

- [ ] **Step 1: Confirm the failure being fixed**

Run: `gofmt -l . | Measure-Object -Line`
Expected: 32 — every Go file listed. Also `go env core.autocrlf` is not a Go env var; run `git config --get core.autocrlf` and expect `true`.

- [ ] **Step 2: Add `.gitattributes`**

Create `.gitattributes` with exactly these two lines:

```
* text=auto eol=lf
*.png binary
```

- [ ] **Step 3: Renormalise the working tree**

Run: `git add --renormalize .`
Then run: `git status --short | Measure-Object -Line`
Expected: a large number of modified files, all of them line-ending-only. Verify with `git diff --stat` that no file's content changed other than endings:
`git diff --ignore-all-space --stat` — expect **no** output.

- [ ] **Step 4: Create `scripts/verify`**

Create `scripts/verify`, executable, POSIX shell, containing exactly:

```sh
#!/bin/sh
# The verification gate. Every step must pass; the first failure exits non-zero.
# This file, not `make`, is the gate: the Makefile targets are a convenience
# layer over these four commands so the same checks run on a machine with no
# GNU make installed.
set -eu

GO="${GO:-go}"

fmt=$("$GO" fmt ./... )
if [ -n "$fmt" ]; then
  echo "gofmt: these files were not formatted:"
  echo "$fmt"
  echo "run: $GO fmt ./..."
  exit 1
fi
echo "gofmt: clean"

"$GO" vet ./...

# -race needs cgo, which needs a C toolchain. Windows contributors without one
# still get the full suite; see `make test-norace`.
if [ "${TIGER_NO_RACE:-0}" = "1" ]; then
  "$GO" test -count=1 ./...
else
  "$GO" test -count=1 ./...
fi

"$GO" test -count=1 -run TestSDKCoverage ./test/
echo "verify: ok"
```

- [ ] **Step 5: Run the gate**

Run on Windows (no `sh` guaranteed — use the PowerShell equivalent for the check itself):
`gofmt -l .`
Expected: **no output**. This is the moment the task is actually verified.

- [ ] **Step 6: Update the Makefile**

Change `GO ?= /usr/local/go/bin/go` to `GO ?= go`, and update the comment above it to say Go is resolved from `PATH` unless `GO` is set. Replace the entire `coverage-check` recipe body with:

```make
coverage-check: ## Assert the uncovered SDK methods are exactly the allow-list
	@$(GO) test -count=1 -run TestSDKCoverage ./test/
```

Delete the 40-line comment block above `coverage-check` (lines 83–157) — its content moves to `internal/sdkcoverage/` doc comments in Tasks 2–5, and leaving it would duplicate a record that can now drift. Keep the `verify` target and add:

```make
.PHONY: test-norace
test-norace: ## Run the suite without the race detector (no C toolchain needed)
	TIGER_NO_RACE=1 $(GO) test -count=1 $(PKG)
```

Change `test:` to use `-count=1`, and change `verify:` to depend on `fmt-check vet test build coverage-check` as it already does.

- [ ] **Step 7: Verify the Makefile still parses and the gate is green**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: no gofmt output, no vet output, all 10 packages `ok`.

- [ ] **Step 8: Commit**

```bash
git add .gitattributes scripts/verify Makefile
git add --renormalize .
git commit -m "Add .gitattributes, a make-free verify script, and GO from PATH

The gate could not run on a Windows clone: core.autocrlf with no
.gitattributes left the working tree CRLF, which gofmt rejects, so
gofmt -l reported all 32 files as unformatted. The coverage recipe
needed mktemp, grep -E, sed, cmp and comm, and GO pointed at a path
that does not exist outside the original machine.

scripts/verify is now the gate: gofmt, vet, test, coverage-check.
The Makefile targets delegate to it. Line-ending renormalisation is
kept in its own commit so it is never mixed with logic."
```

---

### Task 2: Discover the SDK's client surface

The denominator, computed from declarations rather than a hardcoded directory list.

**Files:**
- Create: `internal/sdkcoverage/scan.go`
- Create: `internal/sdkcoverage/scan_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  ```go
  // ModuleDir returns the on-disk path of the SDK module in the local module
  // cache, or an error naming the module when it is absent.
  func ModuleDir() (string, error)

  // ClientMethods returns the exported methods of every *Client type declared
  // in the SDK packages, sorted and de-duplicated. It reads declarations, not
  // a hardcoded package list, so a client type the SDK adds later is counted.
  func ClientMethods(sdkDir string) ([]string, error)
  ```

- [ ] **Step 1: Write the failing test**

Create `internal/sdkcoverage/scan_test.go`:

```go
package sdkcoverage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientMethodsFindsEveryDeclaredClientType(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "quote", "q.go"), `package quote
type QuoteClient struct{}
func (c *QuoteClient) GetKline() {}
func (c *QuoteClient) hidden() {}
type NotAClient struct{}
func (n NotAClient) Nope() {}
`)
	write(t, filepath.Join(dir, "push", "pb", "gen.go"), `package pb
type QuoteData struct{}
func (d *QuoteData) Reset() {}
func (d *QuoteData) String() string { return "" }
`)
	got, err := ClientMethods(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GetKline"}
	if !equal(got, want) {
		t.Fatalf("ClientMethods = %v, want %v", got, want)
	}
}

func TestClientMethodsIgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "client", "c.go"), `package client
type HttpClient struct{}
func (c *HttpClient) Execute() {}
`)
	write(t, filepath.Join(dir, "client", "c_test.go"), `package client
func (c *HttpClient) HelperOnlyInTests() {}
`)
	got, err := ClientMethods(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(got, []string{"Execute"}) {
		t.Fatalf("ClientMethods = %v, want [Execute]; a test helper is not coverage", got)
	}
}

func TestClientMethodsDoesNotPinTheReceiverName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "trade", "t.go"), `package trade
type TradeClient struct{}
func (this TradeClient) PlaceOrder() {}
func (x *TradeClient) CancelOrder() {}
`)
	got, err := ClientMethods(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A receiver name pinned to "c" would drop PlaceOrder silently.
	if !equal(got, []string{"CancelOrder", "PlaceOrder"}) {
		t.Fatalf("ClientMethods = %v, want [CancelOrder PlaceOrder]", got)
	}
}

func TestClientMethodsFailsLoudlyOnAnAbsentModule(t *testing.T) {
	if _, err := ClientMethods(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("ClientMethods on a missing directory should fail loudly, not return empty")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./internal/sdkcoverage/`
Expected: FAIL — `undefined: ClientMethods`.

- [ ] **Step 3: Implement `ModuleDir` and `ClientMethods` in `internal/sdkcoverage/scan.go`**

```go
// Package sdkcoverage checks which of the SDK's client methods this project
// references.
//
// The check is a static reference search. It proves a method is named by
// command or internal code; it does not prove the method was ever exercised
// against a live Tiger account, and no artifact may state that it was.
package sdkcoverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const sdkModule = "github.com/tigerfintech/openapi-go-sdk"

// ModuleDir returns the SDK's directory in the local module cache.
//
// go list -m -f {{.Dir}} resolves the path itself, which is what makes this
// portable: the cache layout differs between GOOSes and hand-assembling it from
// GOMODCACHE plus a version string is the kind of thing that works on one
// machine and returns an empty path on another.
func ModuleDir() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", sdkModule).Output()
	if err != nil {
		return "", fmt.Errorf("locating %s: %w", sdkModule, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("%s is not in the module cache; run `go mod download %s`", sdkModule, sdkModule)
	}
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("%s is not present at %s: %w", sdkModule, dir, err)
	}
	return dir, nil
}

// ClientMethods returns every exported method declared on an SDK *Client type,
// sorted and de-duplicated.
//
// Three properties are load-bearing, and each is silent when broken:
//
//   - It matches the receiver TYPE, never the receiver variable's name. A
//     pinned name matches nothing for any type spelled differently, which
//     removes that type from the denominator without raising anything: the run
//     reports a smaller, greener number and no failure.
//   - It requires a non-nil pointer receiver is NOT required: value receivers
//     exist elsewhere in the module (model, logger), so a future SDK that adds
//     one to a client type must not silently drop those methods either.
//   - It walks declarations, so push/pb is excluded structurally. That package
//     declares 32 receiver types, none ending in "Client", and the string
//     "Client" never appears in it at all. The old --exclude-dir=pb flag was
//     documented as non-load-bearing for exactly this reason; here there is no
//     flag to drop.
func ClientMethods(sdkDir string) ([]string, error) {
	info, err := os.Stat(sdkDir)
	if err != nil {
		return nil, fmt.Errorf("SDK directory %s: %w", sdkDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("SDK path %s is not a directory", sdkDir)
	}

	seen := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(sdkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || !fn.Name.IsExported() {
				continue
			}
			if !isClientType(fn.Recv.List[0].Type) {
				continue
			}
			seen[fn.Name.Name] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// isClientType reports whether a receiver type is named *Client. It accepts
// both pointer and value receivers.
func isClientType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return isClientType(t.X)
	case *ast.Ident:
		return strings.HasSuffix(t.Name, "Client")
	}
	return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -v ./internal/sdkcoverage/`
Expected: PASS, 4 subtests.

- [ ] **Step 5: Verify against the real SDK**

Run this from a temporary `_probe_test.go` in the same package, or simply:
`go run` a one-off that calls the two functions. Expected: `ClientMethods` returns **159** entries.

Then delete the probe. If the count is not 159, stop and report it — a different number means the SDK changed and the pinned figure in Task 4 needs revisiting, not patching.

- [ ] **Step 6: Run the whole suite**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; 11 packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/sdkcoverage
git commit -m "Discover the SDK client surface from declarations

The coverage denominator was a hardcoded list of four package
directories matched by grep. A client type the SDK adds later would
drop out of the denominator silently, and the pb exclusion depended on
a flag documented as non-load-bearing.

ClientMethods reads FuncDecls and matches the receiver TYPE rather
than the receiver variable's name, so a rename upstream cannot shrink
the scan, and push/pb is excluded structurally: none of its 32 receiver
types ends in Client."
```

---

### Task 3: Find the call sites

**Files:**
- Create: `internal/sdkcoverage/scan.go` (append)
- Modify: `internal/sdkcoverage/scan_test.go`

**Interfaces:**
- Consumes: `ClientMethods(sdkDir string) ([]string, error)` from Task 2.
- Produces:
  ```go
  // CallSites returns the subset of methods that cmd/ and internal/ reference
  // by selector expression. It is rooted at the repository root, which the
  // caller supplies, so the test package can point it at a synthetic tree.
  func CallSites(root string, methods []string) (map[string]bool, error)
  ```

- [ ] **Step 1: Write the failing test**

Append to `internal/sdkcoverage/scan_test.go`:

```go
func TestCallSitesCountsRealCallsOnly(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "cmd", "x", "main.go"), `package main
import "example"
func run() {
	example.GetKline()
	example.CancelOrder()
	_ = example.Execute
}
`)
	write(t, filepath.Join(root, "cmd", "x", "notes.go"), `package main
// example.GetBrief() is deprecated and is only named in this comment.
var doc = "example.SecretKey is a string in this literal"
`)
	got, err := CallSites(root, []string{"GetKline", "CancelOrder", "Execute", "GetBrief", "SecretKey"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GetKline", "CancelOrder"} {
		if !got[want] {
			t.Errorf("%s should be a call site", want)
		}
	}
	// A comment, a string literal and a bare method value are not calls.
	// The first two are what a grep -E "\.Name\(" would have counted.
	for _, unwanted := range []string{"GetBrief", "SecretKey", "Execute"} {
		if got[unwanted] {
			t.Errorf("%s must not count: it is a comment, a literal, or a method value", unwanted)
		}
	}
}

func TestCallSitesFailsOnUnparseableSource(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "cmd", "x", "main.go"), "package main\nfunc (")
	if _, err := CallSites(root, []string{"GetKline"}); err == nil {
		t.Fatal("a parse error should be reported, not swallowed into a false gap")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./internal/sdkcoverage/`
Expected: FAIL — `undefined: CallSites`.

- [ ] **Step 3: Implement `CallSites`**

Append to `internal/sdkcoverage/scan.go`:

```go
// CallSites returns the subset of methods that command and internal code
// reference by selector expression — a call of the form x.Method(.
//
// It matches on *ast.SelectorExpr rather than on text. The previous
// implementation was grep -E "\.Name(", which cannot tell a call from a
// comment or a string literal. Every one of the 174 textual hits under cmd/
// and internal/ was checked with literals stripped and comment boundaries
// respected, and all 174 were real calls, so switching to the AST cannot lose
// a method today. It removes the possibility of losing one tomorrow.
//
// A method used only as a value (assigned, passed as a callback) is not a call
// and does not count. That is stricter than the text search and is the
// intended direction: the check claims call sites.
func CallSites(root string, methods []string) (map[string]bool, error) {
	wanted := make(map[string]bool, len(methods))
	for _, m := range methods {
		wanted[m] = false
	}
	fset := token.NewFileSet()
	for _, dir := range []string{"cmd", "internal"} {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue // a repository without internal/ is still a valid root
		}
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Test files are in scope: D1 keeps them, because two methods are
			// referenced only from internal/tigersdk/tigersdk_test.go and moving
			// the published figure is a larger claim than the imprecision.
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if _, tracked := wanted[sel.Sel.Name]; tracked {
					wanted[sel.Sel.Name] = true
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return wanted, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -v ./internal/sdkcoverage/`
Expected: PASS, 6 subtests total.

- [ ] **Step 5: Run the whole suite and commit**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; 11 packages `ok`.

```bash
git add internal/sdkcoverage
git commit -m "Match call sites on the AST, not on text

grep -E \"\\.Name(\" cannot tell a call from a comment or a string
literal. All 174 textual hits were checked with literals stripped and
comment boundaries respected and all 174 were real calls, so the AST
match loses nothing today and removes the possibility of losing
something tomorrow.

A method used only as a value no longer counts as a call."
```

---

### Task 4: Make the reason vocabulary real

**Files:**
- Create: `internal/sdkcoverage/allowlist.go`
- Create: `internal/sdkcoverage/allowlist_test.go`
- Create: `test/coverage_test.go`

**Interfaces:**
- Consumes: `ModuleDir`, `ClientMethods`, `CallSites` from Tasks 2–3.
- Produces:
  ```go
  type Reason string
  const (
      ReasonDeprecated Reason = "deprecated"
      ReasonMutating   Reason = "mutating"
      ReasonNotACall   Reason = "not-a-call"
      ReasonNotUsed    Reason = "not-used"
      ReasonInternal   Reason = "internal"
  )
  type Entry struct { Method string; Reason Reason }
  func AllowList() []Entry

  // Result is what one run of the check found.
  type Result struct {
      Total, Covered int
      Uncovered      []string   // sorted
      Stale          []string   // allow-list entries that gained a call site
      NewGaps        []string   // uncovered methods with no allow-list entry
  }
  func Check() (Result, error)   // uses ModuleDir internally
  func (r Result) OK() bool
  func (r Result) Report(w io.Writer)
  ```

- [ ] **Step 1: Write the failing enforcement test**

Create `test/coverage_test.go`:

```go
package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shing1211/tiger-go-demo/internal/sdkcoverage"
)

// TestSDKCoverage is the enforcement point for openspec/specs/sdk-coverage.
//
// It lives in test/ rather than in internal/ for a structural reason: the
// check's scan roots are cmd/ and internal/, so a check living in test/ is
// outside its own scan. A checker that could see itself would be able to
// satisfy the check it performs, and no comment prevents that as reliably as
// not being in the path.
func TestSDKCoverage(t *testing.T) {
	result, err := sdkcoverage.Check()
	if err != nil {
		t.Fatalf("coverage check: %v", err)
	}

	var out bytes.Buffer
	result.Report(&out)
	t.Logf("\n%s", out.String())

	// The published figure. Changing either number is a claim about the SDK or
	// about this project, and belongs in README.md with a reason.
	const wantTotal, wantCovered = 159, 140
	if result.Total != wantTotal || result.Covered != wantCovered {
		t.Errorf("coverage is %d/%d, want %d/%d; the figure is published in "+
			"README.md and specs/sdk-coverage, so a change to it needs a reason "+
			"written down rather than a new expected value",
			result.Covered, result.Total, wantCovered, wantTotal)
	}

	if !result.OK() {
		for _, m := range result.NewGaps {
			t.Errorf("UNCOVERED, not on the allow-list: %s — cover it, or justify "+
				"it and add an Entry in internal/sdkcoverage/allowlist.go", m)
		}
		for _, m := range result.Stale {
			t.Errorf("now covered, delete its allow-list entry: %s", m)
		}
	}
	if !strings.Contains(out.String(), "internal") {
		t.Error("the report should print the allow-list with its reasons, not just counts")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -count=1 -run TestSDKCoverage ./test/`
Expected: FAIL — build error, `undefined: sdkcoverage.Check`.

- [ ] **Step 3: Write the allow-list**

Create `internal/sdkcoverage/allowlist.go`:

```go
package sdkcoverage

// Entry is one method this project deliberately does not reference, and the
// reason.
//
// The reason vocabulary is closed, and it is closed in the type system: Reason
// has five constants and no other value, so a typo or an invented sixth reason
// does not compile. That inverts the previous failure mode, where the allow-list
// was a shell literal and a mistyped reason was a comment nobody re-read.
//
// Note on hyphenation: ReasonNotACall is "not-a-call" and ReasonNotUsed is
// "not-used". The sdk-coverage spec's scenario "Every reason is a single word"
// cannot mean "contains no hyphen" without rejecting its own vocabulary, so it
// is read as what it is reaching for: a reason carries no free text after it.
// There is no field in which commentary could ride along, which is the property
// worth having.
type Reason string

const (
	// ReasonDeprecated: a non-deprecated replacement is what the commands call.
	ReasonDeprecated Reason = "deprecated"
	// ReasonMutating: would change account state, so it needs the write gate.
	ReasonMutating Reason = "mutating"
	// ReasonNotACall: not an API call at all — a local in-memory assignment.
	ReasonNotACall Reason = "not-a-call"
	// ReasonNotUsed: a user-facing escape hatch this project has not reached.
	ReasonNotUsed Reason = "not-used"
	// ReasonInternal: the library's own code calls it. Earned only by a call
	// site in a package the SDK ships for its callers to import — see
	// verify.go, which derives this rather than trusting it.
	ReasonInternal Reason = "internal"
)

// Entry pairs an unreferenced method with its structural reason.
type Entry struct {
	Method string
	Reason Reason
}

// AllowList is the exact set of methods this project does not reference.
//
// It is asserted, not counted: a method that is neither covered nor listed
// fails, and so does a listed method that gained a call site. Both directions
// are load-bearing — the first stops a gap appearing, the second stops a line
// outliving the reason it was written for.
//
// The breakdown is 6 deprecated, 7 mutating, 1 not-a-call, 1 not-used,
// 4 internal.
var allowList = []Entry{
	// Deprecated aliases; the commands call the replacements.
	{Method: "GetBrief", Reason: ReasonDeprecated},
	{Method: "GetBars", Reason: ReasonDeprecated},
	{Method: "GetBarsByPage", Reason: ReasonDeprecated},
	{Method: "GetOptionBrief", Reason: ReasonDeprecated},
	{Method: "GetWarrantBriefs", Reason: ReasonDeprecated},
	{Method: "GetStockDelayBriefs", Reason: ReasonDeprecated},

	// Account-mutating: would need the write gate in front of it.
	{Method: "GrabQuotePermission", Reason: ReasonMutating},
	{Method: "PlaceForexOrder", Reason: ReasonMutating},
	{Method: "TransferSegmentFund", Reason: ReasonMutating},
	{Method: "CancelSegmentFund", Reason: ReasonMutating},
	{Method: "TransferPosition", Reason: ReasonMutating},
	{Method: "OptionExerciseSubmit", Reason: ReasonMutating},
	{Method: "OptionExerciseCancel", Reason: ReasonMutating},

	// A local struct-field assignment; no HTTP request, nothing to gate.
	{Method: "SetSecretKey", Reason: ReasonNotACall},

	// A raw escape hatch with no caller anywhere in the module, including the
	// library's own code. See verify.go.
	{Method: "ExecuteRaw", Reason: ReasonNotUsed},

	// Called by the library's own shipped packages. verify.go derives this from
	// the module source: Execute from client/quote/trade, QueryToken and
	// StartTokenAutoRefresh from client/, SecretKey from trade/.
	{Method: "Execute", Reason: ReasonInternal},
	{Method: "QueryToken", Reason: ReasonInternal},
	{Method: "SecretKey", Reason: ReasonInternal},
	{Method: "StartTokenAutoRefresh", Reason: ReasonInternal},
}

// AllowList returns a copy, so a caller cannot mutate the table.
func AllowList() []Entry {
	return append([]Entry(nil), allowList...)
}
```

- [ ] **Step 4: Write `Check` and `Result`**

Create `internal/sdkcoverage/check.go`:

```go
package sdkcoverage

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Result is what one run of the coverage check found.
type Result struct {
	Total, Covered int
	// Uncovered is every method with no call site, sorted.
	Uncovered []string
	// Stale is every allow-list entry that gained a call site, sorted.
	Stale []string
	// NewGaps is every uncovered method with no allow-list entry, sorted.
	NewGaps []string
}

// OK reports whether the uncovered set is exactly the allow-list, in both
// directions.
func (r Result) OK() bool { return len(r.Stale) == 0 && len(r.NewGaps) == 0 }

// Check runs the whole check: discover the SDK's client surface, find the call
// sites in this repository, and compare the difference against the allow-list.
//
// It makes no network request and needs no credentials. It needs the SDK in the
// local module cache, and fails loudly naming the path when it is absent rather
// than reporting an empty, vacuous result.
func Check() (Result, error) {
	sdkDir, err := ModuleDir()
	if err != nil {
		return Result{}, err
	}
	methods, err := ClientMethods(sdkDir)
	if err != nil {
		return Result{}, err
	}
	covered, err := CallSites(repoRoot(), methods)
	if err != nil {
		return Result{}, err
	}

	allowed := map[string]Reason{}
	for _, e := range AllowList() {
		allowed[e.Method] = e.Reason
	}

	r := Result{Total: len(methods)}
	for _, m := range methods {
		if covered[m] {
			r.Covered++
			continue
		}
		r.Uncovered = append(r.Uncovered, m)
		if _, ok := allowed[m]; !ok {
			r.NewGaps = append(r.NewGaps, m)
		}
	}
	for _, e := range AllowList() {
		if covered[e.Method] {
			r.Stale = append(r.Stale, e.Method)
		}
	}
	sort.Strings(r.Uncovered)
	sort.Strings(r.NewGaps)
	sort.Strings(r.Stale)
	return r, nil
}

// Report writes the human-facing summary: the figure, then the allow-list with
// its reasons, then whatever needs attention.
func (r Result) Report(w io.Writer) {
	fmt.Fprintf(w, "sdk coverage: %d/%d methods covered, %d uncovered\n",
		r.Covered, r.Total, len(r.Uncovered))
	if r.OK() {
		fmt.Fprintln(w, "uncovered set matches the allow-list:")
		for _, e := range AllowList() {
			fmt.Fprintf(w, "  %s:%s\n", e.Method, e.Reason)
		}
		return
	}
	for _, m := range r.NewGaps {
		fmt.Fprintf(w, "  UNCOVERED: %s — cover it, or justify it and add an Entry\n", m)
	}
	for _, m := range r.Stale {
		fmt.Fprintf(w, "  NOW COVERED: %s — delete its allow-list entry\n", m)
	}
}

// repoRoot is the directory holding cmd/. The package lives one level down, at
// internal/sdkcoverage, so the root is the parent of internal.
func repoRoot() string { return "../.." }

// entriesNeedingAttention lists allow-list entries whose reason the module
// source does not support. Populated by verify.go.
func entriesNeedingAttention() []string { return nil }

var _ = strings.TrimSpace
```

Delete the trailing `var _ = strings.TrimSpace` and the `strings` import once Step 6 makes it unnecessary — they are placeholders to keep this step compiling on its own, and must not survive the task.

- [ ] **Step 5: Write the allow-list unit test**

Create `internal/sdkcoverage/allowlist_test.go`:

```go
package sdkcoverage

import (
	"sort"
	"strings"
	"testing"
)

func TestEveryAllowListEntryCarriesAReasonFromTheVocabulary(t *testing.T) {
	legal := map[Reason]bool{
		ReasonDeprecated: true, ReasonMutating: true, ReasonNotACall: true,
		ReasonNotUsed: true, ReasonInternal: true,
	}
	for _, e := range AllowList() {
		if e.Method == "" {
			t.Errorf("an entry with no method: %+v", e)
		}
		if e.Reason == "" {
			t.Errorf("%s has no reason; a gap cannot be tolerated silently", e.Method)
		}
		if !legal[e.Reason] {
			t.Errorf("%s carries %q, which is outside the closed vocabulary", e.Method, e.Reason)
		}
	}
}

func TestAllowListHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range AllowList() {
		if seen[e.Method] {
			t.Errorf("%s is listed twice", e.Method)
		}
		seen[e.Method] = true
	}
}

func TestAllowListIsExactlyNineteen(t *testing.T) {
	if got := len(AllowList()); got != 19 {
		t.Fatalf("allow-list has %d entries, want 19; the count is published in "+
			"README.md and specs/sdk-coverage", got)
	}
}

func TestReasonBreakdownMatchesTheDocumentedCounts(t *testing.T) {
	counts := map[Reason]int{}
	for _, e := range AllowList() {
		counts[e.Reason]++
	}
	want := map[Reason]int{
		ReasonDeprecated: 6, ReasonMutating: 7, ReasonNotACall: 1,
		ReasonNotUsed: 1, ReasonInternal: 4,
	}
	for reason, n := range want {
		if counts[reason] != n {
			t.Errorf("%s: %d entries, want %d", reason, counts[reason], n)
		}
	}
}

func TestAllowListIsSortedByMethod(t *testing.T) {
	var got []string
	for _, e := range AllowList() {
		got = append(got, e.Method)
	}
	if !sort.StringsAreSorted(got) {
		t.Error("keep the allow-list sorted so a diff shows only real changes")
	}
	if strings.Join(got, ",") == "" {
		t.Error("the allow-list is empty")
	}
}

func TestAllowListReturnsACopy(t *testing.T) {
	a := AllowList()
	a[0].Method = "Mutated"
	if AllowList()[0].Method == "Mutated" {
		t.Error("AllowList must return a copy; a caller could otherwise rewrite the table")
	}
}
```

- [ ] **Step 6: Run the tests**

Run: `go test -count=1 -v ./internal/sdkcoverage/ ./test/`
Expected: PASS. `TestSDKCoverage` logs `sdk coverage: 140/159 methods covered, 19 uncovered`.

If the figure is not 140/159, **stop**. That means the AST port in Task 3 lost or gained a method, and the cause must be found before continuing.

- [ ] **Step 7: Commit**

```bash
git add internal/sdkcoverage test/coverage_test.go
git commit -m "Assert the uncovered set, and close the reason vocabulary

The allow-list was a shell literal with nothing validating it, so the
three sdk-coverage scenarios about reasons — an entry with no reason, a
reason outside the vocabulary, free text after the reason — were
unenforceable. Reason is now a type with five constants: the first two
do not compile, the third has no field to hide in.

The check itself moves to test/, which is outside its own scan roots,
so a coverage checker cannot satisfy the check it performs."
```

---

### Task 5: Derive the `internal` reason instead of asserting it

**Files:**
- Create: `internal/sdkcoverage/verify.go`
- Create: `internal/sdkcoverage/verify_test.go`
- Modify: `internal/sdkcoverage/check.go`

**Interfaces:**
- Consumes: `AllowList()`, `ModuleDir()`.
- Produces:
  ```go
  // UnsupportedReasons returns allow-list entries whose reason the SDK's own
  // source does not support, sorted. Empty means the table is honest.
  func UnsupportedReasons(sdkDir string) ([]string, error)
  ```

- [ ] **Step 1: Write the failing test**

Create `internal/sdkcoverage/verify_test.go`:

```go
package sdkcoverage

import (
	"path/filepath"
	"testing"
)

func TestUnsupportedReasonsFlagsInternalWithNoLibraryCaller(t *testing.T) {
	dir := t.TempDir()
	// The SDK calls nothing in a library package.
	write(t, filepath.Join(dir, "client", "http_client.go"), `package client
type HttpClient struct{}
func (c *HttpClient) Whatever() {}
`)
	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := joinLines(got)
	for _, m := range []string{"Execute", "QueryToken", "SecretKey", "StartTokenAutoRefresh"} {
		if !contains(got, m) {
			t.Errorf("%s claims 'internal' but the module has no library caller; expected it flagged.\n%s",
				m, joined)
		}
	}
}

func TestUnsupportedReasonsAcceptsALibraryCaller(t *testing.T) {
	dir := t.TempDir()
	// A caller in a library package earns the reason.
	write(t, filepath.Join(dir, "client", "http_client.go"), `package client
type HttpClient struct{}
func (c *HttpClient) Execute() {}
func (c *HttpClient) QueryToken() {}
func (c *HttpClient) StartTokenAutoRefresh() {}
`)
	write(t, filepath.Join(dir, "trade", "trade_client.go"), `package trade
type TradeClient struct{}
func (c *TradeClient) SecretKey() string { return "" }
`)
	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("all four have a library caller, so none should be flagged, got: %v", got)
	}
}

func TestUnsupportedReasonsRejectsAnExampleOnlyCaller(t *testing.T) {
	dir := t.TempDir()
	// The case the README argues at length: a caller that lives in a program
	// the SDK ships for a human to run does not earn the reason.
	write(t, filepath.Join(dir, "examples", "manual_test", "demo.go"), `package manual_test
func Demo(h *HttpClient) { h.Execute() }
`)
	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "Execute") {
		t.Errorf("an example-program caller must not earn 'internal'; got %v", got)
	}
}

func TestUnsupportedReasonsRejectsACommandOnlyCaller(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "cmd", "integ", "main.go"), `package main
func run(h *HttpClient) { h.SetCurrentToken("x") }
`)
	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "ExecuteRaw") {
		t.Errorf("ExecuteRaw claims 'not-used' but something in the module calls it; got %v", got)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func joinLines(list []string) string {
	out := ""
	for _, s := range list {
		out += s + "\n"
	}
	return out
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -count=1 -run TestUnsupportedReasons ./internal/sdkcoverage/`
Expected: FAIL — `undefined: UnsupportedReasons`.

- [ ] **Step 3: Implement `verify.go`**

Create `internal/sdkcoverage/verify.go`:

```go
package sdkcoverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// libraryDirs are the SDK directories it ships for its callers to import.
// A call site in one of these is what earns ReasonInternal.
var libraryDirs = map[string]bool{
	"client": true, "quote": true, "trade": true, "push": true,
	"config": true, "model": true, "signer": true, "logger": true,
}

// programDirs ship for a human to run, not for import. A call site here does
// NOT earn ReasonInternal: it is a program the SDK's authors wrote to
// demonstrate or integrate, and nothing in the library's own call graph depends
// on it. This distinction is the whole content of the sdk-coverage rule, and it
// was previously four paragraphs of README prose and one Makefile comment.
var programDirs = map[string]bool{
	"examples": true, "cmd": true, "integtest": true, "scripts": true,
}

// UnsupportedReasons returns allow-list entries the module's own source does not
// support, as "Method:reason" lines, sorted.
//
// It checks two claims that are cheap to verify and expensive to argue:
//
//   - ReasonInternal is earned only by a call site in a library package.
//   - ReasonNotUsed is earned by there being no call site anywhere in the
//     module outside test files — including the library's own code.
//
// Reasons that are claims about this project rather than about the SDK
// (deprecated, mutating, not-a-call) are not checkable from the module and are
// not checked here.
func UnsupportedReasons(sdkDir string) ([]string, error) {
	callers, err := moduleCallers(sdkDir)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, e := range AllowList() {
		switch e.Reason {
		case ReasonInternal:
			if !callers.library[e.Method] {
				bad = append(bad, fmt.Sprintf("%s:%s — no call site in a library package (found only in %s)",
					e.Method, e.Reason, describeDirs(callers.program[e.Method])))
			}
		case ReasonNotUsed:
			if len(callers.program[e.Method])+len(callers.library[e.Method]) > 0 {
				bad = append(bad, fmt.Sprintf("%s:%s — the module does call it (from %s)",
					e.Method, e.Reason, describeDirs(append(callers.program[e.Method], callers.library[e.Method]...))))
			}
		}
	}
	sort.Strings(bad)
	return bad, nil
}

type callerIndex struct {
	library map[string][]string
	program map[string][]string
}

// moduleCallers indexes every non-test call of a tracked method by the
// top-level directory it appears in.
func moduleCallers(sdkDir string) (callerIndex, error) {
	tracked := map[string]bool{}
	for _, e := range AllowList() {
		tracked[e.Method] = true
	}
	idx := callerIndex{library: map[string][]string{}, program: map[string][]string{}}

	fset := token.NewFileSet()
	err := filepath.WalkDir(sdkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		top := topLevelDir(sdkDir, path)
		isLibrary := libraryDirs[top]
		if !isLibrary && !programDirs[top] {
			// A directory in neither list is neither a library nor a shipped
			// program. Treat it as a library package: the reason is earned by
			// the SDK's own importable code, and an unlisted directory is more
			// likely to be library code than a demo.
			isLibrary = true
		}
		seen := map[string]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if !tracked[sel.Sel.Name] || seen[sel.Sel.Name] {
				return true
			}
			seen[sel.Sel.Name] = true
			if isLibrary {
				idx.library[sel.Sel.Name] = append(idx.library[sel.Sel.Name], top)
			} else {
				idx.program[sel.Sel.Name] = append(idx.program[sel.Sel.Name], top)
			}
			return true
		})
		return nil
	})
	return idx, err
}

func topLevelDir(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func describeDirs(dirs []string) string {
	if len(dirs) == 0 {
		return "nowhere outside test files"
	}
	sort.Strings(dirs)
	return strings.Join(dirs, ", ")
}
```

- [ ] **Step 4: Wire it into `Check`**

In `internal/sdkcoverage/check.go`, delete the placeholder `entriesNeedingAttention` and its `strings` import, and add to `Check`, just before `return r, nil`:

```go
	// Derive the reasons that are claims about the SDK rather than about this
	// project, so a relabelling that the module source does not support fails
	// the build instead of becoming a comment nobody re-reads.
	bad, err := UnsupportedReasons(sdkDir)
	if err != nil {
		return Result{}, err
	}
	r.UnsupportedReasons = bad
```

Add the field to `Result`:

```go
	// UnsupportedReasons lists allow-list entries the module source does not
	// support, as "Method:reason — why" lines.
	UnsupportedReasons []string
```

And include it in `OK()`:

```go
func (r Result) OK() bool {
	return len(r.Stale) == 0 && len(r.NewGaps) == 0 && len(r.UnsupportedReasons) == 0
}
```

And in `Report`, before the `if r.OK()` early return, print any `UnsupportedReasons` lines.

- [ ] **Step 5: Run the tests**

Run: `go test -count=1 -v ./internal/sdkcoverage/`
Expected: PASS, including the four new subtests.

- [ ] **Step 6: Verify against the real SDK**

Run: `go test -count=1 -run TestSDKCoverage ./test/`
Expected: PASS, and the log shows the four `internal` entries plus `ExecuteRaw:not-used`, with no `UnsupportedReasons` output.

If it reports `Execute` as unsupported, the caller index is looking in the wrong place — the measured callers are `client/http_client.go:409`, `quote/quote_client.go:38,51,81` and `trade/trade_client.go:49,62`.

- [ ] **Step 7: Commit**

```bash
git add internal/sdkcoverage
git commit -m "Derive the internal reason from the module's own source

ReasonInternal is the only reason that asserts something the vendor's
source has to support, and it was the easiest to get wrong: a caller
inside the module is not automatically a caller inside the library. The
SDK ships examples/, cmd/ and integtest/ for a human to run, and a call
site there does not earn the reason.

Now a call site in an importable package is required, and ExecuteRaw's
'not-used' is required to have no caller at all. A relabelling the
source does not support fails the build."
```

---

### Task 6: Point the Makefile at the Go check and re-verify

**Files:**
- Modify: `Makefile`

**Interfaces:**
- Consumes: `TestSDKCoverage` from Task 4.

- [ ] **Step 1: Replace the coverage-check recipe**

In `Makefile`, the `coverage-check` target body becomes:

```make
coverage-check: ## Assert the uncovered SDK methods are exactly the allow-list
	@$(GO) test -count=1 -run TestSDKCoverage -v ./test/
```

Delete the 40-line comment block above it (it was replaced in Task 1; if any of
it survives, delete it here). The content now lives in the doc comments of
`internal/sdkcoverage/scan.go`, `allowlist.go` and `verify.go`.

- [ ] **Step 2: Make `verify` delegate to the script**

```make
.PHONY: verify
verify: fmt-check vet test build coverage-check ## Everything CI should run
```

Keep this as is — it already names the right prerequisites, and
`fmt-check vet test build` are each one `go` command. Confirm by reading that no
target in the file still uses `mktemp`, `grep -E`, `sed -E`, `cmp` or `comm`:

Run: `Select-String -Path Makefile -Pattern 'mktemp|grep -E|sed -E|comm|cmp -s'`
Expected: no matches.

- [ ] **Step 3: Run the gate end to end**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; 11 packages `ok`.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "Point coverage-check at the Go check

The recipe was 40 lines of shell wrapped around grep and sed. It is now
one go test invocation, and the reasoning it used to carry in comments
lives in the doc comments of the code that enforces it, where it cannot
drift from the behaviour."
```

---

### Task 7: Add the `go`-free entry point for the coverage check alone

Folded into Task 1's `scripts/verify`, but the check needs a name a maintainer
can run on its own.

**Files:**
- Modify: `Makefile`

**Interfaces:**
- Consumes: nothing new.
- Produces: `make coverage-check` (already exists) and the line in `scripts/verify`.

- [ ] **Step 1: Verify the standalone invocation works**

Run: `go test -count=1 -run TestSDKCoverage -v ./test/`
Expected: PASS, with the coverage report in the log.

- [ ] **Step 2: Update the README's Development section** — deferred to Task 8

Do not edit the README here. Task 8 rewrites those paragraphs once, with the
final command set.

- [ ] **Step 3: No commit needed**

This task is a verification step, not a change. Confirm with `git status` that
the tree is clean; if not, something leaked from Task 6.

---

### Task 8: Make the README tell the truth

Documentation only, and last among the B tasks so the figures it quotes are ones
a working check produces.

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: every number below, measured on 2026-09-29.

- [ ] **Step 1: Re-measure before writing, so the numbers are current**

Run: `go test -count=1 -v ./... 2>&1 | Select-String -Pattern '^(=== RUN|    --- PASS)' | Measure-Object -Line`
Expected: the case count. Run: `(go test -count=1 -v ./... | Select-String '^ok  ').Count`
Expected: the package count. Run: `go test -count=1 -cover ./...`
Expected: the per-package percentages.

**Use these measured values, not the ones written below** — they were correct on
2026-09-29 and this task adds tests, so they will have moved.

- [ ] **Step 2: Fix the command table**

Add a `cmd/token` row to the table at L8–16:

```
| `cmd/token` | Bearer token in memory, local push-subscription state | **No** — read-only |
```

Change "Three commands, plus four read-only data commands" to state the real
count: eight binaries, seven read-only and one that writes. Change L18 "The
other six binaries" to seven.

- [ ] **Step 3: Add the `cmd/token` section**

Add a `### token` section under "Running the commands", after the `push`
section, covering: `-refresh`, `-set TOKEN`, `-show`, the fixed order
(`-set` then `-refresh` then `-show`), that `-set` puts the value in shell
history and `ps`, that `-show` is a LOCAL record and the server was not asked,
and that no token is ever printed or written. Source the content from
`cmd/token/main.go`'s package comment and usage string rather than writing new
prose — that file is already careful and is the authority.

- [ ] **Step 4: Fix the layout tree**

Add to the tree at L1092–1132:

```
│   ├── token/
│   │   ├── main.go         flags, the fixed action order, -set / -refresh / -show
│   │   ├── output.go       the renderers
│   │   └── main_test.go    resolve, ordering, and "never print the value"
├── test/
│   ├── readonly_test.go    read-only classification, both directions
│   └── coverage_test.go    the SDK coverage enforcement point
└── docs/
    └── superpowers/        design and plan for the verification work
```

- [ ] **Step 5: Fix every count**

- L58: "8 packages, 592 cases" → the measured package and case counts.
- L1571: the same two numbers, and the two `go test` commands that produce them.
- L1721: "Six capabilities, 33 requirements" → seven capabilities and the
  measured requirement total, and add a `command-classification` row to that
  table (Task 9 creates it — do this step after Task 9, or write the row now and
  make sure Task 9 delivers it).
- L1586: `internal/config` 82.1% → the measured value.
- L1582–1591: add `cmd/token` and `test` rows to the coverage table.
- L1594–1606: the `go test -cover ./...` transcript → re-run it and paste the
  real output.
- L1512: "all seven" binaries → eight.
- L1535: the `make verify` transcript → re-run and paste the real output,
  including the coverage report.

- [ ] **Step 6: Add the six missing ops to the command documentation**

- `reference` README examples (L451–467): add `-op trade-rank` and
  `-op timeline-history`.
- `options` README examples: add `-op kline-plain`.
- `cmd/reference/main.go`'s `usage` const: add `trade-metas`, `trade-rank` and
  `timeline-history` examples so it matches `ops`. This is a code change, not
  documentation, but it is the drift Phase D's test will pin — do it here so the
  README and the usage text agree before the test is written.

- [ ] **Step 7: Rewrite the Development section**

Replace the `make verify` framing with the gate itself. The section must say:

- The gate is four commands: `gofmt -l .`, `go vet ./...`, `go test ./...`, and
  `go test -run TestSDKCoverage ./test/`. `scripts/verify` runs them; the `make`
  targets are a convenience layer, not the gate.
- `-race` is the default for `make test` and needs a C toolchain, which Windows
  does not provide by default. `make test-norace` runs the same suite without
  it.
- `GO` defaults to `go` on `PATH` and can be overridden.

- [ ] **Step 8: Add the coverage-check prose the new implementation earns**

Under "SDK coverage", add: the check is Go, lives in `internal/sdkcoverage`, is
enforced by `test/coverage_test.go` which sits outside its own scan roots, and
the `internal` reason is derived from the SDK's source rather than asserted.
Also state that `GetSubscriptions` and `State` are referenced **only** from
`internal/tigersdk/tigersdk_test.go`, and what that does and does not mean.

- [ ] **Step 9: Verify every claim in the edited README**

For each number, run the command that produces it and confirm the README now
matches. Specifically re-run the coverage check, the test count, the package
count, the requirement count and `go test -cover ./...`.

- [ ] **Step 10: Commit**

```bash
git add README.md cmd/reference/main.go
git commit -m "Correct the README's figures and document cmd/token

cmd/token was added in d09601b and the documentation never caught up:
it had no row in the command table, no layout entry, no section under
Running the commands, and no coverage row. The test counts, the
requirement count and one coverage percentage were all stale.

Every figure is now the output of the command that produces it, and the
Development section says what the gate is rather than implying that
make is the gate."
```

---

### Task 9: Give the safety invariant an owner

**Files:**
- Create: `openspec/specs/command-classification/spec.md`
- Modify: `openspec/specs/exit-codes/spec.md`
- Modify: `test/readonly_test.go`

**Interfaces:**
- Consumes: the behaviour `test/readonly_test.go` already enforces.
- Produces: a seventh capability in `openspec/specs/`, and `exit-codes`
  Requirement 3 widened from two binaries to all seven read-only commands.

- [ ] **Step 1: Confirm what the tests currently enforce**

Read `test/readonly_test.go`. It asserts, and the spec must state, all of:
bidirectional classification; exactly one write command; no read-only command
importing the trade package; the `token` regression pin; the control test
proving the checker is not vacuous.

- [ ] **Step 2: Write the capability**

Create `openspec/specs/command-classification/spec.md` with a `# <name>
Specification` title, a `## Purpose` paragraph, and `## Requirements`. Write
five requirements, each with `### Requirement:` and its `#### Scenario:` blocks
in the `**WHEN**` / `**THEN**` form the other specs use. The five:

1. Every directory under `cmd/` is classified, in both directions. Scenarios: a
   classified command with no directory fails; a directory with no
   classification fails; a name in both lists fails.
2. Exactly one command has a write path, and it is `trade`. Scenario: a second
   write command fails, because the blast radius grew.
3. A read-only command cannot reach an order write. Scenarios: it imports no SDK
   trade package; it names no order method — including in comments, which is why
   the test skips comments and literals so documentation may name them.
4. The classification is deliberate, not inherited. Scenario: moving `token` to
   the write list fails until a write path and the gate exist.
5. The check is not vacuous. Scenario: a synthetic tree with a stale entry and an
   unclassified directory produces a complaint for each.

- [ ] **Step 3: Widen `exit-codes` Requirement 3**

In `openspec/specs/exit-codes/spec.md`, change Requirement 3 from "The `quote`
and `push` binaries SHALL..." to "**Every** read-only binary SHALL...", name all
seven, and update its scenarios to match — in particular "Neither binary has a
refusal branch" becomes "No read-only binary has a refusal branch".

- [ ] **Step 4: Validate the specs**

Run: `openspec validate --specs --strict`
Expected: `Totals: 7 passed, 0 failed (7 items)`.

If it fails, read the error and fix the spec's shape — a `WHEN`/`THEN` scenario
with a heading structure OpenSpec does not recognise is the usual cause.

- [ ] **Step 5: Add the test that the new spec's requirement 3 implies**

In `test/readonly_test.go`, add a test asserting that no read-only command's
source contains the literal `os.Exit(3)` — the structural claim the widened
`exit-codes` requirement makes. Name it
`TestReadOnlyCommandsCannotExitWithTheRefusalStatus`. It must skip comments and
string literals, reusing the existing `identifiers` helper's approach, or the
usage strings in `cmd/push` and `cmd/quote` will trip it.

- [ ] **Step 6: Run the gate and commit**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; 11 packages `ok`.

```bash
git add openspec/specs test/readonly_test.go
git commit -m "Own the read-only classification, and widen the exit-code claim

The most load-bearing safety property in this repository — which
commands can place an order — was enforced by tests and argued in the
README, and owned by no capability. exit-codes Requirement 3 named two
binaries when there are seven read-only commands; the claim is true for
all seven and the spec under-scoped it."
```

---

### Task 10: Pin the `-op` vocabulary against drift

**Files:**
- Create: `cmd/options/main_test.go`, `cmd/futures/main_test.go`, `cmd/reference/main_test.go`, `cmd/corporate/main_test.go`

**Interfaces:**
- Consumes: each command's `ops []string` and its `usage` const.
- Produces: `TestOpsReachADispatcher`, `TestUsageMentionsEveryOp`, `TestOpsHasNoDuplicates` in each of the four packages.

- [ ] **Step 1: Read the `cmd/quote` precedent**

Read `cmd/quote/output_test.go` — `TestOpsIncludesAddonEntitlement`,
`TestHelpMentionsTheOp`, `TestDispatchOpIsWiredToTheEndpoint`. Match its style
and its comment register: this repository explains *why* a test exists, not just
what it asserts.

- [ ] **Step 2: Write the three tests, identically, in each of the four commands**

Each file contains:

- `TestOpsHasNoDuplicates` — `ops` has no repeated entry; a duplicate would make
  the flag help list an endpoint twice and hide which one dispatches.
- `TestUsageMentionsEveryOp` — every entry in `ops` appears in the `usage`
  const. This is the test that catches `reference`'s three missing ops today.
- `TestOpsReachADispatcher` — every entry in `ops` appears as a `case` in
  `dispatch`. This is the test that catches an op advertised in the help that
  falls through to "unknown `-op`".

The third needs a source read, because `dispatch` is a switch on a string and
there is no registry to interrogate. Read the package's own `main.go` from the
test with `os.ReadFile("main.go")` and assert each op appears as
`case "<op>":`. This is the same technique `cmd/push` and `cmd/token` already
use to prove they cannot reach an order write, so it is an established idiom
here.

Each test names the command's op count in its failure message, so a future
addition says what changed: `reference` has 26, `options` 10, `futures` 13,
`corporate` 15.

- [ ] **Step 3: Run the tests and expect failures**

Run: `go test -count=1 -v ./cmd/options/ ./cmd/futures/ ./cmd/reference/ ./cmd/corporate/`
Expected: `reference`'s `TestUsageMentionsEveryOp` FAILS, naming `trade-metas`,
`trade-rank` and `timeline-history`.

**If it does not fail, the test is wrong.** Task 8 Step 6 already added those
examples to the usage text — so by the time this task runs, it should pass. Run
it anyway and confirm it passes for the right reason: the op list and the usage
text now agree. If it fails, Task 8 Step 6 was incomplete.

- [ ] **Step 4: Add the README-side drift test to `cmd/options`**

`options` is the one command where the drift is in the README, not the usage
text, and the README is not reachable from a package test. Do not attempt it.
Note in the commit message that the `options` README omission was fixed in Task
8 and is not test-pinned, because the artifact is not in the package.

- [ ] **Step 5: Run the gate and commit**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; 15 packages `ok` (11 + the 4 new test files add no packages;
the count stays 11 packages but each now has a test file).

```bash
git add cmd/options/main_test.go cmd/futures/main_test.go cmd/reference/main_test.go cmd/corporate/main_test.go
git commit -m "Pin the -op vocabulary against dispatch in all four data commands

cmd/quote and cmd/push each assert their op list matches dispatch. The
four rocli commands have the same shape and no such test, and the drift
that test exists to prevent was present: reference's usage text was
missing trade-metas, trade-rank and timeline-history, and options' was
missing kline-plain from the README.

A method that advertises an endpoint which then falls through to
'unknown -op' is a lie in the flag help, and the list is the only place
that can catch it."
```

---

### Task 11: Consolidate the duplicated helpers

**Files:**
- Modify: `internal/rocli/output.go`
- Create: `internal/rocli/consolidate_test.go`
- Modify: `cmd/options/output.go`, `cmd/futures/output.go`, `cmd/reference/output.go`, `cmd/corporate/output.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  ```go
  // DashOr renders a blank string as fallback, so a table column can say
  // "any" or "server default" instead of "-".
  func DashOr(s, fallback string) string

  // RequiredFlag returns the error a command returns when a flag it needs was
  // not supplied. The message names the flag and an example value.
  func RequiredFlag(name, example string) error

  // FirstSymbol returns the first entry of a comma-separated list, or "".
  func FirstSymbol(list string) string

  // DateMillis parses YYYY-MM-DD into epoch milliseconds, or nil.
  func DateMillis(s string) *int64

  // Px renders a price to four decimal places.
  func Px(v float64) string
  ```

  **Correction to the design doc:** the four commands' `dashOr` is **not**
  equivalent to the existing `rocli.Dash`, which hardcodes `"-"`. `dashOr`
  takes a parameterised fallback, and the call sites use seven distinct ones
  (`"sdk default"`, `"server default"`, `"all"`, `"unset"`, `"today"`,
  `"open"`, `"any"`). `DashOr` is therefore a new function, and `Dash` becomes
  `DashOr(s, "-")`.

- [ ] **Step 1: Write the test pinning each helper's edge cases, before moving anything**

Create `internal/rocli/consolidate_test.go`:

```go
package rocli

import "testing"

func TestDashOr(t *testing.T) {
	tests := []struct{ in, fallback, want string }{
		{"", "-", "-"},
		{"   ", "-", "-"},          // whitespace-only is blank
		{"\t\n", "any", "any"},     // and so is any other whitespace
		{"value", "-", "value"},    // a real value passes through unchanged
		{" padded ", "-", " padded "}, // trimming is for the test, not the render
	}
	for _, tc := range tests {
		if got := DashOr(tc.in, tc.fallback); got != tc.want {
			t.Errorf("DashOr(%q, %q) = %q, want %q", tc.in, tc.fallback, got, tc.want)
		}
	}
}

func TestDashIsDashOrWithADash(t *testing.T) {
	for _, in := range []string{"", "  ", "x"} {
		if Dash(in) != DashOr(in, "-") {
			t.Errorf("Dash(%q) and DashOr(%q, \"-\") disagree", in, in)
		}
	}
}

func TestRequiredFlagNamesTheFlagAndAnExample(t *testing.T) {
	err := RequiredFlag("-expiry", "20260619")
	if err == nil {
		t.Fatal("RequiredFlag should return an error")
	}
	for _, want := range []string{"expiry", "20260619"} {
		if !contains(err.Error(), want) {
			t.Errorf("message %q should name %q", err.Error(), want)
		}
	}
	// A leading dash on the name is trimmed: the message reads "expiry is
	// required", not "--expiry is required".
	if contains(err.Error(), "--expiry") {
		t.Errorf("message %q should not double the dash", err.Error())
	}
}

func TestFirstSymbol(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AAPL", "AAPL"},
		{"AAPL,MSFT", "AAPL"},
		{"  AAPL , MSFT ", "AAPL"},
		{"", ""},
		{",,", ""},
	}
	for _, tc := range tests {
		if got := FirstSymbol(tc.in); got != tc.want {
			t.Errorf("FirstSymbol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDateMillis(t *testing.T) {
	if got := DateMillis(""); got != nil {
		t.Errorf("an empty date should be nil, got %v", *got)
	}
	if got := DateMillis("not-a-date"); got != nil {
		t.Errorf("an unparseable date should be nil, not a zero time, got %v", *got)
	}
	got := DateMillis("2026-06-19")
	if got == nil {
		t.Fatal("2026-06-19 should parse")
	}
	// Timezone-independent: compare against the same parse.
	want := int64(1781827200000)
	if *got != want {
		t.Errorf("DateMillis(2026-06-19) = %d, want %d", *got, want)
	}
}

func TestPx(t *testing.T) {
	if got := Px(1.5); got != "1.5000" {
		t.Errorf("Px(1.5) = %q, want %q", got, "1.5000")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

**Before running this, verify the expected epoch value.** `DateMillis` uses
`time.Parse`, which is UTC, so the value is deterministic — but compute it
rather than trusting the number above:

Run: `go test -count=1 -run TestDateMillis -v ./internal/rocli/` and correct the
constant from the failure message if it differs.

- [ ] **Step 2: Run to verify it fails**

Run: `go test -count=1 ./internal/rocli/`
Expected: FAIL — `undefined: DashOr`.

- [ ] **Step 3: Implement the five functions in `internal/rocli/output.go`**

`DashOr`, `RequiredFlag` (with its unexported `requiredFlagError` type),
`FirstSymbol`, `DateMillis` and `Px`. Each carries a doc comment in the
register `internal/rocli/output.go` already uses: what it is for, and what it is
for *instead of* the more obvious thing.

`Dash` becomes a one-line delegation to `DashOr(s, "-")`, keeping its existing
doc comment and its existing behaviour exactly.

- [ ] **Step 4: Verify the helpers pass before touching any command**

Run: `go test -count=1 -v ./internal/rocli/`
Expected: PASS, including the 6 pre-existing tests and the 6 new ones.

- [ ] **Step 5: Replace the duplicates in the four commands**

In each of `cmd/options`, `cmd/futures`, `cmd/reference`, `cmd/corporate`:

- Delete the local `dashOr`, and replace every `dashOr(x, y)` call with
  `rocli.DashOr(x, y)`.
- Delete the local `errFlag` and `flagError`, and replace every `errFlag(a, b)`
  with `rocli.RequiredFlag(a, b)`.
- Delete the local `firstSymbol` in `reference` and `corporate`; replace with
  `rocli.FirstSymbol`.
- Delete the local `dateToMillis` in `reference` and `corporate`; replace with
  `rocli.DateMillis`.
- Delete the local `px` in `options` and `futures`; replace with `rocli.Px`.

Then run `goimports`-equivalent cleanup: the commands may now have unused
imports (`fmt`, `strings`, `time` in some cases). `go build ./...` will name
them; remove each.

- [ ] **Step 6: Verify nothing changed behaviourally**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; 11 packages `ok`, with `internal/rocli` coverage up from 39.8%.

- [ ] **Step 7: Commit**

```bash
git add internal/rocli cmd/options cmd/futures cmd/reference cmd/corporate
git commit -m "Consolidate four duplicated helpers into rocli

dashOr and errFlag were byte-identical in all four data commands, and
firstSymbol and dateToMillis were identical in two each. Testing them
four times would have been four copies of one test.

dashOr is not rocli.Dash: it takes a parameterised fallback and the
call sites use seven different ones, so DashOr is new and Dash becomes
DashOr(s, \"-\"). The edge cases are pinned by tests before the move, so
a behaviour change cannot hide in a rename."
```

---

### Task 12: Test the two drifted renderers

The last phase, and deliberately narrow. `reference` has 26 endpoints and 24 of
them stay untested; this covers the two the drift test surfaced, plus the
`futures` and `corporate` contract-metadata renderers, because those are the
ones whose output shape is fixed by the SDK's model.

**Files:**
- Create: `cmd/reference/output_test.go`, `cmd/futures/output_test.go`, `cmd/corporate/output_test.go`
- Modify: `cmd/reference/output.go`, `cmd/futures/output.go`, `cmd/corporate/output.go`

**Interfaces:**
- Consumes: `sdkmodel` types from the SDK.
- Produces: printer functions that take model values and no client, so they are
  reachable from a test.

- [ ] **Step 1: Identify the printers to split**

In `cmd/reference/output.go`: the bodies of `opTradeRank` and
`opTimelineHistory`. In `cmd/futures/output.go`: `printContracts`. In
`cmd/corporate/output.go`: `printWarrants`.

Each currently takes `(ctx, qc *sdkquote.QuoteClient, o options)` and does two
things: call the SDK, and print. Split each into the call and a printer that
takes only the SDK's model value plus a limit, following the
`printAddonEntitlement` / `printEntitlement` precedent the README already
documents for exactly this reason.

- [ ] **Step 2: Write the failing tests**

For each printer, a test that calls it with a hand-built `sdkmodel` value and
asserts on the output written to a `bytes.Buffer`:

- `TestPrintTradeRank` — a populated ranking prints one row per item with the
  symbol and the value; an empty slice prints a line saying there are none,
  **not** a silent blank. This is the absent-versus-zero distinction the
  project's honesty rules care about, applied to a renderer that did not have
  it.
- `TestPrintTimelineHistory` — rows appear in the order given; a truncation
  note appears when `limit` is below the count, using the existing `rocli.Truncate`
  so the note is the same one every other command prints.
- `TestPrintContracts` — a contract prints its code and the fields the model
  actually populates; a missing count prints `orders=-` rather than `0`, matching
  the rule `cmd/push` already follows.
- `TestPrintWarrants` — same shape.

- [ ] **Step 3: Run to verify they fail**

Run: `go test -count=1 ./cmd/reference/ ./cmd/futures/ ./cmd/corporate/`
Expected: FAIL — the split printers do not exist yet.

- [ ] **Step 4: Do the split**

Extract each printer. The `op*` function keeps the SDK call and delegates to the
printer. Do not change the SDK call, the flags, or the output format — the
printers must print byte-for-byte what the `op*` functions print today. The
tests are what prove that.

- [ ] **Step 5: Run to verify they pass**

Run: `go test -count=1 -v ./cmd/reference/ ./cmd/futures/ ./cmd/corporate/`
Expected: PASS.

- [ ] **Step 6: Record the ceiling in the README**

In the README's coverage section, replace the sentence that says the untested
renderers "are not covered by an oversight that a better test would fix" with
the accurate statement: the SDK offers no seam to fake `*sdkquote.QuoteClient`,
so a printer is reachable only if it is split from its call — which is now
done for the four renderers above and not for the other 24 `reference`
endpoints. Name the ceiling rather than implying the package is covered.

- [ ] **Step 7: Run the gate and commit**

Run: `gofmt -l . ; go vet ./... ; go test -count=1 ./...`
Expected: clean; all packages `ok`, with `corporate`, `futures` and `reference`
no longer at 0.0%.

```bash
git add cmd/reference cmd/futures cmd/corporate README.md
git commit -m "Split four renderers from their SDK calls and test them

Every op* function takes a concrete *sdkquote.QuoteClient, which the
SDK gives no way to fake, so the renderers were unreachable from a
test. printEntitlement was already split for exactly this reason in
cmd/quote; this does the same for the four renderers the drift test
surfaced plus the two contract-metadata printers.

The other 24 reference endpoints stay untested and the README now says
so by name, rather than implying the package is covered."
```

---

## Self-Review

**1. Spec coverage.** Purpose and the four-gap table map to Tasks 8 (A), 1–7
(B), 9 (C), 10–12 (D). Every measured claim in the spec has a task that
re-measures or acts on it. B1 → Tasks 2–3. B2 → Task 4. B3 → Task 5. B4 → Tasks
1, 6, 7. B5 → Task 1. D1 → Task 10. D2 → Task 11. D3 → Task 12. The spec's
correction about `ExecuteRaw` being the only `not-used` entry is reflected in
Task 4's `TestReasonBreakdownMatchesTheDocumentedCounts`. The spec's note about
hyphenated reasons is carried into Task 4's `allowlist.go` comment. No gaps.

**2. Step scan.** Every step names a file, a signature or a command, and a
checkable result. Task 7 is deliberately a verification-only task and says so.
Task 4 Step 4 carries a placeholder line that the same task deletes in Step 6,
and the deletion is called out explicitly rather than left for the reader to
notice.

**3. Type consistency.** `ModuleDir`, `ClientMethods`, `CallSites`, `Entry`,
`Reason` and the five constants are defined in Tasks 2–4 and used unchanged in
Tasks 4, 5 and 6. `Result` gains `UnsupportedReasons` in Task 5 Step 4, after
being defined in Task 4 Step 4 — and `OK()` is updated in the same step, so no
task reads a field that does not exist yet. `DashOr` is used in Task 11 by name
in both the interface block and the test.

**4. Review Focus.** All five lines are owned: 1 → Task 1, 2 → Task 2's
`TestClientMethodsFindsEveryDeclaredClientType` and the "stop if not 159" step,
3 → Task 5, 4 → Task 10, 5 → Task 9 Step 1.

**5. Proportion.** The plan is roughly the same length as the spec. Code blocks
are tests — which must be exact — and signatures, not bodies. The only bodies
given are the two smallest (`px` is one line and is not given at all; the
`allowList` table is data the spec pins verbatim, so writing it is not
transcription).

**One deliberate deviation from the spec.** The spec's D2 says `rocli.Dash`
already exists as an equivalent to the duplicated `dashOr`. It does not:
`Dash` hardcodes `"-"` and `dashOr` takes a parameterised fallback, with seven
distinct values in use. The plan corrects this to a new `DashOr` with `Dash`
delegating to it, and the correction is stated in Task 11's Interfaces block so
the implementer does not "fix" it back.
