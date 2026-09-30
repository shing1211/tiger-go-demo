package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shing1211/tiger-go-demo/internal/rocli"
)

// TestParseIdentifier covers the OCC-style option identifier parser that feeds
// GetOptionDepth, GetOptionTradeTicks and GetOptionTimeline. Getting it wrong
// would send a plausible-looking but incorrect contract to Tiger, so both the
// happy path and every rejection are pinned.
//
// Expiries are parsed at midnight in the exchange's own timezone, exactly as
// the SDK does it, so the expected timestamps below are built with LoadLocation
// rather than in UTC.
func TestParseIdentifier(t *testing.T) {
	midnight := func(tz string, y int, m time.Month, d int) int64 {
		t.Helper()
		loc, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatalf("load %s: %v", tz, err)
		}
		return time.Date(y, m, d, 0, 0, 0, 0, loc).UnixMilli()
	}

	tests := []struct {
		name       string
		id         string
		wantSymbol string
		wantRight  string
		wantStrike string
		wantExpiry int64
	}{
		{
			name:       "US call",
			id:         "AAPL 250117C00200000",
			wantSymbol: "AAPL",
			wantRight:  "CALL",
			wantStrike: "200.000",
			wantExpiry: midnight("America/New_York", 2025, 1, 17),
		},
		{
			name:       "US put, lower case input",
			id:         "aapl 250117p00200000",
			wantSymbol: "AAPL",
			wantRight:  "PUT",
			wantStrike: "200.000",
			wantExpiry: midnight("America/New_York", 2025, 1, 17),
		},
		{
			name:       "HK underlying uses HK midnight",
			id:         "0700.HK 250630C00400000",
			wantSymbol: "0700.HK",
			wantRight:  "CALL",
			wantStrike: "400.000",
			wantExpiry: midnight("Asia/Hong_Kong", 2025, 6, 30),
		},
		{
			name:       "extra spaces are tolerated",
			id:         "  AAPL   250117C00150000  ",
			wantSymbol: "AAPL",
			wantRight:  "CALL",
			wantStrike: "150.000",
			wantExpiry: midnight("America/New_York", 2025, 1, 17),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIdentifier(tc.id)
			if err != nil {
				t.Fatalf("parseIdentifier(%q) failed: %v", tc.id, err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("Symbol = %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.Right != tc.wantRight {
				t.Errorf("Right = %q, want %q", got.Right, tc.wantRight)
			}
			if got.Strike != tc.wantStrike {
				t.Errorf("Strike = %q, want %q", got.Strike, tc.wantStrike)
			}
			if got.Expiry != tc.wantExpiry {
				t.Errorf("Expiry = %d (%s), want %d (%s)",
					got.Expiry, time.UnixMilli(got.Expiry).UTC().Format("2006-01-02"),
					tc.wantExpiry, time.UnixMilli(tc.wantExpiry).UTC().Format("2006-01-02"))
			}
		})
	}
}

// TestParseIdentifierRejects checks the malformed inputs a user is most likely
// to type. Each must be a clear local error, not a request to Tiger.
func TestParseIdentifierRejects(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantMsg string
	}{
		{"no space", "AAPL250117C00200000", "UNDERLYING YYMMDD"},
		{"empty", "", "UNDERLYING YYMMDD"},
		{"contract too short", "AAPL 250117C", "too short"},
		{"bad right", "AAPL 250117X00200000", "C (call) or P (put)"},
		{"non-numeric strike", "AAPL 250117C00A00000", "not numeric"},
		{"impossible date", "AAPL 991301C00200000", "invalid expiry date"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseIdentifier(tc.id)
			if err == nil {
				t.Fatalf("parseIdentifier(%q) should have failed", tc.id)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q should mention %q", err, tc.wantMsg)
			}
		})
	}
}

// TestParseIdentifiersRequiresInput checks the guard that stops an empty -ids
// from turning into a request with no contracts in it.
func TestParseIdentifiersRequiresInput(t *testing.T) {
	_, err := parseIdentifiers(nil)
	if err == nil {
		t.Fatal("parseIdentifiers(nil) should fail")
	}
	if !strings.Contains(err.Error(), "-ids") {
		t.Errorf("error should name the flag, got %v", err)
	}
}

// TestExpiryMillis checks the YYYY-MM-DD -> epoch-ms conversion used by the
// chain and analysis endpoints, including that a bad date is rejected locally.
func TestExpiryMillis(t *testing.T) {
	got, err := expiryMillis("2025-01-17", "AAPL")
	if err != nil {
		t.Fatalf("expiryMillis failed: %v", err)
	}
	// The expiry is midnight exchange-local, as Tiger's option API expects.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	want := time.Date(2025, 1, 17, 0, 0, 0, 0, loc).UnixMilli()
	if got != want {
		t.Errorf("expiryMillis = %d (%s), want %d", got, time.UnixMilli(got).Format("2006-01-02"), want)
	}
	if _, err := expiryMillis("17/01/2025", "AAPL"); err == nil {
		t.Error("a non-ISO date should be rejected")
	} else if !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error should state the expected format, got %v", err)
	}
}

