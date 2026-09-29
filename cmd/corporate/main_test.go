package main

import (
	"os"
	"strings"
	"testing"
)

// These tests pin the -op vocabulary against the dispatcher and the usage text.
// cmd/quote and cmd/push each carry the equivalent check for their own
// vocabulary; the four commands built on internal/rocli had the same ops-slice
// shape and no such test, and the drift that test exists to prevent was present
// in three of them: reference's usage text was missing trade-metas, trade-rank
// and timeline-history, and this file's README counterpart was missing three
// reference ops and options' kline-plain.
//
// An op that is listed in the flag help but not handled by the dispatcher is a
// lie in the help text, and the list is the only place that can catch it.

// TestOpsHasNoDuplicates: a repeated name makes the flag help list an endpoint
// twice, and which of the two entries dispatches becomes a question about source
// order rather than about intent.
func TestOpsHasNoDuplicates(t *testing.T) {
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
	}
}

// TestUsageMentionsEveryOp checks the -h text, since -h is the only invocation
// that works with no credentials and is therefore the only documentation a user
// can see before configuring anything. An endpoint reachable but undocumented
// is a feature nobody can find.
func TestUsageMentionsEveryOp(t *testing.T) {
	for _, op := range ops {
		if !strings.Contains(usage, op) {
			t.Errorf("ops lists %q but the usage text never mentions it; -h is the "+
				"only documentation visible before credentials are configured.\nusage:\n%s",
				op, usage)
		}
	}
}

// TestOpsReachADispatcher reads the package's own main.go and asserts every op
// appears as a switch case. dispatch takes a live client, so there is no registry
// to interrogate at runtime and no way to call it hermetically; reading the
// source is the same technique cmd/push and cmd/token use to prove they cannot
// reach an order write, so it is an established idiom here rather than a new one.
func TestOpsReachADispatcher(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(src)
	for _, op := range ops {
		if !strings.Contains(body, `case "`+op+`":`) {
			t.Errorf("-op %q is listed in ops and advertised in -h, but dispatch has "+
				"no case for it, so it would fall through to the unknown-op error", op)
		}
	}
	// And the reverse: a case with no ops entry is an endpoint the flag help
	// does not advertise. Less harmful than the other direction, still drift.
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `case "`) || !strings.HasSuffix(line, `":`) {
			continue
		}
		op := strings.TrimSuffix(strings.TrimPrefix(line, `case "`), `":`)
		if op == "" {
			continue
		}
		found := false
		for _, listed := range ops {
			if listed == op {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("dispatch handles %q but ops does not list it, so -h never "+
				"advertises it", op)
		}
	}
}
