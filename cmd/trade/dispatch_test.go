package main

// These tests cover the safety property that makes the read commands safe: a
// read reaches the SDK and never reaches the write gate.
//
// They run with dry-run ON and --confirm-live OFF, which is the configuration
// in which every write is refused. A read command must still return nil against
// a fake client, and must name exactly one SDK method. If any of the three
// gated operations were ever wired into a read case, or if a read case grew a
// cfg.Writable() call, these tests fail.

import (
	"context"
	"io"
	"os"

	"strings"
	"testing"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"

	"github.com/shing1211/tiger-go-demo/internal/config"
)

// fakeTrade records the name of the SDK method each command calls. Every method
// returns an empty, non-nil result so the printers run their full output path.
type fakeTrade struct {
	called string
}

func (f *fakeTrade) note(name string) { f.called = name }

func (f *fakeTrade) Assets(sdkmodel.AssetsRequest) ([]sdkmodel.Asset, error) {
	f.note("Assets")
	return []sdkmodel.Asset{{Account: "acct", Currency: "USD"}}, nil
}

func (f *fakeTrade) PrimeAssets(sdkmodel.AssetsRequest) (*sdkmodel.PrimeAsset, error) {
	f.note("PrimeAssets")
	return &sdkmodel.PrimeAsset{AccountID: "acct"}, nil
}

func (f *fakeTrade) AggregateAssets(sdkmodel.AggregateAssetsRequest) (*sdkmodel.AggregateAssets, error) {
	f.note("AggregateAssets")
	return &sdkmodel.AggregateAssets{AccountID: "acct"}, nil
}

func (f *fakeTrade) AnalyticsAsset(sdkmodel.AnalyticsAssetRequest) ([]sdkmodel.AnalyticsAsset, error) {
	f.note("AnalyticsAsset")
	return []sdkmodel.AnalyticsAsset{{Date: "2025-01-02"}}, nil
}

func (f *fakeTrade) Positions(sdkmodel.PositionsRequest) ([]sdkmodel.Position, error) {
	f.note("Positions")
	return []sdkmodel.Position{{Symbol: "AAPL"}}, nil
}

func (f *fakeTrade) Orders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error) {
	f.note("Orders")
	return []sdkmodel.Order{{ID: 1}}, nil
}

func (f *fakeTrade) ActiveOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error) {
	f.note("ActiveOrders")
	return []sdkmodel.Order{{ID: 1}}, nil
}

func (f *fakeTrade) InactiveOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error) {
	f.note("InactiveOrders")
	return []sdkmodel.Order{{ID: 1}}, nil
}

func (f *fakeTrade) FilledOrders(sdkmodel.OrdersRequest) ([]sdkmodel.Order, error) {
	f.note("FilledOrders")
	return []sdkmodel.Order{{ID: 1}}, nil
}

func (f *fakeTrade) GetOrder(sdkmodel.GetOrderRequest) (*sdkmodel.Order, error) {
	f.note("GetOrder")
	return &sdkmodel.Order{ID: 1, Symbol: "AAPL"}, nil
}

func (f *fakeTrade) OrderTransactions(sdkmodel.OrderTransactionsRequest) ([]sdkmodel.Transaction, error) {
	f.note("OrderTransactions")
	return []sdkmodel.Transaction{{OrderID: 1, Symbol: "AAPL"}}, nil
}

func (f *fakeTrade) Contract(string, string) ([]sdkmodel.Contract, error) {
	f.note("Contract")
	return []sdkmodel.Contract{{Symbol: "AAPL"}}, nil
}

func (f *fakeTrade) Contract3(string, string) (*sdkmodel.Contract, error) {
	f.note("Contract3")
	return &sdkmodel.Contract{Symbol: "AAPL"}, nil
}

func (f *fakeTrade) Contracts([]string, string) ([]sdkmodel.Contract, error) {
	f.note("Contracts")
	return []sdkmodel.Contract{{Symbol: "AAPL"}}, nil
}

