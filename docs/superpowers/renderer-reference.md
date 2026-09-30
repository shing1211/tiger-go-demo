# Renderer reference — verified signatures

Every `print*` renderer in the four CLI packages, with the SDK type it actually
takes. **Verify against this file or `cmd/<pkg>/output.go` before writing a task
brief.** Do not take type names from a design spec or plan: several were wrong,
and each wrong guess cost a rework cycle.

## Why this file exists

The 49-renderer split ran as 51 planned tasks across four phases. Briefs written
from the plan named eight SDK types that do not exist, and every implementer had
to stop, read the real code, and correct them:

| Brief said | Actually is |
|---|---|
| `sdkmodel.OptionBrief` | `sdkmodel.Brief` |
| `sdkmodel.OptionDepth` | `sdkmodel.Depth` |
| `sdkmodel.OptionTradeTick` | `sdkmodel.TradeTick` |
| `sdkmodel.MarketScannerResult` | `sdkmodel.ScannerResult` |
| `sdkmodel.Industry` | `sdkmodel.IndustryItem` |
| `sdkmodel.KlineBar` | `sdkmodel.KlineItem` |
| `sdkmodel.OvernightQuote` | `sdkmodel.QuoteOvernight` |
| `[]string` for symbol names | `[]sdkmodel.SymbolName` (options and futures differ) |

None of those were implementer errors. **The code is authoritative; a brief is not.**

## The convention

- A renderer takes plain data only — a slice, a pointer, an int, a string. It
  never takes a client and never takes the `options` struct.
- `limit int` is the last parameter on any renderer that caps rows.
- Renderers write to the package-level `out io.Writer`, except the ones that
  take an explicit `w io.Writer` because they are shared or return an error.
- Every renderer has an empty-result branch. Default string: `  (no rows returned)`
  (two leading spaces). Exists: `  (no data returned)`, `  (no tags returned)`,
  `  (no quota returned)`, `  (no permissions returned)`, `  (no trading times returned)`,
  `  (no contracts returned)`.
- The limit guard is the bare form `if i >= limit` (or `j` in the one nested
  inner loop that uses it). There are no `Limit > 0 &&` guards left anywhere.
- **Every limit guard keeps its `rocli.Truncate` call inside it.** Dropping that
  call is silent: the guard still stops output early, so a test that only checks
  rows-past-the-limit-are-absent still passes, but the
  `... N more row(s) not shown` line vanishes. This happened to all nine futures
  renderers at once. `truncation_guard_test.go` in each package now fails if any
  guard loses its call.

## cmd/options

```go
printExpiration(exps []sdkmodel.OptionExpiration, limit int)
printChains(chains []sdkmodel.OptionChain, limit int, greeks bool)
printGreeks(side string, leg sdkmodel.OptionLeg)                    // per-leg, no empty branch
printOptionBriefs(briefs []sdkmodel.Brief, limit int)
printKlines(klines []sdkmodel.Kline, period string, limit int)
printKlinesPlain(klines []sdkmodel.Kline, period string, limit int)
printDepth(depths []sdkmodel.Depth, limit int)
printTicks(ticks []sdkmodel.TradeTick, limit int)
printOptionTimeline(tls []sdkmodel.Timeline, limit int)
printOptionSymbols(syms []sdkmodel.OptionSymbol, limit int)
printAnalysis(results []sdkmodel.OptionAnalysis, limit int)
```

`printOptionBriefs` and `printBriefs` in reference take the *same* `sdkmodel.Brief`
but are different functions in different packages. The `Option` infixes elsewhere
(`printOptionSymbols`, `printOptionTimeline`) exist only to avoid a name clash with
reference.

## cmd/futures

```go
printExchanges(exchanges []sdkmodel.FutureExchange, limit int)
printContracts(contracts []sdkmodel.FutureContractInfo)           // no limit
printFutureQuotes(quotes []sdkmodel.FutureQuote, limit int)
printFutureKlines(klines []sdkmodel.FutureKline, period string, limit int)
printKlinePageBars(bars []sdkmodel.FutureKlineItem, limit int)
printFutureDepth(depth *sdkmodel.FutureDepth, limit int)          // pointer
printFutureTicks(ticks []sdkmodel.FutureTradeTickItem, limit int)
printTradingTimes(tt *sdkmodel.FutureTradingTime, limit int)       // pointer
printHistoryMain(hists []sdkmodel.FutureMainContractHistory, limit int)
```

