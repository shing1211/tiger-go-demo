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
// Only three commands write: place, modify and cancel. The other
// len(readCommands) are reads — account state, order history, contracts, funds,
// transfers and option exercise — and ignore the gate. `preview` is a
// Tiger-side dry run that still costs a round trip but places nothing.
//
// The reads are split across two files on purpose. main.go holds the flag set,
// the gate and the three write paths; reads.go holds the read-only endpoints.
// No function in reads.go takes a *config.Config, which is what makes "a read
// cannot reach the gate" a structural fact rather than a convention.
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

// commands is every -command value, kept in one place so the flag help, the
// usage text and the dispatcher cannot drift apart.
//
// readCommands are safe to run in dry-run mode: they change nothing on the
// account. writeCommands are the only three that go through the gate.
var readCommands = []string{
	"assets", "positions", "orders", "active-orders", "inactive-orders", "filled-orders",
	"get-order", "order-transactions", "preview",
	"contract", "contract3", "contracts", "quote-contract", "derivative-contracts",
	"managed-accounts", "prime-assets", "aggregate-assets", "analytics-asset",
	"estimate-tradable-quantity",
	"segment-fund-available", "segment-fund-history", "fund-details", "funding-history",
	"position-transfer-records", "position-transfer-detail", "position-transfer-external-records",
	"option-exercise-check", "option-exercise-positions", "option-exercise-records",
}

var writeCommands = []string{"place", "modify", "cancel"}

func allCommands() []string {
	return append(append([]string{}, readCommands...), writeCommands...)
}

type options struct {
	command      string
	symbol       string
	symbols      string
	secType      string
	action       string
	orderType    string
	quantity     int64
	limitPrice   float64
	timeInForce  string
	market       string
	currency     string
	orderID      int64
	localOrderID int64
	limit        int
	states       string

	// Instrument coordinates, shared by the contract, order-fill and
	// tradable-quantity lookups.
	expiry string
	strike string
	right  string

	// Account / segment selectors.
	segType      string
	segTypes     string
	baseCurrency string
	fundType     string
	subAccount   string
	fromSegment  string
	toSegment    string
	transferID   string
	segment      bool

	// Option exercise inputs.
	contractID    int64
	exerciseType  string
	executingDate string
	itmRate       int
	isForce       bool

	// Filters and paging.
	sinceDate string
	toDate    string
	begin     int64
	end       int64
	start     int
	page      int
	size      int
	status    string
	orderBy   string

	confirmLive bool
	dryRunOnly  bool
	configPath  string
	verbose     bool
}

