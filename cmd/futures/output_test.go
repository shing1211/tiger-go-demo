package main

import (
	"bytes"
	"strings"
	"testing"

	sdkmodel "github.com/tigerfintech/openapi-go-sdk/model"
)

// capture redirects the package-level out for the duration of one call and
// returns what was written.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	saved := out
	out = &buf
	defer func() { out = saved }()
	fn()
	return buf.String()
}

// printContracts already took model values rather than a client, so it needed no
// split to become testable -- only a test. The two assertions are the ones this
// project's honesty rules care about: a missing value renders as a dash rather
// than a blank that reads as an empty column, and an empty result SAYS it is
// empty rather than printing a bare header.
func TestPrintContracts(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		got := capture(t, func() {
			printContracts([]sdkmodel.FutureContractInfo{
				{ContractCode: "CLmain", Exchange: "COMEX", Name: "Crude Oil (Light Sweet)",
					ContractMonth: "202609", Currency: "USD", Multiplier: 1000,
					LastTradingDate: "2026-08-20", Continuous: true},
			})
		})
		for _, want := range []string{"CLmain", "COMEX", "Crude Oil", "202609", "USD", "1000.00", "2026-08-20"} {
			if !strings.Contains(got, want) {
				t.Errorf("output should contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("absent fields are dashed, not blank", func(t *testing.T) {
		got := capture(t, func() {
			printContracts([]sdkmodel.FutureContractInfo{{ContractCode: "X"}})
		})
		if !strings.Contains(got, "-") {
			t.Errorf("absent fields should render as a dash; got:\n%s", got)
		}
		// A zero multiplier is a real zero, not an absent value, so it prints as
		// a number rather than a dash. The two are different facts and the
		// output has to keep them apart. The column is two decimal places.
		if !strings.Contains(got, "0.00") {
			t.Errorf("a zero multiplier is a real value and should print as one; got:\n%s", got)
		}
	})

	t.Run("empty says so", func(t *testing.T) {
		got := capture(t, func() { printContracts(nil) })
		if !strings.Contains(got, "no contracts") {
			t.Errorf("an empty result should say so rather than print a bare header; got:\n%s", got)
		}
	})
}
