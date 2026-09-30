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
