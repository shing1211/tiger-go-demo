package main

import (
	"context"
	"fmt"
	"io"
	"time"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"
)

func printMarketState(ctx context.Context, qc *sdkquote.QuoteClient, market string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	states, err := qc.GetMarketState(market)
	if err != nil {
		return fmt.Errorf("get market state (%s): %w", market, err)
	}
	fmt.Fprintf(w, "== market state ==\n")
	if len(states) == 0 {
		fmt.Fprintln(w, "  (no data returned)")
		return nil
	}
	for _, s := range states {
		fmt.Fprintf(w, "  %-4s status=%-12s market_status=%-12s open=%s\n",
			s.Market, s.Status, s.MarketStatus, orDash(s.OpenTime))
	}
	return nil
}

func printBriefs(ctx context.Context, qc *sdkquote.QuoteClient, symbols []string, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	briefs, err := qc.GetRealTimeQuote(sdkmodel.BriefRequest{Symbols: symbols})
	if err != nil {
		return fmt.Errorf("get real-time quote: %w", err)
	}
	fmt.Fprintf(w, "\n== real-time quotes ==\n")
	fmt.Fprintf(w, "  %-10s %10s %10s %10s %12s %10s\n", "SYMBOL", "LAST", "CHANGE", "CHG%", "VOLUME", "TIME")
	for _, b := range briefs {
		fmt.Fprintf(w, "  %-10s %10.4f %10.4f %9.2f%% %12d %10s\n",
			b.Symbol, b.LatestPrice, b.Change, b.ChangeRate, b.Volume, msToTime(b.LatestTime))
	}
	return nil
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
	fmt.Fprintf(w, "\n== k-lines (period=%s) ==\n", period)
	for _, k := range klines {
		fmt.Fprintf(w, "  %s (next_page_token=%s)\n", k.Symbol, orDash(k.NextPageToken))
		fmt.Fprintf(w, "  %-22s %10s %10s %10s %10s %12s\n", "TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME")
		for _, it := range k.Items {
			fmt.Fprintf(w, "  %-22s %10.4f %10.4f %10.4f %10.4f %12d\n",
				msToTime(it.Time), it.Open, it.High, it.Low, it.Close, it.Volume)
		}
	}
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
