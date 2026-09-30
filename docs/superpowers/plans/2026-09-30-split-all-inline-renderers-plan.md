# Split all inline renderers — Implementation Plan

> **Status: COMPLETE.** All 51 tasks are implemented and the final gate is green
> (`gofmt -l .` clean, `go vet ./...` clean, `go test ./...` 14/14 packages).
> Two tasks were no-ops because the code was already split (`printContracts` in
> futures, `printScannerTags` and `printBriefs` in reference), and one needed
> tests only (`printWarrants` in corporate).
>
> **This plan contained three errors that the code corrected.** Kept visible
> rather than edited out, because each one cost rework:
> 1. Type names in the task briefs were frequently wrong (`sdkmodel.OptionBrief`,
>    `MarketScannerResult`, `OvernightQuote`, …). The code is authoritative.
>    [`docs/superpowers/renderer-reference.md`](../renderer-reference.md) now
>    carries the verified signature for all 60 renderers.
> 2. "Limit guard: bare `i >= limit`" was not enough — nine futures renderers
>    lost their `rocli.Truncate` call and the briefs did not say to keep it.
>    `truncation_guard_test.go` in all four packages now enforces this.
> 3. Review Focus #3 said the futures kline limit applies to the inner loop
>    only. The base code limited both loops. Restored to match the base.
>
> Also settled after the plan closed: `opChain`/`printChains` was never given a
> task, so the spec's "2 sites updated" checklist item was left at 1/2. Finished
> in `d0c0290`, which also made `-limit 0` mean "no rows" uniformly.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split every `op*` handler in cmd/options, cmd/futures, cmd/corporate, and cmd/reference into a thin SDK-call wrapper and a pure rendering function, making each renderer testable offline without a live account.

**Architecture:** Each handler keeps its `*sdkquote.QuoteClient` and SDK call. The formatting logic moves to a new `print*` function that takes only plain data (slices, ints) and writes to the package-level `out io.Writer`. Handlers pass `o.Limit` to the renderer. Tests use the existing `capture` helper to swap `out` for a `bytes.Buffer`.

**Tech Stack:** Go 1.21+, standard `testing` package, `bytes.Buffer`, `fmt`, `github.com/tigerfintech/openapi-go-sdk/model` and `quote` packages.

**Spec:** `docs/superpowers/specs/2026-09-30-split-all-inline-renderers-design.md`

---

## Global Constraints

- Every commit leaves `gofmt`, `go vet`, and `go test ./cmd/options/... ./cmd/futures/... ./cmd/corporate/... ./cmd/reference/...` green
- `-race` flag requires cgo on Windows; suppress with `TIGER_NO_RACE=1`
- Empty-result string: `  (no rows returned)` (note leading two spaces)
- Limit guard: bare `i >= limit` form
- Each renderer is committed separately; gate green before next task
- Use the `capture` helper already present in each package's `_test.go`

---

## Review Focus

1. **`opExpiration` and `opChain`** (options) — change from `if o.Limit > 0 && i >= o.Limit` to bare `i >= limit`
2. **Shared renderer names** — options/opQuote → `printOptionBriefs` (uses OptionBrief); reference/opDelayed → `printBriefs` (uses Brief); both named `printBriefs` in their original handlers but are different functions
3. **`opKline` (futures)** — nested loop over kline.Items; limit applies to inner loop only
4. **`opScanner`** (reference) — 4 group renderers plus outer `printScannerRows`; groups skipped when empty
5. **`opStockFundamental` and `opScannerTags`** — call `rocli.JSON` in the renderer; propagate errors

---

## Phase 1 — cmd/options

### Task 1: Split opExpiration → printExpiration
Files: Modify: `cmd/options/output.go:28-53`
Interfaces: Produces: `printExpiration(exps []sdkmodel.OptionExpiration, limit int)`

