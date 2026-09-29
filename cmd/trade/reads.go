package main

// This file holds the read-only endpoints of cmd/trade.
//
// Every handler here has the same shape as the read commands in main.go: check
// the flags, honour the context deadline, call exactly one SDK method, print.
// The SDK error is always wrapped with the endpoint that produced it.
//
// # Safety
//
// None of these functions takes a *config.Config, so none of them can reach
// config.Writable and none of them can reach the write gate. That is a
// structural property, not a convention: the gate is unreachable from this file
// because the value that owns it is never passed in. Adding a write here would
// require changing a signature, which is exactly the review signal we want.
//
// The shared formatting helpers (Dash, Truncate, MSFmt, List) come from
// internal/rocli. That package exposes no trade client and no write path; this
// file uses only its pure output helpers.
//
// The seven TradeClient methods that are NOT wired up here, deliberately:
// PlaceForexOrder, TransferSegmentFund, CancelSegmentFund, TransferPosition,
// OptionExerciseSubmit and OptionExerciseCancel all mutate account state, and
// SetSecretKey is a local field assignment rather than an API call.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"

	"github.com/shing1211/tiger-go-demo/internal/rocli"
)

// ---- contracts ----

// printContract looks up one contract by symbol (wire: contract).
func printContract(ctx context.Context, tc tradeClient, o options) error {
	symbol, err := requireSymbol(o, "contract")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.Contract(symbol, o.secType)
	if err != nil {
		return fmt.Errorf("get contract (%s, sec_type=%s): %w", symbol, o.secType, err)
	}
	rocli.Section(os.Stdout, "contract (%s, sec_type=%s)", symbol, o.secType)
	printContractRows(rows, o.limit)
	return nil
}

// printContract3 is the same lookup against version 3.0 of the endpoint (wire:
// contract, version 3.0), which returns one object instead of an items list.
func printContract3(ctx context.Context, tc tradeClient, o options) error {
	symbol, err := requireSymbol(o, "contract3")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	row, err := tc.Contract3(symbol, o.secType)
	if err != nil {
		return fmt.Errorf("get contract v3 (%s, sec_type=%s): %w", symbol, o.secType, err)
	}
	rocli.Section(os.Stdout, "contract v3.0 (%s, sec_type=%s)", symbol, o.secType)
	if row == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	printContractRows([]sdkmodel.Contract{*row}, o.limit)
	return nil
}

// printContracts looks up several contracts at once (wire: contracts).
func printContracts(ctx context.Context, tc tradeClient, o options) error {
	symbols, err := requireSymbols(o, "contracts")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.Contracts(symbols, o.secType)
	if err != nil {
		return fmt.Errorf("get contracts (%s, sec_type=%s): %w", strings.Join(symbols, ","), o.secType, err)
	}
	rocli.Section(os.Stdout, "contracts (%s, sec_type=%s)", strings.Join(symbols, ","), o.secType)
	printContractRows(rows, o.limit)
	return nil
}

// printQuoteContract lists the tradable option / warrant / iwarrant contracts on
// one underlying (wire: quote_contract). All three inputs are required: the
// endpoint returns a chain, and without an expiry there is nothing to key it on.
func printQuoteContract(ctx context.Context, tc tradeClient, o options) error {
	symbol, err := requireSymbol(o, "quote-contract")
	if err != nil {
		return err
	}
	if err := requireDerivativeSecType(o, "quote-contract"); err != nil {
		return err
	}
	expiry, err := requireExpiry(o, "quote-contract")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.QuoteContract(symbol, o.secType, expiry)
	if err != nil {
		return fmt.Errorf("get quote contract (%s, sec_type=%s, expiry=%s): %w", symbol, o.secType, expiry, err)
	}
	rocli.Section(os.Stdout, "quote contract (%s, sec_type=%s, expiry=%s)", symbol, o.secType, expiry)
	printContractRows(rows, o.limit)
	return nil
}

