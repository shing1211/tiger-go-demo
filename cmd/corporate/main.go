// Command corporate exercises Tiger's corporate-action and Tiger-specific
// instrument endpoints.
//
// Every endpoint here is read-only: no order path, no trade client, and no call
// to config.Writable.
//
// The endpoints fall into three groups:
//
//   - corporate actions (dividends, splits, the earnings calendar, IPOs, symbol
//     changes, delistings, and the combined action query)
//   - capital flow (inflow/outflow distribution by order size)
//   - other instruments (HK warrants, and mutual-fund NAV data)
//
// Usage:
//
//	corporate -op <endpoint> [flags]
//
// Examples:
//
//	go run ./cmd/corporate -op dividend -symbols AAPL -market US
//	go run ./cmd/corporate -op ipo -symbols BABA -market US
//	go run ./cmd/corporate -op capital-flow -symbol AAPL -market US
//	go run ./cmd/corporate -op warrant-quote -symbols 12345.HK
//	go run ./cmd/corporate -op fund-quote -symbols 00001.HK
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
	"dividend", "split", "earnings", "ipo", "symbol-change", "delisting", "actions",
	"capital-flow", "capital-distribution",
	"warrant-filter", "warrant-quote",
	"fund-symbols", "fund-contracts", "fund-quote", "fund-history",
}

type options struct {
	rocli.Common

	op         string
	symbol     string
	symbols    string
	actionType string
	beginDate  string
	endDate    string
	underlying string
	issuer     string
	expireYM   string
	sortField  string
	sortDir    string
	period     string
}

func run(args []string) error {
	fs := flag.NewFlagSet("corporate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options
	fs.StringVar(&o.op, "op", "dividend", "endpoint: "+strings.Join(ops, "|"))
	fs.StringVar(&o.symbol, "symbol", "", "single symbol, for endpoints that take only one")
	fs.StringVar(&o.symbols, "symbols", "AAPL", "comma-separated symbols, e.g. AAPL,MSFT")
	fs.StringVar(&o.actionType, "action-type", "", "action type for -op actions, e.g. dividend, split, bonus")
	fs.StringVar(&o.beginDate, "begin-date", "", "range start, YYYY-MM-DD")
	fs.StringVar(&o.endDate, "end-date", "", "range end, YYYY-MM-DD")
	fs.StringVar(&o.underlying, "underlying", "", "underlying symbol for -op warrant-filter")
	fs.StringVar(&o.issuer, "issuer", "", "issuer name filter for -op warrant-filter")
	fs.StringVar(&o.expireYM, "expire-ym", "", "expiry year-month filter for -op warrant-filter, e.g. 2025-06")
	fs.StringVar(&o.sortField, "sort-field", "", "sort field for -op warrant-filter")
	fs.StringVar(&o.sortDir, "sort-dir", "", "sort direction for -op warrant-filter: asc or desc")
	fs.StringVar(&o.period, "period", "1d", "capital-flow window, e.g. 1d, 5d, 1m, 3m, 1y, ytd")
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
	case "dividend":
		return opCorporateAction(ctx, qc, o, actionDividend)
	case "split":
		return opCorporateAction(ctx, qc, o, actionSplit)
	case "earnings":
		return opCorporateAction(ctx, qc, o, actionEarnings)
	case "actions":
		return opCorporateAction(ctx, qc, o, actionAny)
	case "ipo":
		return opIPO(ctx, qc, o)
	case "symbol-change":
		return opSymbolChange(ctx, qc, o)
	case "delisting":
		return opDelisting(ctx, qc, o)
	case "capital-flow":
		return opCapitalFlow(ctx, qc, o)
	case "capital-distribution":
		return opCapitalDistribution(ctx, qc, o)
	case "warrant-filter":
		return opWarrantFilter(ctx, qc, o)
	case "warrant-quote":
		return opWarrantQuote(ctx, qc, o)
	case "fund-symbols":
		return opFundSymbols(ctx, qc, o)
	case "fund-contracts":
		return opFundContracts(ctx, qc, o)
	case "fund-quote":
		return opFundQuote(ctx, qc, o)
	case "fund-history":
		return opFundHistory(ctx, qc, o)
	default:
		return fmt.Errorf("unknown -op %q (want: %s)", o.op, strings.Join(ops, ", "))
	}
}

const usage = `corporate — Tiger corporate actions and instrument reference (read-only)

Covers corporate actions (dividends, splits, the earnings calendar, IPOs, symbol
changes, delistings and the combined action query), capital-flow distribution
by order size, Hong Kong warrants (filter and quote) and mutual-fund NAV data
(symbols, contracts, quotes, history).

This command never places, modifies or cancels an order, and needs no
credentials for -h. Credentials come from TIGER_ID / TIGER_PRIVATE_KEY, or from
a YAML file passed with -config.

NOTE: nothing here has been validated against the live Tiger API. A Tiger
simulated account cannot authenticate against OpenAPI at all. Which
-action-type values the combined action query accepts is a Tiger-side contract.

Examples:
  go run ./cmd/corporate -op dividend -symbols AAPL -market US
  go run ./cmd/corporate -op split -symbols AAPL -market US
  go run ./cmd/corporate -op earnings -symbols AAPL -market US
  go run ./cmd/corporate -op actions -symbols AAPL -market US -action-type dividend
  go run ./cmd/corporate -op ipo -symbols BABA -market US
  go run ./cmd/corporate -op symbol-change -symbols AAPL -market US
  go run ./cmd/corporate -op delisting -symbols AAPL -market US
  go run ./cmd/corporate -op capital-flow -symbol AAPL -market US -period 1d
  go run ./cmd/corporate -op capital-distribution -symbol AAPL -market US
  go run ./cmd/corporate -op warrant-filter -underlying 700 -market HK
  go run ./cmd/corporate -op warrant-quote -symbols 12345.HK
  go run ./cmd/corporate -op fund-symbols
  go run ./cmd/corporate -op fund-contracts -symbols 00001.HK
  go run ./cmd/corporate -op fund-quote -symbols 00001.HK
  go run ./cmd/corporate -op fund-history -symbols 00001.HK -limit 20

Flags:
`
