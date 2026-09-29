package rocli

import (
	"strings"
	"testing"
	"time"
)

// These pin the edge cases of the helpers that were byte-identical in all four
// data commands, BEFORE the move. Once four copies become one, a behaviour
// change can hide inside a rename, and a test written after the move would
// only record whatever the move happened to produce.

// TestDashOr: the fallback is parameterised, which is why this could not simply
// reuse Dash. The call sites pass seven different ones -- "sdk default",
// "server default", "all", "unset", "today", "open", "any" -- and collapsing
// them to a dash would silently replace seven meanings.
func TestDashOr(t *testing.T) {
	tests := []struct {
		in, fallback, want string
	}{
		{"", "-", "-"},
		{"", "any", "any"},
		{"   ", "-", "-"},             // whitespace-only is blank
		{"\t\n", "any", "any"},        // and so is any other whitespace
		{"value", "-", "value"},       // a real value passes through unchanged
		{" padded ", "-", " padded "}, // trimming is the caller's business, not the renderer's
	}
	for _, tc := range tests {
		if got := DashOr(tc.in, tc.fallback); got != tc.want {
			t.Errorf("DashOr(%q, %q) = %q, want %q", tc.in, tc.fallback, got, tc.want)
		}
	}
}

// TestDashIsDashOrWithADash keeps the two from drifting into different answers
// for the same input, which is the whole risk of having both.
func TestDashIsDashOrWithADash(t *testing.T) {
	for _, in := range []string{"", "  ", "x", "\t"} {
		if Dash(in) != DashOr(in, "-") {
			t.Errorf("Dash(%q) and DashOr(%q, \"-\") disagree", in, in)
		}
	}
}

// TestRequiredFlagNamesTheFlagAndAnExample: the message has to be diagnosable
// without a network or credentials, so it names the flag and a value that would
// work.
func TestRequiredFlag(t *testing.T) {
	err := RequiredFlag("-expiry", "20260619")
	if err == nil {
		t.Fatal("RequiredFlag should return an error")
	}
	for _, want := range []string{"expiry", "20260619"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q should name %q", err.Error(), want)
		}
	}
	// A leading dash on the name is trimmed: the message reads "expiry is
	// required", not "--expiry is required".
	if strings.Contains(err.Error(), "--expiry") {
		t.Errorf("message %q should not double the dash", err.Error())
	}
}

// TestFirstSymbol: the list is upper-cased and blanks dropped on the way, so
// the first entry is the first real one and not an empty string.
func TestFirstSymbol(t *testing.T) {
	tests := []struct{ in, want string }{
		{"AAPL", "AAPL"},
		{"AAPL,MSFT", "AAPL"},
		{"  AAPL , MSFT ", "AAPL"},
		{"aapl", "AAPL"},
		{"", ""},
		{",,", ""},
	}
	for _, tc := range tests {
		if got := FirstSymbol(tc.in); got != tc.want {
			t.Errorf("FirstSymbol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDateMillis: nil for an absent or unparseable date, so the request omits
// the field rather than sending a zero that Tiger would read as 1970. The
// expected value is UTC because time.Parse is, so this is timezone-independent.
func TestDateMillis(t *testing.T) {
	for _, in := range []string{"", "   ", "not-a-date", "2026-13-01", "20260619"} {
		if got := DateMillis(in); got != nil {
			t.Errorf("DateMillis(%q) = %v, want nil", in, *got)
		}
	}
	got := DateMillis("2026-06-19")
	if got == nil {
		t.Fatal("DateMillis(2026-06-19) should parse")
	}
	want := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC).UnixMilli()
	if *got != want {
		t.Errorf("DateMillis(2026-06-19) = %d, want %d", *got, want)
	}
}

// TestPx: four decimal places, so a price column lines up.
func TestPx(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{1.5, "1.5000"},
		{0, "0.0000"},
		{-2.25, "-2.2500"},
		{187.2512, "187.2512"},
	}
	for _, tc := range tests {
		if got := Px(tc.in); got != tc.want {
			t.Errorf("Px(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
