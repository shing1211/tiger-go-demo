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

// TestPrintRealTimeBriefs has no empty-result branch, and that is deliberate:
// the heading and the column header print whether or not the server returned a
// brief, so the absence is visible as a table with no rows rather than as a
// sentence the reader has to interpret. The other 60 renderers in this project
// print "(no rows returned)" here; adding that would change the output.
func TestPrintRealTimeBriefs(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printRealTimeBriefs(w, []sdkmodel.Brief{
				{Symbol: "AAPL", LatestPrice: 187.25, Change: 1.5, ChangeRate: 0.81, Volume: 1234567, LatestTime: 1767225600000},
			})
		})
		for _, want := range []string{
			"== real-time quotes ==",
			"SYMBOL", "LAST", "CHG%",
			fmt.Sprintf("  %-10s %10.4f %10.4f %9.2f%% %12d %10s",
				"AAPL", 187.25, 1.5, 0.81, 1234567, msToTime(1767225600000)),
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("an empty response prints the heading and no rows", func(t *testing.T) {
		got := capture(t, func(w io.Writer) { printRealTimeBriefs(w, nil) })
		for _, want := range []string{"== real-time quotes ==", "SYMBOL"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
		for _, unwanted := range []string{"no rows returned", "no data returned"} {
			if strings.Contains(got, unwanted) {
				t.Errorf("this renderer has no empty-result notice, so output should not contain %q; got:\n%s", unwanted, got)
			}
		}
	})
}

func TestPrintRealTimeKlines(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printRealTimeKlines(w, []sdkmodel.Kline{
				{
					Symbol:        "AAPL",
					NextPageToken: "",
					Items: []sdkmodel.KlineItem{
						{Time: 1767225600000, Open: 185.0, High: 188.5, Low: 184.25, Close: 187.25, Volume: 1000},
						{Time: 1767312000000, Open: 187.25, High: 190.0, Low: 186.0, Close: 189.5, Volume: 2000},
					},
				},
			}, "day")
		})
		// The requested period is the server's paging parameter, not a property
		// of the response, so the renderer takes it and puts it in the heading.
		for _, want := range []string{
			"\n== k-lines (period=day) ==",
			"AAPL (next_page_token=-)", // an omitted token is a dash, not a blank
			"TIME", "CLOSE",
			fmt.Sprintf("  %-22s %10.4f %10.4f %10.4f %10.4f %12d",
				msToTime(1767225600000), 185.0, 188.5, 184.25, 187.25, 1000),
			fmt.Sprintf("  %-22s %10.4f %10.4f %10.4f %10.4f %12d",
				msToTime(1767312000000), 187.25, 190.0, 186.0, 189.5, 2000),
		} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("a present page token prints instead of a dash", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printRealTimeKlines(w, []sdkmodel.Kline{{Symbol: "AAPL", NextPageToken: "tok-2"}}, "week")
		})
		if !strings.Contains(got, "AAPL (next_page_token=tok-2)") {
			t.Errorf("a page token should print verbatim, got:\n%s", got)
		}
		if !strings.Contains(got, "\n== k-lines (period=week) ==") {
			t.Errorf("the heading should carry the requested period, got:\n%s", got)
		}
	})

	t.Run("no symbols prints the heading and nothing else", func(t *testing.T) {
		// The column header sits inside the per-symbol loop, so an empty
		// response prints the heading alone. There is no empty-result notice
		// here either; adding one would change the output.
		got := capture(t, func(w io.Writer) { printRealTimeKlines(w, nil, "day") })
		if got != "\n== k-lines (period=day) ==\n" {
			t.Errorf("an empty response should print the heading only, got:\n%q", got)
		}
	})
}

