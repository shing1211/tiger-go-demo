# Closing the verification gaps — design

**Date:** 2026-09-29
**Scope:** this repository's own verification apparatus. Not the Tiger API, not
the SDK surface, not anything requiring live credentials.

## Purpose

This project makes a specific class of claim: *an artifact describes what was
observed and what was tested, and nothing more.* The rule is normative in
`openspec/specs/verification-honesty/`. This change closes the places where the
repository fails to hold itself to that rule, and the places where the machinery
supposed to enforce it cannot run.

Four gaps, found by measurement rather than by reading:

| | Gap | Consequence |
|---|---|---|
| **A** | The README's figures do not match the repository | The project violates its own honesty spec |
| **B** | `make verify` cannot run on this machine, and the reason-vocabulary rule the spec asserts is unenforced | "Verified" means "a shell script ran on one machine" |
| **C** | The most load-bearing safety invariant is owned by no spec | A test enforces it; nothing states it |
| **D** | Three commands sit at 0.0% coverage, and the drift-test pattern stops at two commands | The gap the pattern exists to prevent is present, uncaught, four times |

**Ordering is B → A → C → D, and the order is load-bearing.** A and C are claims
*about* the verification apparatus. Correcting them before the apparatus works
re-dates the claims rather than fixing them. B first; then the artifacts that
point at B.

## What was measured

Every figure below was produced by running the repository on 2026-09-29, not by
reading it. `go version` reports `go1.26.1 windows/amd64`; the SDK is
`v0.5.2`, the newest published version.

### The suite passes

`go test -count=1 ./...` → 10 `ok` packages, 0 failures. `go vet ./...` clean.

### Gap A — the README contradicts the tree

| README | Measured |
|---|---|
| L58, L1571 — "8 packages, 592 cases" | **10** packages; **652** `===RUN`+`---PASS` lines; 415 test functions, 178 top-level |
| L1721 — "Six capabilities, 33 requirements" | **36** (credential-defence 5, exit-codes 3, sdk-coverage 11, secret-redaction 3, verification-honesty 11, write-gate 3) |
| L1586 — `internal/config` **82.1%** | **81.4%** |
| L18 — "the other six binaries"; L58, L1512 — "all seven" | **eight** binaries; seven read-only |
| L8–16 — command table | 7 rows. **`cmd/token` has no row** |
| L1092–1132 — project layout | omits `cmd/token/` and `test/` |
| L1582–1591 — coverage table | omits `cmd/token` (62.9%) and `test` |
| L569 — `reference` examples | `trade-rank`, `timeline-history` absent |
| `options` examples | `kline-plain` absent from README |
| `reference` `-h` usage text | `trade-metas`, `trade-rank`, `timeline-history` absent |
| "Running the commands" | no `cmd/token` section, though `make run-token` exists |

`cmd/token` was added in `d09601b` and the documentation never caught up.

### Gap B — the gate does not run, for three independent reasons

1. **`gofmt -l .` reports all 32 Go files as unformatted.** Not a formatting
   defect. `git ls-files --eol` shows `i/lf w/crlf` for every file, and
   `core.autocrlf=true` with no `.gitattributes` means the working tree is CRLF
   while the committed blobs are LF. Verified by isolating one file: an LF copy
   is clean under `gofmt -l`, the CRLF copy is not. `gofmt` rejects CRLF
   outright, so `fmt-check` fails on every Windows clone.
2. **`go test -race` exits 2.** `CGO_ENABLED=0` and no gcc; the race detector
   requires cgo. The Makefile hardcodes `-race`.
3. **`make` is not installed, and `GO ?= /usr/local/go/bin/go` names a path
   that does not exist here.** `go` is at `C:\Program Files\Go\bin\go.exe`.
   Separately, `coverage-check` is a shell recipe requiring `mktemp`, `grep -E`,
   `sed -E`, `cmp`, `comm` and `trap`.

Additionally, the `sdk-coverage` spec asserts three rules nothing enforces:
*"An entry without a reason is rejected"*, *"A reason outside the vocabulary is
rejected"*, *"Every reason is a single word"*. The allow-list is an inline
literal in the Makefile with nothing to validate it.