// printDerivativeContracts lists derivative contracts across one or more
// underlyings (wire: quote_contract, same endpoint as quote-contract but with the
// filters passed as a struct). Unlike quote-contract the symbol list and the
// expiry are both optional here.
func printDerivativeContracts(ctx context.Context, tc tradeClient, o options) error {
	if err := requireDerivativeSecType(o, "derivative-contracts"); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	// Account and SecretKey are filled in by the SDK from the client itself.
	req := sdkmodel.DerivativeContractsRequest{
		Symbols: rocli.List(o.symbols),
		SecType: o.secType,
		Expiry:  strings.TrimSpace(o.expiry),
	}
	rows, err := tc.DerivativeContracts(req)
	if err != nil {
		return fmt.Errorf("get derivative contracts (symbols=%s, sec_type=%s, expiry=%s): %w",
			rocli.DashOr(strings.Join(req.Symbols, ","), "any"), o.secType, rocli.DashOr(req.Expiry, "any"), err)
	}
	rocli.Section(os.Stdout, "derivative contracts (sec_type=%s, expiry=%s)", o.secType, rocli.DashOr(req.Expiry, "any"))
	printContractRows(rows, o.limit)
	return nil
}

// printContractRows renders the contract shape shared by all five lookups above.
func printContractRows(rows []sdkmodel.Contract, limit int) {
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return
	}
	fmt.Printf("  %-14s %-6s %-12s %-10s %-6s %-8s %14s %-12s\n",
		"SYMBOL", "SECTYPE", "EXPIRY", "STRIKE", "RIGHT", "CURRENCY", "MULTIPLIER", "IDENTIFIER")
	for i, c := range rows {
		if i >= limit {
			rocli.Truncate(os.Stdout, i, len(rows), limit)
			break
		}
		fmt.Printf("  %-14s %-6s %-12s %-10s %-6s %-8s %14.2f %-12s\n",
			rocli.Dash(c.Symbol), rocli.Dash(c.SecType), rocli.Dash(c.Expiry), rocli.Dash(c.Strike),
			rocli.Dash(c.Right), rocli.Dash(c.Currency), c.Multiplier, rocli.Dash(c.Identifier))
	}
}

// ---- single order ----

// printGetOrder fetches one order by its global id (-order-id) or its
// account-scoped id (-local-order-id). The SDK returns (nil, nil) when the id
// matches nothing, which is reported as a miss rather than as an error.
func printGetOrder(ctx context.Context, tc tradeClient, o options) error {
	if o.orderID == 0 && o.localOrderID == 0 {
		return errors.New("-order-id (global id) or -local-order-id (account id) is required for -command get-order")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	// Account and SecretKey are filled in by the SDK from the client itself.
	order, err := tc.GetOrder(sdkmodel.GetOrderRequest{
		Id:      o.orderID,
		OrderId: o.localOrderID,
	})
	if err != nil {
		return fmt.Errorf("get order (id=%d, local_order_id=%d): %w", o.orderID, o.localOrderID, err)
	}
	fmt.Println("== order ==")
	if order == nil {
		fmt.Println("  (no matching order)")
		return nil
	}
	printOrderHeader()
	printOrderRow(*order)
	return nil
}

// printOrderTransactions returns the fills behind orders (wire:
// order_transactions). Every filter is optional; -order-id narrows it to one
// parent order and -symbol/-sec-type to one instrument.
func printOrderTransactions(ctx context.Context, tc tradeClient, o options) error {
	// This endpoint types the strike as a number, unlike the contract endpoints
	// where the server returns it as a string, so it is parsed rather than passed
	// through.
	strike, err := rocli.Float("strike", o.strike)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.OrderTransactionsRequest{
		OrderId:   o.orderID,
		Symbol:    upperOrEmpty(o.symbol),
		SecType:   o.secType,
		Limit:     o.limit,
		Expiry:    strings.TrimSpace(o.expiry),
		Strike:    strike,
		Right:     strings.TrimSpace(o.right),
		StartDate: o.begin,
		EndDate:   o.end,
	}
	rows, err := tc.OrderTransactions(req)
	if err != nil {
		return fmt.Errorf("get order transactions (order_id=%d, symbol=%s): %w",
			o.orderID, rocli.DashOr(req.Symbol, "any"), err)
	}
	rocli.Section(os.Stdout, "order transactions (order_id=%d, symbol=%s)",
		o.orderID, rocli.DashOr(req.Symbol, "any"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-12s %-12s %-6s %8s %10s %10s %14s %10s %-20s\n",
		"ORDER_ID", "SYMBOL", "ACTION", "QTY", "PRICE", "FILL_PRICE", "FILLED_AMOUNT", "COMMISSION", "TIME")
	for i, tx := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-12d %-12s %-6s %8d %10.4f %10.4f %14.2f %10.2f %-20s\n",
			tx.OrderID, rocli.Dash(tx.Symbol), rocli.Dash(tx.Action), tx.FilledQuantity,
			tx.Price, tx.FilledPrice, tx.FilledAmount, tx.Commission, fillTime(tx))
	}
	return nil
}