func (f *fakeTrade) QuoteContract(string, string, string) ([]sdkmodel.Contract, error) {
	f.note("QuoteContract")
	return []sdkmodel.Contract{{Symbol: "AAPL"}}, nil
}

func (f *fakeTrade) DerivativeContracts(sdkmodel.DerivativeContractsRequest) ([]sdkmodel.Contract, error) {
	f.note("DerivativeContracts")
	return []sdkmodel.Contract{{Symbol: "AAPL"}}, nil
}

func (f *fakeTrade) ManagedAccounts(sdkmodel.ManagedAccountsRequest) ([]sdkmodel.ManagedAccount, error) {
	f.note("ManagedAccounts")
	return []sdkmodel.ManagedAccount{{Account: "acct"}}, nil
}

func (f *fakeTrade) EstimateTradableQuantity(sdkmodel.EstimateTradableQuantityRequest) (*sdkmodel.EstimateTradableQuantity, error) {
	f.note("EstimateTradableQuantity")
	return &sdkmodel.EstimateTradableQuantity{Currency: "USD"}, nil
}

func (f *fakeTrade) SegmentFundAvailable(sdkmodel.SegmentFundRequest) ([]sdkmodel.SegmentFundAvailableItem, error) {
	f.note("SegmentFundAvailable")
	return []sdkmodel.SegmentFundAvailableItem{{FromSegment: "S"}}, nil
}

func (f *fakeTrade) SegmentFundHistory(sdkmodel.SegmentFundRequest) ([]sdkmodel.SegmentFundHistoryItem, error) {
	f.note("SegmentFundHistory")
	return []sdkmodel.SegmentFundHistoryItem{{ID: 1}}, nil
}

func (f *fakeTrade) FundDetails(sdkmodel.FundDetailsRequest) ([]sdkmodel.FundDetails, error) {
	f.note("FundDetails")
	return []sdkmodel.FundDetails{{ID: 1}}, nil
}

func (f *fakeTrade) FundingHistory(sdkmodel.FundingHistoryRequest) ([]sdkmodel.FundingHistoryItem, error) {
	f.note("FundingHistory")
	return []sdkmodel.FundingHistoryItem{{ID: 1}}, nil
}

func (f *fakeTrade) PositionTransferRecords(sdkmodel.PositionTransferRecordsRequest) ([]sdkmodel.PositionTransferRecord, error) {
	f.note("PositionTransferRecords")
	return []sdkmodel.PositionTransferRecord{{ID: "t1"}}, nil
}

func (f *fakeTrade) PositionTransferDetail(sdkmodel.PositionTransferDetailRequest) (*sdkmodel.PositionTransferDetail, error) {
	f.note("PositionTransferDetail")
	return &sdkmodel.PositionTransferDetail{ID: "t1"}, nil
}

func (f *fakeTrade) PositionTransferExternalRecords(sdkmodel.PositionTransferExternalRecordsRequest) ([]sdkmodel.PositionTransferExternalRecord, error) {
	f.note("PositionTransferExternalRecords")
	return []sdkmodel.PositionTransferExternalRecord{{ID: "t1"}}, nil
}

func (f *fakeTrade) OptionExerciseCheck(sdkmodel.OptionExerciseCheckRequest) (*sdkmodel.OptionExerciseCheckResult, error) {
	f.note("OptionExerciseCheck")
	return &sdkmodel.OptionExerciseCheckResult{Symbol: "AAPL"}, nil
}

func (f *fakeTrade) OptionExercisePositions(sdkmodel.OptionExercisePositionRequest) (*sdkmodel.OptionExercisePositionPageResult, error) {
	f.note("OptionExercisePositions")
	return &sdkmodel.OptionExercisePositionPageResult{PageNum: 1, PageSize: 20}, nil
}