// TestOptionTimezone pins the .HK rule the SDK also applies, since it decides
// which day an expiry timestamp lands on.
func TestOptionTimezone(t *testing.T) {
	if got := optionTimezone("AAPL"); got != "America/New_York" {
		t.Errorf("optionTimezone(AAPL) = %q", got)
	}
	if got := optionTimezone("0700.hk"); got != "Asia/Hong_Kong" {
		t.Errorf("optionTimezone(0700.hk) = %q", got)
	}
}

// chainRequest is the other half of what -op chain does before it reaches the
// SDK, and unlike expiryMillis and parseIdentifiers it had no test at all.
//
// The load-bearing assertion is the -itm mapping. "in" and "out" set a boolean
// that reads naturally as InTheMoney, and swapping the two would be a silent
// correctness bug: the command would return a plausible, well-formatted chain
// containing exactly the opposite contracts. The filter cases are checked by
// value, not by presence, so an inversion fails rather than passing.
func TestChainRequest(t *testing.T) {
	base := func(itm string) options {
		return options{Common: rocli.Common{Market: "US", Lang: "en"}, itm: itm}
	}

	t.Run("itm in sets InTheMoney true", func(t *testing.T) {
		req, err := chainRequest("AAPL", "2025-01-17", base("in"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.OptionFilter == nil || req.OptionFilter.InTheMoney == nil {
			t.Fatalf("-itm in should set a filter, got %#v", req.OptionFilter)
		}
		if !*req.OptionFilter.InTheMoney {
			t.Error("-itm in means in-the-money; InTheMoney must be true, not false")
		}
	})

	t.Run("itm out sets InTheMoney false", func(t *testing.T) {
		req, err := chainRequest("AAPL", "2025-01-17", base("out"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.OptionFilter == nil || req.OptionFilter.InTheMoney == nil {
			t.Fatalf("-itm out should set a filter, got %#v", req.OptionFilter)
		}
		if *req.OptionFilter.InTheMoney {
			t.Error("-itm out means out-of-the-money; InTheMoney must be false, not true")
		}
	})

	t.Run("itm all and empty send no filter", func(t *testing.T) {
		for _, itm := range []string{"", "all", "  ALL  "} {
			req, err := chainRequest("AAPL", "2025-01-17", base(itm))
			if err != nil {
				t.Fatalf("itm %q: unexpected error: %v", itm, err)
			}
			if req.OptionFilter != nil {
				t.Errorf("itm %q should not filter, got %#v", itm, req.OptionFilter)
			}
		}
	})

	t.Run("itm is case-insensitive and trimmed", func(t *testing.T) {
		req, err := chainRequest("AAPL", "2025-01-17", base("  IN  "))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.OptionFilter == nil || req.OptionFilter.InTheMoney == nil || !*req.OptionFilter.InTheMoney {
			t.Errorf(`"  IN  " should behave like "in", got %#v`, req.OptionFilter)
		}
	})

	t.Run("an unknown itm is rejected and says what is valid", func(t *testing.T) {
		_, err := chainRequest("AAPL", "2025-01-17", base("sideways"))
		if err == nil {
			t.Fatal("an unknown -itm should be an error, not a silently unsfiltered request")
		}
		for _, want := range []string{"sideways", "in", "out", "all"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention %q, got: %v", want, err)
			}
		}
	})

	t.Run("greeks sets ReturnGreekValue only when asked", func(t *testing.T) {
		o := base("")
		req, err := chainRequest("AAPL", "2025-01-17", o)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.ReturnGreekValue != nil {
			t.Error("without -greeks, ReturnGreekValue should be left unset, not sent false")
		}

		o.greeks = true
		req, err = chainRequest("AAPL", "2025-01-17", o)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.ReturnGreekValue == nil || !*req.ReturnGreekValue {
			t.Errorf("-greeks should set ReturnGreekValue true, got %#v", req.ReturnGreekValue)
		}
	})

	t.Run("the underlying and expiry reach the request", func(t *testing.T) {
		req, err := chainRequest("AAPL", "2025-01-17", base(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(req.OptionBasic) != 1 || req.OptionBasic[0].Symbol != "AAPL" {
			t.Fatalf("the underlying should be the single query item, got %#v", req.OptionBasic)
		}
		if req.OptionBasic[0].Expiry == 0 {
			t.Error("the expiry should be converted to epoch millis, not left zero")
		}
	})

	t.Run("a malformed expiry is rejected before any request is built", func(t *testing.T) {
		if _, err := chainRequest("AAPL", "17/01/2025", base("")); err == nil {
			t.Fatal("a malformed -expiry should be an error")
		}
	})
}
