package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
)

// These tests cover the -op endpoint this command added and the flag plumbing
// around it. Nothing here makes a request: the rendering is exercised against
// hand-built model values, and the SDK call itself needs a live account.

// TestPrintEntitlementDistinguishesAbsentFromZero is the reason printEntitlement
// exists as a separate function. The SDK's model makes the two cases
// distinguishable for the two pointer fields and not for the rest, and the
// output has to reflect exactly that rather than flattening everything to a
// number or a dash.
func TestPrintEntitlementDistinguishesAbsentFromZero(t *testing.T) {
	// Fixed instants in local time, because msToTime formats with the local
	// zone. Building the expectations from the same helper the printer uses
	// keeps this test independent of the machine's timezone.
	ts := func(s string) time.Time {
		t.Helper()
		t2, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return t2
	}
	ms := func(s string) int64 { return ts(s).UnixMilli() }
	at := func(s string) string { return ts(s).Format("2006-01-02 15:04:05") }

	tests := []struct {
		name string
		ent  *sdkmodel.AddonEntitlement
		// want and wantNot are substrings the output must and must not contain.
		want    []string
		wantNot []string
	}{
		{
			name: "nil response says so",
			ent:  nil,
			want: []string{"== addon entitlement ==", "(no entitlement returned)"},
		},
		{
			name: "nil ActivePlan is absent, not an empty plan",
			ent:  &sdkmodel.AddonEntitlement{UserLevel: "pro"},
			want: []string{"user_level  pro", "active_plan (none)", "addons (none)"},
		},
		{
			name: "nil EffectiveEntitlement is absent, not a table of zeros",
			ent: &sdkmodel.AddonEntitlement{
				UserLevel: "pro",
				ActivePlan: &sdkmodel.AddonActivePlan{
					PlanType:   "standard",
					ExpireTime: ms("2025-12-31 23:59:59"),
				},
			},
			want:    []string{"effective_entitlement (absent)", "plan_type=standard"},
			wantNot: []string{"history_stock", "QUOTA"},
		},
		{
			name: "a present but empty detail block prints real zeros",
			// The pointer is non-nil, so the server did send the block. Every
			// field is a plain int, so "absent" and "zero" are the same value
			// here and both must print as 0.
			ent: &sdkmodel.AddonEntitlement{
				UserLevel:            "pro",
				EffectiveEntitlement: &sdkmodel.AddonEntitlementDetail{},
			},
			want: []string{
				"effective_entitlement",
				"QUOTA",
				fmt.Sprintf("    %-22s %10d %10d", "history_stock", 0, 0),
				fmt.Sprintf("    %-22s %10d", "rate_multiple", 0),
			},
			wantNot: []string{"(absent)"},
		},
		{
			name: "a populated detail block prints every quota",
			ent: &sdkmodel.AddonEntitlement{
				UserLevel: "5",
				EffectiveEntitlement: &sdkmodel.AddonEntitlementDetail{
					HistoryStockLimit:       100,
					HistoryStockRemaining:   40,
					HistoryFutureLimit:      20,
					HistoryFutureRemaining:  20,
					HistoryOptionLimit:      50,
					HistoryOptionRemaining:  12,
					SubscribeLimit:          30,
					SubscribeRemaining:      29,
					SubscribeDepthLimit:     3,
					SubscribeDepthRemaining: 2,
					HighFreqLimit:           10,
					MidFreqLimit:            5,
					LowFreqLimit:            1,
					RateMultiple:            4,
				},
			},
			// Expectations are built with the same verbs the printer uses, so
			// this pins the values and the column order without hard-coding a
			// count of spaces.
			want: []string{
				"user_level  5",
				fmt.Sprintf("    %-22s %10d %10d", "history_stock", 100, 40),
				fmt.Sprintf("    %-22s %10d %10d", "history_future", 20, 20),
				fmt.Sprintf("    %-22s %10d %10d", "history_option", 50, 12),
				fmt.Sprintf("    %-22s %10d %10d", "subscribe", 30, 29),
				fmt.Sprintf("    %-22s %10d %10d", "subscribe_depth", 3, 2),
				fmt.Sprintf("    %-22s %10d", "high_freq_limit", 10),
				fmt.Sprintf("    %-22s %10d", "mid_freq_limit", 5),
				fmt.Sprintf("    %-22s %10d", "low_freq_limit", 1),
				fmt.Sprintf("    %-22s %10d", "rate_multiple", 4),
			},
		},
		{
			name: "addons are listed with their flags and times",
			ent: &sdkmodel.AddonEntitlement{
				Addons: []sdkmodel.AddonInfo{
					{
						PlanType:   "market_data",
						Active:     true,
						StartTime:  ms("2025-01-01 00:00:00"),
						ExpireTime: ms("2025-12-31 23:59:59"),
					},
					{
						// A server that omits the times yields zeros; msToTime
						// renders those as "-" rather than 1970.
						PlanType: "options_chain",
					},
				},
			},
			want: []string{
				"addons (2)",
				"market_data" + strings.Repeat(" ", 10) + "true" + strings.Repeat(" ", 4) +
					at("2025-01-01 00:00:00") + "  " + at("2025-12-31 23:59:59"),
				"options_chain" + strings.Repeat(" ", 8) + "false" + strings.Repeat(" ", 3) +
					"-" + strings.Repeat(" ", 20) + "-",
			},
		},
		{
			name: "an empty user level is a dash, not a blank",
			ent:  &sdkmodel.AddonEntitlement{},
			want: []string{"user_level  -"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			printEntitlement(&buf, tc.ent)
			got := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("output should contain %q, got:\n%s", want, got)
				}
			}
			for _, unwanted := range tc.wantNot {
				if strings.Contains(got, unwanted) {
					t.Errorf("output should not contain %q, got:\n%s", unwanted, got)
				}
			}
		})
	}
}

