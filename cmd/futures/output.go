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

// This file holds the per-endpoint request + print logic for cmd/futures.
//
// Each handler is the same shape: check the flags, honour the context
// deadline, call exactly one SDK method, print. The SDK error is always wrapped
// with the endpoint name so a failure says which call produced it.

var out io.Writer = os.Stdout

// ---- exchange list ----

func printExchanges(exchanges []sdkmodel.FutureExchange, limit int) {
	if len(exchanges) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-12s %-36s %-12s\n", "CODE", "NAME", "ZONE")
	for i, e := range exchanges {
		if i >= limit {
			break
		}
		fmt.Fprintf(out, "  %-12s %-36s %-12s\n", rocli.Dash(e.Code), rocli.Dash(e.Name), rocli.Dash(e.ZoneID))
	}
}

// opExchange lists the exchanges on which Tiger quotes futures.
func opExchange(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	exchanges, err := qc.GetFutureExchange()
	if err != nil {
		return fmt.Errorf("get future exchange list: %w", err)
	}
	rocli.Section(out, "future exchanges")
	printExchanges(exchanges, o.Limit)
	return nil
}

// ---- contract metadata ----

// opCurrent resolves the current front-month contract for a product.
func opCurrent(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	code := strings.TrimSpace(o.code)
	if code == "" {
		return errCode("CL, the front-month crude contract")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	info, err := qc.GetCurrentFutureContract(sdkmodel.FutureContractSingleRequest{
		ContractCode: strings.ToUpper(code),
		Type:         o.ftype,
		Lang:         o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get current future contract (%s): %w", strings.ToUpper(code), err)
	}
	rocli.Section(out, "current future contract")
	printContracts([]sdkmodel.FutureContractInfo{*info})
	return nil
}

// opContract looks a single contract up by its code. It is distinct from
// -op current, which resolves the front month of a product.
func opContract(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	code := strings.ToUpper(strings.TrimSpace(o.code))
	if code == "" {
		return errCode("CLmain")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	contracts, err := qc.GetFutureContract(sdkmodel.FutureContractSingleRequest{
		ContractCode: code,
		Type:         o.ftype,
		Lang:         o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future contract (%s): %w", code, err)
	}
	rocli.Section(out, "future contract (%s)", code)
	printContracts(contracts)
	return nil
}

// opContracts lists the tradable contracts on one exchange.
func opContracts(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	exchange := strings.ToUpper(strings.TrimSpace(o.exchange))
	if exchange == "" {
		return errExchange
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	contracts, err := qc.GetFutureContracts(exchange)
	if err != nil {
		return fmt.Errorf("get future contracts (exchange=%s): %w", exchange, err)
	}
	rocli.Section(out, "future contracts (exchange=%s)", exchange)
	printContracts(contracts)
	return nil
}

// opAllContracts lists contracts filtered by type and/or exchange.
func opAllContracts(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.AllFutureContractsRequest{
		Type:     o.ftype,
		Exchange: strings.ToUpper(strings.TrimSpace(o.exchange)),
		Lang:     o.Lang,
	}
	contracts, err := qc.GetAllFutureContracts(req)
	if err != nil {
		return fmt.Errorf("get all future contracts (type=%s exchange=%s): %w",
			rocli.DashOr(o.ftype, "unset"), rocli.DashOr(req.Exchange, "all"), err)
	}
	rocli.Section(out, "all future contracts (type=%s exchange=%s)", rocli.DashOr(o.ftype, "all"), rocli.DashOr(req.Exchange, "all"))
	printContracts(contracts)
	return nil
}

// opContinuous lists the continuous (rolled) contract series.
func opContinuous(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	contracts, err := qc.GetFutureContinuousContracts(sdkmodel.FutureContinuousContractsRequest{
		Type: o.ftype,
		Lang: o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future continuous contracts (type=%s): %w", rocli.DashOr(o.ftype, "unset"), err)
	}
	rocli.Section(out, "future continuous contracts (type=%s)", rocli.DashOr(o.ftype, "all"))
	printContracts(contracts)
	return nil
}

func printContracts(contracts []sdkmodel.FutureContractInfo) {
	if len(contracts) == 0 {
		fmt.Fprintln(out, "  (no contracts returned)")
		return
	}
	fmt.Fprintf(out, "  %-14s %-10s %-34s %-10s %-8s %-8s %-9s %-6s\n",
		"CODE", "EXCHANGE", "NAME", "MONTH", "CURRENCY", "MULT", "LAST_TRADE", "CONT")
	for _, c := range contracts {
		fmt.Fprintf(out, "  %-14s %-10s %-34s %-10s %-8s %-8.2f %-9s %-6v\n",
			rocli.Dash(c.ContractCode), rocli.Dash(c.Exchange), rocli.Dash(c.Name),
			rocli.Dash(c.ContractMonth), rocli.Dash(c.Currency), c.Multiplier,
			rocli.Dash(c.LastTradingDate), c.Continuous)
	}
}

// ---- prices ----

func printFutureQuotes(quotes []sdkmodel.FutureQuote, limit int) {
	if len(quotes) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-14s %10s %10s %10s %12s %12s %10s %-20s\n",
		"CONTRACT", "LAST", "BID", "ASK", "VOLUME", "OPEN_INT", "SETTLE", "TIME")
	for i, q := range quotes {
		if i >= limit {
			break
		}
		fmt.Fprintf(out, "  %-14s %10.4f %10.4f %10.4f %12d %12d %10.4f %-20s\n",
			q.ContractCode, q.LatestPrice, q.BidPrice, q.AskPrice, q.Volume,
			q.OpenInterest, q.Settlement, rocli.MSFmt(q.LatestTime))
	}
}

// opQuote fetches real-time futures quotes.
func opQuote(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	codes := rocli.List(o.codes)
	if len(codes) == 0 {
		return errCodes
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	quotes, err := qc.GetFutureRealTimeQuote(sdkmodel.FutureBriefRequest{
		ContractCodes: codes,
		Lang:          o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future real-time quote (%s): %w", strings.Join(codes, ","), err)
	}
	rocli.Section(out, "future real-time quotes")
	printFutureQuotes(quotes, o.Limit)
	return nil
}

// opKline fetches futures bars. ContractCodes is the multi-contract form; a
// lone -code is folded into it so both flags work here.
func opKline(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	codes := rocli.List(o.codes)
	if c := rocli.List(o.code); len(c) > 0 {
		codes = append(codes, c...)
	}
	if len(codes) == 0 {
		return errCodes
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	klines, err := qc.GetFutureKline(sdkmodel.FutureKlineRequest{
		ContractCodes: codes,
		Period:        o.period,
		BeginTime:     o.Begin,
		EndTime:       o.End,
		Limit:         o.Limit,
		PageToken:     o.pageToken,
		Lang:          o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future kline (period=%s): %w", o.period, err)
	}
	rocli.Section(out, "future k-lines (period=%s)", o.period)
	printFutureKlines(klines, o.period, o.Limit)
	return nil
}

func printFutureKlines(klines []sdkmodel.FutureKline, period string, limit int) {
	if len(klines) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	for _, k := range klines {
		fmt.Fprintf(out, "  series next_page_token=%s (%d bar(s))\n", rocli.Dash(k.NextPageToken), len(k.Items))
		fmt.Fprintf(out, "  %-22s %10s %10s %10s %10s %12s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME", "OPEN_INT")
		for j, it := range k.Items {
			if j >= limit {
				break
			}
			fmt.Fprintf(out, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d %12d\n",
				rocli.MSFmt(it.Time), it.Open, it.High, it.Low, it.Close, it.Volume, it.OpenInterest)
		}
	}
}

// opKlinePage uses the paged variant, which takes a total/page size rather than
// a limit and returns a flat bar list.
func opKlinePage(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	code := strings.ToUpper(strings.TrimSpace(o.code))
	if code == "" {
		return errCode("CLmain")
	}
	pageSize := o.PageSize
	if pageSize <= 0 {
		pageSize = o.Limit
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	bars, err := qc.GetFutureKlineByPage(sdkmodel.FutureKlineByPageRequest{
		ContractCode: code,
		Period:       o.period,
		BeginTime:    o.Begin,
		EndTime:      o.End,
		TotalSize:    o.totalSize,
		PageSize:     pageSize,
		Lang:         o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future kline by page (%s, period=%s, page_size=%d): %w", code, o.period, pageSize, err)
	}
	rocli.Section(out, "future k-lines by page (%s, period=%s)", code, o.period)
	printKlinePageBars(bars, o.Limit)
	return nil
}

func printKlinePageBars(bars []sdkmodel.FutureKlineItem, limit int) {
	if len(bars) == 0 {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	fmt.Fprintf(out, "  %-22s %10s %10s %10s %10s %12s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME", "OPEN_INT")
	for i, b := range bars {
		if i >= limit {
			break
		}
		fmt.Fprintf(out, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d %12d\n",
			rocli.MSFmt(b.Time), b.Open, b.High, b.Low, b.Close, b.Volume, b.OpenInterest)
	}
}

// ---- microstructure ----

func printFutureDepth(depth *sdkmodel.FutureDepth, limit int) {
	if depth == nil {
		fmt.Fprintln(out, "  (no rows returned)")
		return
	}
	n := len(depth.Asks)
	if len(depth.Bids) > n {
		n = len(depth.Bids)
	}
	fmt.Fprintf(out, "  %-10s %12s   %10s %12s\n", "BID", "SIZE", "ASK", "SIZE")
	for i := 0; i < n; i++ {
		if i >= limit {
			break
		}
		bp, bs, ap, as := "-", "-", "-", "-"
		if i < len(depth.Bids) {
			bp, bs = rocli.Px(depth.Bids[i].Price), fmt.Sprintf("%d", depth.Bids[i].Volume)
		}
		if i < len(depth.Asks) {
			ap, as = rocli.Px(depth.Asks[i].Price), fmt.Sprintf("%d", depth.Asks[i].Volume)
		}
		fmt.Fprintf(out, "  %-10s %12s   %10s %12s\n", bp, bs, ap, as)
	}
}

// opDepth fetches the futures order book.
func opDepth(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	codes := rocli.List(o.codes)
	if len(codes) == 0 {
		return errCodes
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	depths, err := qc.GetFutureDepth(sdkmodel.FutureDepthRequest{
		ContractCodes: codes,
		Lang:          o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future depth (%s): %w", strings.Join(codes, ","), err)
	}
	rocli.Section(out, "future order book")
	for _, d := range depths {
		fmt.Fprintf(out, "  %s as_of=%s\n", rocli.Dash(d.ContractCode), rocli.MSFmt(d.Timestamp))
		printFutureDepth(&d, o.Limit)
	}
	return nil
}

// opTicks fetches futures trade ticks for one contract.
func opTicks(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	code := strings.ToUpper(strings.TrimSpace(o.code))
	if code == "" {
		return errCode("CLmain")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	ticks, err := qc.GetFutureTradeTicks(sdkmodel.FutureTradeTicksRequest{
		ContractCode: code,
		Limit:        o.Limit,
		Lang:         o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future trade ticks (%s): %w", code, err)
	}
	rocli.Section(out, "future trade ticks (%s)", code)
	fmt.Fprintf(out, "  %-6s %-22s %-6s %10s %10s\n", "INDEX", "TIME", "DIR", "PRICE", "VOLUME")
	for i, t := range ticks {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(ticks), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-6d %-22s %-6s %10.4f %10d\n",
			t.Index, rocli.MSFmt(t.Time), rocli.Dash(t.Direction), t.Price, t.Volume)
	}
	return nil
}

// ---- sessions ----

// opTradingTimes reports when a contract trades on a given date.
func opTradingTimes(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	code := strings.ToUpper(strings.TrimSpace(o.code))
	if code == "" {
		return errCode("CLmain")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	tt, err := qc.GetFutureTradingTimes(sdkmodel.FutureTradingTimesRequest{
		ContractCode: code,
		TradingDate:  strings.TrimSpace(o.date),
		Lang:         o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future trading times (%s, date=%s): %w", code, rocli.DashOr(o.date, "server default"), err)
	}
	rocli.Section(out, "future trading times")
	if tt == nil {
		fmt.Fprintln(out, "  (no trading times returned)")
		return nil
	}
	fmt.Fprintf(out, "  contract=%s biz_date=%s zone=%s\n",
		rocli.Dash(tt.ContractCode), rocli.Dash(tt.BizDate), rocli.Dash(tt.Zone))
	for i, seg := range tt.TradingTimes {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(tt.TradingTimes), o.Limit)
			break
		}
		fmt.Fprintf(out, "    %-24s %s -> %s\n",
			rocli.Dash(seg.Type), rocli.MSFmt(seg.Start), rocli.MSFmt(seg.End))
	}
	return nil
}

// opHistoryMain reports which contract was the main contract over a window,
// which is what you need to stitch a continuous price series together.
func opHistoryMain(ctx context.Context, qc *sdkquote.QuoteClient, o options) error {
	codes := rocli.List(o.codes)
	if len(codes) == 0 {
		return errCodes
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	hist, err := qc.GetFutureHistoryMainContract(sdkmodel.FutureHistoryMainContractRequest{
		ContractCodes: codes,
		BeginTime:     o.Begin,
		EndTime:       o.End,
		Lang:          o.Lang,
	})
	if err != nil {
		return fmt.Errorf("get future history main contract (%s): %w", strings.Join(codes, ","), err)
	}
	rocli.Section(out, "historical main contracts")
	fmt.Fprintf(out, "  %-14s %-14s %-12s %-12s\n", "SYMBOL", "CONTRACT", "BEGIN", "END")
	for i, h := range hist {
		if i >= o.Limit {
			rocli.Truncate(out, i, len(hist), o.Limit)
			break
		}
		fmt.Fprintf(out, "  %-14s %-14s %-12s %-12s\n",
			rocli.Dash(h.Symbol), rocli.Dash(h.ContractCode), rocli.Dash(h.BeginDate), rocli.Dash(h.EndDate))
	}
	return nil
}

// ---- helpers ----

var (
	errCodes    = rocli.RequiredFlag("-codes", "CLmain,ESmain")
	errCode     = func(example string) error { return rocli.RequiredFlag("-code", example) }
	errExchange = rocli.RequiredFlag("-exchange", "COMEX")
)