// fillTime prefers the server's string timestamp and falls back to the epoch-ms
// field, which is what the "transactedAt" field is for on some accounts.
func fillTime(tx sdkmodel.Transaction) string {
	if t := strings.TrimSpace(tx.TransactedAt); t != "" {
		return t
	}
	return rocli.MSFmt(tx.TransactionTime)
}

// ---- accounts and assets ----

// printManagedAccounts lists the sub-accounts of an institutional account
// (wire: accounts). A personal account returns a single entry: itself.
func printManagedAccounts(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.ManagedAccounts(sdkmodel.ManagedAccountsRequest{})
	if err != nil {
		return fmt.Errorf("get managed accounts: %w", err)
	}
	rocli.Section(os.Stdout, "managed accounts")
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-24s %-16s %-24s %-12s\n", "ACCOUNT", "ACCOUNT_TYPE", "CAPABILITY", "STATUS")
	for i, a := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-24s %-16s %-24s %-12s\n",
			rocli.Dash(a.Account), rocli.Dash(a.AccountType), rocli.Dash(a.Capability), rocli.Dash(a.Status))
	}
	return nil
}

// printPrimeAssets returns the combined (prime) account balance sheet
// (wire: prime_assets). Unlike the plain assets endpoint it nests a per-segment
// breakdown with its own per-currency detail, so it prints as a tree.
func printPrimeAssets(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	res, err := tc.PrimeAssets(sdkmodel.AssetsRequest{
		Segment:      o.segment,
		BaseCurrency: strings.TrimSpace(o.baseCurrency),
	})
	if err != nil {
		return fmt.Errorf("get prime assets (base_currency=%s): %w", rocli.DashOr(o.baseCurrency, "default"), err)
	}
	rocli.Section(os.Stdout, "prime assets")
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	printKVs(
		kv{"account_id", rocli.Dash(res.AccountID)},
		kv{"updated_at", rocli.MSFmt(res.UpdateTimestamp)},
		kv{"segment_count", fmt.Sprintf("%d", len(res.Segments))},
	)
	for _, seg := range res.Segments {
		fmt.Printf("  segment category=%s currency=%s capability=%s leverage=%.2f\n",
			rocli.Dash(seg.Category), rocli.Dash(seg.Currency), rocli.Dash(seg.Capability), seg.Leverage)
		fmt.Printf("    %-20s %14s %16s %16s %16s\n",
			"", "CASH_BALANCE", "AVAILABLE", "NET_LIQUIDATION", "BUYING_POWER")
		fmt.Printf("    %-20s %14.2f %16.2f %16.2f %16.2f\n",
			"", seg.CashBalance, seg.CashAvailableForTrade, seg.NetLiquidation, seg.BuyingPower)
		fmt.Printf("    %-20s %14s %16s %16s %16s\n",
			"", "GROSS_POS_VALUE", "INIT_MARGIN", "UNREALIZED_PL", "REALIZED_PL")
		fmt.Printf("    %-20s %14.2f %16.2f %16.2f %16.2f\n",
			"", seg.GrossPositionValue, seg.InitMargin, seg.UnrealizedPL, seg.RealizedPL)
		for _, ca := range seg.CurrencyAssets {
			fmt.Printf("      currency=%-6s cash=%.2f available=%.2f fx_rate=%.6f\n",
				rocli.Dash(ca.Currency), ca.CashBalance, ca.CashAvailableForTrade, ca.ForexRate)
		}
	}
	return nil
}

// printAggregateAssets returns the whole account rolled up in one base currency
// (wire: aggregate_assets). It is the one-line answer to "what am I worth".
func printAggregateAssets(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	res, err := tc.AggregateAssets(sdkmodel.AggregateAssetsRequest{
		SegType:      strings.TrimSpace(o.segType),
		BaseCurrency: strings.TrimSpace(o.baseCurrency),
	})
	if err != nil {
		return fmt.Errorf("get aggregate assets (seg_type=%s, base_currency=%s): %w",
			rocli.DashOr(o.segType, "all"), rocli.DashOr(o.baseCurrency, "default"), err)
	}
	rocli.Section(os.Stdout, "aggregate assets")
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	printKVs(
		kv{"account_id", rocli.Dash(res.AccountID)},
		kv{"base_currency", rocli.Dash(res.BaseCurrency)},
		kv{"net_liquidation", money(res.NetLiquidation)},
		kv{"gross_position_value", money(res.GrossPositionValue)},
		kv{"cash_balance", money(res.CashBalance)},
	)
	for _, ca := range res.CurrencyAssets {
		fmt.Printf("  %-22s cash=%.2f available=%.2f fx_rate=%.6f\n",
			rocli.Dash(ca.Currency), ca.CashBalance, ca.CashAvailableForTrade, ca.ForexRate)
	}
	return nil
}