// TestPrintIntradayTimelines pins the two things about this renderer that a
// reader would otherwise assume are bugs: the ten-point cap is a literal and not
// the -limit flag, and the notice it prints is the hand-rolled
// "... N more point(s)" rather than the shared rocli.Truncate wording. Both are
// what the code has always printed.
func TestPrintIntradayTimelines(t *testing.T) {
	// points builds n distinguishable intraday points. The price rises by 1 each
	// time, so the test can tell which points printed and which were capped.
	points := func(n int) []sdkmodel.TimelineItem {
		items := make([]sdkmodel.TimelineItem, n)
		for i := range items {
			items[i] = sdkmodel.TimelineItem{
				Time:     int64(1767225600000 + i*60000),
				Price:    float64(100 + i),
				AvgPrice: 100.5 + float64(i),
				Volume:   int64(10 * (i + 1)),
			}
		}
		return items
	}

	t.Run("a bucket is capped at ten points and says how many it hid", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printIntradayTimelines(w, []sdkmodel.Timeline{
				{Symbol: "AAPL", Period: "day", PreClose: 99.5, Intraday: &sdkmodel.TimelineBucket{Items: points(12)}},
			})
		})
		if !strings.Contains(got, fmt.Sprintf("  %-10s period=%-8s pre_close=%.4f", "AAPL", "day", 99.5)) {
			t.Errorf("the session heading should print, got:\n%s", got)
		}
		if !strings.Contains(got, "[intraday] 12 point(s)") {
			t.Errorf("the bucket should report its true size, not the printed size, got:\n%s", got)
		}
		// The first point and the tenth print; the eleventh and twelfth do not.
		if !strings.Contains(got, "price=100.0000") || !strings.Contains(got, "price=109.0000") {
			t.Errorf("points 1..10 should print, got:\n%s", got)
		}
		for _, hidden := range []string{"price=110.0000", "price=111.0000"} {
			if strings.Contains(got, hidden) {
				t.Errorf("point past the ten-point cap should not print (%q), got:\n%s", hidden, got)
			}
		}
		// The exact notice, character for character. It is not rocli.Truncate
		// and must not be rewritten to match it.
		if !strings.Contains(got, "    ... 2 more point(s)\n") {
			t.Errorf("the truncation notice should say two more points, got:\n%s", got)
		}
	})

	t.Run("a bucket of exactly ten points is not truncated", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printIntradayTimelines(w, []sdkmodel.Timeline{
				{Symbol: "AAPL", Intraday: &sdkmodel.TimelineBucket{Items: points(10)}},
			})
		})
		if !strings.Contains(got, "price=109.0000") {
			t.Errorf("the tenth point should print, got:\n%s", got)
		}
		if strings.Contains(got, "more point(s)") {
			t.Errorf("ten points is the cap, not over it, so nothing should be truncated; got:\n%s", got)
		}
	})

	t.Run("a missing or empty bucket is skipped entirely", func(t *testing.T) {
		// Nil buckets and zero-length buckets both fall through the same guard,
		// so a symbol with no after-hours session prints nothing for it - not a
		// heading, and not "0 point(s)".
		got := capture(t, func(w io.Writer) {
			printIntradayTimelines(w, []sdkmodel.Timeline{
				{Symbol: "AAPL", Intraday: &sdkmodel.TimelineBucket{}, AfterHours: nil},
			})
		})
		want := "\n== intraday timeline ==\n" +
			fmt.Sprintf("  %-10s period=%-8s pre_close=%.4f\n", "AAPL", "", 0.0)
		if got != want {
			t.Errorf("only the session heading should print, got:\n%q", got)
		}
	})

	t.Run("all three session buckets print in order", func(t *testing.T) {
		got := capture(t, func(w io.Writer) {
			printIntradayTimelines(w, []sdkmodel.Timeline{
				{
					Symbol:     "AAPL",
					PreHours:   &sdkmodel.TimelineBucket{Items: points(1)},
					Intraday:   &sdkmodel.TimelineBucket{Items: points(1)},
					AfterHours: &sdkmodel.TimelineBucket{Items: points(1)},
				},
			})
		})
		for _, want := range []string{"[pre_hours] 1 point(s)", "[intraday] 1 point(s)", "[after_hours] 1 point(s)"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
		if strings.Index(got, "[pre_hours]") > strings.Index(got, "[intraday]") ||
			strings.Index(got, "[intraday]") > strings.Index(got, "[after_hours]") {
			t.Errorf("buckets should print pre_hours, intraday, after_hours, got:\n%s", got)
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
