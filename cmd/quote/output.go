package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"
)

// dispatchOp routes a single -op to its printer. Every endpoint in ops is a
// read; there is no write case here and no write gate to check.
func dispatchOp(ctx context.Context, qc *sdkquote.QuoteClient, op string, w io.Writer) error {
	switch op {
	case "addon-entitlement":
		return printAddonEntitlement(ctx, qc, w)
	default:
		return fmt.Errorf("unknown -op %q (want: %s)", op, strings.Join(ops, ", "))
	}
}

// printAddonEntitlement reports the account's addon plan: the tier it is on, the
// plan currently in force, each addon attached to it, and the quota that remains
// once the addons are applied (生效后的权益额度明细).
//
// EffectiveEntitlement is a pointer, so a nil means the server sent no detail
// block at all and is printed as such. Its fourteen fields are plain ints with
// omitempty json tags: a field the server omits arrives as 0, and the type
// offers no way to tell that apart from a genuine zero, so both print as 0.
func printAddonEntitlement(ctx context.Context, qc *sdkquote.QuoteClient, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	ent, err := qc.GetAddonEntitlement()
	if err != nil {
		return fmt.Errorf("get addon entitlement: %w", err)
	}
	printEntitlement(w, ent)
	return nil
}

// printEntitlement is separated from the call above so the rendering can be
// tested against a hand-built response, without a credentialed round trip.
func printEntitlement(w io.Writer, ent *sdkmodel.AddonEntitlement) {
	fmt.Fprintf(w, "\n== addon entitlement ==\n")
	if ent == nil {
		fmt.Fprintf(w, "  (no entitlement returned)\n")
		return
	}

	// UserLevel is a FlexString: the server sometimes sends a number here.
	fmt.Fprintf(w, "  user_level  %s\n", orDash(ent.UserLevel.String()))

	if ent.ActivePlan == nil {
		fmt.Fprintf(w, "  active_plan (none)\n")
	} else {
		fmt.Fprintf(w, "  active_plan plan_type=%s expire=%s\n",
			orDash(ent.ActivePlan.PlanType), msToTime(ent.ActivePlan.ExpireTime))
	}

	if len(ent.Addons) == 0 {
		fmt.Fprintf(w, "  addons (none)\n")
	} else {
		fmt.Fprintf(w, "  addons (%d)\n", len(ent.Addons))
		fmt.Fprintf(w, "    %-20s %-7s %-20s %-20s\n", "PLAN_TYPE", "ACTIVE", "START", "EXPIRE")
		for _, a := range ent.Addons {
			fmt.Fprintf(w, "    %-20s %-7t %-20s %-20s\n",
				orDash(a.PlanType), a.Active, msToTime(a.StartTime), msToTime(a.ExpireTime))
		}
	}

	if ent.EffectiveEntitlement == nil {
		fmt.Fprintf(w, "  effective_entitlement (absent)\n")
		return
	}
	e := ent.EffectiveEntitlement
	fmt.Fprintf(w, "  effective_entitlement\n")
	fmt.Fprintf(w, "    %-22s %10s %10s\n", "QUOTA", "LIMIT", "REMAINING")
	for _, q := range []struct {
		name             string
		limit, remaining int
	}{
		{"history_stock", e.HistoryStockLimit, e.HistoryStockRemaining},
		{"history_future", e.HistoryFutureLimit, e.HistoryFutureRemaining},
		{"history_option", e.HistoryOptionLimit, e.HistoryOptionRemaining},
		{"subscribe", e.SubscribeLimit, e.SubscribeRemaining},
		{"subscribe_depth", e.SubscribeDepthLimit, e.SubscribeDepthRemaining},
	} {
		fmt.Fprintf(w, "    %-22s %10d %10d\n", q.name, q.limit, q.remaining)
	}
	// These four have no remaining/limit split in the SDK's model.
	for _, q := range []struct {
		name  string
		limit int
	}{
		{"high_freq_limit", e.HighFreqLimit},
		{"mid_freq_limit", e.MidFreqLimit},
		{"low_freq_limit", e.LowFreqLimit},
		{"rate_multiple", e.RateMultiple},
	} {
		fmt.Fprintf(w, "    %-22s %10d\n", q.name, q.limit)
	}
}

// printMarketStates renders the market-state rows. It is separated from the
// call below so the rendering can be tested without a credentialed round trip.
func printMarketStates(w io.Writer, states []sdkmodel.MarketState) {
	fmt.Fprintf(w, "== market state ==\n")
	if len(states) == 0 {
		fmt.Fprintln(w, "  (no data returned)")
		return
	}
	for _, s := range states {
		fmt.Fprintf(w, "  %-4s status=%-12s market_status=%-12s open=%s\n",
			s.Market, s.Status, s.MarketStatus, orDash(s.OpenTime))
	}
}

func printMarketState(ctx context.Context, qc *sdkquote.QuoteClient, market string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	states, err := qc.GetMarketState(market)
	if err != nil {
		return fmt.Errorf("get market state (%s): %w", market, err)
	}
	printMarketStates(w, states)
	return nil
}