`printFutureKlines` limits **both** the outer series loop and the inner bar loop,
and the inner loop's index is `j`. Confirmed intentional: with `-limit N` a paged
response can yield up to N series x N bars.

## cmd/corporate

```go
printCorporateActions(rows []sdkmodel.CorporateAction, limit int)
printIPOs(rows []sdkmodel.CorporateIPO, limit int)
printSymbolChanges(rows []sdkmodel.CorporateSymbolChange, limit int)
printDelistings(rows []sdkmodel.CorporateDelisting, limit int)
printCapitalFlow(flow *sdkmodel.CapitalFlow, limit int)            // pointer
printCapitalDistribution(dist *sdkmodel.CapitalDistribution)      // pointer, no limit
printWarrants(briefs []sdkmodel.WarrantBrief, limit int)
printFundSymbols(syms []string, limit int)                        // plain strings
printFundContracts(contracts []sdkmodel.FundContractInfo, limit int)
printFundQuotes(quotes []sdkmodel.FundQuote, limit int)
printFundHistory(rows []sdkmodel.FundHistoryQuote, limit int)
```

## cmd/reference

```go
printRefSymbols(syms []string, limit int)
printSymbolNames(names []sdkmodel.SymbolName, limit int)
printStockDetails(details []sdkmodel.StockDetail, limit int)
printStockIndustry(industries []sdkmodel.StockIndustry, limit int)
printBrokerSide(label string, levels []sdkmodel.StockBrokerItem)   // no limit
printStockFundamental(fund map[string]any) error                  // returns error
printFinancialDaily(dailies []sdkmodel.FinancialDailyItem, limit int)
printFinancialReport(reports []sdkmodel.FinancialReportItem, limit int)
printFinancialCurrencies(currencies []sdkmodel.FinancialCurrency, limit int)
printExchangeRates(rates []sdkmodel.ExchangeRate, limit int)
printShortInterest(interests []sdkmodel.ShortInterest, limit int)
printCalendar(calendars []sdkmodel.TradingCalendarItem, limit int)
printScannerRows(res *sdkmodel.ScannerResult, limit int)           // pointer
printScannerGroup(w io.Writer, name string, rows []sdkmodel.ScannerDataRow)
printScannerTags(w io.Writer, groups []sdkmodel.MarketScannerTagGroup) error
printIndustryList(industries []sdkmodel.IndustryItem, limit int)
printIndustryStocks(stocks []sdkmodel.IndustryStock, limit int)
printTradeTicks(ticks []sdkmodel.TradeTick, limit int)
printRefTimeline(tls []sdkmodel.Timeline, limit int)
printRefKlinePageBars(bars []sdkmodel.KlineItem, limit int)
printKlineQuota(quotas []sdkmodel.KlineQuota, limit int)           // slice, not pointer
printTradeMetas(metas []sdkmodel.TradeMeta, limit int)
printQuotePermissions(perms []sdkmodel.QuotePermission, limit int)
printOvernightQuotes(quotes []sdkmodel.QuoteOvernight, limit int)
printTradeRank(w io.Writer, market string, ranks []sdkmodel.TradeRankItem, limit int)
printTimelineHistory(w io.Writer, tls []sdkmodel.Timeline)         // no limit
printTimelineHistoryRows(w io.Writer, tls []sdkmodel.Timeline, limit int)
printBriefs(briefs []sdkmodel.Brief, limit int)
```

`printScannerRows` has **one** group renderer, not four. All four scanner groups
(`base`, `accumulate`, `financial`, `multi_tag`) share the row type
`sdkmodel.ScannerDataRow` and one format string, so four copies would be
duplication; the group names stay in the dispatch loop.

`printKlineQuota` takes a **slice**. The SDK returns `[]KlineQuota`, so a pointer
signature would have made the nil guard unreachable from the handler. Its detail
loop honours `limit` — confirmed intentional; the summary line still reports the
true detail count.

## Documented behaviour decisions

- **`-limit 0` means "no rows" everywhere.** `printChains` was the last renderer
  using the guarded `o.Limit > 0 && i >= o.Limit` form, where 0 meant "print
  everything". Converting it to the bare form changed `-limit 0` for chain users.
  The whole CLI is now uniform; the change is deliberate, not accidental.
- **`printKlineQuota` detail rows honour `limit`** even though the base code left
  them uncapped. Confirmed: keep.
- **futures klines limit both loops**, matching pre-existing behaviour. Confirmed: keep.
