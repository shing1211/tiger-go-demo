package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"

	"github.com/shing1211/tiger-go-demo/internal/rocli"
)

// These cover the renderers that were split from their SDK calls so they can be
// reached without a live account. Every other renderer in this package is still
// untested, and the README says so by name — see "cmd/corporate, cmd/futures and
// cmd/reference are at 0.0%".
//
// Nothing here makes a request. The rendering is exercised against hand-built
// model values; the SDK call needs real credentials, which this project does not
// have.

// capture redirects the package-level out for the duration of one call and
// returns what was written. Every printer in this package writes to out rather
// than taking a writer, so this is the seam that makes them reachable at all.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	saved := out
	out = &buf
	defer func() { out = saved }()
	fn()
	return buf.String()
}

// TestPrintTradeRank covers the split from opTradeRank.
//
// The two cases that matter are the ones this project's honesty rules are about:
// an empty result must SAY it is empty rather than printing an empty table, which
// reads like a query that returned nothing because the market was shut; and a
// truncated result must say how many rows were dropped, so a capped list never
// looks complete.
func TestPrintTradeRank(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printTradeRank(out, "US", []sdkmodel.TradeRankItem{
				{Symbol: "NVDA", Name: "NVIDIA Corp", LatestPr: 141.32, ChangeRate: 8.42, Volume: 41000000, Amount: 5.7e9},
				{Symbol: "AAPL", Name: "Apple Inc", LatestPr: 187.25, ChangeRate: 1.10, Volume: 51000000, Amount: 9.4e9},
			}, 20)
		})
		for _, want := range []string{"NVDA", "NVIDIA Corp", "141.3200", "8.42%", "AAPL", "187.2500"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printTradeRank(out, "US", nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty ranking should say so rather than print a bare header; got:\n%s", got)
		}
	})

	t.Run("truncation is stated", func(t *testing.T) {
		got := capture(t, func() {
			printTradeRank(out, "US", []sdkmodel.TradeRankItem{
				{Symbol: "AAA", LatestPr: 1}, {Symbol: "BBB", LatestPr: 2}, {Symbol: "CCC", LatestPr: 3},
			}, 2)
		})
		if !strings.Contains(got, "AAA") || !strings.Contains(got, "BBB") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "CCC") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
		// rocli.Truncate's wording, so the note is the same one every other
		// command emits and a reader who has seen one has seen all of them.
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated result should say how many rows were dropped; got:\n%s", got)
		}
	})

	t.Run("missing name is dashed, not blank", func(t *testing.T) {
		got := capture(t, func() {
			printTradeRank(out, "US", []sdkmodel.TradeRankItem{{Symbol: "ZZZ"}}, 20)
		})
		if !strings.Contains(got, "-") {
			t.Errorf("an absent name should render as a dash; got:\n%s", got)
		}
	})
}

// TestPrintTimelineHistory covers the split from opTimelineHistory.
//
// The absent-versus-zero distinction is the reason this printer is worth a test:
// a nil bucket means the server sent no such block, and a bucket with no items
// means the block was sent and empty. Those are different facts and the output
// has to keep them apart.
func TestPrintTimelineHistory(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		// Time is epoch milliseconds, rendered by rocli.MSFmt, so the expected
		// strings below are built the same way rather than hard-coded. A
		// hard-coded timestamp here would pass or fail on the machine's
		// timezone.
		at := func(clock string) int64 {
			t.Helper()
			ts, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-06-19 "+clock+":00", time.Local)
			if err != nil {
				t.Fatalf("parse %q: %v", clock, err)
			}
			return ts.UnixMilli()
		}
		got := capture(t, func() {
			printTimelineHistory(out, []sdkmodel.Timeline{
				{Symbol: "AAPL", Period: "2026-06-19", PreClose: 187.10,
					PreHours:   &sdkmodel.TimelineBucket{Items: []sdkmodel.TimelineItem{{Time: at("04:00"), Price: 187.20}}},
					Intraday:   &sdkmodel.TimelineBucket{Items: []sdkmodel.TimelineItem{{Time: at("09:30"), Price: 187.30}}},
					AfterHours: &sdkmodel.TimelineBucket{Items: []sdkmodel.TimelineItem{{Time: at("16:00"), Price: 187.40}}},
				},
			})
		})
		for _, want := range []string{"AAPL", "2026-06-19", "187.1000", "pre_hours", "intraday", "after_hours"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
		if !strings.Contains(got, rocli.MSFmt(at("04:00"))) {
			t.Errorf("output should contain the rendered point time %q, got:\n%s",
				rocli.MSFmt(at("04:00")), got)
		}
	})

	t.Run("absent block is distinguished from an empty one", func(t *testing.T) {
		got := capture(t, func() {
			printTimelineHistory(out, []sdkmodel.Timeline{
				// AfterHours is nil: the server sent no such block. Intraday is
				// present but empty: the block arrived with nothing in it.
				{Symbol: "AAPL", Period: "p", PreClose: 1,
					Intraday: &sdkmodel.TimelineBucket{}},
			})
		})
		if !strings.Contains(got, "after_hours") {
			t.Errorf("an absent block should still be named, so its absence is visible; got:\n%s", got)
		}
		if !strings.Contains(got, "intraday") {
			t.Errorf("an empty block should be named too; got:\n%s", got)
		}
	})

	t.Run("empty list says so", func(t *testing.T) {
		got := capture(t, func() { printTimelineHistory(out, nil) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty timeline should say so; got:\n%s", got)
		}
	})
}
