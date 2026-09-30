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
// reached without a live account. The README names the rest of the package's
// renderers in its coverage ceiling at "cmd/corporate, cmd/futures and
// cmd/reference are at 2–5%".
//
// Nothing here makes a request. The rendering is exercised against hand-built
// model values; the SDK call needs real credentials, which this project does not
// have.

// capture redirects the package-level out for the duration of one call and
// returns what was written. Most printers in this package write to out; the
// three that take a writer (printTradeRank, printTimelineHistory,
// printTimelineHistoryRows) are called with that writer inside capture so the
// same helper works for all of them.
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

func TestPrintBriefs(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printBriefs([]sdkmodel.Brief{
				{Symbol: "AAPL", LatestPrice: 187.2500, Change: 2.05, ChangeRate: 1.10, Volume: 51000000, LatestTime: 1767225600000},
				{Symbol: "NVDA", LatestPrice: 141.3200, Change: 11.10, ChangeRate: 8.52, Volume: 41000000, LatestTime: 1767225600000},
			}, 20)
		})
		for _, want := range []string{"AAPL", "NVDA", "187.2500", "141.3200", "1.10%", "8.52%"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("truncation is stated", func(t *testing.T) {
		got := capture(t, func() {
			printBriefs([]sdkmodel.Brief{
				{Symbol: "AAA", LatestPrice: 1, Change: 0, ChangeRate: 0, Volume: 1, LatestTime: 0},
				{Symbol: "BBB", LatestPrice: 2, Change: 0, ChangeRate: 0, Volume: 2, LatestTime: 0},
				{Symbol: "CCC", LatestPrice: 3, Change: 0, ChangeRate: 0, Volume: 3, LatestTime: 0},
			}, 2)
		})
		if !strings.Contains(got, "AAA") || !strings.Contains(got, "BBB") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "CCC") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated result should say how many rows were dropped; got:\n%s", got)
		}
	})

	t.Run("absent time is dashed, not 1970", func(t *testing.T) {
		got := capture(t, func() {
			printBriefs([]sdkmodel.Brief{
				{Symbol: "ZZZ", LatestPrice: 99.99, Change: 0, ChangeRate: 0, Volume: 0, LatestTime: 0},
			}, 20)
		})
		if !strings.Contains(got, "-") {
			t.Errorf("an absent LatestTime should render as a dash; got:\n%s", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printBriefs(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty briefs list should say so; got:\n%s", got)
		}
	})
}

func TestPrintBrokerSide(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printBrokerSide("bid", []sdkmodel.StockBrokerItem{
				{Level: 1, Price: 187.2500, Brokers: []sdkmodel.BrokerDetail{{ID: "b1", Name: "Goldman Sachs"}, {ID: "b2", Name: "Morgan Stanley"}}},
				{Level: 2, Price: 187.2400, Brokers: []sdkmodel.BrokerDetail{{ID: "b3", Name: "Citadel"}}},
			})
		})
		for _, want := range []string{"bid side:", "level=1", "187.2500", "Goldman Sachs", "Morgan Stanley", "level=2", "187.2400", "Citadel"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
		if !strings.Contains(got, "Goldman Sachs, Morgan Stanley") {
			t.Errorf("broker names should be joined with ', '; got:\n%s", got)
		}
	})

	t.Run("empty side says none", func(t *testing.T) {
		got := capture(t, func() { printBrokerSide("ask", nil) })
		if !strings.Contains(got, "ask: (none)") {
			t.Errorf("a nil side should say none; got:\n%s", got)
		}
	})

	t.Run("a level with no brokers still names its level and price", func(t *testing.T) {
		got := capture(t, func() {
			printBrokerSide("bid", []sdkmodel.StockBrokerItem{
				{Level: 1, Price: 187.2500, Brokers: nil},
			})
		})
		if !strings.Contains(got, "level=1") || !strings.Contains(got, "187.2500") {
			t.Errorf("level and price should print even with no brokers; got:\n%s", got)
		}
	})

	t.Run("missing broker name is dashed", func(t *testing.T) {
		got := capture(t, func() {
			printBrokerSide("bid", []sdkmodel.StockBrokerItem{
				{Level: 1, Price: 100.0000, Brokers: []sdkmodel.BrokerDetail{{ID: "", Name: ""}}},
			})
		})
		if !strings.Contains(got, "-") {
			t.Errorf("a missing broker name should render as a dash; got:\n%s", got)
		}
	})
}

