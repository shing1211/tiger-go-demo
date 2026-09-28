// Command trade reads account state and, behind a hard safety gate,
// places / modifies / cancels orders on a real Tiger account.
//
// # Safety model
//
// Tiger OpenAPI has NO paper or simulated mode: every order goes to a real,
// funded account. There are therefore two independent gates in front of every
// write, and BOTH must be satisfied:
//
//  1. TIGER_DRY_RUN must be false. It defaults to true.
//  2. --confirm-live must be passed on the command line.
//
// If either is missing, the command prints the exact request it WOULD have sent
// and exits non-zero without contacting Tiger. See internal/config.Writable,
// which is the single choke point all three write paths go through.
//
// Read-only subcommands (assets, positions, orders, preview) ignore the gate;
// `preview` is a Tiger-side dry run that still costs a round trip but places
// nothing.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
	"github.com/shing1211/tiger-go-demo/internal/tigersdk"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		switch {
		case config.MissingCredentialErrorIs(err):
			os.Exit(2)
		case config.IsSafetyError(err):
			os.Exit(3) // safety gate refused the write
		default:
			os.Exit(1)
		}
	}
}

type options struct {
	command     string
	symbol      string
	secType     string
	action      string
	orderType   string
	quantity    int64
	limitPrice  float64
	timeInForce string
	market      string
	currency    string
	orderID     int64
	limit       int
	states      string
	confirmLive bool
	dryRunOnly  bool
	configPath  string
	verbose     bool
}