func run(args []string) error {
	fs := flag.NewFlagSet("trade", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options
	fs.StringVar(&o.command, "command", "assets", "operation: "+strings.Join(allCommands(), "|"))
	fs.StringVar(&o.symbol, "symbol", "", "symbol, e.g. AAPL")
	fs.StringVar(&o.symbols, "symbols", "AAPL", "comma-separated symbols, e.g. AAPL,MSFT")
	fs.StringVar(&o.secType, "sec-type", "STK", "security type: STK, OPT, FUT, CASH (derivative contracts: OPT, WAR, IOPT)")
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

	// Instrument coordinates for the contract, fill and quantity endpoints.
	fs.StringVar(&o.expiry, "expiry", "", "contract expiry, e.g. 20260619")
	fs.StringVar(&o.strike, "strike", "", "option strike, e.g. 200")
	fs.StringVar(&o.right, "right", "", "option right: CALL or PUT")

	// Account and segment selectors.
	fs.StringVar(&o.segType, "seg-type", "", "asset segment type filter, e.g. S, C")
	fs.StringVar(&o.segTypes, "seg-types", "", "comma-separated segment types for -command fund-details")
	fs.StringVar(&o.baseCurrency, "base-currency", "", "base currency for aggregation, e.g. USD")
	fs.StringVar(&o.fundType, "fund-type", "", "fund flow type for -command fund-details")
	fs.StringVar(&o.subAccount, "sub-account", "", "sub-account for -command analytics-asset")
	fs.StringVar(&o.fromSegment, "from-segment", "", "source segment for the segment-fund commands")
	fs.StringVar(&o.toSegment, "to-segment", "", "destination segment for the segment-fund commands")
	fs.StringVar(&o.transferID, "transfer-id", "", "position transfer id, required for -command position-transfer-detail")
	fs.BoolVar(&o.segment, "segment", false, "ask for the per-segment breakdown of -command prime-assets")

	// Option exercise inputs.
	fs.Int64Var(&o.contractID, "contract-id", 0, "option contract id, required for -command option-exercise-check")
	fs.StringVar(&o.exerciseType, "type", "", "option exercise type: Exercise or Expire")
	fs.StringVar(&o.executingDate, "executing-date", "", "exercise execution date, YYYY-MM-DD")
	fs.IntVar(&o.itmRate, "itm-rate", 0, "in-the-money rate 0-10 for -command option-exercise-check")
	fs.BoolVar(&o.isForce, "is-force", false, "force exercise; only sent when true")

	// Filters and paging.
	fs.Int64Var(&o.localOrderID, "local-order-id", 0, "account-scoped order id for -command get-order")
	fs.StringVar(&o.sinceDate, "since-date", "", "start date, YYYY-MM-DD")
	fs.StringVar(&o.toDate, "to-date", "", "end date, YYYY-MM-DD")
	fs.Int64Var(&o.begin, "begin", 0, "range start in epoch milliseconds for -command order-transactions (0 = server default)")
	fs.Int64Var(&o.end, "end", 0, "range end in epoch milliseconds for -command order-transactions (0 = server default)")
	fs.IntVar(&o.start, "start", 0, "row offset for -command fund-details (0 = from the beginning)")
	fs.IntVar(&o.page, "page", 0, "page number for paged trade endpoints (0 = server default)")
	fs.IntVar(&o.size, "size", 20, "page size for paged trade endpoints")
	fs.StringVar(&o.status, "status", "", "record status filter, e.g. New, Cancel, Success, Fail")
	fs.StringVar(&o.orderBy, "order-by", "", "sort field for -command option-exercise-records: symbol, expire_date, strike, is_call")

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

// dispatch opens the trade client and hands it to route.
func dispatch(ctx context.Context, s *tigersdk.Session, cfg *config.Config, o options) error {
	return route(ctx, s.Trade(), cfg, o)
}

// route maps -command onto an SDK call.
//
// The read-only cases come first and are listed against readCommands, so the
// table reads as a list of things that are always allowed. Only the three
// cases at the bottom of the switch are gated, and only they take a
// *config.Config — which is the same guarantee the read handlers in reads.go
// give by not accepting one.
func route(ctx context.Context, tc tradeClient, cfg *config.Config, o options) error {
	switch o.command {
	// ---- reads: none of these receives cfg, so none can reach the gate ----
	case "assets":
		return printAssets(ctx, tc, o)
	case "positions":
		return printPositions(ctx, tc, o)
	case "orders", "active-orders", "inactive-orders", "filled-orders":
		return printOrders(ctx, tc, o)
	case "get-order":
		return printGetOrder(ctx, tc, o)
	case "order-transactions":
		return printOrderTransactions(ctx, tc, o)
	case "contract":
		return printContract(ctx, tc, o)
	case "contract3":
		return printContract3(ctx, tc, o)
	case "contracts":
		return printContracts(ctx, tc, o)
	case "quote-contract":
		return printQuoteContract(ctx, tc, o)
	case "derivative-contracts":
		return printDerivativeContracts(ctx, tc, o)
	case "managed-accounts":
		return printManagedAccounts(ctx, tc, o)
	case "prime-assets":
		return printPrimeAssets(ctx, tc, o)
	case "aggregate-assets":
		return printAggregateAssets(ctx, tc, o)
	case "analytics-asset":
		return printAnalyticsAsset(ctx, tc, o)
	case "estimate-tradable-quantity":
		return printEstimateTradableQuantity(ctx, tc, o)
	case "segment-fund-available":
		return printSegmentFundAvailable(ctx, tc, o)
	case "segment-fund-history":
		return printSegmentFundHistory(ctx, tc, o)
	case "fund-details":
		return printFundDetails(ctx, tc, o)
	case "funding-history":
		return printFundingHistory(ctx, tc, o)
	case "position-transfer-records":
		return printPositionTransferRecords(ctx, tc, o)
	case "position-transfer-detail":
		return printPositionTransferDetail(ctx, tc, o)
	case "position-transfer-external-records":
		return printPositionTransferExternalRecords(ctx, tc, o)
	case "option-exercise-check":
		return printOptionExerciseCheck(ctx, tc, o)
	case "option-exercise-positions":
		return printOptionExercisePositions(ctx, tc, o)
	case "option-exercise-records":
		return printOptionExerciseRecords(ctx, tc, o)
	case "preview":
		return doPreview(ctx, cfg, tc, o)
	// ---- the only three gated operations ----
	case "place":
		return doPlace(ctx, cfg, tc, o)
	case "modify":
		return doModify(ctx, cfg, tc, o)
	case "cancel":
		return doCancel(ctx, cfg, tc, o)
	default:
		return fmt.Errorf("unknown -command %q (want: %s)", o.command, strings.Join(allCommands(), ", "))
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

// printOrders serves all four order-list commands. They take the same request
// and return the same rows; the SDK reaches a different endpoint for each, so
// the choice is a parameter here instead of four near-identical handlers.
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
	switch o.command {
	case "active-orders":
		orders, err = tc.ActiveOrders(req)
	case "inactive-orders":
		orders, err = tc.InactiveOrders(req)
	case "filled-orders":
		orders, err = tc.FilledOrders(req)
	default:
		orders, err = tc.Orders(req)
	}
	if err != nil {
		return fmt.Errorf("get orders: %w", err)
	}
	fmt.Printf("== %s ==\n", o.command)
	printOrderHeader()
	for _, ord := range orders {
		printOrderRow(ord)
	}
	return nil
}

// The two formats below share one column layout so the header and the rows
// cannot drift apart: ID, SYMBOL, ACTION, TYPE, QTY, FILLED, LIMIT, STATUS.
const (
	orderHeaderFormat = "  %-12s %-10s %-5s %-8s %8s %8s %10s %-12s\n"
	orderRowFormat    = "  %-12d %-10s %-5s %-8s %8d %8d %10.4f %-12s\n"
)

func printOrderHeader() {
	fmt.Printf(orderHeaderFormat, "ID", "SYMBOL", "ACTION", "TYPE", "QTY", "FILLED", "LIMIT", "STATUS")
}

func printOrderRow(ord sdkmodel.Order) {
	fmt.Printf(orderRowFormat, ord.ID, ord.Symbol, ord.Action, ord.OrderType,
		ord.TotalQuantity, ord.FilledQuantity, ord.LimitPrice, ord.Status)
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
//
// The six TradeClient methods that mutate the account (PlaceForexOrder,
// TransferSegmentFund, CancelSegmentFund, TransferPosition,
// OptionExerciseSubmit, OptionExerciseCancel) are deliberately absent, as is
// SetSecretKey, which is a local field assignment rather than an API call.
// Their absence is the reason the read-only commands cannot be extended into
// writes by accident: adding one here would be a visible, reviewable change.
type tradeClient interface {
	// account state
	Assets(sdkmodel.AssetsRequest) ([]sdkmodel.Asset, error)
	PrimeAssets(sdkmodel.AssetsRequest) (*sdkmodel.PrimeAsset, error)
	AggregateAssets(sdkmodel.AggregateAssetsRequest) (*sdkmodel.AggregateAssets, error)
	AnalyticsAsset(sdkmodel.AnalyticsAssetRequest) ([]sdkmodel.AnalyticsAsset, error)
	Positions(sdkmodel.PositionsRequest) ([]sdkmodel.Position, error)

	// orders
	Orders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error)
	ActiveOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error)
	InactiveOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error)
	FilledOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error)
	GetOrder(sdkmodel.GetOrderRequest) (*sdkmodel.Order, error)
	OrderTransactions(sdkmodel.OrderTransactionsRequest) ([]sdkmodel.Transaction, error)

	// contracts
	Contract(string, string) ([]sdkmodel.Contract, error)
	Contract3(string, string) (*sdkmodel.Contract, error)
	Contracts([]string, string) ([]sdkmodel.Contract, error)
	QuoteContract(string, string, string) ([]sdkmodel.Contract, error)
	DerivativeContracts(sdkmodel.DerivativeContractsRequest) ([]sdkmodel.Contract, error)

	// accounts and funds
	ManagedAccounts(sdkmodel.ManagedAccountsRequest) ([]sdkmodel.ManagedAccount, error)
	EstimateTradableQuantity(sdkmodel.EstimateTradableQuantityRequest) (*sdkmodel.EstimateTradableQuantity, error)
	SegmentFundAvailable(sdkmodel.SegmentFundRequest) ([]sdkmodel.SegmentFundAvailableItem, error)
	SegmentFundHistory(sdkmodel.SegmentFundRequest) ([]sdkmodel.SegmentFundHistoryItem, error)
	FundDetails(sdkmodel.FundDetailsRequest) ([]sdkmodel.FundDetails, error)
	FundingHistory(sdkmodel.FundingHistoryRequest) ([]sdkmodel.FundingHistoryItem, error)

	// position transfers, read side only
	PositionTransferRecords(sdkmodel.PositionTransferRecordsRequest) ([]sdkmodel.PositionTransferRecord, error)
	PositionTransferDetail(sdkmodel.PositionTransferDetailRequest) (*sdkmodel.PositionTransferDetail, error)
	PositionTransferExternalRecords(sdkmodel.PositionTransferExternalRecordsRequest) ([]sdkmodel.PositionTransferExternalRecord, error)

	// option exercise, read side only
	OptionExerciseCheck(sdkmodel.OptionExerciseCheckRequest) (*sdkmodel.OptionExerciseCheckResult, error)
	OptionExercisePositions(sdkmodel.OptionExercisePositionRequest) (*sdkmodel.OptionExercisePositionPageResult, error)
	OptionExerciseRecords(sdkmodel.OptionExercisePageRequest) (*sdkmodel.OptionExerciseRecordPageResult, error)

	// order lifecycle
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

Only three commands are writes: place, modify, cancel. Every other command is a
read, ignores the gate, and is listed below. The write gate is reached from
exactly one place — the gate() helper — and the read commands are not passed
the config it needs.

Read commands:
  account       assets, positions, prime-assets, aggregate-assets,
                analytics-asset, managed-accounts
  orders        orders, active-orders, inactive-orders, filled-orders,
                get-order, order-transactions, preview
  contracts     contract, contract3, contracts, quote-contract,
                derivative-contracts
  estimates     estimate-tradable-quantity
  funds         segment-fund-available, segment-fund-history, fund-details,
                funding-history
  transfers     position-transfer-records, position-transfer-detail,
                position-transfer-external-records
  options       option-exercise-check, option-exercise-positions,
                option-exercise-records

NOT exposed, on purpose: PlaceForexOrder, TransferSegmentFund,
CancelSegmentFund, TransferPosition, OptionExerciseSubmit and
OptionExerciseCancel all mutate the account and are therefore not wired up at
all; SetSecretKey is a local field assignment, not an API call. The read side of
each of those operations (segment-fund-available, segment-fund-history,
funding-history, position-transfer-records, option-exercise-check,
option-exercise-records) IS available above.

Examples:
  go run ./cmd/trade -command assets
  go run ./cmd/trade -command positions
  go run ./cmd/trade -command active-orders
  go run ./cmd/trade -command get-order -order-id 123456
  go run ./cmd/trade -command filled-orders -states NEW,FILLED
  go run ./cmd/trade -command order-transactions -symbol AAPL
  go run ./cmd/trade -command contract -symbol AAPL
  go run ./cmd/trade -command contracts -symbols AAPL,MSFT
  go run ./cmd/trade -command quote-contract -symbol AAPL -sec-type OPT -expiry 20260619
  go run ./cmd/trade -command derivative-contracts -symbols AAPL -sec-type OPT
  go run ./cmd/trade -command managed-accounts
  go run ./cmd/trade -command aggregate-assets -base-currency USD
  go run ./cmd/trade -command analytics-asset -since-date 2025-01-01
  go run ./cmd/trade -command estimate-tradable-quantity -symbol AAPL -action BUY
  go run ./cmd/trade -command segment-fund-available
  go run ./cmd/trade -command fund-details -limit 10
  go run ./cmd/trade -command position-transfer-records
  go run ./cmd/trade -command option-exercise-positions -type Exercise

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
