package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
)

// This file turns user-facing flags into SDK request structs.
//
// Two shapes of option input exist in Tiger's API and the SDK reflects that:
// GetOptionQuote and GetOptionKlineWithOpts take OCC-style identifiers as
// strings and parse them internally, while GetOptionDepth, GetOptionTradeTicks
// and GetOptionTimeline take []model.OptionQueryItem and expect us to supply
// the expiry as an epoch-millisecond timestamp. parseIdentifier below does the
// same job the SDK's own unexported parser does, so both endpoint families can
// be driven from one -ids flag.

// parseIdentifier splits "AAPL 250117C00200000" into its parts.
//
// strike is Tiger's OCC integer divided by 1000, matching the SDK.
func parseIdentifier(id string) (sdkmodel.OptionQueryItem, error) {
	parts := strings.SplitN(strings.TrimSpace(id), " ", 2)
	if len(parts) != 2 {
		return sdkmodel.OptionQueryItem{}, fmt.Errorf(
			"invalid option identifier %q: want \"UNDERLYING YYMMDDC|P STRIKE\", e.g. \"AAPL 250117C00200000\"", id)
	}
	symbol := strings.ToUpper(strings.TrimSpace(parts[0]))
	rest := strings.TrimSpace(parts[1])
	rest = strings.ToUpper(rest)
	// YYMMDD + C|P + 8 strike digits.
	if len(rest) < 15 {
		return sdkmodel.OptionQueryItem{}, fmt.Errorf(
			"invalid option identifier %q: contract part %q is too short (want 15+ characters: YYMMDDC/P + 8 digits)", id, rest)
	}

	loc, err := time.LoadLocation(optionTimezone(symbol))
	if err != nil {
		return sdkmodel.OptionQueryItem{}, fmt.Errorf("option identifier %q: %w", id, err)
	}
	expiry, err := time.ParseInLocation("060102", rest[:6], loc)
	if err != nil {
		return sdkmodel.OptionQueryItem{}, fmt.Errorf("option identifier %q: invalid expiry date %q", id, rest[:6])
	}

	var right string
	switch rest[6] {
	case 'C':
		right = "CALL"
	case 'P':
		right = "PUT"
	default:
		return sdkmodel.OptionQueryItem{}, fmt.Errorf(
			"option identifier %q: position 7 must be C (call) or P (put), got %q", id, string(rest[6]))
	}

	raw, err := strconv.ParseInt(rest[7:], 10, 64)
	if err != nil {
		return sdkmodel.OptionQueryItem{}, fmt.Errorf("option identifier %q: strike %q is not numeric", id, rest[7:])
	}
	// The SDK sends the strike as a decimal string, e.g. 200.000.
	strike := strconv.FormatFloat(float64(raw)/1000.0, 'f', 3, 64)

	return sdkmodel.OptionQueryItem{
		Symbol: symbol,
		Expiry: expiry.UnixMilli(),
		Right:  right,
		Strike: strike,
	}, nil
}

// parseIdentifiers is parseIdentifier over a list, failing on the first bad one
// so the user is told which identifier to fix.
func parseIdentifiers(ids []string) ([]sdkmodel.OptionQueryItem, error) {
	if len(ids) == 0 {
		return nil, errors.New("-ids is required for this endpoint, e.g. -ids \"AAPL 250117C00200000\"")
	}
	out := make([]sdkmodel.OptionQueryItem, 0, len(ids))
	for _, id := range ids {
		item, err := parseIdentifier(id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// optionTimezone mirrors the SDK's inference: .HK underlyings are quoted in
// Hong Kong time, everything else in US Eastern.
func optionTimezone(symbol string) string {
	if strings.HasSuffix(strings.ToUpper(symbol), ".HK") {
		return "Asia/Hong_Kong"
	}
	return "America/New_York"
}

// expiryMillis converts a YYYY-MM-DD expiry into epoch milliseconds in the
// underlying's own timezone, which is what option_chain and option_analysis
// expect.
func expiryMillis(expiry, symbol string) (int64, error) {
	loc, err := time.LoadLocation(optionTimezone(symbol))
	if err != nil {
		return 0, fmt.Errorf("expiry %q: %w", expiry, err)
	}
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(expiry), loc)
	if err != nil {
		return 0, fmt.Errorf("invalid -expiry %q: want YYYY-MM-DD", expiry)
	}
	return t.UnixMilli(), nil
}
