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

// This file holds the per-endpoint request + print logic for cmd/corporate.
//
// Each handler is the same shape: check the flags, honour the context
// deadline, call exactly one SDK method, print. The SDK error is always wrapped
// with the endpoint name so a failure says which call produced it.

var out io.Writer = os.Stdout

// actionType selects which of the four near-identical corporate-action
// endpoints to call. The SDK exposes them separately rather than through one
// filtered query, so the choice is a parameter here instead of four handlers.
type actionType string

const (
	actionDividend actionType = "dividend"
	actionSplit    actionType = "split"
	actionEarnings actionType = "earnings"
	actionAny      actionType = "actions"
)

// opCorporateAction dispatches to the dividend / split / earnings / combined
// corporate-action endpoint. All four take the same request shape.
func opCorporateAction(ctx context.Context, qc *sdkquote.QuoteClient, o options, kind actionType) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	req, err := actionRequest(symbols, o, kind)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}

	var (
		rows []sdkmodel.CorporateAction
		what string
	)
	switch kind {
	case actionDividend:
		rows, err = qc.GetCorporateDividend(req)
		what = "dividends"
	case actionSplit:
		rows, err = qc.GetCorporateSplit(req)
		what = "splits"
	case actionEarnings:
		rows, err = qc.GetCorporateEarningsCalendar(req)
		what = "earnings calendar"
	case actionAny:
		rows, err = qc.GetCorporateAction(req)
		what = "corporate actions"
	default:
		return fmt.Errorf("unsupported action kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("get %s (%s, market=%s): %w", what, strings.Join(symbols, ","), o.Market, err)
	}

	rocli.Section(out, "%s (market=%s)", what, o.Market)
	printCorporateActions(rows, o.Limit)
	return nil
}

func printCorporateActions(rows []sdkmodel.CorporateAction, limit int) {
	if len(rows) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-12s %-14s %-12s %-12s %10s %-8s %s\n",
		"SYMBOL", "ACTION", "EXECUTE", "RECORD", "ANNOUNCED", "AMOUNT", "CCY", "FACTOR")
	for i, r := range rows {
		if i >= limit {
			rocli.Truncate(out, i, len(rows), limit)
			break
		}
		factor := ""
		if r.FromFactor != 0 || r.ToFactor != 0 {
			factor = fmt.Sprintf("%.4g -> %.4g", r.FromFactor, r.ToFactor)
		}
		fmt.Fprintf(out, "  %-12s %-12s %-14s %-12s %-12s %10.4f %-8s %s\n",
			r.Symbol, rocli.Dash(r.ActionType), rocli.Dash(r.ExecuteDate), rocli.Dash(r.RecordDate),
			rocli.Dash(r.AnnouncedDate), r.Amount, rocli.Dash(r.Currency), rocli.Dash(factor))
	}
}

// actionRequest builds the shared CorporateActionRequest. The date bounds are
// epoch milliseconds, so the YYYY-MM-DD flags are converted here; a blank or
// unparseable date simply means "no bound".
func actionRequest(symbols []string, o options, kind actionType) (sdkmodel.CorporateActionRequest, error) {
	req := sdkmodel.CorporateActionRequest{
		Symbols: symbols,
		Market:  o.Market,
	}
	switch kind {
	case actionAny:
		// The combined endpoint requires an explicit action type; there is no
		// "everything" value, so refuse rather than guess.
		if strings.TrimSpace(o.actionType) == "" {
			return req, fmt.Errorf("-action-type is required for -op actions, e.g. -action-type dividend")
		}
		req.ActionType = strings.TrimSpace(o.actionType)
	case actionDividend:
		req.ActionType = "dividend"
	case actionSplit:
		req.ActionType = "split"
	case actionEarnings:
		req.ActionType = "earnings"
	}
	if d := rocli.DateMillis(o.beginDate); d != nil {
		req.BeginDate = d
	}
	if d := rocli.DateMillis(o.endDate); d != nil {
		req.EndDate = d
	}
	return req, nil
}

// ---- other corporate events ----

