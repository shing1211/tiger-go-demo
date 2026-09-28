// Command futures exercises Tiger's futures market-data surface.
//
// Every endpoint here is read-only: no order path, no trade client, and no call
// to config.Writable. Nothing it does can move a position.
//
// Futures endpoints divide into three families, and the flags follow that:
//
//   - contract metadata (current contract, all contracts, continuous contracts,
//     exchange list, trading times, historical main contracts)
//   - prices (real-time quote, k-lines, both plain and paged)
//   - microstructure (depth, trade ticks)
//
// Usage:
//
//	futures -op <endpoint> [flags]
//
// Examples:
//
//	go run ./cmd/futures -op exchange
//	go run ./cmd/futures -op contracts -exchange COMEX
//	go run ./cmd/futures -op quote -codes CLmain,ESmain
//	go run ./cmd/futures -op kline -codes CLmain -period day -limit 10
//	go run ./cmd/futures -op ticks -code CLmain -limit 5
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

// ops is the endpoint list, in one place so the flag help, the usage text and
// the dispatcher cannot drift apart.
var ops = []string{
	"exchange", "current", "contract", "contracts", "all-contracts", "continuous",
	"quote", "kline", "kline-page", "depth", "ticks", "trading-times",
	"history-main",
}

type options struct {
	rocli.Common

	op        string
	codes     string
	code      string
	exchange  string
	ftype     string
	period    string
	pageToken string
	date      string
	totalSize int
}

func run(args []string) error {
	fs := flag.NewFlagSet("futures", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options
	fs.StringVar(&o.op, "op", "exchange", "endpoint: "+strings.Join(ops, "|"))
	fs.StringVar(&o.codes, "codes", "", "comma-separated contract codes, e.g. CLmain,ESmain")
	fs.StringVar(&o.code, "code", "", "single contract code, for endpoints that take only one")
	fs.StringVar(&o.exchange, "exchange", "", "exchange code, e.g. COMEX, NYMEX, HKEX")
	fs.StringVar(&o.ftype, "type", "", "contract type for the metadata endpoints, e.g. ALL, CONTINUOUS")
	fs.StringVar(&o.period, "period", "day", "k-line period: day, week, month, year, 1m, 5m, 15m, 30m, 60m")
	fs.StringVar(&o.pageToken, "page-token", "", "continuation token for -op kline-page")
	fs.StringVar(&o.date, "date", "", "trading date for -op trading-times, YYYY-MM-DD")
	fs.IntVar(&o.totalSize, "total-size", 100, "total rows to request for -op kline-page")
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

	ctx, cancel := rocli.Context(90 * time.Second)
	defer cancel()

	return dispatch(ctx, env, o)
}

func dispatch(ctx context.Context, env *rocli.Env, o options) error {
	qc := env.Quote()

	switch o.op {
	case "exchange":
		return opExchange(ctx, qc, o)
	case "current":
		return opCurrent(ctx, qc, o)
	case "contract":
		return opContract(ctx, qc, o)
	case "contracts":
		return opContracts(ctx, qc, o)
	case "all-contracts":
		return opAllContracts(ctx, qc, o)
	case "continuous":
		return opContinuous(ctx, qc, o)
	case "quote":
		return opQuote(ctx, qc, o)
	case "kline":
		return opKline(ctx, qc, o)
	case "kline-page":
		return opKlinePage(ctx, qc, o)
	case "depth":
		return opDepth(ctx, qc, o)
	case "ticks":
		return opTicks(ctx, qc, o)
	case "trading-times":
		return opTradingTimes(ctx, qc, o)
	case "history-main":
		return opHistoryMain(ctx, qc, o)
	default:
		return fmt.Errorf("unknown -op %q (want: %s)", o.op, strings.Join(ops, ", "))
	}
}

const usage = `futures — Tiger futures market data (read-only)

Covers the futures endpoints of the Tiger OpenAPI quote API: contract metadata
(exchange list, current contract, all contracts, continuous contracts, trading
times, historical main contracts) and market data (real-time quotes, k-lines
plain and paged, order-book depth, trade ticks).

This command never places, modifies or cancels an order, and needs no
credentials for -h. Credentials come from TIGER_ID / TIGER_PRIVATE_KEY, or from
a YAML file passed with -config.

NOTE: nothing here has been validated against the live Tiger API. A Tiger
simulated account cannot authenticate against OpenAPI at all.

Examples:
  go run ./cmd/futures -op exchange
  go run ./cmd/futures -op current -type CONTINUOUS
  go run ./cmd/futures -op contract -code CLmain
  go run ./cmd/futures -op contracts -exchange COMEX
  go run ./cmd/futures -op all-contracts -exchange COMEX -type ALL
  go run ./cmd/futures -op continuous -type ALL
  go run ./cmd/futures -op quote -codes CLmain,ESmain
  go run ./cmd/futures -op kline -codes CLmain -period day -limit 10
  go run ./cmd/futures -op kline-page -code CLmain -period day -page-size 50
  go run ./cmd/futures -op depth -codes CLmain
  go run ./cmd/futures -op ticks -code CLmain -limit 5
  go run ./cmd/futures -op trading-times -code CLmain
  go run ./cmd/futures -op history-main -codes CLmain

Flags:
`
