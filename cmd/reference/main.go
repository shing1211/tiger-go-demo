// Command reference exercises Tiger's reference and fundamental data surface.
//
// Every endpoint here is read-only: no order path, no trade client, and no call
// to config.Writable.
//
// The endpoints fall into four groups:
//
//   - symbol reference (full symbol list, symbol names, stock details, industry
//     classification, broker distribution)
//   - fundamentals (per-symbol fundamentals, financial daily/report series,
//     currency, FX rate, short interest)
//   - calendars and screens (trading calendar, market scanner, scanner tags,
//     industry list and constituents)
//   - Tiger-specific extras (quote permission, k-line quota)
//
// Usage:
//
//	reference -op <endpoint> [flags]
//
// Examples:
//
//	go run ./cmd/reference -op symbols -market US -limit 20
//	go run ./cmd/reference -op stock-details -symbols AAPL,MSFT
//	go run ./cmd/reference -op financial-daily -symbols AAPL -fields revenue,eps
//	go run ./cmd/reference -op calendar -market US -begin 2025-01-01 -end 2025-01-31
//	go run ./cmd/reference -op scanner -market US -page 1 -page-size 10
//	go run ./cmd/reference -op industry-list
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shing1211/tiger-go-demo/internal/rocli"
)

func main() { rocli.Main(func() error { return run(os.Args[1:]) }) }

// ops is the endpoint list, kept in one place so the flag help, the usage text
// and the dispatcher cannot drift apart.
var ops = []string{
	"symbols", "symbol-names", "stock-details", "stock-industry", "stock-broker",
	"stock-fundamental", "financial-daily", "financial-report", "financial-currency",
	"exchange-rate", "short-interest", "calendar", "scanner", "scanner-tags",
	"industry-list", "industry-stocks", "kline-quota", "trade-metas",
	"quote-permission", "trade-rank", "overnight", "timeline-history",
	"ticks", "timeline", "delayed", "kline-page",
}

type options struct {
	rocli.Common

	op          string
	symbols     string
	fields      string
	currencies  string
	beginDate   string
	endDate     string
	periodType  string
	period      string
	includeOTC  bool
	industryID  string
	industryLvl string
	delayMins   int
	totalSize   int
	// Scanner filters are open-ended in Tiger's API, so they arrive as JSON.
	baseFilters string
	sortJSON    string
	multiTags   string
}

func run(args []string) error {
	fs := flag.NewFlagSet("reference", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options
	fs.StringVar(&o.op, "op", "symbols", "endpoint: "+strings.Join(ops, "|"))
	fs.StringVar(&o.symbols, "symbols", "AAPL", "comma-separated symbols, e.g. AAPL,MSFT,0700.HK")
	fs.StringVar(&o.fields, "fields", "", "financial fields to request, e.g. revenue,eps,pe_ttm")
	fs.StringVar(&o.currencies, "currencies", "USD,HKD", "currency codes for -op exchange-rate")
	fs.StringVar(&o.beginDate, "begin-date", "", "start date, YYYY-MM-DD (calendar) or YYYYMMDD (exchange-rate)")
	fs.StringVar(&o.endDate, "end-date", "", "end date, YYYY-MM-DD (calendar) or YYYYMMDD (exchange-rate)")
	fs.StringVar(&o.periodType, "period-type", "", "financial report period type, e.g. annual, quarter")
	fs.StringVar(&o.period, "period", "day", "k-line period for -op kline-page: day, week, month, 1m, 5m, 60m")
	fs.IntVar(&o.totalSize, "total-size", 100, "total bars to walk for -op kline-page")
	fs.BoolVar(&o.includeOTC, "include-otc", false, "include OTC symbols in -op symbols")
	fs.StringVar(&o.industryID, "industry-id", "", "industry id for -op industry-stocks")
	fs.StringVar(&o.industryLvl, "industry-level", "", "industry level for -op industry-list, e.g. 1")
	fs.IntVar(&o.delayMins, "delay-mins", 0, "delayed-quote delay in minutes; non-zero switches -op symbols to delayed briefs")
	fs.StringVar(&o.baseFilters, "base-filters", "", "market scanner base filters, as a JSON array of objects")
	fs.StringVar(&o.sortJSON, "sort", "", "market scanner sort spec, as a JSON object, e.g. {\"field\":\"market_cap\",\"desc\":true}")
	fs.StringVar(&o.multiTags, "multi-tags", "", "scanner multi-tag fields, comma-separated")
	rocli.AddCommonFlags(fs, &o.Common)

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // -h needs no credentials
		}
		return err
	}
	if extra := fs.Args(); len(extra) > 0 {
		return fmt.Errorf("unexpected argument %q", extra[0])
	}

	env, err := rocli.Open(o.ConfigPath, o.Verbose, os.Stderr)
	if err != nil {
		return err
	}
	defer env.Close()

	ctx, cancel := rocli.Context(120 * time.Second)
	defer cancel()

	return dispatch(ctx, env, o)
}

