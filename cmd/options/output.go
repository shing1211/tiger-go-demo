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

// This file holds the per-endpoint request + print logic for cmd/options.
//
// Each handler is the same shape: check the flags, honour the context
// deadline, call exactly one SDK method, print. The SDK error is always wrapped
// with the endpoint name so a failure says which call produced it.

// out is the writer these handlers print to. Every Tiger read in this command
// goes to stdout, matching cmd/quote.
var out io.Writer = os.Stdout

// ---- expiration ----

func opExpiration(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	exps, err := qc.GetOptionExpiration(symbols, o.Market)
	if err != nil {
		return fmt.Errorf("get option expiration (market=%s): %w", o.Market, err)
	}
	rocli.Section(out, "option expirations (market=%s)", o.Market)
	printExpiration(exps, o.Limit)
	return nil
}

func printExpiration(exps []sdkmodel.OptionExpiration, limit int) {
	if len(exps) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for _, e := range exps {
		fmt.Fprintf(out, "  %-10s %d listed expiry date(s), %d contract(s)\n",
			e.Symbol, len(e.Dates), len(e.OptionSymbols))
		for i, d := range e.Dates {
			if i >= limit {
				rocli.Truncate(out, i, len(e.Dates), limit)
				break
			}
			fmt.Fprintf(out, "    %-12s %-8s %s\n", d, dashAt(e.Periods, i), contractCount(e.Counts, i))
		}
	}
}

// ---- chain ----

// opChain fetches the chain for one underlying. Without -expiry it asks Tiger
// for the nearest listed expiry first, which is what a human almost always
// wants and saves a round trip through the terminal.
func opChain(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	underlying := symbols[0]

	expiry := strings.TrimSpace(o.expiry)
	if expiry == "" {
		resolved, err := nearestExpiry(ctx, qc, underlying, o.Market)
		if err != nil {
			return err
		}
		expiry = resolved
		fmt.Fprintf(out, "(using the nearest listed expiry, %s; pass -expiry to override)\n", expiry)
	}

	// The plain form takes (symbol, expiry-date) and does the conversion for us.
	chains, err := qc.GetOptionChain([][2]string{{underlying, expiry}})
	if err != nil {
		return fmt.Errorf("get option chain (%s %s): %w", underlying, expiry, err)
	}
	rocli.Section(out, "option chain: %s %s", underlying, expiry)

	// -greeks and -itm need the request form, which is the only one that
	// carries filters and the return_greek_value switch. Re-issue the same
	// query through it rather than building a second command.
	if o.greeks || (o.itm != "" && o.itm != "all") {
		req, err := chainRequest(underlying, expiry, o)
		if err != nil {
			return err
		}
		chains, err = qc.GetOptionChainByReq(req)
		if err != nil {
			return fmt.Errorf("get option chain with filters (%s %s): %w", underlying, expiry, err)
		}
	}
	printChains(chains, o.Limit, o.greeks)
	return nil
}

// chainRequest builds the filter/greeks flavour of the chain request.
func chainRequest(underlying, expiry string, o options) (sdkmodel.OptionChainRequest, error) {
	ts, err := expiryMillis(expiry, underlying)
	if err != nil {
		return sdkmodel.OptionChainRequest{}, err
	}
	req := sdkmodel.OptionChainRequest{
		OptionBasic: []sdkmodel.OptionQueryItem{{Symbol: underlying, Expiry: ts}},
		Market:      o.Market,
		Lang:        o.Lang,
	}
	if o.greeks {
		req.ReturnGreekValue = boolPtr(true)
	}
	switch strings.ToLower(strings.TrimSpace(o.itm)) {
	case "", "all":
	case "in":
		req.OptionFilter = &sdkmodel.OptionChainFilter{InTheMoney: boolPtr(true)}
	case "out":
		req.OptionFilter = &sdkmodel.OptionChainFilter{InTheMoney: boolPtr(false)}
	default:
		return req, fmt.Errorf("invalid -itm %q (want in, out or all)", o.itm)
	}
	return req, nil
}