// printAnalyticsAsset returns the day-by-day P&L and equity curve behind the
// current balance (wire: analytics_asset).
func printAnalyticsAsset(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.AnalyticsAssetRequest{
		SubAccount: strings.TrimSpace(o.subAccount),
		SegType:    strings.TrimSpace(o.segType),
		Currency:   strings.TrimSpace(o.currency),
		StartDate:  strings.TrimSpace(o.sinceDate),
		EndDate:    strings.TrimSpace(o.toDate),
	}
	rows, err := tc.AnalyticsAsset(req)
	if err != nil {
		return fmt.Errorf("get analytics asset (seg_type=%s, currency=%s, %s..%s): %w",
			rocli.DashOr(req.SegType, "all"), rocli.DashOr(req.Currency, "default"),
			rocli.DashOr(req.StartDate, "open"), rocli.DashOr(req.EndDate, "open"), err)
	}
	rocli.Section(os.Stdout, "analytics asset (currency=%s, %s..%s)",
		rocli.DashOr(req.Currency, "default"), rocli.DashOr(req.StartDate, "open"), rocli.DashOr(req.EndDate, "open"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-12s %-8s %-8s %16s %16s %10s %14s\n",
		"DATE", "CCY", "SEG_TYPE", "HOLDING_VALUE", "CASH_BALANCE", "PNL%", "NET_VALUE_INDEX")
	for i, a := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-12s %-8s %-8s %16.2f %16.2f %9.2f%% %14.4f\n",
			rocli.Dash(a.Date), rocli.Dash(a.Currency), rocli.Dash(a.SegType),
			a.HoldingValue, a.CashBalance, a.PnlRate, a.NetValueIndex)
	}
	return nil
}

// printEstimateTradableQuantity asks Tiger how much of an instrument the
// account could trade right now (wire: estimate_tradable_quantity).
//
// It behaves like a dry run — nothing is placed — but it IS a real round trip
// against the live account, so it is grouped with the reads and stays outside
// the write gate only because it cannot mutate anything.
func printEstimateTradableQuantity(ctx context.Context, tc tradeClient, o options) error {
	symbol, err := requireSymbol(o, "estimate-tradable-quantity")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.EstimateTradableQuantityRequest{
		Symbol:     symbol,
		SecType:    o.secType,
		SegType:    strings.TrimSpace(o.segType),
		Action:     strings.ToUpper(strings.TrimSpace(o.action)),
		OrderType:  strings.ToUpper(strings.TrimSpace(o.orderType)),
		LimitPrice: o.limitPrice,
		Expiry:     strings.TrimSpace(o.expiry),
		Strike:     strings.TrimSpace(o.strike),
		Right:      strings.TrimSpace(o.right),
	}
	res, err := tc.EstimateTradableQuantity(req)
	if err != nil {
		return fmt.Errorf("estimate tradable quantity (%s, %s %s): %w", symbol, req.Action, req.OrderType, err)
	}
	rocli.Section(os.Stdout, "estimate tradable quantity (%s, %s %s)", symbol, req.Action, req.OrderType)
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	printKVs(
		kv{"currency", rocli.Dash(res.Currency)},
		kv{"tradable_quantity", qty(res.TradableQuantity)},
		kv{"max_cash_buy_quantity", qty(res.MaxCashBuyQuantity)},
		kv{"max_margin_buy_quantity", qty(res.MaxMarginBuyQuantity)},
		kv{"max_short_sell_quantity", qty(res.MaxShortSellQuantity)},
		kv{"max_position_sell_quantity", qty(res.MaxPositionSellQuantity)},
		kv{"cash_buying_power", money(res.CashBuyingPower)},
	)
	return nil
}

// ---- sub-account fund transfers (read side) ----