// opIPO reports initial public offerings.
func opIPO(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	// The IPO endpoint filters on the IPO action type, not on -action-type, so
	// build the request directly rather than going through actionRequest's
	// "user must name a type" rule.
	req := actionRequestFor(symbols, o, "ipo")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := qc.GetCorporateIPO(req)
	if err != nil {
		return fmt.Errorf("get IPOs (%s, market=%s): %w", strings.Join(symbols, ","), o.Market, err)
	}
	rocli.Section(out, "IPOs (market=%s)", o.Market)
	printIPOs(rows, o.Limit)
	return nil
}

func printIPOs(rows []sdkmodel.CorporateIPO, limit int) {
	if len(rows) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-14s %-30s %-12s %-14s %12s %-8s\n",
		"SYMBOL", "LISTED", "NAME", "EXECUTE", "PRICE_RANGE", "OFFER", "CCY")
	for i, r := range rows {
		if i >= limit {
			rocli.Truncate(out, i, len(rows), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-14s %-30s %-12s %-14s %12.2f %-8s\n",
			r.Symbol, rocli.Dash(r.ListingDate), rocli.Dash(r.IpoName), rocli.Dash(r.ExecuteDate),
			rocli.Dash(r.PriceRange), r.OfferAmount, rocli.Dash(r.Currency))
	}
}

// opSymbolChange reports ticker renames.
func opSymbolChange(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	req := actionRequestFor(symbols, o, "symbol_change")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := qc.GetCorporateSymbolChange(req)
	if err != nil {
		return fmt.Errorf("get symbol changes (%s, market=%s): %w", strings.Join(symbols, ","), o.Market, err)
	}
	rocli.Section(out, "symbol changes (market=%s)", o.Market)
	printSymbolChanges(rows, o.Limit)
	return nil
}

func printSymbolChanges(rows []sdkmodel.CorporateSymbolChange, limit int) {
	if len(rows) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-16s %-16s %-12s %s\n", "OLD", "NEW", "EXECUTE", "REASON")
	for i, r := range rows {
		if i >= limit {
			rocli.Truncate(out, i, len(rows), limit)
			break
		}
		fmt.Fprintf(out, "  %-16s %-16s %-12s %s\n",
			rocli.Dash(r.OldSymbol), rocli.Dash(r.NewSymbol), rocli.Dash(r.ExecuteDate), rocli.Dash(r.ActionType))
	}
}

// opDelisting reports delistings and their reasons.
func opDelisting(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	req := actionRequestFor(symbols, o, "delisting")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := qc.GetCorporateDelisting(req)
	if err != nil {
		return fmt.Errorf("get delistings (%s, market=%s): %w", strings.Join(symbols, ","), o.Market, err)
	}
	rocli.Section(out, "delistings (market=%s)", o.Market)
	printDelistings(rows, o.Limit)
	return nil
}

func printDelistings(rows []sdkmodel.CorporateDelisting, limit int) {
	if len(rows) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-12s %-12s %-12s %s\n", "SYMBOL", "ANNOUNCED", "EXECUTE", "ACTION", "REASON")
	for i, r := range rows {
		if i >= limit {
			rocli.Truncate(out, i, len(rows), limit)
			break
		}
		fmt.Fprintf(out, "  %-12s %-12s %-12s %-12s %s\n",
			r.Symbol, rocli.Dash(r.AnnouncedDate), rocli.Dash(r.ExecuteDate),
			rocli.Dash(r.ActionType), rocli.Dash(r.Reason))
	}
}

// actionRequestFor builds a request with an explicit action type and no
// -action-type requirement, for the endpoints whose action type is fixed.
func actionRequestFor(symbols []string, o options, action string) sdkmodel.CorporateActionRequest {
	req := sdkmodel.CorporateActionRequest{
		Symbols:    symbols,
		Market:     o.Market,
		ActionType: action,
	}
	if d := rocli.DateMillis(o.beginDate); d != nil {
		req.BeginDate = d
	}
	if d := rocli.DateMillis(o.endDate); d != nil {
		req.EndDate = d
	}
	return req
}

// ---- capital flow ----