func printChains(chains []sdkmodel.OptionChain, limit int, greeks bool) {
	if len(chains) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for _, c := range chains {
		fmt.Fprintf(out, "  %s expiry=%s  %d strike row(s)\n", c.Symbol, rocli.MSFmt(c.Expiry), len(c.Items))
		fmt.Fprintf(out, "  %-10s %-10s %-10s | %-10s %-10s %-10s\n", "PUT", "BID", "ASK", "CALL", "BID", "ASK")
		for i, row := range c.Items {
			if i >= limit {
				rocli.Truncate(out, i, len(c.Items), limit)
				break
			}
			putBid, putAsk, putStrike, putOI := "-", "-", "-", int64(0)
			if row.Put != nil {
				putBid, putAsk, putStrike, putOI = rocli.Px(row.Put.BidPrice), rocli.Px(row.Put.AskPrice), rocli.Dash(row.Put.Strike), row.Put.OpenInterest
			}
			callBid, callAsk, callStrike, callOI := "-", "-", "-", int64(0)
			if row.Call != nil {
				callBid, callAsk, callStrike, callOI = rocli.Px(row.Call.BidPrice), rocli.Px(row.Call.AskPrice), rocli.Dash(row.Call.Strike), row.Call.OpenInterest
			}
			fmt.Fprintf(out, "  %-10s %-10s %-10s | %-10s %-10s %-10s  put_oi=%-10d call_oi=%d\n",
				putStrike, putBid, putAsk, callStrike, callBid, callAsk, putOI, callOI)
			if greeks && row.Call != nil {
				printGreeks("call", *row.Call)
			}
			if greeks && row.Put != nil {
				printGreeks("put ", *row.Put)
			}
		}
	}
}

func printGreeks(side string, leg sdkmodel.OptionLeg) {
	fmt.Fprintf(out, "      %s %-22s delta=%.4f gamma=%.6f theta=%.4f vega=%.4f rho=%.4f iv=%.4f mark=%.4f\n",
		side, rocli.Dash(leg.Identifier),
		leg.Delta, leg.Gamma, leg.Theta, leg.Vega, leg.Rho, leg.ImpliedVol, leg.MarkPrice)
}

// ---- quote ----