// printSegmentFundAvailable reports how much may be moved between segments
// right now (wire: segment_fund_available). The transfer and cancel methods
// that share this request shape are writes and are deliberately not wired up.
func printSegmentFundAvailable(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.SegmentFundAvailable(segmentFundRequest(o))
	if err != nil {
		return fmt.Errorf("get segment fund available (%s -> %s): %w",
			rocli.DashOr(o.fromSegment, "any"), rocli.DashOr(o.toSegment, "any"), err)
	}
	rocli.Section(os.Stdout, "segment fund available (%s -> %s)",
		rocli.DashOr(o.fromSegment, "any"), rocli.DashOr(o.toSegment, "any"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-18s %-8s %18s\n", "FROM_SEGMENT", "CCY", "AMOUNT")
	for i, r := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-18s %-8s %18.2f\n", rocli.Dash(r.FromSegment), rocli.Dash(r.Currency), r.Amount)
	}
	return nil
}

// printSegmentFundHistory lists past segment transfers (wire:
// segment_fund_history).
func printSegmentFundHistory(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.SegmentFundHistory(segmentFundRequest(o))
	if err != nil {
		return fmt.Errorf("get segment fund history (%s -> %s): %w",
			rocli.DashOr(o.fromSegment, "any"), rocli.DashOr(o.toSegment, "any"), err)
	}
	rocli.Section(os.Stdout, "segment fund history (%s -> %s)",
		rocli.DashOr(o.fromSegment, "any"), rocli.DashOr(o.toSegment, "any"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-12s %-16s %-16s %-8s %14s %-12s %-20s\n",
		"ID", "FROM_SEGMENT", "TO_SEGMENT", "CCY", "AMOUNT", "STATUS", "CREATED")
	for i, r := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-12d %-16s %-16s %-8s %14.2f %-12s %-20s\n",
			r.ID, rocli.Dash(r.FromSegment), rocli.Dash(r.ToSegment), rocli.Dash(r.Currency),
			r.Amount, rocli.Dash(r.Status), rocli.MSFmt(r.CreatedAt))
	}
	return nil
}

// segmentFundRequest builds the request shape the available / history / transfer
// / cancel endpoints share. Only the read side is reachable from here.
func segmentFundRequest(o options) sdkmodel.SegmentFundRequest {
	return sdkmodel.SegmentFundRequest{
		FromSegment: strings.TrimSpace(o.fromSegment),
		ToSegment:   strings.TrimSpace(o.toSegment),
		Currency:    strings.TrimSpace(o.currency),
		Limit:       o.limit,
	}
}

// printFundDetails returns the cash ledger: every debit and credit on the
// account (wire: fund_details).
func printFundDetails(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.FundDetailsRequest{
		SegTypes:  rocli.ListRaw(o.segTypes),
		FundType:  strings.TrimSpace(o.fundType),
		Currency:  strings.TrimSpace(o.currency),
		StartDate: strings.TrimSpace(o.sinceDate),
		EndDate:   strings.TrimSpace(o.toDate),
		Start:     o.start,
		Limit:     o.limit,
	}
	rows, err := tc.FundDetails(req)
	if err != nil {
		return fmt.Errorf("get fund details (fund_type=%s, currency=%s, %s..%s): %w",
			rocli.DashOr(req.FundType, "all"), rocli.DashOr(req.Currency, "default"),
			rocli.DashOr(req.StartDate, "open"), rocli.DashOr(req.EndDate, "open"), err)
	}
	rocli.Section(os.Stdout, "fund details (fund_type=%s, currency=%s, %s..%s)",
		rocli.DashOr(req.FundType, "all"), rocli.DashOr(req.Currency, "default"),
		rocli.DashOr(req.StartDate, "open"), rocli.DashOr(req.EndDate, "open"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-14s %-18s %-8s %-14s %-8s %14s %16s %-20s\n",
		"ID", "ACCOUNT", "SEG_TYPE", "FUND_TYPE", "CCY", "AMOUNT", "BALANCE", "OCCUR_TIME")
	for i, f := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-14d %-18s %-8s %-14s %-8s %14.2f %16.2f %-20s\n",
			f.ID.Int64(), rocli.Dash(f.Account), rocli.Dash(f.SegType), rocli.Dash(f.FundType),
			rocli.Dash(f.Currency), f.Amount, f.Balance, rocli.MSFmt(f.OccurTime))
	}
	return nil
}

