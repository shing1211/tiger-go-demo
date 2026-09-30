package main

import (
	"bytes"
	"strings"
	"testing"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"

	"github.com/shing1211/tiger-go-demo/internal/rocli"
)

func capture(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	saved := out
	out = &buf
	defer func() { out = saved }()
	fn()
	return buf.String()
}

func TestPrintChains(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printChains([]sdkmodel.OptionChain{
				{
					Symbol: "AAPL",
					Expiry: 1767225600000,
					Items: []sdkmodel.OptionChainRow{
						{
							Put:  &sdkmodel.OptionLeg{Identifier: "AAPL 250117P00150000", Strike: "150.0000", BidPrice: 5.10, AskPrice: 5.20, OpenInterest: 1200},
							Call: &sdkmodel.OptionLeg{Identifier: "AAPL 250117C00150000", Strike: "150.0000", BidPrice: 8.30, AskPrice: 8.50, OpenInterest: 980},
						},
						{
							Put:  &sdkmodel.OptionLeg{Identifier: "AAPL 250117P00160000", Strike: "160.0000", BidPrice: 10.50, AskPrice: 10.80, OpenInterest: 540},
							Call: &sdkmodel.OptionLeg{Identifier: "AAPL 250117C00160000", Strike: "160.0000", BidPrice: 3.20, AskPrice: 3.30, OpenInterest: 2100},
						},
					},
				},
			}, options{Common: rocli.Common{Limit: 20}})
		})
		for _, want := range []string{"AAPL", "150.0000", "5.1000", "8.3000", "5.2000", "8.5000", "1200", "980"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("absent leg is not a leg priced at zero", func(t *testing.T) {
		got := capture(t, func() {
			printChains([]sdkmodel.OptionChain{
				{
					Symbol: "TSLA",
					Expiry: 1767225600000,
					Items: []sdkmodel.OptionChainRow{
						{Put: nil, Call: &sdkmodel.OptionLeg{Identifier: "TSLA 250117C00200000", Strike: "200.0000", BidPrice: 2.00, AskPrice: 2.10, OpenInterest: 500}},
					},
				},
			}, options{Common: rocli.Common{Limit: 20}})
		})
		if !strings.Contains(got, "put_oi=0") {
			t.Errorf("an absent put should print put_oi=0, got:\n%s", got)
		}
		if !strings.Contains(got, "call_oi=500") {
			t.Errorf("the call leg should still print its own OI; got:\n%s", got)
		}
		if !strings.Contains(got, "-") {
			t.Errorf("an absent put bid should be a dash, got:\n%s", got)
		}
	})

	t.Run("truncation is stated", func(t *testing.T) {
		got := capture(t, func() {
			printChains([]sdkmodel.OptionChain{
				{
					Symbol: "SPY",
					Expiry: 1767225600000,
					Items: []sdkmodel.OptionChainRow{
						{Put: &sdkmodel.OptionLeg{Identifier: "SPY P1", Strike: "1", BidPrice: 1, AskPrice: 1, OpenInterest: 1}, Call: &sdkmodel.OptionLeg{Identifier: "SPY C1", Strike: "1", BidPrice: 1, AskPrice: 1, OpenInterest: 1}},
						{Put: &sdkmodel.OptionLeg{Identifier: "SPY P2", Strike: "2", BidPrice: 2, AskPrice: 2, OpenInterest: 2}, Call: &sdkmodel.OptionLeg{Identifier: "SPY C2", Strike: "2", BidPrice: 2, AskPrice: 2, OpenInterest: 2}},
						{Put: &sdkmodel.OptionLeg{Identifier: "SPY P3", Strike: "3", BidPrice: 3, AskPrice: 3, OpenInterest: 3}, Call: &sdkmodel.OptionLeg{Identifier: "SPY C3", Strike: "3", BidPrice: 3, AskPrice: 3, OpenInterest: 3}},
					},
				},
			}, options{Common: rocli.Common{Limit: 2}})
		})
		if strings.Contains(got, "SPY P3") || strings.Contains(got, "SPY C3") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated result should say how many rows were dropped; got:\n%s", got)
		}
	})

	t.Run("-limit 0 drops nothing here", func(t *testing.T) {
		got := capture(t, func() {
			printChains([]sdkmodel.OptionChain{
				{
					Symbol: "NVDA",
					Expiry: 1767225600000,
					Items: []sdkmodel.OptionChainRow{
						{Put: &sdkmodel.OptionLeg{Identifier: "NVDA P1", Strike: "1", BidPrice: 1, AskPrice: 1, OpenInterest: 1}, Call: &sdkmodel.OptionLeg{Identifier: "NVDA C1", Strike: "1", BidPrice: 1, AskPrice: 1, OpenInterest: 1}},
						{Put: &sdkmodel.OptionLeg{Identifier: "NVDA P2", Strike: "2", BidPrice: 2, AskPrice: 2, OpenInterest: 2}, Call: &sdkmodel.OptionLeg{Identifier: "NVDA C2", Strike: "2", BidPrice: 2, AskPrice: 2, OpenInterest: 2}},
					},
				},
			}, options{Common: rocli.Common{Limit: 0}})
		})
		if !strings.Contains(got, "2 strike row(s)") {
			t.Errorf("all rows should print when limit is 0; got:\n%s", got)
		}
		if strings.Contains(got, "more row(s) not shown") {
			t.Errorf("limit 0 should not emit a truncation note; got:\n%s", got)
		}
	})

	t.Run("greeks print only when asked", func(t *testing.T) {
		leg := sdkmodel.OptionLeg{Identifier: "AAPL 250117C00150000", Strike: "150.0000", BidPrice: 8.30, AskPrice: 8.50, OpenInterest: 980, Delta: 0.5123, Gamma: 0.0314, Theta: -0.0401, Vega: 0.1832, Rho: 0.0210, ImpliedVol: 0.2840, MarkPrice: 8.40}
		gotWithout := capture(t, func() {
			printChains([]sdkmodel.OptionChain{{Symbol: "AAPL", Expiry: 1767225600000, Items: []sdkmodel.OptionChainRow{{Call: &leg}}}}, options{Common: rocli.Common{Limit: 20}, greeks: false})
		})
		if strings.Contains(gotWithout, "delta=") {
			t.Errorf("greeks should not print when greeks flag is false; got:\n%s", gotWithout)
		}

		gotWith := capture(t, func() {
			printChains([]sdkmodel.OptionChain{{Symbol: "AAPL", Expiry: 1767225600000, Items: []sdkmodel.OptionChainRow{{Call: &leg}}}}, options{Common: rocli.Common{Limit: 20}, greeks: true})
		})
		if !strings.Contains(gotWith, "delta=") {
			t.Errorf("greeks should print when greeks flag is true; got:\n%s", gotWith)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() {
			printChains(nil, options{Common: rocli.Common{Limit: 20}})
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty chain list should say so; got:\n%s", got)
		}
	})
}