func run(args []string) error {
	fs := flag.NewFlagSet("trade", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options
	fs.StringVar(&o.command, "command", "assets", "operation: assets|positions|orders|active-orders|preview|place|modify|cancel")
	fs.StringVar(&o.symbol, "symbol", "", "symbol, e.g. AAPL")
	fs.StringVar(&o.secType, "sec-type", "STK", "security type: STK, OPT, FUT, CASH")
	fs.StringVar(&o.action, "action", "BUY", "order action: BUY or SELL")
	fs.StringVar(&o.orderType, "order-type", "LMT", "order type: LMT, MKT, STP, STP_LMT")
	fs.Int64Var(&o.quantity, "quantity", 0, "order quantity in shares")
	fs.Float64Var(&o.limitPrice, "limit-price", 0, "limit price")
	fs.StringVar(&o.timeInForce, "time-in-force", "DAY", "time in force: DAY, GTC, OPG")
	fs.StringVar(&o.market, "market", "US", "market: US, HK, SG, AU")
	fs.StringVar(&o.currency, "currency", "USD", "currency")
	fs.Int64Var(&o.orderID, "order-id", 0, "global order id, required for modify and cancel")
	fs.Int64Var(&o.orderID, "id", 0, "alias for -order-id")
	fs.IntVar(&o.limit, "limit", 20, "max rows for orders/positions")
	fs.StringVar(&o.states, "states", "", "comma-separated order states, e.g. NEW,HELD")
	fs.BoolVar(&o.confirmLive, "confirm-live", false, "REQUIRED to submit a real order (plus TIGER_DRY_RUN=false)")
	fs.BoolVar(&o.dryRunOnly, "dry-run", false, "force dry-run for this invocation regardless of TIGER_DRY_RUN")
	fs.StringVar(&o.configPath, "config", "", "path to a YAML config file (also $TIGER_CONFIG)")
	fs.BoolVar(&o.verbose, "v", false, "verbose logging")

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
	log := logging.NewFromConfig(os.Stderr, logLevel(o.configPath, o.verbose))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := config.Load(config.Options{ConfigPath: o.configPath})
	if err != nil {
		return err
	}
	// A per-invocation -dry-run can only ever tighten the gate.
	if o.dryRunOnly {
		cfg.DryRun = true
	}
	tigersdk.WarnStrayProperties(".", os.Stderr)

	// Trading always needs an account, even for read-only queries.
	if err := cfg.ValidateForTrading(); err != nil {
		return err
	}

	session, err := tigersdk.NewSession(cfg, log)
	if err != nil {
		return err
	}
	defer session.Close()
	log.Info("config loaded:\n%s", cfg.Redacted())
	if !cfg.DryRun {
		log.Warn("DRY RUN IS DISABLED — orders submitted from here are REAL and will be filled.")
	}

	return dispatch(ctx, session, cfg, o)
}

// dispatch routes to the requested operation. It is table-driven over the
// read-only operations, and handles writes explicitly below.
func dispatch(ctx context.Context, s *tigersdk.Session, cfg *config.Config, o options) error {
	tc := s.Trade()

	switch o.command {
	case "assets":
		return printAssets(ctx, tc, o)
	case "positions":
		return printPositions(ctx, tc, o)
	case "orders", "active-orders":
		return printOrders(ctx, tc, o)
	case "preview":
		return doPreview(ctx, cfg, tc, o)
	case "place":
		return doPlace(ctx, cfg, tc, o)
	case "modify":
		return doModify(ctx, cfg, tc, o)
	case "cancel":
		return doCancel(ctx, cfg, tc, o)
	default:
		return fmt.Errorf("unknown -command %q (want: assets, positions, orders, active-orders, preview, place, modify, cancel)", o.command)
	}
}

// ---- read-only ----

func printAssets(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	assets, err := tc.Assets(sdkmodel.AssetsRequest{MarketValue: true})
	if err != nil {
		return fmt.Errorf("get assets: %w", err)
	}
	fmt.Println("== assets ==")
	fmt.Printf("  %-14s %-6s %14s %14s %14s %14s\n", "ACCOUNT", "CCY", "CASH", "BUYING_PWR", "NET_LIQ", "UNREAL_PNL")
	for _, a := range assets {
		fmt.Printf("  %-14s %-6s %14.2f %14.2f %14.2f %14.2f\n",
			a.Account, a.Currency, a.CashValue, a.BuyingPower, a.NetLiquidation, a.UnrealizedPnL)
		for _, seg := range a.Segments {
			fmt.Printf("    segment %-10s category=%-10s cash=%.2f available=%.2f init_margin=%.2f\n",
				seg.Account, seg.Category, seg.CashValue, seg.AvailableFunds, seg.InitMarginReq)
		}
	}
	return nil
}

func printPositions(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	positions, err := tc.Positions(sdkmodel.PositionsRequest{SecType: o.secType})
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}
	fmt.Println("== positions ==")
	fmt.Printf("  %-10s %-8s %10s %10s %12s %12s\n", "SYMBOL", "CCY", "QTY", "AVG_COST", "MKT_VALUE", "UNREAL_PNL")
	for _, p := range positions {
		fmt.Printf("  %-10s %-8s %10.2f %10.4f %12.2f %12.2f\n",
			p.Symbol, p.Currency, p.PositionQty, p.AverageCost, p.MarketValue, p.UnrealizedPnl)
	}
	return nil
}

func printOrders(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.OrdersRequest{Limit: o.limit}
	if s := strings.TrimSpace(o.states); s != "" {
		for _, st := range strings.Split(s, ",") {
			if st = strings.TrimSpace(st); st != "" {
				req.States = append(req.States, st)
			}
		}
	}
	var (
		orders []sdkmodel.Order
		err    error
	)
	if o.command == "active-orders" {
		orders, err = tc.ActiveOrders(req)
	} else {
		orders, err = tc.Orders(req)
	}
	if err != nil {
		return fmt.Errorf("get orders: %w", err)
	}
	fmt.Printf("== %s ==\n", o.command)
	fmt.Printf("  %-12s %-10s %-5s %-8s %8s %8s %10s %-12s\n",
		"ID", "SYMBOL", "ACTION", "TYPE", "QTY", "FILLED", "LIMIT", "STATUS")
	for _, ord := range orders {
		fmt.Printf("  %-12d %-10s %-5s %-8s %8d %8d %10.4f %-12s\n",
			ord.ID, ord.Symbol, ord.Action, ord.OrderType,
			ord.TotalQuantity, ord.FilledQuantity, ord.LimitPrice, ord.Status)
	}
	return nil
}