func (f *fakeTrade) OptionExerciseRecords(sdkmodel.OptionExercisePageRequest) (*sdkmodel.OptionExerciseRecordPageResult, error) {
	f.note("OptionExerciseRecords")
	return &sdkmodel.OptionExerciseRecordPageResult{PageNum: 1, PageSize: 20}, nil
}

func (f *fakeTrade) PreviewOrder(sdkmodel.OrderRequest) (*sdkmodel.PreviewResult, error) {
	f.note("PreviewOrder")
	return &sdkmodel.PreviewResult{Account: "acct", IsPass: true}, nil
}

func (f *fakeTrade) PlaceOrder(sdkmodel.OrderRequest) (*sdkmodel.PlaceOrderResult, error) {
	f.note("PlaceOrder")
	return &sdkmodel.PlaceOrderResult{ID: 1}, nil
}

func (f *fakeTrade) ModifyOrder(int64, sdkmodel.OrderRequest) (*sdkmodel.OrderIDResult, error) {
	f.note("ModifyOrder")
	return &sdkmodel.OrderIDResult{ID: 1}, nil
}

func (f *fakeTrade) CancelOrder(int64) (*sdkmodel.OrderIDResult, error) {
	f.note("CancelOrder")
	return &sdkmodel.OrderIDResult{ID: 1}, nil
}

// lockedDown is the safest possible config: dry run on and no live confirmation,
// so config.Writable refuses every write. A read that returns nil against this
// config has therefore not been through the gate.
func lockedDown() *config.Config {
	return &config.Config{Account: "acct", DryRun: true}
}

// defaultOptions mirrors the flag defaults declared in run, so a test options
// value behaves the way the same command line would.
func defaultOptions() options {
	return options{
		action:      "BUY",
		orderType:   "LMT",
		timeInForce: "DAY",
		market:      "US",
		currency:    "USD",
		secType:     "STK",
		limit:       20,
		size:        20,
		symbols:     "AAPL",
	}
}

// with merges the non-empty fields of o over the flag defaults. A struct
// literal in a test reads better when it names only what the test is about.
func with(base, o options) options {
	if o.command != "" {
		base.command = o.command
	}
	if o.symbol != "" {
		base.symbol = o.symbol
	}
	if o.symbols != "" {
		base.symbols = o.symbols
	}
	if o.secType != "" {
		base.secType = o.secType
	}
	if o.action != "" {
		base.action = o.action
	}
	if o.orderType != "" {
		base.orderType = o.orderType
	}
	if o.quantity != 0 {
		base.quantity = o.quantity
	}
	if o.limitPrice != 0 {
		base.limitPrice = o.limitPrice
	}
	if o.market != "" {
		base.market = o.market
	}
	if o.currency != "" {
		base.currency = o.currency
	}
	if o.orderID != 0 {
		base.orderID = o.orderID
	}
	if o.localOrderID != 0 {
		base.localOrderID = o.localOrderID
	}
	if o.limit != 0 {
		base.limit = o.limit
	}
	if o.states != "" {
		base.states = o.states
	}
	if o.expiry != "" {
		base.expiry = o.expiry
	}
	if o.strike != "" {
		base.strike = o.strike
	}
	if o.right != "" {
		base.right = o.right
	}
	if o.segType != "" {
		base.segType = o.segType
	}
	if o.transferID != "" {
		base.transferID = o.transferID
	}
	if o.segment {
		base.segment = o.segment
	}
	if o.contractID != 0 {
		base.contractID = o.contractID
	}
	if o.exerciseType != "" {
		base.exerciseType = o.exerciseType
	}
	if o.itmRate != 0 {
		base.itmRate = o.itmRate
	}
	if o.isForce {
		base.isForce = o.isForce
	}
	if o.sinceDate != "" {
		base.sinceDate = o.sinceDate
	}
	if o.toDate != "" {
		base.toDate = o.toDate
	}
	if o.page != 0 {
		base.page = o.page
	}
	if o.size != 0 {
		base.size = o.size
	}
	if o.status != "" {
		base.status = o.status
	}
	if o.confirmLive {
		base.confirmLive = o.confirmLive
	}
	return base
}

