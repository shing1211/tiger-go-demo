package main

// The per-payload renderers live here rather than in main.go so the callback
// table stays a table: one line per feed that says which callback it is and
// which delivery count it bumps, with no format strings mixed in.
//
// printQuote keeps the pre-existing [QUOTE] line byte for byte, and is reused
// unchanged for the option, future and crypto feeds, which all deliver the
// same QuoteData payload. The SDK docstring for SubscribeMarket calls that
// feed "market status", but the wire is unambiguous — dataType=Quote with the
// market field set and no symbols — so a whole-market quote prints exactly
// like a per-symbol one and is not labelled as market status anywhere.

import (
	"fmt"

	sdkpb "github.com/tigerfintech/openapi-go-sdk/push/pb"
)

// printQuote renders a QuoteData payload. label is the feed it arrived on,
// so the option and future feeds are distinguishable from plain quotes even
// though the payload is identical.
func printQuote(label string, d *sdkpb.QuoteData) {
	fmt.Printf("[%s] %-10s last=%.4f bid=%.4f/%d ask=%.4f/%d vol=%d status=%s\n",
		label, d.GetSymbol(), d.GetLatestPrice(), d.GetBidPrice(), d.GetBidSize(),
		d.GetAskPrice(), d.GetAskSize(), d.GetVolume(), d.GetMarketStatus())
}

// printKline renders one minute bar. Amount and count are printed because a
// bar with a zero volume and a bar the server never sent look identical in
// the OHLC fields alone.
func printKline(d *sdkpb.KlineData) {
	fmt.Printf("[KLINE] %-10s o=%.4f h=%.4f l=%.4f c=%.4f avg=%.4f vol=%d n=%d amt=%.2f\n",
		d.GetSymbol(), d.GetOpen(), d.GetHigh(), d.GetLow(), d.GetClose(),
		d.GetAvg(), d.GetVolume(), d.GetCount(), d.GetAmount())
}

// printStockTop renders the stock ranking. The server groups the rows by
// indicator name, so the name is printed once per group rather than per row.
func printStockTop(d *sdkpb.StockTopData) {
	for _, group := range d.GetTopData() {
		fmt.Printf("[STOPTOP] market=%s indicator=%s rows=%d\n", d.GetMarket(), group.GetTargetName(), len(group.GetItem()))
		for i, item := range group.GetItem() {
			if i >= 10 {
				fmt.Printf("           ... %d more row(s)\n", len(group.GetItem())-10)
				break
			}
			fmt.Printf("           %-10s last=%.4f value=%.4f\n", item.GetSymbol(), item.GetLatestPrice(), item.GetTargetValue())
		}
	}
}

// printOptionTop renders the option ranking, which carries two shapes in one
// message: a large-order list and a target-value list. BigOrder has a
// direction and a price; the ranked items carry open interest instead.
func printOptionTop(d *sdkpb.OptionTopData) {
	for _, group := range d.GetTopData() {
		fmt.Printf("[OPTTOP] market=%s indicator=%s rows=%d big_orders=%d\n",
			d.GetMarket(), group.GetTargetName(), len(group.GetItem()), len(group.GetBigOrder()))
		for i, item := range group.GetItem() {
			if i >= 10 {
				fmt.Printf("          ... %d more row(s)\n", len(group.GetItem())-10)
				break
			}
			fmt.Printf("          %-24s vol=%.2f amt=%.2f oi=%.2f vol/oi=%.4f\n",
				optionLabel(item.GetSymbol(), item.GetExpiry(), item.GetStrike(), item.GetRight()),
				item.GetTotalVolume(), item.GetTotalAmount(), item.GetTotalOpenInt(), item.GetVolumeToOpenInt())
		}
		for i, bo := range group.GetBigOrder() {
			if i >= 10 {
				fmt.Printf("          ... %d more big order(s)\n", len(group.GetBigOrder())-10)
				break
			}
			fmt.Printf("          %-24s %-4s vol=%.2f price=%.4f amt=%.2f\n",
				optionLabel(bo.GetSymbol(), bo.GetExpiry(), bo.GetStrike(), bo.GetRight()),
				bo.GetDir(), bo.GetVolume(), bo.GetPrice(), bo.GetAmount())
		}
	}
}

// optionLabel assembles the four fields the option rows arrive in separately
// into one cell, so a row can be matched against a subscription symbol.
func optionLabel(symbol, expiry, strike, right string) string {
	return fmt.Sprintf("%s %s %s %s", symbol, expiry, strike, right)
}
