// Command push subscribes to Tiger's real-time push feed (TCP + TLS +
// Protobuf) and prints the callbacks it receives.
//
// This command is read-only: push is a subscription feed and never writes
// orders. It is included because it is the only way to observe order, asset
// and position changes in real time.
//
// Run it with a timeout; it blocks until interrupted:
//
//	go run ./cmd/push -symbols AAPL,MSFT -subscribe quote,tick,account -duration 60s
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"
	sdkpb "github.com/tigerfintech/openapi-go-sdk/push/pb"

	"github.com/tchan/tiger-go-demo/internal/config"
	"github.com/tchan/tiger-go-demo/internal/logging"
	"github.com/tchan/tiger-go-demo/internal/tigersdk"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		if config.MissingCredentialErrorIs(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		symbols    string
		subscribe  string
		account    bool
		duration   time.Duration
		configPath string
		verbose    bool
	)
	fs.StringVar(&symbols, "symbols", "AAPL,MSFT", "comma-separated symbols to subscribe to")
	fs.StringVar(&subscribe, "subscribe", "quote", "comma-separated feeds: quote,tick,depth,account")
	fs.BoolVar(&account, "account", false, "subscribe to account feeds (order/position/asset); implies -subscribe account")
	fs.DurationVar(&duration, "duration", 30*time.Second, "how long to listen before exiting cleanly (0 = until Ctrl-C)")
	fs.StringVar(&configPath, "config", "", "path to a YAML config file (also $TIGER_CONFIG)")
	fs.BoolVar(&verbose, "v", false, "verbose logging")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // help needs no credentials
		}
		return err
	}

	logging.SilenceSDKNoise()
	log := logging.NewFromConfig(os.Stderr, level(configPath, verbose))

	cfg, err := config.Load(config.Options{ConfigPath: configPath})
	if err != nil {
		return err
	}
	tigersdk.WarnStrayProperties(".", os.Stderr)
	log.Info("config loaded:\n%s", cfg.Redacted())

	feeds, err := parseFeeds(subscribe, account)
	if err != nil {
		return err
	}
	syms := splitSymbols(symbols)

	pc, err := tigersdk.Push(cfg, tigersdk.PushOptions{Log: log})
	if err != nil {
		return err
	}

	pc.SetCallbacks(callbacks(log))
	if err := pc.Connect(); err != nil {
		return fmt.Errorf("connect to push server %s: %w", cfg.PushURL, err)
	}
	defer func() {
		if err := pc.Disconnect(); err != nil {
			log.Warn("disconnect: %v", err)
		}
	}()

	for _, f := range feeds {
		var err error
		switch f {
		case "quote":
			err = pc.SubscribeQuote(syms)
		case "tick":
			err = pc.SubscribeTick(syms)
		case "depth":
			err = pc.SubscribeDepth(syms)
		case "account":
			// Empty string means "use the account from config".
			if err = pc.SubscribeOrder(""); err == nil {
				err = pc.SubscribePosition("")
			}
			if err == nil {
				err = pc.SubscribeAsset("")
			}
		}
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", f, err)
		}
		log.Info("subscribed: feed=%s symbols=%v", f, syms)
	}

	// Block until the duration elapses or the user interrupts.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var timeout <-chan time.Time
	if duration > 0 {
		t := time.NewTimer(duration)
		defer t.Stop()
		timeout = t.C
		fmt.Fprintf(os.Stderr, "listening for %s (Ctrl-C to stop early)\n", duration)
	}

	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "\ninterrupted, shutting down")
		return nil
	case <-timeout:
		fmt.Fprintln(os.Stderr, "\nduration elapsed, shutting down")
		return nil
	}
}