// silenceStdout redirects os.Stdout for the duration of a test so the command's
// table output does not interleave with the test report. os.Stdout is a
// variable, so the printers need no seam for this to work.
func silenceStdout(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = saved
		w.Close()
		io.Copy(io.Discard, r)
		r.Close()
	})
}

// TestReadCommandsBypassTheGate runs every read command against a fake client
// and checks two things: it succeeds despite the config refusing all writes,
// and it calls exactly the one SDK method it is supposed to.
func TestReadCommandsBypassTheGate(t *testing.T) {
	tests := []struct {
		command string
		want    string
		o       options
	}{
		{command: "assets", want: "Assets"},
		{command: "positions", want: "Positions"},
		{command: "orders", want: "Orders"},
		{command: "active-orders", want: "ActiveOrders"},
		{command: "inactive-orders", want: "InactiveOrders"},
		{command: "filled-orders", want: "FilledOrders"},
		{command: "get-order", want: "GetOrder", o: options{orderID: 1}},
		{command: "order-transactions", want: "OrderTransactions"},
		{command: "contract", want: "Contract", o: options{symbol: "AAPL"}},
		{command: "contract3", want: "Contract3", o: options{symbol: "AAPL"}},
		{command: "contracts", want: "Contracts", o: options{symbols: "AAPL,MSFT"}},
		{command: "quote-contract", want: "QuoteContract", o: options{symbol: "AAPL", secType: "OPT", expiry: "20260619"}},
		{command: "derivative-contracts", want: "DerivativeContracts", o: options{symbols: "AAPL", secType: "OPT"}},
		{command: "managed-accounts", want: "ManagedAccounts"},
		{command: "prime-assets", want: "PrimeAssets"},
		{command: "aggregate-assets", want: "AggregateAssets"},
		{command: "analytics-asset", want: "AnalyticsAsset"},
		{command: "estimate-tradable-quantity", want: "EstimateTradableQuantity", o: options{symbol: "AAPL"}},
		{command: "segment-fund-available", want: "SegmentFundAvailable"},
		{command: "segment-fund-history", want: "SegmentFundHistory"},
		{command: "fund-details", want: "FundDetails"},
		{command: "funding-history", want: "FundingHistory"},
		{command: "position-transfer-records", want: "PositionTransferRecords"},
		{command: "position-transfer-detail", want: "PositionTransferDetail", o: options{transferID: "t1"}},
		{command: "position-transfer-external-records", want: "PositionTransferExternalRecords"},
		{command: "option-exercise-check", want: "OptionExerciseCheck", o: options{contractID: 1}},
		{command: "option-exercise-positions", want: "OptionExercisePositions"},
		{command: "option-exercise-records", want: "OptionExerciseRecords"},
		{command: "preview", want: "PreviewOrder", o: options{symbol: "AAPL", quantity: 1, limitPrice: 100}},
	}
	if len(tests) != len(readCommands) {
		t.Fatalf("test covers %d commands but readCommands lists %d; keep them in step",
			len(tests), len(readCommands))
	}
	seen := map[string]bool{}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			seen[tc.command] = true
			silenceStdout(t)
			f := &fakeTrade{}
			o := with(defaultOptions(), tc.o)
			o.command = tc.command
			if err := route(context.Background(), f, lockedDown(), o); err != nil {
				t.Fatalf("read %q failed: %v", tc.command, err)
			}
			if f.called != tc.want {
				t.Errorf("read %q called %q, want %q", tc.command, f.called, tc.want)
			}
		})
	}
	for _, c := range readCommands {
		if !seen[c] {
			t.Errorf("command %q is in readCommands but has no test", c)
		}
	}
}