### Gap C — an unowned invariant

`test/readonly_test.go` enforces the most load-bearing safety property in the
repository: which commands are read-only, bidirectionally, and that no read-only
command can reach an order write. It is enforced well — there is a control test
proving the checker is not vacuous. But no capability in `openspec/specs/` owns
it, and `exit-codes` Requirement 3 is scoped to **two** binaries (`quote` and
`push`) when there are **seven** read-only commands.

### Gap D — untested commands, and a pattern that stops short

`corporate`, `futures` and `reference` are at **0.0%**. `cmd/quote` and
`cmd/push` each assert their `ops` slice matches dispatch; the four `rocli`
commands have the same shape and no such test. The drift that test exists to
prevent is present and uncaught — six instances, listed in the table above.

`dashOr` and `errFlag` are byte-identical across all four `rocli` commands while
`internal/rocli` already exports an equivalent (`rocli.Dash`).

## Decisions

Four decisions were put to the reviewer and answered.

**D1 — keep test files in the coverage scan.** Excluding `_test.go` from the
call-site scan would drop the figure from 140/159 to 138/159 and add two
allow-list lines (`GetSubscriptions`, `State`, both referenced only from
`internal/tigersdk/tigersdk_test.go`). Chosen: keep 140/159 and **document** the
two test-only references. Moving a published figure is a larger claim than the
imprecision is worth, and the README already teaches this audience to read a
coverage number narrowly.

**D2 — add a `make`-free verification script, not just a fixed `GO`.** Both, in
fact: `GO ?= go` so the `make` path works wherever `go` is on `PATH`, and
`scripts/verify` so the gate is reachable with no GNU make at all. The README
will state that the `make` targets are a convenience and **the gate is four
commands**.

**D3 — narrow scope for the 0%-coverage commands.** Shared `rocli` helpers,
plus the `trade-rank` and `timeline-history` renderers. The remaining 24
endpoints are untouched. This phase closes a recorded gap; it does not chase a
percentage.

**D4 — the coverage check lives in `test/`, not `internal/`.** See below.

## Design

### Phase B — the coverage gate becomes portable and self-verifying

#### B1. `internal/sdkcoverage/` — the check, in Go

Both halves of the current check are re-implemented with `go/ast` and
`go/parser`. **No new module dependency**: the standard library is sufficient and
`golang.org/x/tools` is not required.

*Denominator.* Parse the four SDK client packages from the module cache and
match `FuncDecl` whose receiver type ends in `Client`, skipping `_test.go`
files. This replaces the shell pipeline's flags by construction: `push/pb`
declares 32 receiver types, **none** ending in `Client`, and the string `Client`
appears **0** times anywhere in that tree — so parsing declarations excludes it
without a flag. `--exclude-dir=pb` was already documented as non-load-bearing
("Do not read a green run as evidence that the flag is doing the work"); it
disappears rather than being carried forward.

The receiver match stays receiver-variable-agnostic, per the existing spec
requirement: the declaration's *shape* is matched, never a hardcoded name.

*Call sites.* Walk `cmd/` and `internal/`, collecting every
`*ast.SelectorExpr` whose `Sel.Name` is in the denominator. This is strictly
narrower than `grep -E "\.Name\("`, which cannot distinguish code from prose.

**The port is behaviour-preserving, and a test pins it.** Narrower matching
could in principle lose coverage, so the claim was measured before committing to
it. Every `\.Name\(` textual hit under `cmd/` and `internal/` — 174 of them —
was re-examined with double-quoted and backtick string literals stripped and the
`//` comment boundary respected. **Zero** hits were anything other than a real
call, so AST matching cannot lose a single method.

`go test` then asserts the result is **140 covered / 159 total / 19 uncovered**,
so the port cannot silently move the published figure. If a future change does
move it, the test fails and the reason must be written down.

#### B2. The reason vocabulary becomes compiler-enforced

`internal/sdkcoverage/allowlist.go` holds the allow-list as
`[]Entry{Method, Reason}`, with `Reason` a defined string type whose only legal
values are five constants. All three unenforced spec scenarios become real: the
first two **do not compile**, and the third is a test.