func TestPrintExpiration(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printExpiration([]sdkmodel.OptionExpiration{
				{
					Symbol:        "AAPL",
					Dates:         []string{"2025-01-17", "2025-01-24"},
					Periods:       []string{"w1", "w2"},
					Counts:        []int{42, 7},
					OptionSymbols: []string{"AAPL 250117C00150000", "AAPL 250124C00150000"},
				},
			}, 20)
		})
		for _, want := range []string{"AAPL", "2025-01-17", "42 contract(s)"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printExpiration(nil, 20)
		})
		if got != "  (no rows returned)\n" {
			t.Errorf("empty exps should print %q, got:\n%s", "  (no rows returned)\n", got)
		}
	})

	t.Run("limit_truncation", func(t *testing.T) {
		got := capture(t, func() {
			printExpiration([]sdkmodel.OptionExpiration{
				{
					Symbol:        "AAPL",
					Dates:         []string{"2025-01-17", "2025-01-24"},
					Periods:       []string{"w1", "w2"},
					Counts:        []int{42, 7},
					OptionSymbols: []string{"AAPL 250117C00150000", "AAPL 250124C00150000"},
				},
			}, 1)
		})
		if !strings.Contains(got, "2025-01-17") {
			t.Errorf("output should contain %q, got:\n%s", "2025-01-17", got)
		}
		if strings.Contains(got, "2025-01-24") {
			t.Errorf("output should NOT contain %q, got:\n%s", "2025-01-24", got)
		}
	})
}

