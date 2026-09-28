// Command quote fetches Tiger OpenAPI market data: real-time briefs,
// historical bars, market state and order-book depth.
//
// It is read-only. It never places, modifies or cancels an order.
//
// Usage:
//
//	quote [flags]
//
// Examples:
//
//	go run ./cmd/quote -symbols AAPL,MSFT
//	go run ./cmd/quote -symbols AAPL -klines -period day -limit 5
//	go run ./cmd/quote -symbols AAPL -depth -market US
//	go run ./cmd/quote -market-state US
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
	"github.com/shing1211/tiger-go-demo/internal/tigersdk"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		// Friendly, non-panicking exit.
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		if config.MissingCredentialErrorIs(err) {
			os.Exit(2) // configuration problem
		}
		os.Exit(1) // runtime problem
	}
}

type options struct {
	symbols     string
	klines      bool
	timeline    bool
	depth       bool
	marketState string
	period      string
	limit       int
	market      string
	configPath  string
	verbose     bool
}

func run(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("quote", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var o options
	fs.StringVar(&o.symbols, "symbols", "AAPL,MSFT", "comma-separated symbols, e.g. AAPL,MSFT,0700.HK")
	fs.BoolVar(&o.klines, "klines", false, "also fetch historical bars (k-lines)")
	fs.BoolVar(&o.timeline, "timeline", false, "also fetch intraday timeline")
	fs.BoolVar(&o.depth, "depth", false, "also fetch order-book depth")
	fs.StringVar(&o.marketState, "market-state", "", "fetch market state for a market (US, HK, SG, AU) and exit")
	fs.StringVar(&o.period, "period", "day", "k-line period: day, week, month, year, 1m, 5m, 15m, 30m, 60m")
	fs.IntVar(&o.limit, "limit", 5, "number of k-lines to print per symbol")
	fs.StringVar(&o.market, "market", "US", "market for depth queries")
	fs.StringVar(&o.configPath, "config", "", "path to a YAML config file (also $TIGER_CONFIG)")
	fs.BoolVar(&o.verbose, "v", false, "verbose logging")

	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // -h/--help is a success, and needs no credentials
		}
		return err
	}
	if extra := fs.Args(); len(extra) > 0 {
		return fmt.Errorf("unexpected argument %q (did you mean -symbols %s?)", extra[0], extra[0])
	}

	logging.SilenceSDKNoise()
	log := logging.NewFromConfig(stderr, logLevel(o.configPath, o.verbose))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := config.Load(config.Options{ConfigPath: o.configPath})
	if err != nil {
		return err
	}
	tigersdk.WarnStrayProperties(".", stderr)
	log.Info("config loaded:\n%s", cfg.Redacted())

	session, err := tigersdk.NewSession(cfg, log)
	if err != nil {
		return err
	}
	defer session.Close()

	qc := session.Quote()

	if o.marketState != "" {
		return printMarketState(ctx, qc, o.marketState, stdout)
	}

	symbols := splitSymbols(o.symbols)
	if len(symbols) == 0 {
		return errors.New("-symbols is empty; give at least one symbol")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}

	if err := printBriefs(ctx, qc, symbols, stdout); err != nil {
		return err
	}
	if o.klines {
		if err := printKlines(ctx, qc, symbols, o.period, o.limit, stdout); err != nil {
			return err
		}
	}
	if o.timeline {
		if err := printTimeline(ctx, qc, symbols, stdout); err != nil {
			return err
		}
	}
	if o.depth {
		if err := printDepth(ctx, qc, symbols, o.market, stdout); err != nil {
			return err
		}
	}
	return nil
}

const usage = `quote — Tiger OpenAPI market data (read-only)

Fetches real-time briefs, historical bars, intraday timeline, market state and
order-book depth. This command never places, modifies or cancels an order.

Credentials come from environment variables (TIGER_ID, TIGER_PRIVATE_KEY, ...)
or a YAML file passed with -config. No credentials are needed for -h.

Examples:
  go run ./cmd/quote -symbols AAPL,MSFT
  go run ./cmd/quote -symbols AAPL -klines -period day -limit 10
  go run ./cmd/quote -symbols AAPL -timeline
  go run ./cmd/quote -symbols AAPL -depth -market US
  go run ./cmd/quote -market-state US

Flags:
`

func logLevel(configPath string, verbose bool) string {
	if verbose {
		return "debug"
	}
	// Cheap pre-read of the YAML so logging works before validation runs.
	if p := config.ResolveConfigPath(configPath, os.Getenv); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if t := strings.TrimSpace(line); strings.HasPrefix(t, "log_level:") {
					return strings.TrimSpace(strings.TrimPrefix(t, "log_level:"))
				}
			}
		}
	}
	return "info"
}

func splitSymbols(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}