// TestUnmarshalAddonEntitlement checks the absent/zero claim against the real
// decoder rather than against hand-built structs: a server response that omits
// effectiveEntitlement must leave the pointer nil, and one that omits a field
// inside the block must leave that int at zero.
func TestUnmarshalAddonEntitlement(t *testing.T) {
	t.Run("omitted pointers stay nil", func(t *testing.T) {
		var got sdkmodel.AddonEntitlement
		if err := json.Unmarshal([]byte(`{"userLevel":2}`), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.UserLevel.String() != "2" {
			t.Errorf("UserLevel = %q; FlexString should accept a number", got.UserLevel)
		}
		if got.ActivePlan != nil {
			t.Errorf("ActivePlan = %+v, want nil for an omitted field", got.ActivePlan)
		}
		if got.EffectiveEntitlement != nil {
			t.Errorf("EffectiveEntitlement = %+v, want nil for an omitted field", got.EffectiveEntitlement)
		}
		if got.Addons != nil {
			t.Errorf("Addons = %+v, want nil for an omitted field", got.Addons)
		}
	})

	t.Run("omitted ints are zero and cannot be told from zero", func(t *testing.T) {
		// {} and {"historyStockLimit":0} decode to the same struct: the fields
		// are plain ints, so the demo prints both as 0 rather than inventing an
		// "absent" state the type cannot represent.
		var empty, explicit sdkmodel.AddonEntitlementDetail
		if err := json.Unmarshal([]byte(`{}`), &empty); err != nil {
			t.Fatalf("unmarshal {}: %v", err)
		}
		if err := json.Unmarshal([]byte(`{"historyStockLimit":0}`), &explicit); err != nil {
			t.Fatalf("unmarshal explicit zero: %v", err)
		}
		if empty != explicit {
			t.Errorf("an omitted int and an explicit zero must decode identically, got %+v vs %+v",
				empty, explicit)
		}
	})

	t.Run("a populated block decodes", func(t *testing.T) {
		var got sdkmodel.AddonEntitlement
		body := `{"userLevel":"pro","activePlan":{"planType":"standard","expireTime":1767225599000},
		          "addons":[{"planType":"market_data","active":true,"startTime":1,"expireTime":2}],
		          "effectiveEntitlement":{"historyStockLimit":100,"historyStockRemaining":40,"rateMultiple":4}}`
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.EffectiveEntitlement == nil {
			t.Fatal("EffectiveEntitlement should be non-nil")
		}
		if got.EffectiveEntitlement.HistoryStockRemaining != 40 {
			t.Errorf("HistoryStockRemaining = %d, want 40", got.EffectiveEntitlement.HistoryStockRemaining)
		}
		if got.ActivePlan == nil || got.ActivePlan.PlanType != "standard" {
			t.Errorf("ActivePlan = %+v", got.ActivePlan)
		}
		if len(got.Addons) != 1 || !got.Addons[0].Active {
			t.Errorf("Addons = %+v", got.Addons)
		}
	})
}

// TestDispatchOpRejectsUnknownOp checks the typo path: a clean message naming
// the unknown value and listing the valid ones, never a silent fallback to the
// default brief fetch.
func TestDispatchOpRejectsUnknownOp(t *testing.T) {
	err := dispatchOp(context.Background(), nil, "no-such-op", &bytes.Buffer{})
	if err == nil {
		t.Fatal("an unknown -op should fail")
	}
	if !strings.Contains(err.Error(), "no-such-op") {
		t.Errorf("error should name the unknown op, got %v", err)
	}
	if !strings.Contains(err.Error(), "addon-entitlement") {
		t.Errorf("error should list the valid ops, got %v", err)
	}
}

// TestDispatchOpIsWiredToTheEndpoint guards against ops and the dispatcher
// drifting apart, which would make the flag help promise an endpoint that does
// not exist.
func TestDispatchOpIsWiredToTheEndpoint(t *testing.T) {
	if len(ops) == 0 {
		t.Fatal("ops should not be empty")
	}
	seen := map[string]bool{}
	for _, op := range ops {
		if op == "" {
			t.Error("ops should not contain an empty name")
		}
		if seen[op] {
			t.Errorf("ops lists %q twice", op)
		}
		seen[op] = true

		// A wired endpoint gets past the default branch. The client is nil, so
		// the printer panics on the call; what matters here is only that we did
		// not get the "unknown -op" error back, which is what an unwired name
		// would produce.
		if err := dispatchOpQuietly(op); err != nil && strings.Contains(err.Error(), "unknown -op") {
			t.Errorf("-op %q is listed in ops but not handled by the dispatcher", op)
		}
	}
}

// dispatchOpQuietly calls dispatchOp with a nil client, recovering the panic a
// wired endpoint causes when it reaches the SDK. It returns the error only if
// the dispatcher refused the name.
func dispatchOpQuietly(op string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = nil // reached the printer, which is what we wanted to know
		}
	}()
	return dispatchOp(context.Background(), nil, op, io.Discard)
}

