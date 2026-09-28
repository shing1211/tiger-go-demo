// Command options exercises Tiger's option market-data surface.
//
// Every endpoint here is read-only. This command has no order path at all: it
// never touches the trade client and never calls config.Writable, so there is
// nothing for the dry-run gate to protect.
//
// Tiger option identifiers have the form "UNDERLYING YYMMDDC|P STRIKE", e.g.
// "AAPL 250117C00200000". Some endpoints take those identifiers directly
// (GetOptionQuote, GetOptionKlineWithOpts); the rest take an OptionQueryItem,
// so requests.go parses the identifier into symbol / expiry / right / strike
// once and reuses it.
//
// Usage:
//
//	options -op <endpoint> [flags]
//
// Examples:
//
//	go run ./cmd/options -op expiration -symbols AAPL,TSLA
//	go run ./cmd/options -op chain -symbols AAPL -expiry 2025-01-17 -greeks
//	go run ./cmd/options -op quote -ids "AAPL 250117C00200000"
//	go run ./cmd/options -op kline -ids "AAPL 250117C00200000" -period day
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
	"expiration", "chain", "quote", "kline", "kline-plain", "depth", "ticks",
	"timeline", "symbols", "analysis",
}

type options struct {
	rocli.Common

	op             string
	symbols        string
	ids            string
	expiry         string
	period         string
	greeks         bool
	itm            string
	pageToken      string
	sortDir        string
	volatilityList bool
}

func run(args []string) error {
	fs := flag.NewFlagSet("options", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options
	fs.StringVar(&o.op, "op", "expiration", "endpoint: "+strings.Join(ops, "|"))
	fs.StringVar(&o.symbols, "symbols", "AAPL", "underlying symbols, e.g. AAPL,TSLA")
	fs.StringVar(&o.ids, "ids", "", `option identifiers, e.g. "AAPL 250117C00200000,AAPL 250117P00200000"`)
	fs.StringVar(&o.expiry, "expiry", "", "expiry date for -op chain, YYYY-MM-DD (default: nearest listed expiry)")
	fs.StringVar(&o.period, "period", "day", "k-line period: day, week, month, year, 1m, 5m, 15m, 30m, 60m")
	fs.BoolVar(&o.greeks, "greeks", false, "request delta/gamma/theta/vega/rho and implied vol for -op chain")
	fs.StringVar(&o.itm, "itm", "", "option chain filter: in, out, all (default all)")
	fs.StringVar(&o.pageToken, "page-token", "", "k-line page token from a previous response")
	fs.StringVar(&o.sortDir, "sort-dir", "", "k-line sort direction, e.g. forward or backward")
	fs.BoolVar(&o.volatilityList, "volatility-list", false, "ask -op analysis for the implied-volatility series")
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
	case "expiration":
		return opExpiration(ctx, qc, o)
	case "chain":
		return opChain(ctx, qc, o)
	case "quote":
		return opQuote(ctx, qc, o)
	case "kline":
		return opKline(ctx, qc, o)
	case "kline-plain":
		return opKlinePlain(ctx, qc, o)
	case "depth":
		return opDepth(ctx, qc, o)
	case "ticks":
		return opTicks(ctx, qc, o)
	case "timeline":
		return opTimeline(ctx, qc, o)
	case "symbols":
		return opSymbols(ctx, qc, o)
	case "analysis":
		return opAnalysis(ctx, qc, o)
	default:
		return fmt.Errorf("unknown -op %q (want: %s)", o.op, strings.Join(ops, ", "))
	}
}

const usage = `options — Tiger option market data (read-only)

Covers the option endpoints of the Tiger OpenAPI quote API: expiries, chains
(plain and with Greeks), real-time quotes, k-lines, depth, trade ticks, the
intraday timeline, the symbol list and implied-volatility analytics.

This command never places, modifies or cancels an order, and needs no
credentials for -h. Credentials come from TIGER_ID / TIGER_PRIVATE_KEY, or from
a YAML file passed with -config.

Option identifiers are "UNDERLYING YYMMDDC|P STRIKE", e.g.
  "AAPL 250117C00200000"      AAPL call, 2025-01-17, strike 200
  "AAPL 250117P00200000"      AAPL put,  2025-01-17, strike 200

NOTE: nothing here has been validated against the live Tiger API. A Tiger
simulated account cannot authenticate against OpenAPI at all.

Examples:
  go run ./cmd/options -op expiration -symbols AAPL,TSLA
  go run ./cmd/options -op chain -symbols AAPL -expiry 2025-01-17
  go run ./cmd/options -op chain -symbols AAPL -greeks -itm in
  go run ./cmd/options -op quote -ids "AAPL 250117C00200000"
  go run ./cmd/options -op kline -ids "AAPL 250117C00200000" -period day -limit 20
  go run ./cmd/options -op kline-plain -ids "AAPL 250117C00200000" -period day
  go run ./cmd/options -op depth -ids "AAPL 250117C00200000"
  go run ./cmd/options -op ticks -ids "AAPL 250117C00200000" -limit 5
  go run ./cmd/options -op timeline -ids "AAPL 250117C00200000"
  go run ./cmd/options -op symbols -market US -limit 20
  go run ./cmd/options -op analysis -symbols AAPL -volatility-list

Flags:
`
