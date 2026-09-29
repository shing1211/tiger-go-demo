package sdkcoverage

import (
	"sort"
	"testing"
)

func TestEveryAllowListEntryCarriesAReasonFromTheVocabulary(t *testing.T) {
	// The type system already refuses a reason outside the vocabulary — this
	// test is the belt to that braces, and it catches the case where someone
	// widens Reason to a plain string to work around a compile error.
	legal := map[Reason]bool{
		ReasonDeprecated: true, ReasonMutating: true, ReasonNotACall: true,
		ReasonNotUsed: true, ReasonInternal: true,
	}
	if len(legal) != 5 {
		t.Fatalf("the vocabulary is %d reasons, want 5; the sdk-coverage spec "+
			"names exactly five and cannot grow one at a time", len(legal))
	}
	for _, e := range AllowList() {
		if e.Method == "" {
			t.Errorf("an entry with no method: %+v", e)
		}
		if e.Reason == "" {
			t.Errorf("%s carries no reason; a gap cannot be tolerated silently", e.Method)
		}
		if !legal[e.Reason] {
			t.Errorf("%s carries %q, which is outside the closed vocabulary", e.Method, e.Reason)
		}
	}
}

func TestAllowListHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range AllowList() {
		if seen[e.Method] {
			t.Errorf("%s is listed twice", e.Method)
		}
		seen[e.Method] = true
	}
}

func TestAllowListIsExactlyNineteen(t *testing.T) {
	if got := len(AllowList()); got != 19 {
		t.Fatalf("allow-list has %d entries, want 19; the count is published in "+
			"README.md and pinned by openspec/specs/sdk-coverage", got)
	}
}

func TestReasonBreakdownMatchesTheDocumentedCounts(t *testing.T) {
	counts := map[Reason]int{}
	for _, e := range AllowList() {
		counts[e.Reason]++
	}
	want := map[Reason]int{
		ReasonDeprecated: 6, ReasonMutating: 7, ReasonNotACall: 1,
		ReasonNotUsed: 1, ReasonInternal: 4,
	}
	for reason, n := range want {
		if counts[reason] != n {
			t.Errorf("%s: %d entries, want %d", reason, counts[reason], n)
		}
	}
}

func TestAllowListIsSortedByMethod(t *testing.T) {
	var got []string
	for _, e := range AllowList() {
		got = append(got, e.Method)
	}
	if !sort.StringsAreSorted(got) {
		t.Error("keep the allow-list sorted by method, so a diff shows only real changes")
	}
}

func TestAllowListReturnsACopy(t *testing.T) {
	a := AllowList()
	a[0].Method = "Mutated"
	if AllowList()[0].Method == "Mutated" {
		t.Error("AllowList must return a copy; a caller could otherwise rewrite the table " +
			"and every later run would inherit the edit")
	}
}

// TestOnlyExecuteRawCarriesNotUsed is a record, not a rule that should drive
// code. RefreshToken, SetCurrentToken and GetAccountSubscriptions were
// "not-used" while uncovered, because nothing in the SDK's importable packages
// calls them — only the example programs in examples/manual_test/ and
// cmd/integ_token_refresh/. cmd/token now calls two of them, so they are
// covered and carry no line. The fact about the SDK is still the fact, and this
// test fails if one of them ever comes back carrying "internal", which would be
// a relabelling the module source does not support.
func TestOnlyExecuteRawCarriesNotUsed(t *testing.T) {
	for _, e := range AllowList() {
		if e.Method == "RefreshToken" || e.Method == "SetCurrentToken" || e.Method == "GetAccountSubscriptions" {
			if e.Reason == ReasonInternal {
				t.Errorf("%s is labelled internal, but the SDK's own importable "+
					"packages never call it — only examples/manual_test/ and "+
					"cmd/integ_token_refresh/ do", e.Method)
			}
		}
	}
}