// parseFeeds validates the -subscribe list.
func parseFeeds(s string, forceAccount bool) ([]string, error) {
	known := map[string]bool{"quote": true, "tick": true, "depth": true, "account": true}
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !known[p] {
			return nil, fmt.Errorf("unknown feed %q (want quote, tick, depth or account)", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if forceAccount && !seen["account"] {
		out = append(out, "account")
	}
	if len(out) == 0 {
		return nil, errors.New("-subscribe selected no feeds")
	}
	return out, nil
}

func callbacks(log *logging.Logger) sdkpush.Callbacks {
	return sdkpush.Callbacks{
		OnConnect:    func() { log.Info("push connected") },
		OnDisconnect: func() { log.Warn("push disconnected") },
		OnKickout:    func(m string) { log.Error("kicked out: %s", m) },
		OnError:      func(err error) { log.Error("push error: %v", err) },

		OnQuote: func(d *sdkpb.QuoteData) {
			fmt.Printf("[QUOTE] %-10s last=%.4f bid=%.4f/%d ask=%.4f/%d vol=%d status=%s\n",
				d.GetSymbol(), d.GetLatestPrice(), d.GetBidPrice(), d.GetBidSize(),
				d.GetAskPrice(), d.GetAskSize(), d.GetVolume(), d.GetMarketStatus())
		},
		OnTick: func(d sdkpush.PushTradeTick) {
			for _, t := range d.Ticks {
				fmt.Printf("[TICK]  %-10s sn=%d price=%.4f vol=%d type=%s cond=%s venue=%s\n",
					d.Symbol, t.Sn, t.Price, t.Volume, t.TickType, t.Cond, t.PartCode)
			}
		},
		OnDepth: func(d *sdkpb.QuoteDepthData) {
			// The order book arrives as parallel slices (price/volume/orderCount),
			// not as a list of structs, so index them in lockstep.
			fmt.Printf("[DEPTH] %-10s\n", d.GetSymbol())
			printBookSide("  bid", d.GetBid())
			printBookSide("  ask", d.GetAsk())
		},

		OnOrder: func(d *sdkpb.OrderStatusData) {
			fmt.Printf("[ORDER] id=%d %-10s %s %s %d/%d @%.4f status=%s%s\n",
				d.GetId(), d.GetSymbol(), d.GetAction(), d.GetOrderType(),
				d.GetFilledQuantity(), d.GetTotalQuantity(), d.GetAvgFillPrice(),
				d.GetStatus(), errorSuffix(d.GetErrorMsg()))
		},
		OnPosition: func(d *sdkpb.PositionData) {
			fmt.Printf("[POS]   %-10s qty=%d avg_cost=%.4f mkt_value=%.2f unreal_pnl=%.2f\n",
				d.GetSymbol(), d.GetPosition(), d.GetAverageCost(), d.GetMarketValue(), d.GetUnrealizedPnl())
		},
		OnAsset: func(d *sdkpb.AssetData) {
			fmt.Printf("[ASSET] account=%s ccy=%s cash=%.2f net_liq=%.2f buying_power=%.2f\n",
				d.GetAccount(), d.GetCurrency(), d.GetCashBalance(), d.GetNetLiquidation(), d.GetBuyingPower())
		},
		OnTransaction: func(d *sdkpb.OrderTransactionData) {
			fmt.Printf("[FILL]  order_id=%d %-10s %d @%.4f\n",
				d.GetOrderId(), d.GetSymbol(), d.GetFilledQuantity(), d.GetFilledPrice())
		},
	}
}

// printBookSide renders up to five levels of one side of the book.
func printBookSide(label string, book *sdkpb.QuoteDepthData_OrderBook) {
	if book == nil {
		fmt.Printf("%s (none)\n", label)
		return
	}
	prices, volumes, counts := book.GetPrice(), book.GetVolume(), book.GetOrderCount()
	n := len(prices)
	if len(volumes) < n {
		n = len(volumes)
	}
	if n == 0 {
		fmt.Printf("%s (empty)\n", label)
		return
	}
	fmt.Printf("%s %d level(s):\n", label, len(prices))
	for i := 0; i < n && i < 5; i++ {
		count := "-"
		if i < len(counts) {
			count = fmt.Sprintf("%d", counts[i])
		}
		fmt.Printf("      %10.4f  vol=%-10d orders=%s\n", prices[i], volumes[i], count)
	}
	if len(prices) > 5 {
		fmt.Printf("      ... %d more level(s)\n", len(prices)-5)
	}
}

func errorSuffix(msg string) string {
	if msg == "" {
		return ""
	}
	return " err=" + msg
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

func level(configPath string, verbose bool) string {
	if verbose {
		return "debug"
	}
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

const usage = `push — Tiger real-time push feed (read-only)

Connects to Tiger's push server over TCP + TLS and prints decoded Protobuf
callbacks. Push is a subscription feed only; it never writes orders.

Feeds: quote, tick, depth, account (account = order + position + asset).
No credentials are needed for -h.

Examples:
  go run ./cmd/push -symbols AAPL,MSFT -subscribe quote
  go run ./cmd/push -symbols AAPL -subscribe tick -duration 2m
  go run ./cmd/push -subscribe account -account -duration 5m
  go run ./cmd/push -symbols AAPL -subscribe quote,depth -v

Flags:
`