// TestWritesAreRefusedByDefault is the other half of the property: the three
// write commands must be blocked by the same config that lets every read
// through, and they must not reach the SDK.
func TestWritesAreRefusedByDefault(t *testing.T) {
	for _, command := range writeCommands {
		t.Run(command, func(t *testing.T) {
			silenceStdout(t)
			f := &fakeTrade{}
			o := with(defaultOptions(), options{command: command, symbol: "AAPL", quantity: 1, limitPrice: 100, orderID: 7})
			err := route(context.Background(), f, lockedDown(), o)
			if err == nil {
				t.Fatalf("write %q was allowed with dry-run on and no --confirm-live", command)
			}
			if !config.IsSafetyError(err) {
				t.Errorf("write %q failed with %v, want a safety refusal", command, err)
			}
			if f.called != "" {
				t.Errorf("write %q reached the SDK (%s) despite being refused", command, f.called)
			}
		})
	}
}

// TestUnknownCommandIsRejected pins the error a typo produces: a clean message
// naming the unknown value, not a silent fallback to a default read.
func TestUnknownCommandIsRejected(t *testing.T) {
	silenceStdout(t)
	f := &fakeTrade{}
	err := route(context.Background(), f, lockedDown(), with(defaultOptions(), options{command: "no-such-command"}))
	if err == nil {
		t.Fatal("an unknown command should fail")
	}
	if !strings.Contains(err.Error(), "no-such-command") {
		t.Errorf("error should name the command, got %v", err)
	}
	if config.IsSafetyError(err) || config.MissingCredentialErrorIs(err) {
		t.Errorf("an unknown command is not a credential or safety failure, got %v", err)
	}
	if f.called != "" {
		t.Errorf("an unknown command reached the SDK (%s)", f.called)
	}
}

// TestMissingInputsAreRejectedLocally checks that a read refuses a request it
// cannot fill in before any round trip, with a message that names the flag.
func TestMissingInputsAreRejectedLocally(t *testing.T) {
	tests := []struct {
		name    string
		command string
		o       options
		// clear lists fields to blank after the flag defaults are applied, for
		// the case where the flag default itself is what must go missing.
		clear []string
		want  string
	}{
		{name: "get-order without an id", command: "get-order", want: "-order-id"},
		{name: "contract without a symbol", command: "contract", clear: []string{"symbol"}, want: "-symbol"},
		{name: "contracts without symbols", command: "contracts", clear: []string{"symbols"}, want: "-symbols"},
		{name: "quote-contract with the default sec-type", command: "quote-contract", o: options{symbol: "AAPL"}, want: "-sec-type"},
		{name: "quote-contract without an expiry", command: "quote-contract", o: options{symbol: "AAPL", secType: "OPT"}, want: "-expiry"},
		{name: "derivative-contracts with the default sec-type", command: "derivative-contracts", want: "-sec-type"},
		{name: "position-transfer-detail without an id", command: "position-transfer-detail", want: "-transfer-id"},
		{name: "option-exercise-check without a contract id", command: "option-exercise-check", want: "-contract-id"},
		{name: "option-exercise-check with a bad type", command: "option-exercise-check", o: options{contractID: 1, exerciseType: "nope"}, want: "-type"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			silenceStdout(t)
			f := &fakeTrade{}
			o := with(defaultOptions(), tc.o)
			o.command = tc.command
			for _, field := range tc.clear {
				switch field {
				case "symbol":
					o.symbol = ""
				case "symbols":
					o.symbols = ""
				}
			}
			err := route(context.Background(), f, lockedDown(), o)
			if err == nil {
				t.Fatalf("%q should have refused the missing input", tc.command)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should name %s", err, tc.want)
			}
			if f.called != "" {
				t.Errorf("%q called the SDK (%s) despite missing input", tc.command, f.called)
			}
		})
	}
}