// printFundingHistory lists account funding and transfer records. It shares the
// transfer_fund endpoint with TransferSegmentFund, which is a write and is not
// wired up; only the history half is exposed here.
func printFundingHistory(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.FundingHistory(sdkmodel.FundingHistoryRequest{
		SegType: strings.TrimSpace(o.segType),
	})
	if err != nil {
		return fmt.Errorf("get funding history (seg_type=%s): %w", rocli.DashOr(o.segType, "all"), err)
	}
	rocli.Section(os.Stdout, "funding history (seg_type=%s)", rocli.DashOr(o.segType, "all"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-12s %-20s %-8s %-8s %14s %-12s %-12s %-20s\n",
		"ID", "REF_ID", "TYPE", "CCY", "AMOUNT", "BUSINESS_DATE", "STATUS", "CREATED")
	for i, h := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-12d %-20s %-8s %-8s %14.2f %-12s %-12s %-20s\n",
			h.ID, rocli.Dash(h.RefID), rocli.Dash(h.TypeDesc), rocli.Dash(h.Currency),
			h.Amount, rocli.Dash(h.BusinessDate), rocli.Dash(h.Status), rocli.MSFmt(h.CreatedAt))
	}
	return nil
}

// ---- position transfers (read side) ----

// printPositionTransferRecords lists internal position transfers between
// sub-accounts (wire: position_transfer_records). The transfer itself,
// TransferPosition, is a write and is not wired up.
func printPositionTransferRecords(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.PositionTransferRecords(positionTransferRequest(o))
	if err != nil {
		return fmt.Errorf("get position transfer records (market=%s, symbol=%s, status=%s): %w",
			o.market, rocli.DashOr(o.symbol, "any"), rocli.DashOr(o.status, "any"), err)
	}
	rocli.Section(os.Stdout, "position transfer records (market=%s, symbol=%s, status=%s)",
		o.market, rocli.DashOr(o.symbol, "any"), rocli.DashOr(o.status, "any"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	for i, r := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-24s %-18s %-18s %-8s %-12s %-20s %d item(s)\n",
			rocli.Dash(r.ID), rocli.Dash(r.FromAccount), rocli.Dash(r.ToAccount), rocli.Dash(r.Market),
			rocli.Dash(r.Status), rocli.MSFmt(r.SubmitTime), len(r.Transfers))
		printTransferItems(r.Transfers)
	}
	return nil
}

// printPositionTransferDetail fetches one internal transfer by its id (wire:
// position_transfer_detail).
func printPositionTransferDetail(ctx context.Context, tc tradeClient, o options) error {
	id := strings.TrimSpace(o.transferID)
	if id == "" {
		return errors.New("-transfer-id is required for -command position-transfer-detail")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	res, err := tc.PositionTransferDetail(sdkmodel.PositionTransferDetailRequest{ID: id})
	if err != nil {
		return fmt.Errorf("get position transfer detail (%s): %w", id, err)
	}
	rocli.Section(os.Stdout, "position transfer detail (%s)", id)
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	printKVs(
		kv{"id", rocli.Dash(res.ID)},
		kv{"from_account", rocli.Dash(res.FromAccount)},
		kv{"to_account", rocli.Dash(res.ToAccount)},
		kv{"market", rocli.Dash(res.Market)},
		kv{"status", rocli.Dash(res.Status)},
		kv{"submit_time", rocli.MSFmt(res.SubmitTime)},
		kv{"update_time", rocli.MSFmt(res.UpdateTime)},
		kv{"remark", rocli.Dash(res.Remark)},
	)
	printTransferItems(res.Transfers)
	return nil
}

// printPositionTransferExternalRecords lists position transfers involving an
// external counterparty (wire: position_transfer_external_records).
func printPositionTransferExternalRecords(ctx context.Context, tc tradeClient, o options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	rows, err := tc.PositionTransferExternalRecords(positionTransferRequest(o))
	if err != nil {
		return fmt.Errorf("get external position transfer records (market=%s, symbol=%s, status=%s): %w",
			o.market, rocli.DashOr(o.symbol, "any"), rocli.DashOr(o.status, "any"), err)
	}
	rocli.Section(os.Stdout, "external position transfer records (market=%s, symbol=%s, status=%s)",
		o.market, rocli.DashOr(o.symbol, "any"), rocli.DashOr(o.status, "any"))
	if len(rows) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-24s %-8s %-12s %12s %-10s %-12s %-20s\n",
		"ID", "MARKET", "SYMBOL", "QUANTITY", "DIRECTION", "STATUS", "SUBMIT_TIME")
	for i, r := range rows {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(rows), o.limit)
			break
		}
		fmt.Printf("  %-24s %-8s %-12s %12d %-10s %-12s %-20s\n",
			rocli.Dash(r.ID), rocli.Dash(r.Market), rocli.Dash(r.Symbol), r.Quantity,
			rocli.Dash(r.Direction), rocli.Dash(r.Status), rocli.MSFmt(r.SubmitTime))
	}
	return nil
}