func opQuote(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	ids := rocli.ListRaw(o.ids)
	if len(ids) == 0 {
		return errIDs
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	// GetOptionQuote takes the identifiers as-is and parses them internally.
	briefs, err := qc.GetOptionQuote(ids)
	if err != nil {
		return fmt.Errorf("get option quote: %w", err)
	}
	rocli.Section(out, "option quotes")
	printOptionBriefs(briefs, o.Limit)
	return nil
}

func printOptionBriefs(briefs []sdkmodel.Brief, limit int) {
	if len(briefs) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-28s %-6s %-10s %10s %10s %10s %12s\n", "SYMBOL", "RIGHT", "STRIKE", "LAST", "BID", "ASK", "VOLUME")
	for i, b := range briefs {
		if i >= limit {
			rocli.Truncate(out, i, len(briefs), limit)
			break
		}
		fmt.Fprintf(out, "  %-28s %-6s %-10s %10.4f %10.4f %10.4f %12d\n",
			b.Symbol, b.Right, rocli.Dash(b.Strike), b.LatestPrice, b.BidPrice, b.AskPrice, b.Volume)
	}
}

func printKlines(klines []sdkmodel.Kline, period string, limit int) {
	if len(klines) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	rocli.Section(out, "option k-lines (period=%s)", period)
	for _, k := range klines {
		fmt.Fprintf(out, "  %s next_page_token=%s\n", k.Symbol, rocli.Dash(k.NextPageToken))
		fmt.Fprintf(out, "  %-22s %10s %10s %10s %10s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME")
		for i, it := range k.Items {
			if i >= limit {
				rocli.Truncate(out, i, len(k.Items), limit)
				break
			}
			fmt.Fprintf(out, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d\n",
				rocli.MSFmt(it.Time), it.Open, it.High, it.Low, it.Close, it.Volume)
		}
	}
}

func printKlinesPlain(klines []sdkmodel.Kline, period string, limit int) {
	if len(klines) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	rocli.Section(out, "option k-lines, server-selected range (period=%s)", period)
	for _, k := range klines {
		fmt.Fprintf(out, "  %s next_page_token=%s\n", k.Symbol, rocli.Dash(k.NextPageToken))
		fmt.Fprintf(out, "  %-22s %10s %10s %10s %10s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME")
		for i, it := range k.Items {
			if i >= limit {
				rocli.Truncate(out, i, len(k.Items), limit)
				break
			}
			fmt.Fprintf(out, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d\n",
				rocli.MSFmt(it.Time), it.Open, it.High, it.Low, it.Close, it.Volume)
		}
	}
}

// ---- kline ----

// opKline uses GetOptionKlineWithOpts, the variant that accepts a limit and a
// sort direction, so those flags reach Tiger instead of being dropped.
func opKline(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	ids := rocli.ListRaw(o.ids)
	if len(ids) == 0 {
		return errIDs
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	klines, err := qc.GetOptionKlineWithOpts(ids, o.period, o.Begin, o.End, o.Limit, o.sortDir)
	if err != nil {
		return fmt.Errorf("get option kline (period=%s, sort=%s): %w", o.period, rocli.DashOr(o.sortDir, "sdk default"), err)
	}
	printKlines(klines, o.period, o.Limit)
	return nil
}

// opKlinePlain uses the simpler GetOptionKline, which sends no limit or sort
// direction and lets Tiger pick the range. -op kline is the one to prefer;
// this exists so both SDK entry points are exercised.
func opKlinePlain(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	ids := rocli.ListRaw(o.ids)
	if len(ids) == 0 {
		return errIDs
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	klines, err := qc.GetOptionKline(ids, o.period, o.Begin, o.End)
	if err != nil {
		return fmt.Errorf("get option kline (period=%s): %w", o.period, err)
	}
	printKlinesPlain(klines, o.period, o.Limit)
	return nil
}

// ---- depth ----

func printDepth(depths []sdkmodel.Depth, limit int) {
	if len(depths) == 0 {
		fmt.Fprintf(out, "  (no rows returned)\n")
		return
	}
	for _, d := range depths {
		fmt.Fprintf(out, "  %s\n", d.Symbol)
		n := len(d.Asks)
		if len(d.Bids) > n {
			n = len(d.Bids)
		}
		fmt.Fprintf(out, "  %-10s %10s %10s   %10s %10s\n", "BID", "SIZE", "COUNT", "ASK", "SIZE")
		for i := 0; i < n; i++ {
			if i >= limit {
				rocli.Truncate(out, i, n, limit)
				break
			}
			bp, bs, bc := "-", "-", "-"
			if i < len(d.Bids) {
				bp, bs, bc = rocli.Px(d.Bids[i].Price), itoa(int64(d.Bids[i].Volume)), itoa(int64(d.Bids[i].Count))
			}
			ap, as := "-", "-"
			if i < len(d.Asks) {
				ap, as = rocli.Px(d.Asks[i].Price), itoa(d.Asks[i].Volume)
			}
			fmt.Fprintf(out, "  %-10s %10s %10s   %10s %10s\n", bp, bs, bc, ap, as)
		}
	}
}

func opDepth(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	items, err := parseIdentifiers(rocli.ListRaw(o.ids))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	depths, err := qc.GetOptionDepth(sdkmodel.OptionDepthRequest{
		OptionBasic: items,
		Market:      o.Market,
		Lang:        o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get option depth: %w", err)
	}
	rocli.Section(out, "option order book (market=%s)", o.Market)
	printDepth(depths, o.Limit)
	return nil
}

// ---- ticks ----

func printTicks(ticks []sdkmodel.TradeTick, limit int) {
	if len(ticks) == 0 {
		fmt.Fprintf(out, "  (no rows returned)\n")
		return
	}
	for _, t := range ticks {
		fmt.Fprintf(out, "  %s (%d..%d, %d tick(s))\n", t.Symbol, t.BeginIndex, t.EndIndex, len(t.Items))
		fmt.Fprintf(out, "  %-22s %-8s %10s %10s\n", "TIME", "COND", "PRICE", "VOLUME")
		for i, it := range t.Items {
			if i >= limit {
				rocli.Truncate(out, i, len(t.Items), limit)
				break
			}
			fmt.Fprintf(out, "  %-22s %-8s %10.4f %10d\n",
				rocli.MSFmt(it.Time), rocli.Dash(it.Cond), it.Price, it.Volume)
		}
	}
}

func opTicks(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	items, err := parseIdentifiers(rocli.ListRaw(o.ids))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	ticks, err := qc.GetOptionTradeTicks(sdkmodel.OptionTradeTicksRequest{Contracts: items})
	if err != nil {
		return fmt.Errorf("get option trade ticks: %w", err)
	}
	rocli.Section(out, "option trade ticks")
	printTicks(ticks, o.Limit)
	return nil
}

// ---- timeline ----

func printOptionTimeline(tls []sdkmodel.Timeline, limit int) {
	if len(tls) == 0 {
		fmt.Fprintf(out, "  (no rows returned)\n")
		return
	}
	for _, t := range tls {
		fmt.Fprintf(out, "  %-28s period=%-8s pre_close=%.4f\n", t.Symbol, t.Period, t.PreClose)
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

func opTimeline(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	items, err := parseIdentifiers(rocli.ListRaw(o.ids))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	tls, err := qc.GetOptionTimeline(sdkmodel.OptionTimelineRequest{
		OptionQuery: items,
		Market:      o.Market,
		Lang:        o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get option timeline: %w", err)
	}
	rocli.Section(out, "option intraday timeline")
	printOptionTimeline(tls, o.Limit)
	return nil
}

// ---- symbols ----

func printOptionSymbols(syms []sdkmodel.OptionSymbol, limit int) {
	if len(syms) == 0 {
		fmt.Fprintf(out, "  (no rows returned)\n")
		return
	}
	for i, s := range syms {
		if i >= limit {
			rocli.Truncate(out, i, len(syms), limit)
			break
		}
		name := s.NameEN
		if name == "" {
			name = s.NameCN
		}
		fmt.Fprintf(out, "  %-28s %-8s %-40s\n", rocli.Dash(s.Symbol), rocli.Dash(s.Market), rocli.Dash(name))
	}
}

func opSymbols(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	syms, err := qc.GetOptionSymbols(sdkmodel.OptionSymbolsRequest{Market: o.Market, Lang: o.Lang})
	if err != nil {
		return fmt.Errorf("get option symbols (market=%s): %w", o.Market, err)
	}
	rocli.Section(out, "option symbols (market=%s)", o.Market)
	fmt.Fprintf(out, "  %-28s %-8s %-40s\n", "SYMBOL", "MARKET", "NAME")
	printOptionSymbols(syms, o.Limit)
	return nil
}

// ---- analysis ----

func printAnalysis(results []sdkmodel.OptionAnalysis, limit int) {
	if len(results) == 0 {
		fmt.Fprintf(out, "  (no rows returned)\n")
		return
	}
	for _, a := range results {
		fmt.Fprintf(out, "  %-10s iv30d=%.4f hist_vol=%.4f iv/hv=%.4f call/put=%.4f vol_points=%d\n",
			a.Symbol, a.ImpliedVol30Days, a.HisVolatility, a.IvHisVRatio, a.CallPutRatio, len(a.VolatilityList))
		if m := a.ImpliedVolMetric; m != nil {
			fmt.Fprintf(out, "    metric period=%-8s percentile=%.2f rank=%.2f\n",
				rocli.Dash(m.Period), m.Percentile, m.Rank)
		}
		for i, p := range a.VolatilityList {
			if i >= limit {
				rocli.Truncate(out, i, len(a.VolatilityList), limit)
				break
			}
			fmt.Fprintf(out, "    %-22s iv=%.4f percentile=%.2f rank=%.2f hist_vol=%.4f\n",
				rocli.MSFmt(p.Timestamp), p.ImpliedVol, p.Percentile, p.Rank, p.HisVolatility)
		}
	}
}

func opAnalysis(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	symbols := rocli.List(o.symbols)
	if len(symbols) == 0 {
		return errSymbols
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.OptionAnalysisRequest{
		Market: o.Market,
		Lang:   o.Lang,
	}
	for _, s := range symbols {
		entry := sdkmodel.OptionAnalysisSymbol{Symbol: s}
		if o.volatilityList {
			entry.RequireVolatilityList = boolPtr(true)
		}
		req.Symbols = append(req.Symbols, entry)
	}
	out2, err := qc.GetOptionAnalysis(req)
	if err != nil {
		return fmt.Errorf("get option analysis (%s): %w", strings.Join(symbols, ","), err)
	}
	rocli.Section(out, "option analysis")
	printAnalysis(out2, o.Limit)
	return nil
}

// ---- helpers ----

// nearestExpiry asks Tiger for the closest listed expiry of an underlying, so
// that -op chain works without the user having to look one up first.
func nearestExpiry(ctx context.Context, qc *sdkquote.QuoteClient, symbol, market string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context: %w", err)
	}
	exps, err := qc.GetOptionExpiration([]string{symbol}, market)
	if err != nil {
		return "", fmt.Errorf("get option expiration for %s (to resolve the nearest expiry): %w", symbol, err)
	}
	for _, e := range exps {
		if len(e.Dates) > 0 {
			return e.Dates[0], nil
		}
	}
	return "", fmt.Errorf("no listed expiry returned for %s in %s; pass -expiry YYYY-MM-DD", symbol, market)
}

var (
	errSymbols = rocli.RequiredFlag("-symbols", "AAPL,TSLA")
	errIDs     = rocli.RequiredFlag("-ids", `"AAPL 250117C00200000"`)
)

func itoa(v int64) string { return fmt.Sprintf("%d", v) }

func boolPtr(b bool) *bool { return &b }

func dashAt(s []string, i int) string {
	if i < len(s) {
		return rocli.Dash(s[i])
	}
	return "-"
}
func contractCount(counts []int, i int) string {
	if i < len(counts) {
		return fmt.Sprintf("%6d contract(s)", counts[i])
	}
	return "        -"
}
