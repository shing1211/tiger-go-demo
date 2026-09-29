package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shing1211/tiger-go-demo/internal/sdkcoverage"
)

// TestSDKCoverage is the enforcement point for openspec/specs/sdk-coverage.
//
// It lives in test/ rather than in internal/ for a structural reason. The
// check's scan roots are cmd/ and internal/, so a check living in test/ is
// outside its own scan. A checker that could see itself would be able to
// satisfy the check it performs, and the way to prevent that is not a comment
// about the exclusion list but the absence of a path.
func TestSDKCoverage(t *testing.T) {
	result, err := sdkcoverage.Check()
	if err != nil {
		t.Fatalf("coverage check: %v", err)
	}

	var out bytes.Buffer
	result.Report(&out)
	t.Logf("\n%s", out.String())

	// The published figure. Changing either number is a claim about the SDK or
	// about this project, and belongs in README.md with a reason written down.
	// Accepting a new number here instead would let the figure drift silently,
	// which is the failure this whole check exists to prevent.
	const wantTotal, wantCovered = 159, 140
	if result.Total != wantTotal || result.Covered != wantCovered {
		t.Errorf("coverage is %d/%d, want %d/%d. The figure is published in "+
			"README.md and in openspec/specs/sdk-coverage, so a change to it needs "+
			"a reason written down there rather than a new value here.",
			result.Covered, result.Total, wantCovered, wantTotal)
	}

	if !result.OK() {
		for _, m := range result.NewGaps {
			t.Errorf("UNCOVERED, not on the allow-list: %s — cover it, or justify "+
				"it and add an Entry to internal/sdkcoverage/allowlist.go", m)
		}
		for _, m := range result.Stale {
			t.Errorf("now covered, delete its allow-list entry: %s", m)
		}
	}
	// The report has to carry the reasons, not just the counts. A coverage
	// number on its own cannot distinguish a deliberate exclusion from an
	// oversight, which is the distinction the whole table exists to make.
	for _, want := range []string{"deprecated", "mutating", "not-a-call", "not-used", "internal"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report should print the allow-list with its reasons; %q is missing", want)
		}
	}
}