// printRealTimeBriefs renders the real-time quote table. It is separated from
// the call below so the rendering can be tested without a credentialed round
// trip. There is no empty-result branch, by design: the heading and the column
// header always print, so an empty response reads as a table with no rows.
func printRealTimeBriefs(w io.Writer, briefs []sdkmodel.Brief) {
	fmt.Fprintf(w, "\n== real-time quotes ==\n")
	fmt.Fprintf(w, "  %-10s %10s %10s %10s %12s %10s\n", "SYMBOL", "LAST", "CHANGE", "CHG%", "VOLUME", "TIME")
	for _, b := range briefs {
		fmt.Fprintf(w, "  %-10s %10.4f %10.4f %9.2f%% %12d %10s\n",
			b.Symbol, b.LatestPrice, b.Change, b.ChangeRate, b.Volume, msToTime(b.LatestTime))
	}
}

func printBriefs(ctx context.Context, qc *sdkquote.QuoteClient, symbols []string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	briefs, err := qc.GetRealTimeQuote(sdkmodel.BriefRequest{Symbols: symbols})
	if err != nil {
		return fmt.Errorf("get real-time quote: %w", err)
	}
	printRealTimeBriefs(w, briefs)
	return nil
}

// printRealTimeKlines renders one block of bars per symbol. It is separated from
// the call below so the rendering can be tested without a credentialed round
// trip. The requested period is a paging parameter the server echo is not
// guaranteed to carry, so it is passed in and printed in the heading.
func printRealTimeKlines(w io.Writer, klines []sdkmodel.Kline, period string) {
	fmt.Fprintf(w, "\n== k-lines (period=%s) ==\n", period)
	for _, k := range klines {
		fmt.Fprintf(w, "  %s (next_page_token=%s)\n", k.Symbol, orDash(k.NextPageToken))
		fmt.Fprintf(w, "  %-22s %10s %10s %10s %10s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME")
		for _, it := range k.Items {
			fmt.Fprintf(w, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d\n",
				msToTime(it.Time), it.Open, it.High, it.Low, it.Close, it.Volume)
		}
	}
}

func printKlines(ctx context.Context, qc *sdkquote.QuoteClient, symbols []string, period string, limit int, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	if limit <= 0 {
		limit = 5
	}
	klines, err := qc.GetKline(sdkmodel.KlineRequest{
		Symbols: symbols,
		Period:  period,
		Limit:   limit,
		Right:   "forward", // newest bars last
	})
	if err != nil {
		return fmt.Errorf("get kline (period=%s): %w", period, err)
	}
	printRealTimeKlines(w, klines, period)
	return nil
}

func printTimeline(ctx context.Context, qc *sdkquote.QuoteClient, symbols []string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	tl, err := qc.GetTimeline(symbols)
	if err != nil {
		return fmt.Errorf("get timeline: %w", err)
	}
	fmt.Fprintf(w, "\n== intraday timeline ==\n")
	for _, t := range tl {
		// The timeline payload is large; print a shape summary per session
		// bucket plus the first few points, not every point.
		fmt.Fprintf(w, "  %-10s period=%-8s pre_close=%.4f\n", t.Symbol, t.Period, t.PreClose)
		buckets := []struct {
			name string
			b    *sdkmodel.TimelineBucket
		}{
			{"pre_hours", t.PreHours},
			{"intraday", t.Intraday},
			{"after_hours", t.AfterHours},
		}
		for _, bk := range buckets {
			if bk.b == nil || len(bk.b.Items) == 0 {
				continue
			}
			fmt.Fprintf(w, "  [%s] %d point(s)\n", bk.name, len(bk.b.Items))
			for i, it := range bk.b.Items {
				if i >= 10 {
					fmt.Fprintf(w, "    ... %d more point(s)\n", len(bk.b.Items)-10)
					break
				}
				fmt.Fprintf(w, "    %-22s price=%.4f avg=%.4f volume=%d\n",
					msToTime(it.Time), it.Price, it.AvgPrice, it.Volume)
			}
		}
	}
	return nil
}

func printDepth(ctx context.Context, qc *sdkquote.QuoteClient, symbols []string, market string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	depths, err := qc.GetQuoteDepth(sdkmodel.DepthQuoteRequest{Symbols: symbols, Market: market})
	if err != nil {
		return fmt.Errorf("get depth: %w", err)
	}
	fmt.Fprintf(w, "\n== order book depth (market=%s) ==\n", market)
	for _, d := range depths {
		fmt.Fprintf(w, "  %s\n", d.Symbol)
		n := len(d.Asks)
		if len(d.Bids) > n {
			n = len(d.Bids)
		}
		fmt.Fprintf(w, "  %-10s %10s %10s   %10s %10s\n", "BID", "SIZE", "COUNT", "ASK", "SIZE")
		for i := 0; i < n && i < 10; i++ {
			var bp, bs, bc, ap, as string
			if i < len(d.Bids) {
				bp, bs, bc = fmt.Sprintf("%.4f", d.Bids[i].Price), fmt.Sprintf("%d", d.Bids[i].Volume), fmt.Sprintf("%d", d.Bids[i].Count)
			} else {
				bp, bs, bc = "-", "-", "-"
			}
			if i < len(d.Asks) {
				ap, as = fmt.Sprintf("%.4f", d.Asks[i].Price), fmt.Sprintf("%d", d.Asks[i].Volume)
			} else {
				ap, as = "-", "-"
			}
			fmt.Fprintf(w, "  %-10s %10s %10s   %10s %10s\n", bp, bs, bc, ap, as)
		}
	}
	return nil
}

func msToTime(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