// opCapitalFlow reports money flowing in and out, split by order size.
func opCapitalFlow(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbol := rocli.FirstSymbol(o.symbol)
	if symbol == "" {
		return errSymbol
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	flow, err := qc.GetCapitalFlow(symbol, o.Market, o.period)
	if err != nil {
		return fmt.Errorf("get capital flow (%s, market=%s, period=%s): %w", symbol, o.Market, o.period, err)
	}
	rocli.Section(out, "capital flow (%s, period=%s)", symbol, o.period)
	printCapitalFlow(flow, o.Limit)
	return nil
}

func printCapitalFlow(flow *sdkmodel.CapitalFlow, limit int) {
	if flow == nil {
		fmt.Fprintln(out, "  (no data returned)")
		return
	}
	fmt.Fprintf(out, "  %d bucket(s)\n", len(flow.Items))
	fmt.Fprintf(out, "  %-22s %16s %s\n", "TIME", "NET_INFLOW", "TS")
	for i, it := range flow.Items {
		if i >= limit {
			rocli.Truncate(out, i, len(flow.Items), limit)
			break
		}
		fmt.Fprintf(out, "  %-22s %16.2f %s\n", rocli.Dash(it.Time), it.NetInflow, rocli.MSFmt(it.Timestamp))
	}
}

// opCapitalDistribution reports the current-day inflow/outflow breakdown.
func opCapitalDistribution(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbol := rocli.FirstSymbol(o.symbol)
	if symbol == "" {
		return errSymbol
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	dist, err := qc.GetCapitalDistribution(symbol, o.Market)
	if err != nil {
		return fmt.Errorf("get capital distribution (%s, market=%s): %w", symbol, o.Market, err)
	}
	rocli.Section(out, "capital distribution (%s)", symbol)
	printCapitalDistribution(dist)
	return nil
}

func printCapitalDistribution(dist *sdkmodel.CapitalDistribution) {
	if dist == nil {
		fmt.Fprintln(out, "  (no data returned)")
		return
	}
	fmt.Fprintf(out, "  net_inflow=%.2f\n", dist.NetInflow)
	fmt.Fprintf(out, "  %-10s %16s %16s\n", "SIZE", "INFLOW", "OUTFLOW")
	fmt.Fprintf(out, "  %-10s %16.2f %16.2f\n", "all", dist.InAll, dist.OutAll)
	fmt.Fprintf(out, "  %-10s %16.2f %16.2f\n", "big", dist.InBig, dist.OutBig)
	fmt.Fprintf(out, "  %-10s %16.2f %16.2f\n", "mid", dist.InMid, dist.OutMid)
	fmt.Fprintf(out, "  %-10s %16.2f %16.2f\n", "small", dist.InSmall, dist.OutSmall)
}

// ---- warrants ----

// opWarrantFilter screens Hong Kong warrants.
func opWarrantFilter(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	res, err := qc.GetWarrantFilter(sdkmodel.WarrantFilterRequest{
		Symbol:        strings.ToUpper(strings.TrimSpace(o.underlying)),
		Page:          o.Page,
		PageSize:      o.PageSize,
		SortFieldName: o.sortField,
		SortDir:       o.sortDir,
		IssuerName:    o.issuer,
		ExpireYm:      o.expireYM,
		Lang:          o.Lang,
	})
	if err != nil {
		return fmt.Errorf("warrant filter (underlying=%s, issuer=%s, expire_ym=%s): %w",
			rocli.DashOr(o.underlying, "any"), rocli.DashOr(o.issuer, "any"), rocli.DashOr(o.expireYM, "any"), err)
	}
	rocli.Section(out, "warrant filter")
	if res == nil {
		fmt.Fprintln(out, "  (no data returned)")
		return nil
	}
	fmt.Fprintf(out, "  %d match(es), page %d, page_size %d\n", res.Total, res.Page, res.PageSize)
	printWarrants(res.Items, o.Limit)
	return nil
}

// opWarrantQuote returns real-time warrant quotes.
func opWarrantQuote(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	briefs, err := qc.GetWarrantQuote(sdkmodel.WarrantBriefsRequest{Symbols: symbols, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get warrant quote (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "warrant quotes")
	printWarrants(briefs, o.Limit)
	return nil
}

func printWarrants(briefs []sdkmodel.WarrantBrief, limit int) {
	if len(briefs) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-14s %-28s %10s %9s %12s %-10s %-12s\n",
		"SYMBOL", "NAME", "LAST", "CHG%", "VOLUME", "UNDERLYING", "EXPIRY")
	for i, b := range briefs {
		if i >= limit {
			rocli.Truncate(out, i, len(briefs), limit)
			break
		}
		fmt.Fprintf(out, "  %-14s %-28s %10.4f %8.2f%% %12d %-10s %-12s\n",
			rocli.Dash(b.Symbol), rocli.Dash(b.Name), b.LatestPrice, b.ChangeRate, b.Volume,
			rocli.Dash(b.Underlying), rocli.Dash(b.ExpiryDate))
	}
	return
}

// ---- mutual funds ----

func opFundSymbols(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	syms, err := qc.GetFundSymbols(sdkmodel.FundSymbolsRequest{Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get fund symbols: %w", err)
	}
	rocli.Section(out, "fund symbols (%d total)", len(syms))
	printFundSymbols(syms, o.Limit)
	return nil
}

func printFundSymbols(syms []string, limit int) {
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

func opFundContracts(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	contracts, err := qc.GetFundContracts(sdkmodel.FundContractsRequest{Symbols: symbols, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get fund contracts (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "fund contracts")
	printFundContracts(contracts, o.Limit)
	return nil
}

func printFundContracts(contracts []sdkmodel.FundContractInfo, limit int) {
	if len(contracts) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-14s %-30s %-8s %-10s %-12s %10s %8s\n",
		"SYMBOL", "NAME", "CCY", "TYPE", "INCEPTION", "NAV", "ER")
	for i, c := range contracts {
		if i >= limit {
			rocli.Truncate(out, i, len(contracts), limit)
			break
		}
		fmt.Fprintf(out, "  %-14s %-30s %-8s %-10s %-12s %10.4f %8.4f\n",
			rocli.Dash(c.Symbol), rocli.Dash(c.Name), rocli.Dash(c.Currency), rocli.Dash(c.FundType),
			rocli.Dash(c.Inception), c.NetAssetVal, c.ExpenseRatio)
	}
}

func opFundQuote(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	quotes, err := qc.GetFundQuote(sdkmodel.FundQuoteRequest{Symbols: symbols, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get fund quote (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "fund quotes")
	fmt.Fprintf(out, "  %-14s %12s %10s %9s %-12s\n", "SYMBOL", "NAV", "CHANGE", "CHG%", "DATE")
	for i, q := range quotes {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(quotes), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-14s %12.4f %10.4f %8.2f%% %-12s\n",
			rocli.Dash(q.Symbol), q.LatestNav, q.Change, q.ChangeRate, rocli.Dash(q.Date))
	}
	return nil
}

func opFundHistory(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := qc.GetFundHistoryQuote(sdkmodel.FundHistoryQuoteRequest{
		Symbols:   symbols,
		BeginTime: o.Begin,
		EndTime:   o.End,
		Limit:     o.Limit,
		Lang:      o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get fund history quote (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "fund NAV history")
	fmt.Fprintf(out, "  %-14s %-12s %12s\n", "SYMBOL", "DATE", "NAV")
	for i, r := range rows {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(rows), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-14s %-12s %12.4f\n", rocli.Dash(r.Symbol), rocli.Dash(r.Date), r.Nav)
	}
	return nil
}

// ---- helpers ----

var (
	errSymbols = rocli.RequiredFlag("-symbols", "AAPL,MSFT")
	errSymbol  = rocli.RequiredFlag("-symbol", "AAPL")
)

// dateToMillis converts a YYYY-MM-DD flag into the *int64 epoch-millisecond
// bound the corporate-action endpoints use. A blank or unparseable date yields
// nil, i.e. no bound, rather than a bogus timestamp.