Steps:
- [ ] Write failing test in `cmd/options/output_test.go` — TestPrintExpiration with subtests: populated (exps with 2 dates, check symbol+date+count), empty (nil, expect "  (no rows returned)"), limit_truncation (2 dates, limit=1, expect first only)
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/options/... -run TestPrintExpiration -v` — expect FAIL
- [ ] Extract `printExpiration` from opExpiration — new func with empty guard, bare `i >= limit`, handler calls `printExpiration(exps, o.Limit)`. nearExpiry stays with qc.
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/options/... -run TestPrintExpiration -v` — expect PASS
- [ ] Gate and commit

### Task 2: Split opQuote → printOptionBriefs  
Files: Modify: `cmd/options/output.go:168-192`
Interfaces: Produces: `printOptionBriefs(briefs []sdkmodel.OptionBrief, limit int)`

Steps:
- [ ] Write failing test — TestPrintOptionBriefs with subtests: populated (1 brief), empty (nil), limit (2 briefs, limit=1)
- [ ] Run test — expect FAIL
- [ ] Extract printOptionBriefs from opQuote — empty guard, row loop with bare `i >= limit`
- [ ] Run test — expect PASS  
- [ ] Gate and commit

### Task 3: Split opKline → printKlines AND opKlinePlain → printKlinesPlain
Files: Modify: `cmd/options/output.go:196-255`
Interfaces: Produces: `printKlines(klines []sdkmodel.OptionKline, period string, limit int)`, `printKlinesPlain`

Steps:
- [ ] Write failing tests — TestPrintKlines (populated, empty, limit_truncates_inner_loop), TestPrintKlinesPlain (populated, empty)
- [ ] Run tests — expect FAIL
- [ ] Extract both renderers — nested loop structure; inner loop uses bare `i >= limit`; handlers call with period and o.Limit
- [ ] Run tests — expect PASS
- [ ] Gate and commit

### Task 4: Split opDepth → printDepth
Files: Modify: `cmd/options/output.go:259-300`
Interfaces: Produces: `printDepth(depths []sdkmodel.OptionDepth, limit int)`

