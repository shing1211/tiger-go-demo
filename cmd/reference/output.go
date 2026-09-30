package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"

	"github.com/shing1211/tiger-go-demo/internal/rocli"
)

// This file holds the per-endpoint request + print logic for cmd/reference.
//
// Each handler is the same shape: check the flags, honour the context
// deadline, call exactly one SDK method, print. The SDK error is always wrapped
// with the endpoint name so a failure says which call produced it.

var out io.Writer = os.Stdout

// ---- symbol reference ----

// printRefSymbols renders the full tradable symbol list.
func printRefSymbols(syms []string, limit int) {
	if len(syms) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for i, s := range syms {
		if i >= limit {
			rocli.Truncate(out, i, len(syms), limit)
			break
		}
		fmt.Fprintf(out, "  %s\n", s)
	}
}

// opSymbols returns the full tradable symbol list for a market.
func opSymbols(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	syms, err := qc.GetSymbols(sdkmodel.SymbolsRequest{
		Market:     o.Market,
		SecType:    o.SecType,
		IncludeOtc: o.includeOTC,
		Lang:       o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get symbols (market=%s sec_type=%s): %w", o.Market, o.SecType, err)
	}
	rocli.Section(out, "symbols (market=%s, sec_type=%s, %d total)", o.Market, o.SecType, len(syms))
	printRefSymbols(syms, o.Limit)
	return nil
}

// printSymbolNames renders the symbol list with display names.
func printSymbolNames(names []sdkmodel.SymbolName, limit int) {
	if len(names) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-16s %-8s %-40s\n", "SYMBOL", "MARKET", "NAME")
	for i, n := range names {
		if i >= limit {
			rocli.Truncate(out, i, len(names), limit)
			break
		}
		fmt.Fprintf(out, "  %-16s %-8s %-40s\n", rocli.Dash(n.Symbol), rocli.Dash(n.Market), rocli.Dash(n.Name))
	}
}

// opSymbolNames returns the same list with display names.
func opSymbolNames(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	names, err := qc.GetSymbolNames(sdkmodel.SymbolsRequest{
		Market:     o.Market,
		SecType:    o.SecType,
		IncludeOtc: o.includeOTC,
		Lang:       o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get symbol names (market=%s sec_type=%s): %w", o.Market, o.SecType, err)
	}
	rocli.Section(out, "symbol names (market=%s, %d total)", o.Market, len(names))
	printSymbolNames(names, o.Limit)
	return nil
}

// printStockDetails renders per-symbol descriptive and valuation fields.
func printStockDetails(details []sdkmodel.StockDetail, limit int) {
	if len(details) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for i, d := range details {
		if i >= limit {
			rocli.Truncate(out, i, len(details), limit)
			break
		}
		name := d.NameEN
		if name == "" {
			name = d.NameCN
		}
		fmt.Fprintf(out, "  %-12s %-34s %-8s %-8s %-8s %-20s\n",
			rocli.Dash(d.Symbol), rocli.Dash(name), rocli.Dash(d.Market),
			rocli.Dash(d.Currency), rocli.Dash(d.SecType), rocli.Dash(d.Industry))
		fmt.Fprintf(out, "    sector=%-24s exchange=%-10s listed=%s\n",
			rocli.Dash(d.Sector), rocli.Dash(d.Exchange), rocli.MSFmt(d.ListingDate))
		fmt.Fprintf(out, "    market_cap=%.2f float_cap=%.2f shares=%.0f eps_ttm=%.4f pe_ttm=%.4f\n",
			d.MarketCap, d.CirculationCap, d.TotalShares, d.EpsTtm, d.PeRatioTtm)
	}
}

// opStockDetails returns per-symbol descriptive and valuation fields.
func opStockDetails(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	details, err := qc.GetStockDetails(sdkmodel.StockDetailsRequest{
		Symbols: symbols,
		SecType: o.SecType,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get stock details (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "stock details")
	printStockDetails(details, o.Limit)
	return nil
}

// printStockIndustry renders the GICS-style classification ladder.
func printStockIndustry(industries []sdkmodel.StockIndustry, limit int) {
	if len(industries) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-8s %-24s %-24s %-28s\n", "SYMBOL", "LEVEL", "SECTOR", "GROUP", "INDUSTRY")
	for i, r := range industries {
		if i >= limit {
			rocli.Truncate(out, i, len(industries), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-8s %-24s %-24s %-28s\n",
			rocli.Dash(r.Symbol), rocli.Dash(r.Level), rocli.Dash(r.GSector), rocli.Dash(r.GGroup), rocli.Dash(r.GInd))
		if r.GSubInd != "" {
			fmt.Fprintf(out, "    sub-industry=%s\n", r.GSubInd)
		}
	}
}

// opStockIndustry returns the GICS-style classification ladder. The endpoint
// takes a single symbol, so only the first of -symbols is used.
func opStockIndustry(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbol := rocli.FirstSymbol(o.symbols)
	if symbol == "" {
		return errSymbol
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := qc.GetStockIndustry(sdkmodel.StockIndustryRequest{
		Symbol:  symbol,
		Market:  o.Market,
		SecType: o.SecType,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get stock industry (%s): %w", symbol, err)
	}
	rocli.Section(out, "stock industry")
	printStockIndustry(rows, o.Limit)
	return nil
}

// opStockBroker returns the broker-by-broker distribution of a stock's trades.
func opStockBroker(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbol := rocli.FirstSymbol(o.symbols)
	if symbol == "" {
		return errSymbol
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	b, err := qc.GetStockBroker(sdkmodel.StockBrokerRequest{
		Symbol:  symbol,
		Limit:   o.Limit,
		SecType: o.SecType,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get stock broker (%s): %w", symbol, err)
	}
	rocli.Section(out, "broker distribution (%s)", symbol)
	if b == nil {
		fmt.Fprintln(out, "  (no data returned)")
		return nil
	}
	printBrokerSide("bid", b.LevelBidList)
	printBrokerSide("ask", b.LevelAskList)
	return nil
}

func printBrokerSide(label string, levels []sdkmodel.StockBrokerItem) {
	if len(levels) == 0 {
		fmt.Fprintf(out, "  %s: (none)\n", label)
		return
	}
	fmt.Fprintf(out, "  %s side:\n", label)
	for _, lv := range levels {
		var names []string
		for _, br := range lv.Brokers {
			names = append(names, rocli.Dash(br.Name))
		}
		fmt.Fprintf(out, "    level=%-4d price=%-10.4f %s\n", lv.Level, lv.Price, strings.Join(names, ", "))
	}
}

// ---- fundamentals ----

// printStockFundamental renders Tiger's fundamental bundle as JSON.
func printStockFundamental(fund map[string]any) error {
	if len(fund) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return nil
	}
	return rocli.JSON(out, fund)
}

// opStockFundamental returns Tiger's fundamental bundle. The SDK hands back a
// raw map because the field set is server-defined, so it is dumped as JSON
// rather than forced into invented columns.
func opStockFundamental(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	fund, err := qc.GetStockFundamental(sdkmodel.StockFundamentalRequest{
		Symbols: symbols,
		Market:  o.Market,
		SecType: o.SecType,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get stock fundamental (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "stock fundamental (%s)", strings.Join(symbols, ","))
	printStockFundamental(fund)
	return nil
}

// printFinancialDaily renders a daily time series of named fundamental fields.
func printFinancialDaily(dailies []sdkmodel.FinancialDailyItem, limit int) {
	if len(dailies) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-20s %-14s %18s\n", "SYMBOL", "FIELD", "DATE", "VALUE")
	for i, it := range dailies {
		if i >= limit {
			rocli.Truncate(out, i, len(dailies), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-20s %-14s %18.4f\n",
			it.Symbol, it.Field, rocli.MSFmt(it.Date), it.Value)
	}
}

// opFinancialDaily returns a daily time series of named fundamental fields.
//
// -fields is required by the API, so an empty value is refused here rather than
// sent as an empty list and reported as an opaque server error.
func opFinancialDaily(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	fields := rocli.ListRaw(o.fields)
	if len(fields) == 0 {
		return rocli.RequiredFlag("-fields", "revenue,eps")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	items, err := qc.GetFinancialDaily(sdkmodel.FinancialDailyRequest{
		Symbols:   symbols,
		Market:    o.Market,
		Fields:    fields,
		BeginDate: o.beginDate,
		EndDate:   o.endDate,
	})
	if err != nil {
		return fmt.Errorf("get financial daily (%s, fields=%s): %w", strings.Join(symbols, ","), strings.Join(fields, ","), err)
	}
	rocli.Section(out, "financial daily")
	printFinancialDaily(items, o.Limit)
	return nil
}

// printFinancialReport renders period-report figures (annual, quarterly, ...).
func printFinancialReport(reports []sdkmodel.FinancialReportItem, limit int) {
	if len(reports) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-20s %-8s %-12s %-12s %18s\n", "SYMBOL", "FIELD", "CCY", "FILED", "PERIOD_END", "VALUE")
	for i, it := range reports {
		if i >= limit {
			rocli.Truncate(out, i, len(reports), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-20s %-8s %-12s %-12s %18s\n",
			it.Symbol, it.Field, rocli.Dash(it.Currency), rocli.Dash(it.FilingDate), rocli.Dash(it.PeriodEndDate), rocli.Dash(it.Value))
	}
}

// opFinancialReport returns period-report figures (annual, quarterly, ...).
// Its date bounds are epoch milliseconds, so the YYYY-MM-DD flags are converted.
func opFinancialReport(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	fields := rocli.ListRaw(o.fields)
	if len(fields) == 0 {
		return rocli.RequiredFlag("-fields", "revenue,net_income")
	}
	req := sdkmodel.FinancialReportRequest{
		Symbols:    symbols,
		Market:     o.Market,
		Fields:     fields,
		PeriodType: o.periodType,
	}
	if d := rocli.DateMillis(o.beginDate); d != nil {
		req.BeginDate = d
	}
	if d := rocli.DateMillis(o.endDate); d != nil {
		req.EndDate = d
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	items, err := qc.GetFinancialReport(req)
	if err != nil {
		return fmt.Errorf("get financial report (%s, fields=%s, period_type=%s): %w",
			strings.Join(symbols, ","), strings.Join(fields, ","), rocli.DashOr(o.periodType, "unset"), err)
	}
	rocli.Section(out, "financial report")
	printFinancialReport(items, o.Limit)
	return nil
}

// printFinancialCurrencies renders the trading currency of each symbol.
func printFinancialCurrencies(currencies []sdkmodel.FinancialCurrency, limit int) {
	if len(currencies) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-8s %-8s\n", "SYMBOL", "MARKET", "CCY")
	for i, c := range currencies {
		if i >= limit {
			rocli.Truncate(out, i, len(currencies), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-8s %-8s\n", rocli.Dash(c.Symbol), rocli.Dash(c.Market), rocli.Dash(c.Currency))
	}
}

// opFinancialCurrency reports the trading currency of each symbol.
func opFinancialCurrency(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	ccies, err := qc.GetFinancialCurrency(sdkmodel.FinancialCurrencyRequest{
		Symbols: symbols,
		Market:  o.Market,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get financial currency (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "financial currency")
	printFinancialCurrencies(ccies, o.Limit)
	return nil
}

// printExchangeRates renders FX rates.
func printExchangeRates(rates []sdkmodel.ExchangeRate, limit int) {
	if len(rates) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-8s %-12s %-8s %18s\n", "CCY", "BASE", "DATE", "RATE")
	for i, r := range rates {
		if i >= limit {
			rocli.Truncate(out, i, len(rates), limit)
			break
		}
		fmt.Fprintf(out, "  %-8s %-12s %-8s %18.6f\n",
			rocli.Dash(r.Currency), rocli.Dash(r.BaseCurrency), rocli.Dash(r.Date), r.Rate)
	}
}

// opExchangeRate returns FX rates. This endpoint wants YYYYMMDD dates, not the
// YYYY-MM-DD used by the calendar, so the dashes are stripped.
func opExchangeRate(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	currencies := rocli.List(o.currencies)
	if len(currencies) == 0 {
		return rocli.RequiredFlag("-currencies", "USD,HKD")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rates, err := qc.GetFinancialExchangeRate(sdkmodel.FinancialExchangeRateRequest{
		CurrencyList: currencies,
		BeginDate:    compactDate(o.beginDate),
		EndDate:      compactDate(o.endDate),
		Timezone:     "US/Eastern",
		Lang:         o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get exchange rate (%s): %w", strings.Join(currencies, ","), err)
	}
	rocli.Section(out, "exchange rates")
	printExchangeRates(rates, o.Limit)
	return nil
}

// printShortInterest renders short-interest statistics.
func printShortInterest(interests []sdkmodel.ShortInterest, limit int) {
	if len(interests) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-12s %14s %14s %10s %10s %10s\n", "SYMBOL", "SETTLED", "SHORT", "PREV", "%FLOAT", "DAYS", "CHG%")
	for i, s := range interests {
		if i >= limit {
			rocli.Truncate(out, i, len(interests), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-12s %14.0f %14.0f %10.2f %10.2f %9.2f%%\n",
			s.Symbol, rocli.Dash(s.SettlementDate), s.ShortInterest, s.ShortInterestPrevious,
			s.PercentOfFloat, s.DaysToCover, s.PercentChange)
	}
}

// opShortInterest returns short-interest statistics.
func opShortInterest(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := qc.GetShortInterest(sdkmodel.ShortInterestRequest{
		Symbols: symbols,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get short interest (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "short interest")
	printShortInterest(rows, o.Limit)
	return nil
}

// ---- calendars and screens ----

// printCalendar renders which days a market trades.
func printCalendar(calendars []sdkmodel.TradingCalendarItem, limit int) {
	if len(calendars) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-8s %-12s %-10s %-16s\n", "MARKET", "DATE", "TRADING", "SESSION")
	for i, d := range calendars {
		if i >= limit {
			rocli.Truncate(out, i, len(calendars), limit)
			break
		}
		fmt.Fprintf(out, "  %-8s %-12s %-10v %-16s\n",
			rocli.Dash(d.Market), rocli.Dash(d.Date), d.IsTrading, rocli.Dash(d.SessionType))
	}
}

// opCalendar reports which days a market trades.
func opCalendar(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	days, err := qc.GetTradingCalendar(sdkmodel.TradingCalendarRequest{
		Market:    o.Market,
		BeginDate: o.beginDate,
		EndDate:   o.endDate,
		Lang:      o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get trading calendar (market=%s, %s..%s): %w",
			o.Market, rocli.DashOr(o.beginDate, "open"), rocli.DashOr(o.endDate, "open"), err)
	}
	rocli.Section(out, "trading calendar (market=%s)", o.Market)
	printCalendar(days, o.Limit)
	return nil
}

// printScannerRows renders one page of screener matches.
//
// The page line comes before the rows and not inside them: a screener result
// spans as many pages as the caller asked for, so without it a reader cannot
// tell which slice of a multi-page result they are looking at. The cursor is on
// the same line because it is the handle for the next page, and it is dashed
// rather than blank when the server sends none.
func printScannerRows(res *sdkmodel.ScannerResult, limit int) {
	if res == nil {
		fmt.Fprintln(out, "  (no data returned)")
		return
	}
	fmt.Fprintf(out, "  page %d/%d, %d match(es), page_size=%d cursor=%s\n",
		res.Page, res.TotalPage, res.TotalCount, res.PageSize, rocli.Dash(res.CursorID))
	for i, it := range res.Items {
		if i >= limit {
			rocli.Truncate(out, i, len(res.Items), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-8s\n", rocli.Dash(it.Symbol), rocli.Dash(it.Market))
		for _, group := range []struct {
			name string
			rows []sdkmodel.ScannerDataRow
		}{
			{"base", it.BaseDataList},
			{"accumulate", it.AccumulateDataList},
			{"financial", it.FinancialDataList},
			{"multi_tag", it.MultiTagDataList},
		} {
			printScannerGroup(out, group.name, group.rows)
		}
	}
}

// printScannerGroup renders one named block of a match's screener data.
//
// All four groups a match can carry hold the same row type, so one renderer
// takes the group name rather than four near-identical ones existing. An absent
// group prints nothing: the screener omits a group the query did not ask for,
// and an empty heading would claim the server sent one and found nothing in it.
func printScannerGroup(w io.Writer, name string, rows []sdkmodel.ScannerDataRow) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "    [%s]\n", name)
	for _, row := range rows {
		fmt.Fprintf(w, "      %-24s %-24s %s\n", rocli.Dash(row.Name), rocli.Dash(row.Value), fmt.Sprintf("%.4f", row.Data))
	}
}

// opScanner runs the stock screener. Its filters are open-ended server-side
// structures, so they arrive as JSON on the command line and are validated
// before the request goes out.
func opScanner(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	req := sdkmodel.MarketScannerRequest{
		Market:   o.Market,
		Page:     o.Page,
		PageSize: o.PageSize,
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = o.Limit
	}
	if err := rocli.JSONFlag("base-filters", o.baseFilters, &req.BaseFilterList); err != nil {
		return err
	}
	if err := rocli.JSONFlag("sort", o.sortJSON, &req.SortFieldData); err != nil {
		return err
	}
	if tags := rocli.ListRaw(o.multiTags); len(tags) > 0 {
		req.MultiTagsFields = tags
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	res, err := qc.MarketScanner(req)
	if err != nil {
		return fmt.Errorf("market scanner (market=%s page=%d page_size=%d): %w", o.Market, req.Page, req.PageSize, err)
	}
	rocli.Section(out, "market scanner (market=%s)", o.Market)
	printScannerRows(res, o.Limit)
	return nil
}

// printScannerTags renders the scanner tag groups.
func printScannerTags(w io.Writer, groups []sdkmodel.MarketScannerTagGroup) error {
	if len(groups) == 0 {
		fmt.Fprintln(w, "  (no tags returned)")
		return nil
	}
	return rocli.JSON(w, groups)
}

// opScannerTags lists the multi-tag fields the screener understands.
func opScannerTags(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	groups, err := qc.GetMarketScannerTags(sdkmodel.MarketScannerTagsRequest{
		Market:          o.Market,
		MultiTagsFields: rocli.ListRaw(o.multiTags),
		Lang:            o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get market scanner tags (market=%s): %w", o.Market, err)
	}
	rocli.Section(out, "market scanner tags (market=%s)", o.Market)
	return printScannerTags(out, groups)
}

// printIndustryList renders Tiger's industry classification table.
//
// The empty case skips the header as well as the rows: a header with nothing
// under it reads like a query that matched nothing, which is a different claim
// from the server having sent no classification table at all.
func printIndustryList(industries []sdkmodel.IndustryItem, limit int) {
	if len(industries) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-8s %-40s\n", "ID", "LEVEL", "NAME")
	for i, it := range industries {
		if i >= limit {
			rocli.Truncate(out, i, len(industries), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-8s %-40s\n", rocli.Dash(it.ID), rocli.Dash(it.Level), rocli.Dash(it.Name))
	}
}

// opIndustryList lists Tiger's industry classifications.
func opIndustryList(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	items, err := qc.GetIndustryList(sdkmodel.IndustryListRequest{
		IndustryLevel: o.industryLvl,
		Lang:          o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get industry list (level=%s): %w", rocli.DashOr(o.industryLvl, "all"), err)
	}
	rocli.Section(out, "industry list (level=%s)", rocli.DashOr(o.industryLvl, "all"))
	printIndustryList(items, o.Limit)
	return nil
}

// printIndustryStocks renders the constituents of one industry.
func printIndustryStocks(stocks []sdkmodel.IndustryStock, limit int) {
	if len(stocks) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-34s %10s %9s\n", "SYMBOL", "NAME", "CHANGE", "CHG%")
	for i, s := range stocks {
		if i >= limit {
			rocli.Truncate(out, i, len(stocks), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-34s %10.4f %8.2f%%\n",
			rocli.Dash(s.Symbol), rocli.Dash(s.Name), s.Change, s.ChangeRate)
	}
}

// opIndustryStocks lists the constituents of one industry.
func opIndustryStocks(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	id := strings.TrimSpace(o.industryID)
	if id == "" {
		return rocli.RequiredFlag("-industry-id", "1001 (see -op industry-list)")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	stocks, err := qc.GetIndustryStocks(sdkmodel.IndustryStocksRequest{
		IndustryID: id,
		Market:     o.Market,
		Lang:       o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get industry stocks (industry_id=%s market=%s): %w", id, o.Market, err)
	}
	rocli.Section(out, "industry stocks (industry_id=%s)", id)
	printIndustryStocks(stocks, o.Limit)
	return nil
}

// ---- extras ----

// printTradeTicks renders the tick stream of each requested symbol.
//
// The limit is applied per tick, not across the whole response: each entry in
// ticks is one symbol's index range, so a shared counter would let the first
// symbol's ticks use up the budget and leave later symbols with none, which
// reads like those symbols never traded. The tick heading therefore always
// prints, even when every row under it is capped away, so a symbol that did
// trade is still visible.
func printTradeTicks(ticks []sdkmodel.TradeTick, limit int) {
	if len(ticks) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for _, t := range ticks {
		fmt.Fprintf(out, "  %s (%d..%d, %d tick(s))\n", t.Symbol, t.BeginIndex, t.EndIndex, len(t.Items))
		fmt.Fprintf(out, "  %-22s %-8s %10s %12s\n", "TIME", "COND", "PRICE", "VOLUME")
		for i, it := range t.Items {
			if i >= limit {
				rocli.Truncate(out, i, len(t.Items), limit)
				break
			}
			fmt.Fprintf(out, "  %-22s %-8s %10.4f %12d\n",
				rocli.MSFmt(it.Time), rocli.Dash(it.Cond), it.Price, it.Volume)
		}
	}
}

// opTicks returns stock trade ticks. Unlike the option and futures tick
// endpoints this one takes a symbol list plus an index range, which is what
// makes it usable for equities.
func opTicks(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	ticks, err := qc.GetTradeTick(sdkmodel.TradeTickRequest{
		Symbols: symbols,
		Limit:   o.Limit,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get trade ticks (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "trade ticks")
	printTradeTicks(ticks, o.Limit)
	return nil
}

// printRefTimeline renders today's intraday timeline.
//
// The limit applies to each session bucket on its own, so -limit reads as
// "points per session" and a reader who asked for 20 still sees 20 pre-market,
// 20 regular and 20 after-hours points rather than 60 of whichever came first.
// An absent or empty bucket prints nothing here, unlike the historical timeline,
// which names the sessions it has none for: this endpoint returns only the
// sessions it has, so a missing bucket carries no information worth a heading.
func printRefTimeline(tls []sdkmodel.Timeline, limit int) {
	if len(tls) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for _, t := range tls {
		fmt.Fprintf(out, "  %-12s period=%-8s pre_close=%.4f\n", t.Symbol, t.Period, t.PreClose)
		for _, b := range []struct {
			name   string
			bucket *sdkmodel.TimelineBucket
		}{
			{"pre_hours", t.PreHours},
			{"intraday", t.Intraday},
			{"after_hours", t.AfterHours},
		} {
			if b.bucket == nil || len(b.bucket.Items) == 0 {
				continue
			}
			fmt.Fprintf(out, "  [%s] %d point(s)\n", b.name, len(b.bucket.Items))
			for i, it := range b.bucket.Items {
				if i >= limit {
					rocli.Truncate(out, i, len(b.bucket.Items), limit)
					break
				}
				fmt.Fprintf(out, "    %-22s price=%.4f avg=%.4f volume=%d\n",
					rocli.MSFmt(it.Time), it.Price, it.AvgPrice, it.Volume)
			}
		}
	}
}

// opTimeline returns the intraday timeline via the v3 request form, which also
// covers crypto when -sec-type CC is used.
func opTimeline(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	tls, err := qc.GetTimelineByReq(sdkmodel.TimelineRequest{
		Symbols: symbols,
		SecType: o.SecType,
	})
	if err != nil {
		return fmt.Errorf("get timeline (sec_type=%s): %w", o.SecType, err)
	}
	rocli.Section(out, "intraday timeline")
	printRefTimeline(tls, o.Limit)
	return nil
}

// opDelayed returns delayed stock quotes (wire: quote_delay), which is the
// endpoint entitled accounts use instead of GetRealTimeQuote.
func opDelayed(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	briefs, err := qc.GetDelayedQuote(sdkmodel.StockDelayBriefsRequest{
		Symbols: symbols,
		SecType: o.SecType,
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get delayed quote (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "delayed quotes")
	printBriefs(briefs, o.Limit)
	return nil
}

// opKlinePage walks the paged k-line endpoint and returns a flat bar list,
// which is what you want when a series is longer than one page.
func opKlinePage(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbol := rocli.FirstSymbol(o.symbols)
	if symbol == "" {
		return errSymbol
	}
	pageSize := o.PageSize
	if pageSize <= 0 {
		pageSize = o.Limit
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	bars, err := qc.GetKlineByPage(sdkmodel.KlineByPageRequest{
		Symbol:    symbol,
		SecType:   o.SecType,
		Period:    o.period,
		BeginTime: o.Begin,
		EndTime:   o.End,
		TotalSize: o.totalSize,
		PageSize:  pageSize,
		Right:     "forward",
		Lang:      o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get kline by page (%s, period=%s, page_size=%d): %w", symbol, o.period, pageSize, err)
	}
	rocli.Section(out, "k-lines by page (%s, period=%s)", symbol, o.period)
	fmt.Fprintf(out, "  %-22s %10s %10s %10s %10s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME")
	for i, b := range bars {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(bars), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d\n",
			rocli.MSFmt(b.Time), b.Open, b.High, b.Low, b.Close, b.Volume)
	}
	return nil
}

// opKlineQuota reports how much k-line quota the account has left, which is the
// first thing to check when a bar query starts failing.
func opKlineQuota(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	quotas, err := qc.GetKlineQuota(sdkmodel.KlineQuotaRequest{WithDetails: true, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get kline quota: %w", err)
	}
	rocli.Section(out, "k-line quota")
	if len(quotas) == 0 {
		fmt.Fprintln(out, "  (no quota returned)")
		return nil
	}
	for _, q := range quotas {
		fmt.Fprintf(out, "  %-20s used=%-8d quota=%-8d detail=%d\n", rocli.Dash(q.Method), q.Used, q.Quota, len(q.Detail))
		for _, d := range q.Detail {
			fmt.Fprintf(out, "    %v\n", d)
		}
	}
	return nil
}

// opTradeMetas returns trading metadata (lot size, tick size, shortable flags).
func opTradeMetas(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	metas, err := qc.GetTradeMetas(sdkmodel.TradeMetasRequest{Symbols: symbols, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get trade metas (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "trade metadata")
	fmt.Fprintf(out, "  %-12s %8s %10s %12s %-10s %-12s\n", "SYMBOL", "LOT", "MIN_TICK", "SPREAD", "SHORTABLE", "MARGINABLE")
	for i, m := range metas {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(metas), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %8d %10.4f %12.4f %-10s %-12s\n",
			rocli.Dash(m.Symbol), m.LotSize, m.MinTick, m.SpreadScale,
			rocli.Dash(m.ShortableFlag), rocli.Dash(m.MarginableFlag))
	}
	return nil
}

// opQuotePermission reports which market-data permissions the account holds and
// when they expire. It is a read; GrabQuotePermission, which CLAIMS a
// permission, is deliberately not wired up here.
func opQuotePermission(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	perms, err := qc.GetQuotePermission(sdkmodel.QuotePermissionRequest{
		BeginDate: o.beginDate,
		EndDate:   o.endDate,
		Lang:      o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get quote permission (%s..%s): %w", rocli.DashOr(o.beginDate, "open"), rocli.DashOr(o.endDate, "open"), err)
	}
	rocli.Section(out, "quote permissions")
	if len(perms) == 0 {
		fmt.Fprintln(out, "  (no permissions returned)")
		return nil
	}
	for i, p := range perms {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(perms), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-30s expires_at=%s\n", rocli.Dash(p.Name), rocli.MSFmt(p.ExpireAt))
	}
	return nil
}

// opTradeRank returns the market's movers board.
//
// It is split from printTradeRank so the rendering can be reached without a live
// account: qc is a concrete *sdkquote.QuoteClient with no interface and no
// injectable transport, so anything holding one is untestable offline. The call
// stays here; everything that formats a row moves below.
func opTradeRank(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	ranks, err := qc.GetTradeRank(sdkmodel.TradeRankRequest{Market: o.Market, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get trade rank (market=%s): %w", o.Market, err)
	}
	printTradeRank(out, o.Market, ranks, o.Limit)
	return nil
}

// opOvernight returns overnight-session quotes.
func opOvernight(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	quotes, err := qc.GetQuoteOvernight(sdkmodel.QuoteOvernightRequest{Symbols: symbols, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get overnight quotes (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "overnight quotes")
	fmt.Fprintf(out, "  %-12s %10s %10s %10s %12s %-20s\n", "SYMBOL", "LAST", "BID", "ASK", "VOLUME", "TIME")
	for i, q := range quotes {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(quotes), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %10.4f %10.4f %10.4f %12d %-20s\n",
			rocli.Dash(q.Symbol), q.LatestPrice, q.BidPrice, q.AskPrice, q.Volume, rocli.MSFmt(q.Timestamp))
	}
	return nil
}

// opTimelineHistory returns intraday timeline data for a past date.
func opTimelineHistory(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	tls, err := qc.GetTimelineHistory(sdkmodel.TimelineHistoryRequest{
		Symbols: symbols,
		Date:    o.beginDate,
		Right:   "forward",
		Lang:    o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get timeline history (%s, date=%s): %w", strings.Join(symbols, ","), rocli.DashOr(o.beginDate, "today"), err)
	}
	rocli.Section(out, "historical timeline")
	printTimelineHistoryRows(out, tls, o.Limit)
	return nil
}

// ---- shared print helpers ----

// printTradeRank renders the movers board.
//
// An empty result says so rather than printing a bare header, because a table
// with column headings and no rows reads like a query that returned nothing for a
// reason the reader has to guess at.
func printTradeRank(w io.Writer, market string, ranks []sdkmodel.TradeRankItem, limit int) {
	rocli.Section(w, "trade rank (market=%s)", market)
	if len(ranks) == 0 {
		fmt.Fprintln(w, "  (no rows returned)")
		return
	}
	fmt.Fprintf(w, "  %-12s %-28s %10s %9s %12s %16s\n", "SYMBOL", "NAME", "LAST", "CHG%", "VOLUME", "AMOUNT")
	for i, r := range ranks {
		if i >= limit {
			rocli.Truncate(w, i, len(ranks), limit)
			break
		}
		fmt.Fprintf(w, "  %-12s %-28s %10.4f %8.2f%% %12d %16.2f\n",
			rocli.Dash(r.Symbol), rocli.Dash(r.Name), r.LatestPr, r.ChangeRate, r.Volume, r.Amount)
	}
}

// printTimelineHistory renders intraday and extended-session timelines.
//
// The three session buckets are named even when one is absent, and the two ways a
// bucket can be unsatisfying are kept distinct: a nil bucket means the server sent
// no such block, while a bucket with no items means the block arrived and was
// empty. Those are different facts, so neither silently disappears.
func printTimelineHistory(w io.Writer, tls []sdkmodel.Timeline) {
	rocli.Section(w, "historical timeline")
	if len(tls) == 0 {
		fmt.Fprintln(w, "  (no rows returned)")
		return
	}
	printTimelineHistoryRows(w, tls, defaultTimelinePointLimit)
}

// defaultTimelinePointLimit caps points per session bucket. A timeline is a dense
// series, and a 2000-point bucket would bury every other line in the output.
const defaultTimelinePointLimit = 20

// printTimelineHistoryRows is the row loop, separated so opTimelineHistory can
// apply the caller's -limit while the printer keeps a sensible default. Both call
// the same rendering, so there is one format and not two.
func printTimelineHistoryRows(w io.Writer, tls []sdkmodel.Timeline, limit int) {
	for _, t := range tls {
		fmt.Fprintf(w, "  %-12s period=%-8s pre_close=%.4f\n", t.Symbol, t.Period, t.PreClose)
		for _, b := range []struct {
			name   string
			bucket *sdkmodel.TimelineBucket
		}{
			{"pre_hours", t.PreHours},
			{"intraday", t.Intraday},
			{"after_hours", t.AfterHours},
		} {
			if b.bucket == nil || len(b.bucket.Items) == 0 {
				fmt.Fprintf(w, "  [%s] no data\n", b.name)
				continue
			}
			fmt.Fprintf(w, "  [%s] %d point(s)\n", b.name, len(b.bucket.Items))
			for i, it := range b.bucket.Items {
				if i >= limit {
					rocli.Truncate(w, i, len(b.bucket.Items), limit)
					break
				}
				fmt.Fprintf(w, "    %-22s price=%.4f avg=%.4f volume=%d\n",
					rocli.MSFmt(it.Time), it.Price, it.AvgPrice, it.Volume)
			}
		}
	}
}

func printBriefs(briefs []sdkmodel.Brief, limit int) {
	if len(briefs) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %10s %10s %9s %12s %-20s\n", "SYMBOL", "LAST", "CHANGE", "CHG%", "VOLUME", "TIME")
	for i, b := range briefs {
		if i >= limit {
			rocli.Truncate(out, i, len(briefs), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %10.4f %10.4f %8.2f%% %12d %-20s\n",
			b.Symbol, b.LatestPrice, b.Change, b.ChangeRate, b.Volume, rocli.MSFmt(b.LatestTime))
	}
}

// ---- helpers ----

var (
	errSymbols = rocli.RequiredFlag("-symbols", "AAPL,MSFT")
	errSymbol  = rocli.RequiredFlag("-symbols", "AAPL")
)

// compactDate turns YYYY-MM-DD into the YYYYMMDD the exchange-rate endpoint
// expects. An empty input stays empty, which means "server default".
func compactDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(s, "-", "")
}

// dateToMillis converts a YYYY-MM-DD flag into the *int64 epoch-millisecond
// bound the financial-report endpoint uses. A blank or unparseable date yields
// nil, i.e. no bound, rather than a bogus timestamp.
