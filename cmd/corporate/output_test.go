package main

import (
	"bytes"
	"strings"
	"testing"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
)

// capture redirects the package-level out for the duration of one call and
// returns what was written. printWarrants already took model values, so it needed
// no split to become reachable -- only a test.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	saved := out
	out = &buf
	defer func() { out = saved }()
	fn()
	return buf.String()
}

// TestPrintWarrants covers the truncation contract: -limit caps the rows printed
// AND says how many were dropped, so a capped list never looks complete. That is
// the property worth pinning, because a silently short list reads as a list.
func TestPrintWarrants(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printWarrants([]sdkmodel.WarrantBrief{
				{Symbol: "12345.HK", Name: "XYZ Warrant", LatestPrice: 0.12, ChangeRate: 5.5,
					Volume: 1000, Underlying: "700", ExpiryDate: "2026-12-30"},
			}, 20)
		})
		for _, want := range []string{"12345.HK", "XYZ Warrant", "0.1200", "5.50%", "700", "2026-12-30"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("truncation is stated", func(t *testing.T) {
		got := capture(t, func() {
			printWarrants([]sdkmodel.WarrantBrief{
				{Symbol: "AAA"}, {Symbol: "BBB"}, {Symbol: "CCC"},
			}, 2)
		})
		if !strings.Contains(got, "AAA") || !strings.Contains(got, "BBB") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		// Matched as a field, not as a substring: a one-character symbol would
		// match the C in EXPIRY and in any other column heading, so the
		// assertion would pass or fail for a reason unrelated to truncation.
		if strings.Contains(got, "CCC") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated result should say how many rows were dropped; got:\n%s", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printWarrants(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty result should say so rather than print a bare header; got:\n%s", got)
		}
	})
}

func TestPrintSymbolChanges(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printSymbolChanges([]sdkmodel.CorporateSymbolChange{
				{Symbol: "AAPL", ActionType: "RENAME", ExecuteDate: "2025-01-17", OldSymbol: "Apple", NewSymbol: "Apple Inc."},
			}, 20)
		})
		if !strings.Contains(got, "Apple") {
			t.Errorf("output should contain Apple, got:\n%s", got)
		}
		if !strings.Contains(got, "Apple Inc.") {
			t.Errorf("output should contain 'Apple Inc.', got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printSymbolChanges(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printSymbolChanges([]sdkmodel.CorporateSymbolChange{
				{Symbol: "AAA", OldSymbol: "AAA", NewSymbol: "AAA_NEW"},
				{Symbol: "BBB", OldSymbol: "BBB", NewSymbol: "BBB_NEW"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintDelistings(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printDelistings([]sdkmodel.CorporateDelisting{
				{Symbol: "XYZ", ActionType: "VOLUNTARY", ExecuteDate: "2025-01-17"},
			}, 20)
		})
		if !strings.Contains(got, "XYZ") {
			t.Errorf("output should contain XYZ, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printDelistings(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printDelistings([]sdkmodel.CorporateDelisting{
				{Symbol: "AAA"},
				{Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintCapitalFlow(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printCapitalFlow(&sdkmodel.CapitalFlow{
				Items: []sdkmodel.CapitalFlowItem{
					{Time: "2025-01-17 10:00:00", NetInflow: 12345.0, Timestamp: 1705488000},
				},
			}, 20)
		})
		if !strings.Contains(got, "12345") {
			t.Errorf("output should contain 12345, got:\n%s", got)
		}
	})

	t.Run("empty/nil", func(t *testing.T) {
		got := capture(t, func() { printCapitalFlow(nil, 20) })
		if !strings.Contains(got, "no data") {
			t.Errorf("nil flow should say 'no data returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printCapitalFlow(&sdkmodel.CapitalFlow{
				Items: []sdkmodel.CapitalFlowItem{
					{Time: "AAA"},
					{Time: "BBB"},
				},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintCapitalDistribution(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printCapitalDistribution(&sdkmodel.CapitalDistribution{
				NetInflow: 1000000.0,
			})
		})
		if !strings.Contains(got, "1000000") {
			t.Errorf("output should contain 1000000, got:\n%s", got)
		}
	})

	t.Run("empty/nil", func(t *testing.T) {
		got := capture(t, func() { printCapitalDistribution(nil) })
		if !strings.Contains(got, "no data") {
			t.Errorf("nil should say 'no data returned', got:\n%s", got)
		}
	})
}

func TestPrintIPOs(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printIPOs([]sdkmodel.CorporateIPO{
				{Symbol: "RIVN", IpoName: "Rivian", ListingPrice: 78.0, ListingDate: "2025-01-17"},
			}, 20)
		})
		if !strings.Contains(got, "RIVN") {
			t.Errorf("output should contain RIVN, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printIPOs(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printIPOs([]sdkmodel.CorporateIPO{
				{Symbol: "AAA"},
				{Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFundSymbols(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFundSymbols([]string{"AAPL"}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain AAPL, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printFundSymbols(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printFundSymbols([]string{"AAA", "BBB"}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFundContracts(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFundContracts([]sdkmodel.FundContractInfo{
				{Symbol: "AAPL", Name: "Class A", NetAssetVal: 100.0},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain AAPL, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printFundContracts(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printFundContracts([]sdkmodel.FundContractInfo{
				{Symbol: "AAA"},
				{Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFundQuotes(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFundQuotes([]sdkmodel.FundQuote{
				{Symbol: "AAPL", LatestNav: 150.5, ChangeRate: 0.025},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain AAPL, got:\n%s", got)
		}
		if !strings.Contains(got, "150.5000") {
			t.Errorf("output should contain 150.5000, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printFundQuotes(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printFundQuotes([]sdkmodel.FundQuote{
				{Symbol: "AAA"},
				{Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFundHistory(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFundHistory([]sdkmodel.FundHistoryQuote{
				{Date: "2025-01-17", Nav: 150.5},
			}, 20)
		})
		if !strings.Contains(got, "2025-01-17") {
			t.Errorf("output should contain 2025-01-17, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printFundHistory(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printFundHistory([]sdkmodel.FundHistoryQuote{
				{Symbol: "AAA", Date: "2025-01-17"},
				{Symbol: "BBB", Date: "2025-01-18"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintCorporateActions(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printCorporateActions([]sdkmodel.CorporateAction{
				{Symbol: "AAPL", ActionType: "CASH_DIVIDEND", RecordDate: "2025-01-17", PayDate: "2025-01-24", Amount: 0.24},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain AAPL, got:\n%s", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := capture(t, func() { printCorporateActions(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("empty result should say 'no rows returned', got:\n%s", got)
		}
	})

	t.Run("limit", func(t *testing.T) {
		got := capture(t, func() {
			printCorporateActions([]sdkmodel.CorporateAction{
				{Symbol: "AAA", ActionType: "CASH_DIVIDEND"},
				{Symbol: "BBB", ActionType: "CASH_DIVIDEND"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("row within limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("row past limit should not print; got:\n%s", got)
		}
	})
}
