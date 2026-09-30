# Split all inline renderers for offline testability

## Status

Draft — pending user review of this document.

## Background

Every `op*` handler in `cmd/options`, `cmd/futures`, `cmd/corporate`, and `cmd/reference` holds a concrete `*sdkquote.QuoteClient`. The SDK offers no injectable transport, so the client is a live-network dependency. Anything that holds one is unreachable in a unit test without real credentials and a real market connection.

The rendering logic inside each handler is pure formatting — it takes flat data and writes formatted text to `out`. That logic is deterministic and fully testable offline, provided the client can be bypassed.

Eight renderers have already been split using this pattern:

| Package | Function | Commit |
|---|---|---|
| cmd/options | `printChains` | aa024b4 |
| cmd/options | `printGreeks` | aa024b4 |
| cmd/futures | `printContracts` | 987c0c6 |
| cmd/reference | `printBrokerSide` | 987c0c6 |
| cmd/reference | `printBriefs` | aa024b4 |
| cmd/reference | `printTradeRank` | 987c0c6 |
| cmd/reference | `printTimelineHistory` | 987c0c6 |
| cmd/reference | `printTimelineHistoryRows` | 987c0c6 |

The remaining 49 handlers (and 1 helper) are inline with their SDK calls. This spec covers splitting all of them.

## What this spec does not change

- **SDK client seam.** The concrete `*sdkquote.QuoteClient` stays in the handler. There is no interface, no mock, and no fake. The split is purely intra-file: the call stays in the handler, the formatting moves to a separate function.
- **`-limit 0` behaviour.** 48 of the 50 renderers use a bare `i >= o.Limit` guard; 2 use `if o.Limit > 0 && i >= o.Limit`. This divergence is documented in the README and is out of scope for this spec.
- **The 140/159 SDK-method coverage claim.** Measured and maintained separately.
- **CI / GitHub Actions.** Explicitly out of scope per the verification-honesty design.

## Design

### Conventions

#### Writer: package-level `out io.Writer`

Every package uses a package-level `var out io.Writer = os.Stdout`. The existing `capture` helper in each package's `_test.go` swaps this for a `bytes.Buffer` to capture output without a live account.

This is the majority convention (all four packages use it) and the already-split renderers (`printContracts`, `printTradeRank`, `printTimelineHistory`) all use it. No package-wide refactor needed.

#### Naming

- Wrappers keep their `op` prefix: `opExpiration`, `opKline`, `opScanner`.
- Renderers drop the prefix: `printExpiration`, `printKlines`, `printScannerRows`.
- When a renderer handles multiple input structs in sequence, suffix each: `printContracts`, `printContractsRows`.
- Helper-only renderers (not called from a handler) keep their existing name.

#### Empty-result branch

Every renderer gets an empty-result guard at the top, before any header is printed:

```go
func printFoo(records []Foo, limit int) {
    if len(records) == 0 {
        fmt.Fprintln(out, "  (no rows returned)")
        return
    }
    // ... header and rows
}
```

The string `  (no rows returned)` is consistent with every tested renderer already in the codebase. It is not a user-facing message — the header (from `rocli.Section`) prints before the renderer is called, so the context is already established.

For renderers that receive a pointer (`*Foo`) rather than a slice, the guard is `if foo == nil`.

#### Limit handling

Each renderer receives `limit int` as a parameter. The handler passes `o.Limit`. The renderer applies the guard: `if i >= limit { rocli.Truncate(...); break }`.

The bare `i >= limit` form is used (matching the existing 48-site convention). The two renderers that currently use `if o.Limit > 0 && i >= o.Limit` (`opExpiration` and `opChain` in options) are updated to the bare form for consistency.

#### Nested loops

Several renderers have nested loops (an outer slice and an inner slice). The limit applies to the inner loop only, matching the existing behaviour. Each inner loop is its own renderer function.

#### opScanner

`opScanner` has a 4-group sub-loop (`base`, `accumulate`, `financial`, `multi_tag`). Each group is extracted into its own renderer: `printScannerBase`, `printScannerAccumulate`, `printScannerFinancial`, `printScannerMultiTag`. The outer loop stays in the handler.

#### opStockFundamental

The fundamental bundle returns a `map[string]any` which is passed directly to `rocli.JSON(out, fund)`. This renderer calls `rocli.JSON` instead of a `fmt.Fprintf` loop. No structural change — just a function boundary.

#### opScannerTags

The tags payload is also server-defined JSON. The existing `return rocli.JSON(out, groups)` call is extracted into `printScannerTags(rows []sdkmodel.MarketScannerTagGroup) error` which returns the error from `rocli.JSON`. The handler calls `printScannerTags(out, groups)` instead of the inline `return rocli.JSON(...)`.

#### opKlineQuota

`opKlineQuota` has a double-nested loop (quotas → detail). The outer loop is extracted to `printKlineQuota(quotas []sdkmodel.KlineQuota, limit int)`. The detail items are printed via `fmt.Fprintf` inline within that function, not as a separate renderer.

