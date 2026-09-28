package main

import (
	"strings"
	"testing"
	"time"
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