// TestOpsIncludesAddonEntitlement is a regression pin on the endpoint this
// command exists to cover.
func TestOpsIncludesAddonEntitlement(t *testing.T) {
	found := false
	for _, op := range ops {
		if op == "addon-entitlement" {
			found = true
		}
	}
	if !found {
		t.Errorf("ops = %v, want it to include addon-entitlement", ops)
	}
}

// TestHelpMentionsTheOp checks the -h text, since -h is the only invocation
// that works with no credentials and is therefore the only documentation a user
// can see before configuring anything.
func TestHelpMentionsTheOp(t *testing.T) {
	if !strings.Contains(usage, "addon-entitlement") {
		t.Errorf("the usage text should mention the new endpoint, got:\n%s", usage)
	}
}

// capture runs fn with a fresh buffer and returns what it wrote. cmd/quote's
// renderers take an explicit writer instead of sharing a package-level out, so
// the swap happens at the call site rather than behind a global.
func capture(t *testing.T, fn func(w io.Writer)) string {
	t.Helper()
	var buf bytes.Buffer
	fn(&buf)
	return buf.String()
}

func TestPrintMarketStates(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printMarketStates(w, []sdkmodel.MarketState{
				{Market: "US", Status: "normal", MarketStatus: "Trading", OpenTime: "2025-01-02 09:30:00"},
			})
		})
		// The row is built with the printer's own verb, so this pins the value
		// and the column order without hard-coding a count of spaces.
		for _, want := range []string{
			"== market state ==",
			fmt.Sprintf("  %-4s status=%-12s market_status=%-12s open=%s",
				"US", "normal", "Trading", "2025-01-02 09:30:00"),
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func(w io.Writer) { printMarketStates(w, nil) })
		for _, want := range []string{"== market state ==", "  (no data returned)"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})
}

// TestSplitSymbols covers the -symbols parsing the default path uses, including
// the case that must not reach Tiger: an empty flag.
func TestSplitSymbols(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"commas and spaces", " aapl , MSFT ", []string{"AAPL", "MSFT"}},
		{"empty entries dropped", "AAPL,,MSFT,", []string{"AAPL", "MSFT"}},
		{"already upper", "0700.HK", []string{"0700.HK"}},
		{"empty", "", nil},
		{"only separators", " , , ", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSymbols(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitSymbols(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitSymbols(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}