func TestPrintGreeks(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printGreeks("put ", sdkmodel.OptionLeg{
				Identifier: "AAPL 250117P00150000", Strike: "150.0000",
				BidPrice: 5.10, AskPrice: 5.20, OpenInterest: 1200,
				Delta: 0.3000, Gamma: 0.0400, Theta: -0.0500,
				Vega: 0.2000, Rho: -0.0100, ImpliedVol: 0.2500, MarkPrice: 5.15,
			})
		})
		for _, want := range []string{
			"put ", "AAPL 250117P00150000",
			"delta=0.3000", "gamma=0.040000", "theta=-0.0500",
			"vega=0.2000", "rho=-0.0100", "iv=0.2500", "mark=5.1500",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("missing identifier is dashed", func(t *testing.T) {
		got := capture(t, func() {
			printGreeks("call", sdkmodel.OptionLeg{
				Identifier:   "",
				Strike:       "200.0000",
				BidPrice:     0,
				AskPrice:     0,
				OpenInterest: 0,
				Delta:        0,
				Gamma:        0,
				Theta:        0,
				Vega:         0,
				Rho:          0,
				ImpliedVol:   0,
				MarkPrice:    0,
			})
		})
		if !strings.Contains(got, "-") {
			t.Errorf("a missing identifier should render as a dash; got:\n%s", got)
		}
		if !strings.Contains(got, "call") {
			t.Errorf("the side label should appear verbatim; got:\n%s", got)
		}
		if !strings.Contains(got, "delta=0.0000") {
			t.Errorf("a zero delta should print as 0.0000, not a dash; got:\n%s", got)
		}
	})

	t.Run("the caller owns the label padding", func(t *testing.T) {
		gotPut := capture(t, func() {
			printGreeks("put ", sdkmodel.OptionLeg{Identifier: "X", Strike: "1", Delta: 1, Gamma: 0, Theta: 0, Vega: 0, Rho: 0, ImpliedVol: 0, MarkPrice: 0})
		})
		gotCall := capture(t, func() {
			printGreeks("call", sdkmodel.OptionLeg{Identifier: "X", Strike: "1", Delta: 1, Gamma: 0, Theta: 0, Vega: 0, Rho: 0, ImpliedVol: 0, MarkPrice: 0})
		})
		if !strings.Contains(gotPut, "put ") || !strings.Contains(gotCall, "call") {
			t.Errorf("the printer should emit the label exactly as given; got put:\n%s\ncall:\n%s", gotPut, gotCall)
		}
	})
}

func TestPrintOptionBriefs(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printOptionBriefs([]sdkmodel.Brief{
				{
					Symbol:      "AAPL 250117C200",
					Right:       "C",
					Strike:      "200.0000",
					LatestPrice: 12.5,
					BidPrice:    12.0,
					AskPrice:    13.0,
					Volume:      1234,
				},
			}, 20)
		})
		if !strings.Contains(got, "AAPL 250117C200") {
			t.Errorf("output should contain symbol, got:\n%s", got)
		}
		if !strings.Contains(got, "12.5000") {
			t.Errorf("output should contain formatted price 12.5000, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printOptionBriefs(nil, 20)
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty briefs should say 'no rows', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printOptionBriefs([]sdkmodel.Brief{
				{Symbol: "AAPL 250117C200", Right: "C", Strike: "200.0000", LatestPrice: 12.5, BidPrice: 12.0, AskPrice: 13.0, Volume: 1234},
				{Symbol: "AAPL 250117C250", Right: "C", Strike: "250.0000", LatestPrice: 8.5, BidPrice: 8.0, AskPrice: 9.0, Volume: 5678},
			}, 1)
		})
		if !strings.Contains(got, "AAPL 250117C200") {
			t.Errorf("first brief should appear, got:\n%s", got)
		}
		if strings.Contains(got, "AAPL 250117C250") {
			t.Errorf("second brief should not appear due to limit, got:\n%s", got)
		}
	})
}

func TestPrintKlines(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printKlines([]sdkmodel.Kline{
				{
					Symbol:        "AAPL",
					NextPageToken: "",
					Items: []sdkmodel.KlineItem{
						{Time: 1737116400000, Open: 100.0, High: 101.0, Low: 99.0, Close: 100.5, Volume: 1234},
					},
				},
			}, "w1", 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain AAPL, got:\n%s", got)
		}
		if !strings.Contains(got, "100.5000") {
			t.Errorf("output should contain 100.5000, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printKlines(nil, "w1", 20)
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty klines should say 'no rows', got:\n%s", got)
		}
	})

	t.Run("limit_truncates_inner_loop", func(t *testing.T) {
		got := capture(t, func() {
			printKlines([]sdkmodel.Kline{
				{
					Symbol:        "AAPL",
					NextPageToken: "",
					Items: []sdkmodel.KlineItem{
						{Time: 1737116400000, Open: 100.0, High: 101.0, Low: 99.0, Close: 100.5, Volume: 1234},
						{Time: 1737116460000, Open: 100.5, High: 102.0, Low: 100.0, Close: 101.5, Volume: 2345},
					},
				},
			}, "w1", 1)
		})
		if !strings.Contains(got, "100.5000") {
			t.Errorf("first item should appear, got:\n%s", got)
		}
		if strings.Contains(got, "101.5000") {
			t.Errorf("second item should not appear due to limit, got:\n%s", got)
		}
	})
}