**"Single word" needs a definition, and the current words do not fit it.** Two
of the five reasons in use are hyphenated — `not-a-call` and `not-used` — so a
literal no-whitespace rule would reject the vocabulary the same spec defines. The
meaning the scenario is reaching for is *no free text after the reason*, and the
test enforces exactly that: a reason is either one of the five constants or it
is nothing at all, with no field in which commentary could ride along. The words
themselves are left as they are, because renaming them would churn the Makefile,
the README and the spec for no gain.

The counts in use today are 6 `deprecated`, 7 `mutating`, 1 `not-a-call`, 1
`not-used`, 4 `internal` — 19 in total, which is the figure the spec pins.

This inverts the failure mode. Today a typo'd reason is a comment nobody
re-reads. After this it does not build.

#### B3. `internal` becomes verified, not argued

The `sdk-coverage` spec says the library-internal reason is earned only by a call
site in a package the SDK ships for its callers to import, and *not* by a call
site in an example or demonstration program. That is currently four README
paragraphs of careful reasoning in a Makefile comment.

It is checkable, because the distinction is structural in the SDK's own layout.
Measured non-test call sites:

| Method | Caller directories | Earned by |
|---|---|---|
| `Execute` | `client`, `quote`, `trade`, `cmd` | 3 library dirs |
| `QueryToken` | `client`, `examples` | `client` |
| `SecretKey` | `trade`, `examples` | `trade` |
| `StartTokenAutoRefresh` | `client`, `cmd`, `examples` | `client` |
| `ExecuteRaw` | none | `not-used` — as claimed |
| `RefreshToken` | `examples` only | escape hatch — as claimed |
| `SetCurrentToken` | `cmd` only | escape hatch — as claimed |

Only `ExecuteRaw` still carries a `not-used` line: `cmd/token` now calls
`RefreshToken` and `SetCurrentToken`, so they are covered and have left the
allow-list. The table records the **SDK-side** fact, which holds for all three
regardless of whether this project calls them — and which the spec requires to
survive that fact, so that relabelling any of them as `internal` remains a
visible change rather than a silent one.

Library packages: `client`, `quote`, `trade`, `push`, `config`, `model`,
`signer`, `logger`. Program directories, shipped for a human to run: `cmd`,
`examples`, `integtest`, `scripts`.

The check derives the reason from the module's own source and fails if an entry
claims `internal` without a qualifying call site. The escape-hatch
classification is derived the same way, so the README's claim that "the SDK does
not call any of `ExecuteRaw`, `RefreshToken` or `SetCurrentToken`" becomes a
test outcome rather than a recorded observation.

#### B4. The check lives in `test/`, and cannot count itself

**D4.** The scan roots are `cmd/` and `internal/`. `test/` is outside them, so a
coverage test in `test/` is not in its own scan.

This closes the one self-referential failure a coverage checker can have — the
checker satisfying the check it performs — structurally, rather than with a
comment explaining why the exclusion list happens not to include itself. It also
matches the repository's existing idiom: `test/readonly_test.go` is a repo-wide
invariant with no natural home inside any single command, and so is this.

`internal/sdkcoverage/` holds the logic so it is unit-testable;
`test/coverage_test.go` is the enforcement point and joins the ordinary
`go test ./...` run. A control test builds a synthetic module cache to prove the
checker objects to a new gap, a stale entry, a bad reason and a widened receiver
pattern — the same technique `TestTheCheckIsNotVacuous` already uses.

#### B5. Unblock the gate

- **`.gitattributes`** with `* text=auto eol=lf`, plus a working-tree
  renormalise. Correct for a Go repository on its own merits, and the fix for
  `gofmt` failing on every Windows clone.
- **`GO ?= go`** so the `make` path honours `PATH`.
- **`scripts/verify`** — `gofmt -l .`, `go vet ./...`, `go test ./...`, and the
  coverage check — runnable with no GNU make. `make verify` delegates to the
  same four commands.
- **`test-norace`** target, because the race detector needs a C toolchain on
  Windows. `-race` remains the default because it is the right default.
- **CI is out of scope.** `.github/` does not exist; adding it is a separate
  decision with a different risk profile.

### Phase A — the README tells the truth