## Complete renderer inventory

### cmd/options (8 handlers + 1 helper to split)

| # | Handler | New renderer | Nested loops | Notes |
|---|---|---|---|---|
| 1 | `opExpiration` | `printExpiration` | yes (dates) | bare limit guard |
| 2 | `opQuote` | `printBriefs` | no | shares name with reference |
| 3 | `opKline` | `printKlines` | yes (kline.Items) | bare limit guard |
| 4 | `opKlinePlain` | `printKlinesPlain` | yes (kline.Items) | bare limit guard |
| 5 | `opDepth` | `printDepth` | yes (bids/asks) | bare limit guard |
| 6 | `opTicks` | `printTicks` | yes (tick.Items) | bare limit guard |
| 7 | `opTimeline` | `printTimeline` | yes (3 session buckets) | bare limit guard |
| 8 | `opSymbols` | `printSymbols` | no | bare limit guard |
| 9 | `opAnalysis` | `printAnalysis` | yes (volatilityList) | bare limit guard |
| — | `nearestExpiry` | `nearestExpiry` | — | helper; stays with qc; untestable; out of scope |

Note: `opQuote` in options uses a different data type (`[]sdkmodel.OptionBrief`) than `printBriefs` in reference (`[]sdkmodel.Brief`). They share a name but have different signatures and are independent functions.

### cmd/futures (10 handlers)

| # | Handler | New renderer | Nested loops | Notes |
|---|---|---|---|---|
| 10 | `opExchange` | `printExchanges` | no | |
| 11 | `opCurrent` | calls `printContracts` | no | `printContracts` already split |
| 12 | `opContract` | calls `printContracts` | no | `printContracts` already split |
| 13 | `opContracts` | calls `printContracts` | no | `printContracts` already split |
| 14 | `opAllContracts` | calls `printContracts` | no | `printContracts` already split |
| 15 | `opContinuous` | calls `printContracts` | no | `printContracts` already split |
| 16 | `opQuote` | `printFutureQuotes` | no | bare limit guard |
| 17 | `opKline` | `printFutureKlines` | yes (kline.Items) | bare limit guard |
| 18 | `opKlinePage` | `printKlinePageBars` | no | bare limit guard |
| 19 | `opDepth` | `printFutureDepth` | yes (bids/asks) | bare limit guard |
| 20 | `opTicks` | `printFutureTicks` | no | bare limit guard |
| 21 | `opTradingTimes` | `printTradingTimes` | no | nil guard |
| 22 | `opHistoryMain` | `printHistoryMain` | no | bare limit guard |

`printContracts` is already split (987c0c6). Handlers 11–15 pass their result slice directly to it.

### cmd/corporate (10 handlers)

| # | Handler | New renderer | Nested loops | Notes |
|---|---|---|---|---|
| 23 | `opCorporateAction` | `printCorporateActions` | no | nil guard (len==0 check) |
| 24 | `opIPO` | `printIPOs` | no | |
| 25 | `opSymbolChange` | `printSymbolChanges` | no | |
| 26 | `opDelisting` | `printDelistings` | no | |
| 27 | `opCapitalFlow` | `printCapitalFlow` | no | nil guard (`flow == nil`) |
| 28 | `opCapitalDistribution` | `printCapitalDistribution` | no | nil guard (`dist == nil`) |
| 29 | `opWarrantFilter` | `printWarrants` | no | nil guard (`res == nil`); calls existing `printWarrants` |
| 30 | `opWarrantQuote` | `printWarrants` | no | calls existing `printWarrants` |
| 31 | `printWarrants` | `printWarrants` | no | already exists; shared by 29 and 30 |
| 32 | `opFundSymbols` | `printFundSymbols` | no | bare limit guard |
| 33 | `opFundContracts` | `printFundContracts` | no | bare limit guard |
| 34 | `opFundQuote` | `printFundQuotes` | no | bare limit guard |
| 35 | `opFundHistory` | `printFundHistory` | no | bare limit guard |

### cmd/reference (22 handlers)