func dispatch(ctx context.Context, env *rocli.Env, o options) error {
	qc := env.Quote()

	switch o.op {
	case "symbols":
		return opSymbols(ctx, qc, o)
	case "symbol-names":
		return opSymbolNames(ctx, qc, o)
	case "stock-details":
		return opStockDetails(ctx, qc, o)
	case "stock-industry":
		return opStockIndustry(ctx, qc, o)
	case "stock-broker":
		return opStockBroker(ctx, qc, o)
	case "stock-fundamental":
		return opStockFundamental(ctx, qc, o)
	case "financial-daily":
		return opFinancialDaily(ctx, qc, o)
	case "financial-report":
		return opFinancialReport(ctx, qc, o)
	case "financial-currency":
		return opFinancialCurrency(ctx, qc, o)
	case "exchange-rate":
		return opExchangeRate(ctx, qc, o)
	case "short-interest":
		return opShortInterest(ctx, qc, o)
	case "calendar":
		return opCalendar(ctx, qc, o)
	case "scanner":
		return opScanner(ctx, qc, o)
	case "scanner-tags":
		return opScannerTags(ctx, qc, o)
	case "industry-list":
		return opIndustryList(ctx, qc, o)
	case "industry-stocks":
		return opIndustryStocks(ctx, qc, o)
	case "kline-quota":
		return opKlineQuota(ctx, qc, o)
	case "trade-metas":
		return opTradeMetas(ctx, qc, o)
	case "quote-permission":
		return opQuotePermission(ctx, qc, o)
	case "trade-rank":
		return opTradeRank(ctx, qc, o)
	case "overnight":
		return opOvernight(ctx, qc, o)
	case "timeline-history":
		return opTimelineHistory(ctx, qc, o)
	case "ticks":
		return opTicks(ctx, qc, o)
	case "timeline":
		return opTimeline(ctx, qc, o)
	case "delayed":
		return opDelayed(ctx, qc, o)
	case "kline-page":
		return opKlinePage(ctx, qc, o)
	default:
		return fmt.Errorf("unknown -op %q (want: %s)", o.op, strings.Join(ops, ", "))
	}
}

const usage = `reference — Tiger reference and fundamental data (read-only)

Covers symbol reference (full lists, names, stock details, industry
classification, broker distribution), fundamentals (per-symbol fundamentals,
financial daily and report series, currency, FX rate, short interest), calendars
and screens (trading calendar, market scanner and its tags, industry list and
constituents), plus a few Tiger-specific extras (quote permission, k-line
quota, trade metadata, trade rank, overnight quotes, historical timeline).

This command never places, modifies or cancels an order, and needs no
credentials for -h. Credentials come from TIGER_ID / TIGER_PRIVATE_KEY, or from
a YAML file passed with -config.

NOTE: nothing here has been validated against the live Tiger API. A Tiger
simulated account cannot authenticate against OpenAPI at all. Which -fields
values a financial endpoint accepts is a Tiger-side contract; the ones in the
examples come from the SDK's documentation, not from a live response.

Examples:
  go run ./cmd/reference -op symbols -market US -limit 20
  go run ./cmd/reference -op symbol-names -market HK -limit 20
  go run ./cmd/reference -op stock-details -symbols AAPL,MSFT
  go run ./cmd/reference -op stock-industry -symbols AAPL
  go run ./cmd/reference -op stock-broker -symbol AAPL
  go run ./cmd/reference -op stock-fundamental -symbols AAPL
  go run ./cmd/reference -op financial-daily -symbols AAPL -fields revenue,eps
  go run ./cmd/reference -op financial-report -symbols AAPL -period-type annual
  go run ./cmd/reference -op financial-currency -symbols AAPL
  go run ./cmd/reference -op exchange-rate -currencies USD,HKD
  go run ./cmd/reference -op short-interest -symbols AAPL,TSLA
  go run ./cmd/reference -op calendar -market US -begin-date 2025-01-01 -end-date 2025-01-31
  go run ./cmd/reference -op scanner -market US -page 1 -page-size 10
  go run ./cmd/reference -op scanner -market US -base-filters '[{"field":"market_cap","min":10000000000}]'
  go run ./cmd/reference -op scanner-tags -market US
  go run ./cmd/reference -op industry-list -industry-level 1
  go run ./cmd/reference -op industry-stocks -industry-id 1001
  go run ./cmd/reference -op kline-quota -symbols AAPL
  go run ./cmd/reference -op quote-permission
  go run ./cmd/reference -op ticks -symbols AAPL -limit 5
  go run ./cmd/reference -op timeline -symbols AAPL
  go run ./cmd/reference -op delayed -symbols AAPL
  go run ./cmd/reference -op kline-page -symbols AAPL -period day -page-size 50

Flags:
`