// positionTransferRequest builds the filter shared by the internal and external
// transfer-record endpoints. AccountID is left empty on purpose: the SDK fills
// it in from the client's own account.
func positionTransferRequest(o options) sdkmodel.PositionTransferRecordsRequest {
	return sdkmodel.PositionTransferRecordsRequest{
		SinceDate: strings.TrimSpace(o.sinceDate),
		ToDate:    strings.TrimSpace(o.toDate),
		Status:    strings.TrimSpace(o.status),
		Market:    strings.ToUpper(strings.TrimSpace(o.market)),
		Symbol:    upperOrEmpty(o.symbol),
	}
}

func printTransferItems(items []sdkmodel.TransferItem) {
	for _, it := range items {
		fmt.Printf("      %-12s %12d %-6s %-12s %-10s %-6s\n",
			rocli.Dash(it.Symbol), it.Quantity, rocli.Dash(it.SecType),
			rocli.Dash(it.Expiry), rocli.Dash(it.Strike), rocli.Dash(it.Right))
	}
}

// ---- option exercise (read side) ----

// printOptionExerciseCheck previews what exercising or expiring one contract
// would do to the underlying stock position (wire: option_exercise_check).
//
// The matching submit and cancel calls are writes and are deliberately absent;
// this is the pre-flight half only.
func printOptionExerciseCheck(ctx context.Context, tc tradeClient, o options) error {
	if o.contractID <= 0 {
		return errors.New("-contract-id is required for -command option-exercise-check")
	}
	kind, err := exerciseType(o, "option-exercise-check")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.OptionExerciseCheckRequest{
		ContractId:    o.contractID,
		Type:          kind,
		Quantity:      float64(o.quantity),
		ExecutingDate: strings.TrimSpace(o.executingDate),
	}
	// Both of these are tri-state on the wire (absent / true / false), so they
	// are only sent when the flag actually sets them.
	if o.isForce {
		req.IsForce = &o.isForce
	}
	if o.itmRate > 0 {
		req.ItmRate = &o.itmRate
	}
	res, err := tc.OptionExerciseCheck(req)
	if err != nil {
		return fmt.Errorf("option exercise check (contract_id=%d, type=%s): %w",
			o.contractID, rocli.DashOr(kind, "default"), err)
	}
	rocli.Section(os.Stdout, "option exercise check (contract_id=%d, type=%s)",
		o.contractID, rocli.DashOr(kind, "default"))
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	printKVs(
		kv{"symbol", rocli.Dash(res.Symbol)},
		kv{"position", qty(res.Position)},
		kv{"available_quantity", qty(res.AvailableQuantity)},
		kv{"stk_position", qty(res.StkPosition)},
		kv{"stk_position_change", qty(res.StkPositionChange)},
		kv{"stk_position_before", qty(res.StkPositionBefore)},
		kv{"stk_position_after", qty(res.StkPositionAfter)},
	)
	return nil
}

// printOptionExercisePositions lists the option positions that are eligible for
// exercise or expiry (wire: option_exercise_position).
func printOptionExercisePositions(ctx context.Context, tc tradeClient, o options) error {
	kind, err := exerciseType(o, "option-exercise-positions")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	res, err := tc.OptionExercisePositions(sdkmodel.OptionExercisePositionRequest{Type: kind})
	if err != nil {
		return fmt.Errorf("get option exercise positions (type=%s): %w", rocli.DashOr(kind, "default"), err)
	}
	rocli.Section(os.Stdout, "option exercise positions (type=%s)", rocli.DashOr(kind, "default"))
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	fmt.Printf("  page %d/%d, %d item(s)\n", res.PageNum, res.PageCount, res.ItemCount)
	if len(res.Items) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-12s %-12s %-12s %-12s %-10s %-6s %12s %12s\n",
		"CONTRACT_ID", "SYMBOL", "STK_SYMBOL", "EXPIRY", "STRIKE", "C/P", "POSITION", "AVAILABLE")
	for i, p := range res.Items {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(res.Items), o.limit)
			break
		}
		fmt.Printf("  %-12d %-12s %-12s %-12s %-10s %-6s %12.2f %12.2f\n",
			p.ContractId, rocli.Dash(p.Symbol), rocli.Dash(p.StkSymbol), rocli.Dash(p.ExpireDate),
			rocli.Dash(p.Strike), rocli.Dash(p.CallPut), p.Position, p.AvailableQuantity)
	}
	return nil
}