Documentation only, but it edits normative-sounding claims, which is why it
follows B: the figures it quotes should be ones a working check produces.

Every correction is in the measured table above. Beyond the figures: add the
`cmd/token` row, the `cmd/token/` and `test/` layout entries, the `cmd/token`
section under "Running the commands", the `coverage-check` transcript including
`cmd/token`, and the statement that `GetSubscriptions` and `State` are
referenced only from a test file.

New prose, in the register the existing safety section already uses: what the
gate is (four commands, not `make`), what it needs (a C toolchain for `-race`),
and what the two test-only references do and do not mean.

### Phase C — the safety invariant gets an owner

Add `openspec/specs/command-classification/`, written against the behaviour
`test/readonly_test.go` already enforces:

- every directory under `cmd/` is classified read-only or write, in **both**
  directions — a stale entry and an unclassified directory both fail;
- exactly one write command, and it is `trade`;
- a read-only command imports no SDK trade package and names no order method;
- moving `token` to the write list is a deliberate edit that fails loudly;
- every read-only command is structurally unable to reach exit 3.

That last requirement forces a correction to `exit-codes` Requirement 3, which
names two binaries. The claim is true for all seven; the spec under-scopes it.
`openspec validate --specs --strict` must pass afterwards (currently 6/6).

### Phase D — the untested commands

**D1. Vocabulary-drift test, all five `-op` commands.** Assert that every entry
in `ops` reaches a printer, that the usage text mentions every op, and that no
op is listed twice. This is the `cmd/quote` pattern generalised. It would have
caught the six live instances already present: `reference` is missing
`trade-rank` and `timeline-history` from the README and `trade-metas`,
`trade-rank` and `timeline-history` from its own `-h`; `options` is missing
`kline-plain` from the README.

**D2. De-duplicate before testing.** `dashOr` and `errFlag` are byte-identical
across all four `rocli` commands and `rocli.Dash` already exists. Consolidate
into `internal/rocli`, then test once. This is a prerequisite for D3, not
additional scope: the alternative is four copies of the same test.

**D3. `corporate` / `futures` / `reference` off 0.0%, narrowly.** The blocker
is real and the README already diagnoses it: every `op*` function takes
`*sdkquote.QuoteClient`, a concrete SDK struct with no interface, no injectable
constructor and no transport seam. The `cmd/quote` precedent is the answer —
`printEntitlement` is testable because `printAddonEntitlement` is separate. For
the selected renderers, split the call from the rendering and test the rendering
against hand-built `sdkmodel` values. The SDK call itself stays untested, and the
README says so.

## Constraints

- **No new module dependency.** Standard library only for B1–B3.
- **Every commit leaves `gofmt`, `go vet` and `go test` green.** The suite is
  never red between commits, only at the end of a phase.
- **The published coverage figure does not move.** 140/159 is pinned by test.
- **No claim gets stronger than its evidence.** Every new statement in the
  README is either measured by a command a reader can re-run, or explicitly
  marked as not verified.
- **The write gate is untouched.** Nothing here adds, removes or weakens
  `config.Writable`, and no read-only command gains an SDK import.

## Risks

**Renormalising line endings touches every file.** `.gitattributes` plus a
renormalise produces a whole-tree diff. It is mechanical and reviewable, and it
must land in a commit of its own so it is never mixed with logic.

**A test that shells out to `go` must resolve the SDK location portably.** Use
`go list -m -f {{.Dir}}`, verified to return the module cache path directly, and
fail loudly with the path when the library is absent — which the existing spec
already requires.

**Consolidating the duplicated helpers touches four commands' call sites.** A
behaviour-preserving rename plus a test that pins each helper's edge cases
(empty string, whitespace-only, valid value) before the move.

**Narrowing `exit-codes` Requirement 3 is a spec edit, not just an addition.**
The new capability and the widened requirement land together, and both validate.

## Out of scope

- GitHub Actions or any CI configuration.
- The six deliberately uncovered account-mutating SDK methods.
- Anything requiring valid Tiger credentials. The repository's position — that
  nothing here is validated against live data — is unchanged and remains stated.
- The 24 `reference` endpoints not named in D3.