// ---- write paths ----
// Each one calls cfg.Writable() BEFORE building any request, and prints the
// would-be payload when refused.

func doPreview(ctx context.Context, cfg *config.Config, tc tradeClient, o options) error {
	order, err := buildOrder(cfg, o)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	// PreviewOrder asks Tiger to validate the order without placing it. It is a
	// read with respect to the book, so it is allowed even in dry-run mode.
	res, err := tc.PreviewOrder(order)
	if err != nil {
		return fmt.Errorf("preview order: %w", err)
	}
	fmt.Println("== order preview (nothing was placed) ==")
	fmt.Printf("  account=%s pass=%v\n", res.Account, res.IsPass)
	fmt.Printf("  commission=%.2f %s\n", res.Commission, res.CommissionCurrency)
	fmt.Printf("  init_margin=%.2f %s maint_margin=%.2f %s\n",
		res.InitMargin, res.MarginCurrency, res.MaintMargin, res.MarginCurrency)
	fmt.Printf("  excess_liquidity=%.2f available_EE=%.2f\n", res.ExcessLiquidity, res.AvailableEE)
	if res.Message != "" {
		fmt.Printf("  message: %s\n", res.Message)
	}
	return nil
}

func doPlace(ctx context.Context, cfg *config.Config, tc tradeClient, o options) error {
	order, err := buildOrder(cfg, o)
	if err != nil {
		return err
	}
	action := "place_order"
	if err := gate(cfg, o, action, order); err != nil {
		return err
	}
	res, err := tc.PlaceOrder(order)
	if err != nil {
		return fmt.Errorf("place order: %w", err)
	}
	fmt.Println("== order placed ==")
	fmt.Printf("  global_id=%d order_id=%d\n", res.ID, res.OrderID)
	for _, ord := range res.Orders {
		fmt.Printf("  %s %s %s qty=%d status=%s\n", ord.Symbol, ord.Action, ord.OrderType, ord.TotalQuantity, ord.Status)
	}
	return nil
}

func doModify(ctx context.Context, cfg *config.Config, tc tradeClient, o options) error {
	if o.orderID == 0 {
		return errors.New("-order-id (or -id) is required for -command modify")
	}
	order, err := buildOrder(cfg, o)
	if err != nil {
		return err
	}
	order.ID = o.orderID
	action := "modify_order"
	if err := gate(cfg, o, action, order); err != nil {
		return err
	}
	res, err := tc.ModifyOrder(o.orderID, order)
	if err != nil {
		return fmt.Errorf("modify order: %w", err)
	}
	fmt.Printf("== order modified: global_id=%d ==\n", res.ID)
	return nil
}

func doCancel(ctx context.Context, cfg *config.Config, tc tradeClient, o options) error {
	if o.orderID == 0 {
		return errors.New("-order-id (or -id) is required for -command cancel")
	}
	action := "cancel_order"
	if err := gate(cfg, o, action, map[string]any{"id": o.orderID}); err != nil {
		return err
	}
	res, err := tc.CancelOrder(o.orderID)
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}
	fmt.Printf("== order cancelled: global_id=%d ==\n", res.ID)
	return nil
}

// gate is the single choke point for writes. It refuses unless dry-run is off
// AND --confirm-live was given, and on refusal it dumps the exact request that
// would have been sent.
func gate(cfg *config.Config, o options, action string, payload any) error {
	if err := cfg.Writable(o.confirmLive); err != nil {
		fmt.Fprintf(os.Stderr, "\n--- WOULD HAVE SENT ---\napi:   %s\npayload: ", action)
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintf(os.Stderr, "<unencodable: %v>\n", err)
		}
		fmt.Fprintln(os.Stderr, "--- END (nothing was sent) ---")
		return err
	}
	fmt.Fprintf(os.Stderr, "confirmed: submitting %s LIVE\n", action)
	return nil
}