Steps:
- [ ] Write failing test — TestPrintDepth (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printDepth — n = max(len(d.Bids), len(d.Asks)); inner loop with bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 5: Split opTicks → printTicks
Files: Modify: `cmd/options/output.go:304-330`
Interfaces: Produces: `printTicks(ticks []sdkmodel.OptionTradeTick, limit int)`

Steps:
- [ ] Write failing test — TestPrintTicks (populated, empty, limit_truncates_inner_loop)
- [ ] Run test — expect FAIL
- [ ] Extract printTicks — outer loop over ticks, inner loop over t.Items with bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 6: Split opTimeline → printOptionTimeline
Files: Modify: `cmd/options/output.go:334-376`
Interfaces: Produces: `printOptionTimeline(tls []sdkmodel.Timeline, limit int)`

Steps:
- [ ] Write failing test — TestPrintOptionTimeline (populated, empty, limit_per_bucket)
- [ ] Run test — expect FAIL
- [ ] Extract printOptionTimeline — 3 bucket groups looped inline, each bucket's items use bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 7: Split opSymbols → printOptionSymbols
Files: Modify: `cmd/options/output.go:380-402`
Interfaces: Produces: `printOptionSymbols(syms []sdkmodel.OptionSymbol, limit int)`

Steps:
- [ ] Write failing test — TestPrintOptionSymbols (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printOptionSymbols — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 8: Split opAnalysis → printAnalysis
Files: Modify: `cmd/options/output.go:406-447`
Interfaces: Produces: `printAnalysis(results []sdkmodel.OptionAnalysis, limit int)`

Steps:
- [ ] Write failing test — TestPrintAnalysis (populated, empty, limit_truncates_volatility_list)
- [ ] Run test — expect FAIL
- [ ] Extract printAnalysis — inner loop over a.VolatilityList with bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

Phase 1 end gate: gofmt + go vet + TIGER_NO_RACE=1 go test ./cmd/options/... should pass

---

## Phase 2 — cmd/futures

### Task 9: Split opExchange → printExchanges
Files: Modify: `cmd/futures/output.go:27-45`
Interfaces: Produces: `printExchanges(exchanges []sdkmodel.Exchange, limit int)`

Steps:
- [ ] Write failing test — TestPrintExchanges with subtests: populated (2 exchanges), empty (nil), limit_truncation (3 exchanges, limit=2)
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/futures/... -run TestPrintExchanges -v` — expect FAIL
- [ ] Extract `printExchanges` from opExchange — new func with empty guard, bare `i >= limit`
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/futures/... -run TestPrintExchanges -v` — expect PASS
- [ ] Gate and commit

### Task 10: Handlers opCurrent/opContract/opContracts/opAllContracts/opContinuous — already call printContracts (already split, no changes needed, no new tests needed)

### Task 11: Split opQuote (futures) → printFutureQuotes
Files: Modify: `cmd/futures/output.go:167-195`
Interfaces: Produces: `printFutureQuotes(quotes []sdkmodel.FutureQuote, limit int)`

Steps:
- [ ] Write failing test — TestPrintFutureQuotes (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFutureQuotes from opQuote — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 12: Split opKline (futures) → printFutureKlines
Files: Modify: `cmd/futures/output.go:199-240`
Interfaces: Produces: `printFutureKlines(klines []sdkmodel.FutureKline, period string, limit int)`
Note: inner loop uses variable `j` (not `i`) — preserve that

Steps:
- [ ] Write failing test — TestPrintFutureKlines with subtests: populated, empty, limit_truncates_inner_loop (3 klines with 3 items each, limit=2 on inner loop)
- [ ] Run test — expect FAIL
- [ ] Extract printFutureKlines — nested loop structure; inner loop uses bare `j >= limit` with variable `j`; handler calls with period and o.Limit
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 13: Split opKlinePage → printKlinePageBars
Files: Modify: `cmd/futures/output.go:244-279`
Interfaces: Produces: `printKlinePageBars(bars []sdkmodel.KlineBar, limit int)`

Steps:
- [ ] Write failing test — TestPrintKlinePageBars (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printKlinePageBars — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 14: Split opDepth (futures) → printFutureDepth
Files: Modify: `cmd/futures/output.go:284-323`
Interfaces: Produces: `printFutureDepth(depth *sdkmodel.FutureDepth, limit int)`

Steps:
- [ ] Write failing test — TestPrintFutureDepth (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFutureDepth — n = max(len(depth.Bids), len(depth.Asks)); inner loop with bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 15: Split opTicks (futures) → printFutureTicks
Files: Modify: `cmd/futures/output.go:326-353`
Interfaces: Produces: `printFutureTicks(ticks []sdkmodel.FutureTradeTick, limit int)`

Steps:
- [ ] Write failing test — TestPrintFutureTicks (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFutureTicks — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 16: Split opTradingTimes → printTradingTimes
Files: Modify: `cmd/futures/output.go:358-390`
Interfaces: Produces: `printTradingTimes(tt *sdkmodel.FutureTradingTimes, limit int)`
Note: receives `*sdkmodel.FutureTradingTimes` not slice; nil guard with "  (no trading times returned)"

Steps:
- [ ] Write failing test — TestPrintTradingTimes with subtests: populated, nil (expect "  (no trading times returned)"), limit
- [ ] Run test — expect FAIL
- [ ] Extract printTradingTimes — `if tt == nil` guard with empty string, bare `i >= limit`; handler passes o.Limit
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 17: Split opHistoryMain → printHistoryMain
Files: Modify: `cmd/futures/output.go:394-422`
Interfaces: Produces: `printHistoryMain(hists []sdkmodel.FutureHistory, limit int)`

Steps:
- [ ] Write failing test — TestPrintHistoryMain (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printHistoryMain — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

Phase 2 end gate: gofmt + go vet + TIGER_NO_RACE=1 go test ./cmd/futures/... should pass

---

## Phase 3 — cmd/corporate

### Task 18: Split opCorporateAction → printCorporateActions
Files: Modify: `cmd/corporate/output.go:38-96`
Interfaces: Produces: `printCorporateActions(rows []sdkmodel.CorporateAction, limit int)`
Note: handler already has `if len(rows) == 0` inline — remove it from handler, move to renderer

Steps:
- [ ] Write failing test — TestPrintCorporateActions with subtests: populated, empty (nil, expect "  (no rows returned)"), limit
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/corporate/... -run TestPrintCorporateActions -v` — expect FAIL
- [ ] Extract printCorporateActions from opCorporateAction — add empty guard with "  (no rows returned)"; remove inline guard from handler; bare `i >= limit`
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/corporate/... -run TestPrintCorporateActions -v` — expect PASS
- [ ] Gate and commit

### Task 19: Split opIPO → printIPOs
Files: Modify: `cmd/corporate/output.go:133-166`
Interfaces: Produces: `printIPOs(rows []sdkmodel.IPO, limit int)`

Steps:
- [ ] Write failing test — TestPrintIPOs (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printIPOs — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 20: Split opSymbolChange → printSymbolChanges
Files: Modify: `cmd/corporate/output.go:169-197`
Interfaces: Produces: `printSymbolChanges(rows []sdkmodel.SymbolChange, limit int)`

Steps:
- [ ] Write failing test — TestPrintSymbolChanges (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printSymbolChanges — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 21: Split opDelisting → printDelistings
Files: Modify: `cmd/corporate/output.go:200-229`
Interfaces: Produces: `printDelistings(rows []sdkmodel.Delisting, limit int)`

Steps:
- [ ] Write failing test — TestPrintDelistings (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printDelistings — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 22: Split opCapitalFlow → printCapitalFlow
Files: Modify: `cmd/corporate/output.go:251-278`
Interfaces: Produces: `printCapitalFlow(flow *sdkmodel.CapitalFlow, limit int)`
Note: receives `*sdkmodel.CapitalFlow`; nil guard with "  (no data returned)"; the bucket count line and header also move to renderer

Steps:
- [ ] Write failing test — TestPrintCapitalFlow with subtests: populated, nil (expect "  (no data returned)"), limit
- [ ] Run test — expect FAIL
- [ ] Extract printCapitalFlow — `if flow == nil` guard with "  (no data returned)"; bucket count line and header move to renderer; bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 23: Split opCapitalDistribution → printCapitalDistribution
Files: Modify: `cmd/corporate/output.go:281-305`
Interfaces: Produces: `printCapitalDistribution(dist *sdkmodel.CapitalDistribution)`
Note: receives `*sdkmodel.CapitalDistribution`; nil guard; no loop so no limit param

Steps:
- [ ] Write failing test — TestPrintCapitalDistribution with subtests: populated, nil (expect "  (no data returned)" — no limit subtest since no loop)
- [ ] Run test — expect FAIL
- [ ] Extract printCapitalDistribution — `if dist == nil` guard; no limit parameter
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 24: Split opWarrantFilter and opWarrantQuote — printWarrants already exists and is already a standalone function. Add tests for it only.
Files: Modify: `cmd/corporate/output_test.go` only
Interfaces: N/A — printWarrants already exists

Steps:
- [ ] Write failing test — TestPrintWarrants (populated, empty, limit) — only test file changes
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/corporate/... -run TestPrintWarrants -v` — expect FAIL
- [ ] No implementation changes needed — printWarrants already exists and is already a standalone function
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/corporate/... -run TestPrintWarrants -v` — expect PASS
- [ ] Gate and commit (only test file changes)

### Task 25: Split opFundSymbols → printFundSymbols
Files: Modify: `cmd/corporate/output.go:377-394`
Interfaces: Produces: `printFundSymbols(syms []sdkmodel.FundSymbol, limit int)`

Steps:
- [ ] Write failing test — TestPrintFundSymbols (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFundSymbols — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 26: Split opFundContracts → printFundContracts
Files: Modify: `cmd/corporate/output.go:396-421`
Interfaces: Produces: `printFundContracts(contracts []sdkmodel.FundContract, limit int)`

Steps:
- [ ] Write failing test — TestPrintFundContracts (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFundContracts — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 27: Split opFundQuote → printFundQuotes
Files: Modify: `cmd/corporate/output.go:423-446`
Interfaces: Produces: `printFundQuotes(quotes []sdkmodel.FundQuote, limit int)`

Steps:
- [ ] Write failing test — TestPrintFundQuotes (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFundQuotes — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 28: Split opFundHistory → printFundHistory
Files: Modify: `cmd/corporate/output.go:448-476`
Interfaces: Produces: `printFundHistory(hists []sdkmodel.FundHistory, limit int)`

Steps:
- [ ] Write failing test — TestPrintFundHistory (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFundHistory — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

Phase 3 end gate: gofmt + go vet + TIGER_NO_RACE=1 go test ./cmd/corporate/... should pass

---

## Phase 4 — cmd/reference

### Task 29: Split opSymbols → printRefSymbols (name avoids clash with options' printOptionSymbols)
Files: Modify: `cmd/reference/output.go:27-49`
Interfaces: Produces: `printRefSymbols(syms []sdkmodel.Symbol, limit int)`

Steps:
- [ ] Write failing test — TestPrintRefSymbols with subtests: populated (2 symbols), empty (nil), limit_truncation (3 symbols, limit=2)
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/reference/... -run TestPrintRefSymbols -v` — expect FAIL
- [ ] Extract `printRefSymbols` from opSymbols — new func with empty guard, bare `i >= limit`; name avoids clash with options' printOptionSymbols
- [ ] Run: `TIGER_NO_RACE=1 go test ./cmd/reference/... -run TestPrintRefSymbols -v` — expect PASS
- [ ] Gate and commit

### Task 30: Split opSymbolNames → printSymbolNames
Files: Modify: `cmd/reference/output.go:52-75`
Interfaces: Produces: `printSymbolNames(names []string, limit int)`

Steps:
- [ ] Write failing test — TestPrintSymbolNames (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printSymbolNames — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 31: Split opStockDetails → printStockDetails
Files: Modify: `cmd/reference/output.go:78-113`
Interfaces: Produces: `printStockDetails(details []sdkmodel.StockDetail, limit int)`

Steps:
- [ ] Write failing test — TestPrintStockDetails (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printStockDetails — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 32: Split opStockIndustry → printStockIndustry
Files: Modify: `cmd/reference/output.go:117-148`
Interfaces: Produces: `printStockIndustry(industries []sdkmodel.StockIndustry, limit int)`

Steps:
- [ ] Write failing test — TestPrintStockIndustry (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printStockIndustry — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 33: Split opStockFundamental → printStockFundamental
Files: Modify: `cmd/reference/output.go:198-221`
Interfaces: Produces: `func printStockFundamental(fund map[string]any) error`
Note: calls rocli.JSON which returns error; renderer signature uses `if len(fund) == 0` guard; handler ignores returned error

Steps:
- [ ] Write failing test — TestPrintStockFundamental with subtests: populated (map with keys), empty (nil map or len==0 — no limit test since no loop)
- [ ] Run test — expect FAIL
- [ ] Extract printStockFundamental — calls rocli.JSON which returns error; signature `func printStockFundamental(fund map[string]any) error`; uses `if len(fund) == 0` guard; handler ignores returned error
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 34: Split opFinancialDaily → printFinancialDaily
Files: Modify: `cmd/reference/output.go:227-260`
Interfaces: Produces: `printFinancialDaily(dailies []sdkmodel.FinancialDaily, limit int)`

Steps:
- [ ] Write failing test — TestPrintFinancialDaily (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFinancialDaily — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 35: Split opFinancialReport → printFinancialReport
Files: Modify: `cmd/reference/output.go:264-304`
Interfaces: Produces: `printFinancialReport(reports []sdkmodel.FinancialReport, limit int)`

Steps:
- [ ] Write failing test — TestPrintFinancialReport (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFinancialReport — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 36: Split opFinancialCurrency → printFinancialCurrencies
Files: Modify: `cmd/reference/output.go:307-333`
Interfaces: Produces: `printFinancialCurrencies(currencies []sdkmodel.FinancialCurrency, limit int)`

Steps:
- [ ] Write failing test — TestPrintFinancialCurrencies (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printFinancialCurrencies — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 37: Split opExchangeRate → printExchangeRates
Files: Modify: `cmd/reference/output.go:337-366`
Interfaces: Produces: `printExchangeRates(rates []sdkmodel.ExchangeRate, limit int)`

Steps:
- [ ] Write failing test — TestPrintExchangeRates (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printExchangeRates — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 38: Split opShortInterest → printShortInterest
Files: Modify: `cmd/reference/output.go:369-396`
Interfaces: Produces: `printShortInterest(interests []sdkmodel.ShortInterest, limit int)`

Steps:
- [ ] Write failing test — TestPrintShortInterest (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printShortInterest — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 39: Split opCalendar → printCalendar
Files: Modify: `cmd/reference/output.go:401-426`
Interfaces: Produces: `printCalendar(calendars []sdkmodel.Calendar, limit int)`

Steps:
- [ ] Write failing test — TestPrintCalendar (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printCalendar — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 40: Split opScanner → printScannerRows + 4 group renderers
Files: Modify: `cmd/reference/output.go:431-491`
Interfaces: Produces: `printScannerRows(result *sdkmodel.MarketScannerResult, limit int)`, plus 4 group renderers each taking `w io.Writer` as first param
Note: printScannerRows receives `*sdkmodel.MarketScannerResult`; nil guard "  (no data returned)"; 4 group renderers write directly to `w io.Writer` (not to package `out`); groups are skipped when empty (continue, not printed)

Steps:
- [ ] Write failing test — TestPrintScannerRows with subtests: populated (result with groups), nil (expect "  (no data returned)"), limit; and TestPrintScannerGroups with subtests: base_group shows data, base_group_empty_skipped shows nothing for empty group
- [ ] Run test — expect FAIL
- [ ] Extract printScannerRows and 4 group renderers — printScannerRows takes `*sdkmodel.MarketScannerResult` and limit; 4 group renderers each take `w io.Writer` as first param and write directly to it; groups skipped when empty (continue, not printed)
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 41: Split opScannerTags → printScannerTags
Files: Modify: `cmd/reference/output.go:494-514`
Interfaces: Produces: `func printScannerTags(w io.Writer, groups []sdkmodel.MarketScannerTagGroup) error`
Note: takes `w io.Writer` because it calls rocli.JSON; empty guard "  (no tags returned)"

Steps:
- [ ] Write failing test — TestPrintScannerTags with subtests: populated (groups with tags), empty (nil or len==0, expect "  (no tags returned)")
- [ ] Run test — expect FAIL
- [ ] Extract printScannerTags — takes `w io.Writer` as first param because it calls rocli.JSON; signature `func printScannerTags(w io.Writer, groups []sdkmodel.MarketScannerTagGroup) error`; uses `if len(groups) == 0` guard with "  (no tags returned)"
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 42: Split opIndustryList → printIndustryList
Files: Modify: `cmd/reference/output.go:517-538`
Interfaces: Produces: `printIndustryList(industries []sdkmodel.Industry, limit int)`

Steps:
- [ ] Write failing test — TestPrintIndustryList (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printIndustryList — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 43: Split opIndustryStocks → printIndustryStocks
Files: Modify: `cmd/reference/output.go:541-568`
Interfaces: Produces: `printIndustryStocks(stocks []sdkmodel.IndustryStock, limit int)`

Steps:
- [ ] Write failing test — TestPrintIndustryStocks (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printIndustryStocks — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 44: Split opTicks (reference) → printTradeTicks
Files: Modify: `cmd/reference/output.go:575-605`
Interfaces: Produces: `printTradeTicks(ticks []sdkmodel.TradeTick, limit int)`

Steps:
- [ ] Write failing test — TestPrintTradeTicks with subtests: populated, empty, limit_truncates_inner_loop (2 ticks with 3 items each, limit=2 on inner loop)
- [ ] Run test — expect FAIL
- [ ] Extract printTradeTicks — nested loop structure; outer loop over ticks, inner loop over tick.Items; inner loop uses bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 45: Split opTimeline (reference) → printRefTimeline
Files: Modify: `cmd/reference/output.go:609-650`
Interfaces: Produces: `printRefTimeline(tls []sdkmodel.Timeline, limit int)`

Steps:
- [ ] Write failing test — TestPrintRefTimeline with subtests: populated (with data in 3 buckets), empty (nil), limit_per_bucket (each bucket's items limited independently)
- [ ] Run test — expect FAIL
- [ ] Extract printRefTimeline — 3 bucket groups looped inline; each bucket's items use bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 46: opDelayed — already calls printBriefs which is already split. No changes. Skip.

### Task 47: Split opKlinePage (reference) → printRefKlinePageBars
Files: Modify: `cmd/reference/output.go:677-714`
Interfaces: Produces: `printRefKlinePageBars(bars []sdkmodel.KlineBar, limit int)`

Steps:
- [ ] Write failing test — TestPrintRefKlinePageBars (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printRefKlinePageBars — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 48: Split opKlineQuota → printKlineQuota
Files: Modify: `cmd/reference/output.go:718-738`
Interfaces: Produces: `printKlineQuota(quota *sdkmodel.KlineQuota, limit int)`
Note: nil guard "  (no quota returned)"; detail loop is inline (not a separate renderer)

Steps:
- [ ] Write failing test — TestPrintKlineQuota with subtests: populated, nil (expect "  (no quota returned)"), limit
- [ ] Run test — expect FAIL
- [ ] Extract printKlineQuota — `if quota == nil` guard with "  (no quota returned)"; detail loop is inline (not a separate renderer); bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 49: Split opTradeMetas → printTradeMetas
Files: Modify: `cmd/reference/output.go:741-765`
Interfaces: Produces: `printTradeMetas(metas []sdkmodel.TradeMeta, limit int)`

Steps:
- [ ] Write failing test — TestPrintTradeMetas (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printTradeMetas — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 50: Split opQuotePermission → printQuotePermissions
Files: Modify: `cmd/reference/output.go:770-795`
Interfaces: Produces: `printQuotePermissions(perms []sdkmodel.QuotePermission, limit int)`
Note: empty guard "  (no permissions returned)"

Steps:
- [ ] Write failing test — TestPrintQuotePermissions with subtests: populated, empty (nil or len==0, expect "  (no permissions returned)"), limit
- [ ] Run test — expect FAIL
- [ ] Extract printQuotePermissions — `if len(perms) == 0` guard with "  (no permissions returned)"; bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

### Task 51: Split opOvernight → printOvernightQuotes
Files: Modify: `cmd/reference/output.go:816-839`
Interfaces: Produces: `printOvernightQuotes(quotes []sdkmodel.OvernightQuote, limit int)`

Steps:
- [ ] Write failing test — TestPrintOvernightQuotes (populated, empty, limit)
- [ ] Run test — expect FAIL
- [ ] Extract printOvernightQuotes — empty guard, bare `i >= limit`
- [ ] Run test — expect PASS
- [ ] Gate and commit

Phase 4 end gate: gofmt + go vet + TIGER_NO_RACE=1 go test ./cmd/reference/... should pass

---

## Final gate

After all 4 phases:
- [ ] gofmt ./...
- [ ] go vet ./...
- [ ] TIGER_NO_RACE=1 go test ./cmd/options/... ./cmd/futures/... ./cmd/corporate/... ./cmd/reference/...
- [ ] Measure and update README coverage table
- [ ] Archive this plan and the design spec