func TestPrintKlinesPlain(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printKlinesPlain([]sdkmodel.Kline{
				{
					Symbol:        "AAPL",
					NextPageToken: "",
					Items: []sdkmodel.KlineItem{
						{Time: 1737116400000, Open: 100.0, High: 101.0, Low: 99.0, Close: 100.5, Volume: 1234},
					},
				},
			}, "w1", 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain AAPL, got:\n%s", got)
		}
		if !strings.Contains(got, "100.5000") {
			t.Errorf("output should contain 100.5000, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printKlinesPlain(nil, "w1", 20)
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty klines should say 'no rows', got:\n%s", got)
		}
	})
}

func TestPrintDepth(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printDepth([]sdkmodel.Depth{
				{
					Symbol: "AAPL",
					Bids:   []sdkmodel.DepthLevel{{Price: 100.0, Volume: 10, Count: 2}},
					Asks:   []sdkmodel.DepthLevel{{Price: 101.0, Volume: 5, Count: 1}},
				},
			}, 20)
		})
		for _, want := range []string{"AAPL", "100.0000"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printDepth(nil, 20)
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty depths should say 'no rows', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printDepth([]sdkmodel.Depth{
				{
					Symbol: "AAPL",
					Bids: []sdkmodel.DepthLevel{
						{Price: 100.0, Volume: 10, Count: 2},
						{Price: 99.0, Volume: 8, Count: 1},
					},
					Asks: []sdkmodel.DepthLevel{},
				},
			}, 1)
		})
		if !strings.Contains(got, "100.0000") {
			t.Errorf("first bid should appear, got:\n%s", got)
		}
		if strings.Contains(got, "99.0000") {
			t.Errorf("second bid should not appear due to limit, got:\n%s", got)
		}
	})
}

func TestPrintTicks(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printTicks([]sdkmodel.TradeTick{
				{
					Symbol:     "AAPL",
					BeginIndex: 0,
					EndIndex:   1,
					Items:      []sdkmodel.TradeTickItem{{Time: 1737116400000, Cond: "=", Price: 100.5, Volume: 100}},
				},
			}, 20)
		})
		for _, want := range []string{"AAPL", "100.5000"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printTicks(nil, 20)
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty ticks should say 'no rows', got:\n%s", got)
		}
	})

	t.Run("limit_truncates_inner_loop", func(t *testing.T) {
		got := capture(t, func() {
			printTicks([]sdkmodel.TradeTick{
				{
					Symbol:     "AAPL",
					BeginIndex: 0,
					EndIndex:   2,
					Items: []sdkmodel.TradeTickItem{
						{Time: 1737116400000, Cond: "=", Price: 100.5, Volume: 100},
						{Time: 1737116460000, Cond: "=", Price: 101.0, Volume: 200},
					},
				},
			}, 1)
		})
		if !strings.Contains(got, "100.5000") {
			t.Errorf("first tick should appear, got:\n%s", got)
		}
		if strings.Contains(got, "101.0000") {
			t.Errorf("second tick should not appear due to limit, got:\n%s", got)
		}
	})
}

func TestPrintOptionTimeline(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printOptionTimeline([]sdkmodel.Timeline{
				{
					Symbol:   "AAPL",
					Period:   "2025-01-17",
					PreClose: 100.0,
					Intraday: &sdkmodel.TimelineBucket{
						Items: []sdkmodel.TimelineItem{{Time: 1737116400000, Price: 100.5, AvgPrice: 100.4, Volume: 1234}},
					},
				},
			}, 20)
		})
		for _, want := range []string{"AAPL", "100.5000"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() {
			printOptionTimeline(nil, 20)
		})
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty timeline should say 'no rows', got:\n%s", got)
		}
	})

	t.Run("limit_per_bucket", func(t *testing.T) {
		got := capture(t, func() {
			printOptionTimeline([]sdkmodel.Timeline{
				{
					Symbol:   "AAPL",
					Period:   "2025-01-17",
					PreClose: 100.0,
					Intraday: &sdkmodel.TimelineBucket{
						Items: []sdkmodel.TimelineItem{
							{Time: 1737116400000, Price: 100.5, AvgPrice: 100.4, Volume: 1234},
							{Time: 1737116460000, Price: 101.0, AvgPrice: 100.9, Volume: 2345},
						},
					},
				},
			}, 1)
		})
		if !strings.Contains(got, "100.5000") {
			t.Errorf("first item should appear, got:\n%s", got)
		}
		if strings.Contains(got, "101.0000") {
			t.Errorf("second item should not appear due to limit, got:\n%s", got)
		}
	})
}