// tradeClient is the subset of the SDK trade client this command uses, kept as
// an interface so it can be faked in tests.
type tradeClient interface {
	Assets(sdkmodel.AssetsRequest) ([]sdkmodel.Asset, error)
	Positions(sdkmodel.PositionsRequest) ([]sdkmodel.Position, error)
	Orders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error)
	ActiveOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error)
	PreviewOrder(sdkmodel.OrderRequest) (*sdkmodel.PreviewResult, error)
	PlaceOrder(sdkmodel.OrderRequest) (*sdkmodel.PlaceOrderResult, error)
	ModifyOrder(int64, sdkmodel.OrderRequest) (*sdkmodel.OrderIDResult, error)
	CancelOrder(int64) (*sdkmodel.OrderIDResult, error)
}

func buildOrder(cfg *config.Config, o options) (sdkmodel.OrderRequest, error) {
	if o.symbol == "" {
		return sdkmodel.OrderRequest{}, errors.New("-symbol is required")
	}
	if o.quantity <= 0 {
		return sdkmodel.OrderRequest{}, errors.New("-quantity must be > 0")
	}
	action := strings.ToUpper(strings.TrimSpace(o.action))
	if action != "BUY" && action != "SELL" {
		return sdkmodel.OrderRequest{}, fmt.Errorf("invalid -action %q (want BUY or SELL)", o.action)
	}
	ot := strings.ToUpper(strings.TrimSpace(o.orderType))
	if ot != "LMT" && ot != "MKT" {
		return sdkmodel.OrderRequest{}, fmt.Errorf("invalid -order-type %q (want LMT or MKT)", o.orderType)
	}
	if ot == "LMT" && o.limitPrice <= 0 {
		return sdkmodel.OrderRequest{}, errors.New("-limit-price is required for a LMT order")
	}
	secType := strings.ToUpper(strings.TrimSpace(o.secType))

	return sdkmodel.OrderRequest{
		Account:       cfg.Account,
		Symbol:        strings.ToUpper(strings.TrimSpace(o.symbol)),
		SecType:       secType,
		Market:        strings.ToUpper(strings.TrimSpace(o.market)),
		Currency:      strings.ToUpper(strings.TrimSpace(o.currency)),
		Action:        action,
		OrderType:     ot,
		TotalQuantity: o.quantity,
		LimitPrice:    o.limitPrice,
		TimeInForce:   strings.ToUpper(strings.TrimSpace(o.timeInForce)),
		// SecretKey is only set for institutional accounts. It must NEVER be
		// sent as an empty string: Tiger rejects that with biz_param_error(1010).
		SecretKey: cfg.SecretKey,
	}, nil
}

const usage = `trade — Tiger account queries and (gated) order management

Tiger OpenAPI has NO paper or simulated mode. Every order written here goes to
a real, funded account. Two independent gates protect every write:

    1. TIGER_DRY_RUN must be false   (it defaults to TRUE)
    2. --confirm-live must be passed on the command line

If either is missing, the command prints the request it would have sent and
exits 3 without contacting Tiger.

Read-only commands (assets, positions, orders, active-orders, preview) do not
require the gate.

Examples:
  go run ./cmd/trade -command assets
  go run ./cmd/trade -command positions
  go run ./cmd/trade -command active-orders

  # Validate an order with Tiger without placing it (allowed in dry-run):
  go run ./cmd/trade -command preview -symbol AAPL -quantity 1 -limit-price 100

  # See what WOULD be sent (default dry-run, no --confirm-live):
  go run ./cmd/trade -command place -symbol AAPL -quantity 1 -limit-price 100

  # Actually place it — both gates satisfied:
  TIGER_DRY_RUN=false go run ./cmd/trade -command place -symbol AAPL \
      -quantity 1 -limit-price 100 --confirm-live

Flags:
`

func logLevel(configPath string, verbose bool) string {
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
