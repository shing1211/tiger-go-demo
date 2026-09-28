package rocli

import (
	"errors"
	"strings"
	"testing"

	"github.com/shing1211/tiger-go-demo/internal/config"
)

// TestExitCode pins the exit-code convention every binary in this project
// shares. cmd/trade depends on 2 meaning "fix your config" and 3 meaning "the
// safety gate refused", so these must not drift.
func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"other", errors.New("boom"), 1},
		{"dry-run refusal", &config.DryRunError{}, 3},
		{"unconfirmed refusal", &config.NotConfirmedError{}, 3},
		{"missing credentials", &config.MissingCredentialError{
			Missing: []string{config.EnvTigerID},
		}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Fatalf("ExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestList checks the comma-splitting used by every -symbols flag.
func TestList(t *testing.T) {
	got := List(" aapl , MSFT ,, 0700.hk ")
	want := []string{"AAPL", "MSFT", "0700.HK"}
	if len(got) != len(want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(List("")) != 0 {
		t.Error(`List("") should be empty`)
	}
}

// TestListRaw makes sure the case-preserving variant really preserves case,
// which matters for option identifiers and currency codes.
func TestListRaw(t *testing.T) {
	got := ListRaw("AAPL 250117C00200000, aapl 250117P00200000")
	if len(got) != 2 || got[0] != "AAPL 250117C00200000" || got[1] != "aapl 250117P00200000" {
		t.Fatalf("ListRaw = %v", got)
	}
}

// TestJSONFlag covers the scanner-filter flags, including the error path for
// malformed JSON, which must fail before any request is sent.
func TestJSONFlag(t *testing.T) {
	var out []map[string]interface{}
	if err := JSONFlag("base-filters", `[{"field":"market_cap","min":100}]`, &out); err != nil {
		t.Fatalf("valid JSON rejected: %v", err)
	}
	if len(out) != 1 || out[0]["field"] != "market_cap" {
		t.Fatalf("decoded = %v", out)
	}
	// An empty flag must leave the target untouched: it means "no filter".
	if err := JSONFlag("base-filters", "   ", &out); err != nil {
		t.Fatalf("empty value rejected: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("empty flag should not append, got %v", out)
	}
	if err := JSONFlag("base-filters", "{oops", &out); err == nil {
		t.Error("malformed JSON should be an error")
	} else if !strings.Contains(err.Error(), "-base-filters") {
		t.Errorf("error should name the flag, got %v", err)
	}
}

// TestFloatAndIntRejectJunk checks that a bad numeric flag is refused locally
// instead of being forwarded to Tiger as a zero.
func TestFloatAndIntRejectJunk(t *testing.T) {
	if _, err := Float("limit-price", "abc"); err == nil {
		t.Error("Float should reject non-numeric input")
	}
	if v, err := Float("limit-price", ""); err != nil || v != 0 {
		t.Errorf(`Float("") = %v, %v; want 0, nil`, v, err)
	}
	if v, err := Int("page", " 42 "); err != nil || v != 42 {
		t.Errorf(`Int(" 42 ") = %v, %v; want 42, nil`, v, err)
	}
	if _, err := Int("page", "4.2"); err == nil {
		t.Error("Int should reject a float")
	}
}

// TestMSFmtAndDash pin the placeholder behaviour the tables rely on: a zero
// timestamp and an empty string both render as "-" so columns stay aligned.
func TestMSFmtAndDash(t *testing.T) {
	if got := MSFmt(0); got != "-" {
		t.Errorf("MSFmt(0) = %q, want %q", got, "-")
	}
	if got := MSFmt(1737072000000); got == "-" || !strings.Contains(got, "2025") {
		t.Errorf("MSFmt(1737072000000) = %q, want a 2025 timestamp", got)
	}
	if got := Dash("  "); got != "-" {
		t.Errorf("Dash(blank) = %q, want %q", got, "-")
	}
	if got := Dash("AAPL"); got != "AAPL" {
		t.Errorf("Dash(AAPL) = %q", got)
	}
}

// TestTruncate checks the "we printed fewer rows than exist" note, so a broad
// query never silently looks complete.
func TestTruncate(t *testing.T) {
	var b strings.Builder
	Truncate(&b, 5, 50, 5)
	if !strings.Contains(b.String(), "45 more row(s)") {
		t.Errorf("expected a truncation note, got %q", b.String())
	}
	b.Reset()
	Truncate(&b, 5, 5, 5)
	if b.String() != "" {
		t.Errorf("nothing should be printed when nothing was dropped, got %q", b.String())
	}
}