// printOptionExerciseRecords lists exercise / expiry requests already submitted
// (wire: option_exercise_record). The submit and cancel calls on the same
// records are writes and are deliberately absent.
func printOptionExerciseRecords(ctx context.Context, tc tradeClient, o options) error {
	kind, err := exerciseType(o, "option-exercise-records")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	req := sdkmodel.OptionExercisePageRequest{
		Page:    o.page,
		Size:    o.size,
		Status:  strings.TrimSpace(o.status),
		Type:    kind,
		Symbol:  upperOrEmpty(o.symbol),
		OrderBy: strings.TrimSpace(o.orderBy),
	}
	res, err := tc.OptionExerciseRecords(req)
	if err != nil {
		return fmt.Errorf("get option exercise records (page=%d, size=%d, type=%s, status=%s): %w",
			o.page, o.size, rocli.DashOr(kind, "all"), rocli.DashOr(req.Status, "all"), err)
	}
	rocli.Section(os.Stdout, "option exercise records (page=%d, size=%d, type=%s, status=%s)",
		o.page, o.size, rocli.DashOr(kind, "all"), rocli.DashOr(req.Status, "all"))
	if res == nil {
		fmt.Println("  (no data returned)")
		return nil
	}
	fmt.Printf("  page %d/%d, %d item(s)\n", res.PageNum, res.PageCount, res.ItemCount)
	if len(res.Items) == 0 {
		fmt.Println("  (no rows returned)")
		return nil
	}
	fmt.Printf("  %-10s %-12s %-12s %-12s %-10s %-6s %8s %8s %-10s %-12s\n",
		"ID", "CONTRACT_ID", "SYMBOL", "EXPIRY", "STRIKE", "C/P", "REQUESTED", "FILLED", "STATUS", "EXEC_DATE")
	for i, r := range res.Items {
		if i >= o.limit {
			rocli.Truncate(os.Stdout, i, len(res.Items), o.limit)
			break
		}
		fmt.Printf("  %-10d %-12d %-12s %-12s %-10s %-6s %8.2f %8.2f %-10s %-12s\n",
			r.Id, r.ContractId, rocli.Dash(r.Symbol), rocli.Dash(r.ExpireDate), rocli.Dash(r.Strike),
			rocli.Dash(r.CallPut), r.RequestQuantity, r.Quantity, rocli.Dash(r.Status),
			rocli.Dash(r.ExecutingDate))
	}
	return nil
}

// ---- shared helpers ----

// kv is one "name: value" line.
type kv struct{ name, value string }

// printKVs prints a flat, single-object response one field per line. Several
// trade endpoints answer with one named object rather than a row list, and a
// field list is a more honest rendering than invented table columns.
func printKVs(fields ...kv) {
	for _, f := range fields {
		fmt.Printf("  %-24s %s\n", f.name, f.value)
	}
}

func requireSymbol(o options, command string) (string, error) {
	if s := upperOrEmpty(o.symbol); s != "" {
		return s, nil
	}
	return "", fmt.Errorf("-symbol is required for -command %s, e.g. -symbol AAPL", command)
}

func requireSymbols(o options, command string) ([]string, error) {
	if syms := rocli.List(o.symbols); len(syms) > 0 {
		return syms, nil
	}
	return nil, fmt.Errorf("-symbols is required for -command %s, e.g. -symbols AAPL,MSFT", command)
}

func requireExpiry(o options, command string) (string, error) {
	if e := strings.TrimSpace(o.expiry); e != "" {
		return e, nil
	}
	return "", fmt.Errorf("-expiry is required for -command %s, e.g. -expiry 20260619", command)
}

// requireDerivativeSecType rejects the default -sec-type STK. The derivative
// endpoints only understand OPT, WAR and IOPT, and STK is what every other
// command in this binary defaults to, so it is almost always a forgotten flag.
func requireDerivativeSecType(o options, command string) error {
	switch o.secType {
	case "OPT", "WAR", "IOPT":
		return nil
	}
	return fmt.Errorf("-sec-type must be OPT, WAR or IOPT for -command %s (got %q)", command, o.secType)
}

func exerciseType(o options, command string) (string, error) {
	kind := strings.TrimSpace(o.exerciseType)
	switch kind {
	case "", "Exercise", "Expire":
		return kind, nil
	}
	return "", fmt.Errorf("invalid -type %q for -command %s (want Exercise or Expire)", o.exerciseType, command)
}

func upperOrEmpty(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func money(v float64) string { return fmt.Sprintf("%.2f", v) }

func qty(v float64) string { return fmt.Sprintf("%.2f", v) }