func TestPrintRefSymbols(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printRefSymbols([]string{"AAPL", "TSLA"}, 20)
		})
		for _, want := range []string{"AAPL", "TSLA"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printRefSymbols(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printRefSymbols([]string{"AAA", "BBB", "CCC"}, 2)
		})
		if !strings.Contains(got, "AAA") || !strings.Contains(got, "BBB") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "CCC") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated result should say how many rows were dropped; got:\n%s", got)
		}
	})
}

func TestPrintStockIndustry(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printStockIndustry([]sdkmodel.StockIndustry{
				{Symbol: "TECH", Level: "1", GSector: "Technology", GGroup: "Hardware", GInd: "Semiconductors"},
			}, 20)
		})
		for _, want := range []string{"TECH", "Technology", "Semiconductors"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printStockIndustry(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printStockIndustry([]sdkmodel.StockIndustry{
				{Symbol: "AAA"}, {Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintScannerRows(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printScannerRows(&sdkmodel.ScannerResult{
				Page: 1, TotalPage: 1, TotalCount: 1, PageSize: 10,
				Items: []sdkmodel.ScannerResultItem{
					{Symbol: "AAPL", Market: "STOCK", BaseDataList: []sdkmodel.ScannerDataRow{{Index: 1, Name: "rank", Value: "1", Data: 1.0}}},
				},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain %q, got:\n%s", "AAPL", got)
		}
		if strings.Contains(got, "no rows returned") || strings.Contains(got, "no data returned") {
			t.Errorf("empty groups should not print 'no rows returned'; got:\n%s", got)
		}
	})

	t.Run("nil result says so", func(t *testing.T) {
		got := capture(t, func() { printScannerRows(nil, 20) })
		if !strings.Contains(got, "no data") {
			t.Errorf("nil result should say 'no data'; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printScannerRows(&sdkmodel.ScannerResult{
				Items: []sdkmodel.ScannerResultItem{
					{Symbol: "AAA"}, {Symbol: "BBB"},
				},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintCalendar(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printCalendar([]sdkmodel.TradingCalendarItem{
				{Market: "US", Date: "2025-01-17", IsTrading: true, SessionType: "regular"},
			}, 20)
		})
		for _, want := range []string{"2025-01-17", "US", "regular"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printCalendar(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printCalendar([]sdkmodel.TradingCalendarItem{
				{Market: "AAA"}, {Market: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintShortInterest(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printShortInterest([]sdkmodel.ShortInterest{
				{Symbol: "AAPL", SettlementDate: "2025-01-17", ShortInterest: 15000000, ShortInterestPrevious: 14000000, PercentOfFloat: 2.5, DaysToCover: 3.2, PercentChange: 7.1},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain %q, got:\n%s", "AAPL", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printShortInterest(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printShortInterest([]sdkmodel.ShortInterest{
				{Symbol: "AAA"}, {Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintExchangeRates(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printExchangeRates([]sdkmodel.ExchangeRate{
				{Currency: "USD", BaseCurrency: "CNY", Rate: 7.25, Date: "2025-01-17"},
			}, 20)
		})
		for _, want := range []string{"USD", "CNY", "7.250000"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printExchangeRates(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printExchangeRates([]sdkmodel.ExchangeRate{
				{Currency: "AAA"}, {Currency: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFinancialCurrencies(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFinancialCurrencies([]sdkmodel.FinancialCurrency{
				{Symbol: "AAPL", Market: "STOCK", Currency: "USD"},
			}, 20)
		})
		if !strings.Contains(got, "USD") {
			t.Errorf("output should contain %q, got:\n%s", "USD", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printFinancialCurrencies(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printFinancialCurrencies([]sdkmodel.FinancialCurrency{
				{Symbol: "AAA"}, {Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFinancialReport(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFinancialReport([]sdkmodel.FinancialReportItem{
				{Symbol: "AAPL", Field: "revenue", Currency: "USD", FilingDate: "2024-01-01", PeriodEndDate: "2023-12-31", Value: "394328000000"},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain %q, got:\n%s", "AAPL", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printFinancialReport(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printFinancialReport([]sdkmodel.FinancialReportItem{
				{Symbol: "AAA"}, {Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintFinancialDaily(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printFinancialDaily([]sdkmodel.FinancialDailyItem{
				{Symbol: "AAPL", Field: "revenue", Date: 1737116400000, Value: 123456.78},
			}, 20)
		})
		if !strings.Contains(got, "AAPL") {
			t.Errorf("output should contain %q, got:\n%s", "AAPL", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printFinancialDaily(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printFinancialDaily([]sdkmodel.FinancialDailyItem{
				{Symbol: "AAA"}, {Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintStockFundamental(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printStockFundamental(map[string]any{"market_cap": float64(2800000000000)})
		})
		if !strings.Contains(got, "market_cap") {
			t.Errorf("output should contain %q, got:\n%s", "market_cap", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printStockFundamental(nil) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty map should say so; got:\n%s", got)
		}
	})
}

func TestPrintStockDetails(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printStockDetails([]sdkmodel.StockDetail{
				{Symbol: "AAPL", NameEN: "Apple Inc.", Market: "STOCK", MarketCap: 2800000000000, PeRatioTtm: 28.5},
			}, 20)
		})
		for _, want := range []string{"AAPL", "28.5000"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printStockDetails(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printStockDetails([]sdkmodel.StockDetail{
				{Symbol: "AAA"}, {Symbol: "BBB"},
			}, 1)
		})
		if !strings.Contains(got, "AAA") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "BBB") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
	})
}

func TestPrintSymbolNames(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printSymbolNames([]sdkmodel.SymbolName{
				{Symbol: "AAPL", Name: "Apple Inc.", Market: "STOCK"},
				{Symbol: "TSLA", Name: "Tesla Inc.", Market: "STOCK"},
			}, 20)
		})
		for _, want := range []string{"AAPL", "TSLA", "Apple Inc.", "Tesla Inc.", "STOCK"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printSymbolNames(nil, 20) })
		if !strings.Contains(got, "no rows") {
			t.Errorf("an empty list should say so; got:\n%s", got)
		}
	})

	t.Run("limit truncates", func(t *testing.T) {
		got := capture(t, func() {
			printSymbolNames([]sdkmodel.SymbolName{
				{Symbol: "AAA", Name: "A", Market: "M"},
				{Symbol: "BBB", Name: "B", Market: "M"},
				{Symbol: "CCC", Name: "C", Market: "M"},
			}, 2)
		})
		if !strings.Contains(got, "AAA") || !strings.Contains(got, "BBB") {
			t.Errorf("rows within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "CCC") {
			t.Errorf("a row past the limit should not print; got:\n%s", got)
		}
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated result should say how many rows were dropped; got:\n%s", got)
		}
	})
}

func TestPrintScannerTags(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printScannerTags(out, []sdkmodel.MarketScannerTagGroup{
				{Market: "US", MultiTagField: "high_vol", TagList: []sdkmodel.MarketScannerTag{
					{Field: "HIGH_VOL", Name: "High Volatility", Values: []string{"true"}},
				}},
			})
		})
		for _, want := range []string{"HIGH_VOL", "High Volatility", "high_vol"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printScannerTags(out, nil) })
		if !strings.Contains(got, "no tags") {
			t.Errorf("an empty tags list should say so; got:\n%s", got)
		}
	})
}

func TestPrintTimelineHistoryRowsLimit(t *testing.T) {
	t.Run("truncation is stated at non-default limit", func(t *testing.T) {
		at := func(clock string) int64 {
			t.Helper()
			ts, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-06-19 "+clock+":00", time.Local)
			if err != nil {
				t.Fatalf("parse %q: %v", clock, err)
			}
			return ts.UnixMilli()
		}
		got := capture(t, func() {
			printTimelineHistoryRows(out, []sdkmodel.Timeline{
				{Symbol: "AAPL", Period: "2026-06-19", PreClose: 187.10,
					Intraday: &sdkmodel.TimelineBucket{Items: []sdkmodel.TimelineItem{
						{Time: at("09:30"), Price: 187.30, AvgPrice: 187.25, Volume: 1000},
						{Time: at("09:31"), Price: 187.35, AvgPrice: 187.28, Volume: 800},
						{Time: at("09:32"), Price: 187.40, AvgPrice: 187.30, Volume: 1200},
					}},
				},
			}, 2)
		})
		if !strings.Contains(got, "09:30") || !strings.Contains(got, "09:31") {
			t.Errorf("items within the limit should print; got:\n%s", got)
		}
		if strings.Contains(got, "09:32") {
			t.Errorf("items past the limit should not print; got:\n%s", got)
		}
		if !strings.Contains(got, "1 more row(s) not shown") {
			t.Errorf("a truncated bucket should say how many rows were dropped; got:\n%s", got)
		}
	})
}