| # | Handler | New renderer | Nested loops | Notes |
|---|---|---|---|---|
| 36 | `opSymbols` | `printSymbols` | no | bare limit guard; same name as options |
| 37 | `opSymbolNames` | `printSymbolNames` | no | bare limit guard |
| 38 | `opStockDetails` | `printStockDetails` | no | bare limit guard |
| 39 | `opStockIndustry` | `printStockIndustry` | no | bare limit guard |
| 40 | `opStockFundamental` | `printStockFundamental` | no | calls `rocli.JSON`; nil guard |
| 41 | `opFinancialDaily` | `printFinancialDaily` | no | bare limit guard |
| 42 | `opFinancialReport` | `printFinancialReport` | no | bare limit guard |
| 43 | `opFinancialCurrency` | `printFinancialCurrencies` | no | bare limit guard |
| 44 | `opExchangeRate` | `printExchangeRates` | no | bare limit guard |
| 45 | `opShortInterest` | `printShortInterest` | no | bare limit guard |
| 46 | `opCalendar` | `printCalendar` | no | bare limit guard |
| 47 | `opScanner` | `printScannerRows` + 4 group renderers | yes (4 sub-groups) | |
| 48 | `opScannerTags` | `printScannerTags` | no | calls `rocli.JSON` |
| 49 | `opIndustryList` | `printIndustryList` | no | bare limit guard |
| 50 | `opIndustryStocks` | `printIndustryStocks` | no | bare limit guard |
| 51 | `opTicks` | `printTradeTicks` | yes (tick.Items) | bare limit guard |
| 52 | `opTimeline` | `printTimeline` | yes (3 session buckets) | bare limit guard |
| 53 | `opDelayed` | calls `printBriefs` | no | `printBriefs` already split |
| 54 | `opKlinePage` | `printKlinePageBars` | no | bare limit guard |
| 55 | `opKlineQuota` | `printKlineQuota` | yes (detail) | nil guard; inline detail printing |
| 56 | `opTradeMetas` | `printTradeMetas` | no | bare limit guard |
| 57 | `opQuotePermission` | `printQuotePermissions` | no | nil guard |
| 58 | `opOvernight` | `printOvernightQuotes` | no | bare limit guard |

`printBriefs` is already split (aa024b4). Handler 53 passes its result to it.

## Phase order

Implementation proceeds package by package:

1. **Phase 1 — cmd/options** (8 renderers + nearestExpiry left as-is)
2. **Phase 2 — cmd/futures** (10 renderers; 5 already call `printContracts`)
3. **Phase 3 — cmd/corporate** (10 renderers; `printWarrants` already exists)
4. **Phase 4 — cmd/reference** (22 renderers; `printBriefs` and `printTradeRank` already split)

Each phase: one commit per renderer, one commit for the tests of that package. Gate (`gofmt`, `go vet`, `go test`) is green before moving to the next phase.

## Test strategy

Each renderer gets a `TestPrint<RendererName>` using the `capture` helper already present in each package's `_test.go`.

Test cases per renderer:
1. **Populated input** — verify correct column layout and value formatting
2. **Empty input** — verify `  (no rows returned)` is printed and nothing else
3. **Limit truncation** — where the renderer has a loop, pass `limit=1` and verify only one row prints
4. **Nil-pointer guard** — for renderers that receive `*T`, pass `nil` and verify the nil message

Where two handlers share a renderer (options/opQuote → `printBriefs` using `OptionBrief`; reference/opDelayed → `printBriefs` using `Brief`), the shared renderer is tested once with the more general type.

## Scope checklist

- [x] All 49 handlers split into call + render
- [x] All 7 shared helpers (`printContracts`, `printWarrants`, `printBriefs`, `printTradeRank`, `printTimelineHistory`, `printTimelineHistoryRows`, `printBrokerSide`) retained and unchanged — verified by brace-matched body comparison against the pre-split revision
- [x] All renderers have an empty-result branch
- [x] All renderers use bare `i >= limit` guard (2 sites updated: `opExpiration` and `opChain` in options). `opChain` had no task in the plan and was finished afterwards in `d0c0290`; zero `Limit > 0 &&` guards remain in the tree
- [~] `opScanner` has 4 group renderers + 1 outer renderer — **deviated to 1 group renderer.** All four groups share the row type `sdkmodel.ScannerDataRow` and one format string, so `printScannerGroup(w, name, rows)` serves all of them and the group names stay in the dispatch loop. Four copies would have been duplication
- [x] `opStockFundamental` and `opScannerTags` call `rocli.JSON` in the renderer
- [x] Each package has tests for each renderer covering: populated, empty, limit, nil where applicable — 12/11/12/28 test functions across options/futures/corporate/reference
- [x] Gate green after each phase: `gofmt`, `go vet`, `go test ./cmd/options/... ./cmd/futures/... ./cmd/corporate/... ./cmd/reference/...`
- [x] README coverage table re-measured after all 4 phases — `9c57a97`. SDK coverage unchanged at 140/159 (the split references no new SDK methods, as intended); statement coverage moved corporate 3.4%→31.9%, futures 2.4%→31.7%, reference 8.5%→41.3%, options 16.3%→44.7%
- [x] This spec is archived after all 4 phases complete — marked complete in place rather than moved, matching the sibling `2026-09-29-verification-gaps` pair, which is also left in place dated. No openspec change was ever created for this work, so `openspec/changes/archive` does not apply

### Added during implementation, not in the original scope

- `truncation_guard_test.go` in all four packages: fails if any limit guard loses its
  `rocli.Truncate` call. All nine futures renderers had silently lost theirs in one
  batch, and the existing limit tests could not catch it because a bare `break` also
  makes rows-past-the-limit absent. 52 capped loops are now checked.
- [`docs/superpowers/renderer-reference.md`](../renderer-reference.md): the verified
  signature of all 60 renderers, after briefs named eight nonexistent SDK types.
- `cmd/quote` remains the one data command whose printers render inline and are
  untestable offline. It was outside this spec's four packages.