// func TestPrintOptionSymbols(t *testing.T) {
// 	t.Run("populated", func(t *testing.T) {
// 		got := capture(t, func() {
// 			printOptionSymbols([]sdkmodel.OptionSymbol{
// 				{Symbol: "AAPL", Market: "STOCK", NameEN: "Apple Inc."},
// 			}, 20)
// 		})
// 		for _, want := range []string{"AAPL", "Apple Inc."} {
// 			if !strings.Contains(got, want) {
// 				t.Errorf("output should contain %q, got:\n%s", want, got)
// 			}
// 		}
// 	})

// 	t.Run("empty", func(t *testing.T) {
// 		got := capture(t, func() {
// 			printOptionSymbols(nil, 20)
// 		})
// 		if !strings.Contains(got, "no rows") {
// 			t.Errorf("empty symbols should say 'no rows', got:\n%s", got)
// 		}
// 	})

// 	t.Run("limit", func(t *testing.T) {
// 		got := capture(t, func() {
// 			printOptionSymbols([]sdkmodel.OptionSymbol{
// 				{Symbol: "AAPL", Market: "STOCK", NameEN: "Apple Inc."},
// 				{Symbol: "TSLA", Market: "STOCK", NameEN: "Tesla Inc."},
// 			}, 1)
// 		})
// 		if !strings.Contains(got, "AAPL") {
// 			t.Errorf("first symbol should appear, got:\n%s", got)
// 		}
// 		if strings.Contains(got, "TSLA") {
// 			t.Errorf("second symbol should not appear due to limit, got:\n%s", got)
// 		}
// 	})
// }

// func TestPrintAnalysis(t *testing.T) {
// 	t.Run("populated", func(t *testing.T) {
// 		got := capture(t, func() {
// 			printAnalysis([]sdkmodel.OptionAnalysis{
// 				{
// 					Symbol:           "AAPL",
// 					ImpliedVol30Days: 0.25,
// 					HisVolatility:    0.20,
// 					IvHisVRatio:      1.25,
// 					CallPutRatio:     0.8,
// 					VolatilityList:   []sdkmodel.OptionVolatilityPoint{},
// 				},
// 			}, 20)
// 		})
// 		for _, want := range []string{"AAPL", "0.2500"} {
// 			if !strings.Contains(got, want) {
// 				t.Errorf("output should contain %q, got:\n%s", want, got)
// 			}
// 		}
// 	})

// 	t.Run("empty", func(t *testing.T) {
// 		got := capture(t, func() {
// 			printAnalysis(nil, 20)
// 		})
// 		if !strings.Contains(got, "no rows") {
// 			t.Errorf("empty analysis should say 'no rows', got:\n%s", got)
// 		}
// 	})

// 	t.Run("limit_truncates_volatility_list", func(t *testing.T) {
// 		got := capture(t, func() {
// 			printAnalysis([]sdkmodel.OptionAnalysis{
// 				{
// 					Symbol:           "AAPL",
// 					ImpliedVol30Days: 0.25,
// 					HisVolatility:    0.20,
// 					IvHisVRatio:      1.25,
// 					CallPutRatio:     0.8,
// 					VolatilityList: []sdkmodel.OptionVolatilityPoint{
// 						{Timestamp: 1737116400000, ImpliedVol: 0.25, Percentile: 50, Rank: 60, HisVolatility: 0.20},
// 						{Timestamp: 1737202800000, ImpliedVol: 0.26, Percentile: 55, Rank: 65, HisVolatility: 0.21},
// 					},
// 				},
// 			}, 1)
// 		})
// 		if !strings.Contains(got, "1737116400000") {
// 			t.Errorf("first volatility point should appear, got:\n%s", got)
// 		}
// 		if strings.Contains(got, "1737202800000") {
// 			t.Errorf("second volatility point should not appear due to limit, got:\n%s", got)
// 		}
// 	})
// }
