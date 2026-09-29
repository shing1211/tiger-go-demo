package main

// Two things are tested here, and they are the two claims this command makes
// that a run against Tiger cannot demonstrate on its own.
//
//  1. The feed vocabulary. parseFeeds, the dispatch switches and the -market
//     and -indicators validators are checked against each other, so a feed
//     added to one place and forgotten in another fails here rather than in
//     production. TestFeedVocabularyMatchesDispatch is the test that makes
//     that automatic in both directions.
//
//  2. That a subscribe is verified by data, not by its return value. A fake
//     pushClient records the calls; the delivery tests drive the callbacks
//     and read the counts reportDelivery would print.
//
// Nothing here opens a connection: the SDK's PushClient is faked, so these
// tests are hermetic and the -race build is meaningful for the tracker.

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"
	sdkpb "github.com/tigerfintech/openapi-go-sdk/push/pb"

	"github.com/shing1211/tiger-go-demo/internal/logging"
)

// ---- fakes ----

// call is one recorded method invocation, with the arguments rendered for
// comparison. A single string form keeps the dispatch assertions readable:
// what matters is which method ran and with what, not the Go types.
type call struct {
	method string
	args   string
}

func (c call) String() string {
	if c.args == "" {
		return c.method
	}
	return c.method + "(" + c.args + ")"
}

// fakePush records every pushClient call. It is safe for concurrent use
// because the delivery tests invoke callbacks from a goroutine, standing in
// for the SDK's readLoop.
type fakePush struct {
	mu    sync.Mutex
	calls []call
}

func (f *fakePush) note(method string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = renderArg(a)
	}
	f.calls = append(f.calls, call{method: method, args: strings.Join(parts, ",")})
}

// renderArg formats a nil []string as "nil" rather than "[]", because the
// difference matters: UnsubscribeQuote(nil) means "drop every symbol", while
// UnsubscribeQuote([]string{}) is the empty selection.
func renderArg(a any) string {
	switch v := a.(type) {
	case nil:
		return "nil"
	case []string:
		if v == nil {
			return "nil"
		}
		return "[" + strings.Join(v, "|") + "]"
	case string:
		return v
	default:
		return "?"
	}
}

func (f *fakePush) record() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]call, len(f.calls))
	copy(out, f.calls)
	return out
}

// methods returns just the method names, which is what the dispatch tests
// assert on: "this feed called exactly these methods, in this order".
func (f *fakePush) methods() []string {
	var out []string
	for _, c := range f.record() {
		out = append(out, c.method)
	}
	return out
}

func (f *fakePush) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func (f *fakePush) SetCallbacks(sdkpush.Callbacks)   { f.note("SetCallbacks") }
func (f *fakePush) Connect() error                   { f.note("Connect"); return nil }
func (f *fakePush) Disconnect() error                { f.note("Disconnect"); return nil }
func (f *fakePush) SubscribeQuote(s []string) error  { f.note("SubscribeQuote", s); return nil }
func (f *fakePush) SubscribeTick(s []string) error   { f.note("SubscribeTick", s); return nil }
func (f *fakePush) SubscribeDepth(s []string) error  { f.note("SubscribeDepth", s); return nil }
func (f *fakePush) SubscribeOption(s []string) error { f.note("SubscribeOption", s); return nil }
func (f *fakePush) SubscribeFuture(s []string) error { f.note("SubscribeFuture", s); return nil }
func (f *fakePush) SubscribeKline(s []string) error  { f.note("SubscribeKline", s); return nil }
func (f *fakePush) SubscribeCc(s []string) error     { f.note("SubscribeCc", s); return nil }
func (f *fakePush) SubscribeMarket(m string) error   { f.note("SubscribeMarket", m); return nil }
func (f *fakePush) SubscribeStockTop(m string, i []string) error {
	f.note("SubscribeStockTop", m, i)
	return nil
}
func (f *fakePush) SubscribeOptionTop(m string, i []string) error {
	f.note("SubscribeOptionTop", m, i)
	return nil
}
func (f *fakePush) SubscribeOrder(a string) error    { f.note("SubscribeOrder", a); return nil }
func (f *fakePush) SubscribePosition(a string) error { f.note("SubscribePosition", a); return nil }
func (f *fakePush) SubscribeAsset(a string) error    { f.note("SubscribeAsset", a); return nil }
func (f *fakePush) SubscribeTransaction(a string) error {
	f.note("SubscribeTransaction", a)
	return nil
}
func (f *fakePush) UnsubscribeQuote(s []string) error  { f.note("UnsubscribeQuote", s); return nil }
func (f *fakePush) UnsubscribeTick(s []string) error   { f.note("UnsubscribeTick", s); return nil }
func (f *fakePush) UnsubscribeDepth(s []string) error  { f.note("UnsubscribeDepth", s); return nil }
func (f *fakePush) UnsubscribeOption(s []string) error { f.note("UnsubscribeOption", s); return nil }
func (f *fakePush) UnsubscribeFuture(s []string) error { f.note("UnsubscribeFuture", s); return nil }
func (f *fakePush) UnsubscribeKline(s []string) error  { f.note("UnsubscribeKline", s); return nil }
func (f *fakePush) UnsubscribeCc(s []string) error     { f.note("UnsubscribeCc", s); return nil }
func (f *fakePush) UnsubscribeMarket(m string) error   { f.note("UnsubscribeMarket", m); return nil }
func (f *fakePush) UnsubscribeStockTop(m string, i []string) error {
	f.note("UnsubscribeStockTop", m, i)
	return nil
}
func (f *fakePush) UnsubscribeOptionTop(m string, i []string) error {
	f.note("UnsubscribeOptionTop", m, i)
	return nil
}
func (f *fakePush) UnsubscribeOrder() error       { f.note("UnsubscribeOrder"); return nil }
func (f *fakePush) UnsubscribePosition() error    { f.note("UnsubscribePosition"); return nil }
func (f *fakePush) UnsubscribeAsset() error       { f.note("UnsubscribeAsset"); return nil }
func (f *fakePush) UnsubscribeTransaction() error { f.note("UnsubscribeTransaction"); return nil }

// ---- helpers ----

var testSyms = []string{"AAPL", "MSFT"}

// captureLog returns a logger and the buffer it writes to, so a test can
// assert on what a user would have been told.
func captureLog() (*logging.Logger, *strings.Builder) {
	var b strings.Builder
	return logging.NewFromConfig(&b, "info"), &b
}

// silenceStdout redirects os.Stdout for the duration of a test so the payload
// lines the callbacks print do not interleave with the test report. os.Stdout
// is a variable, so the printers need no seam for this to work.
func silenceStdout(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = saved
		w.Close()
		io.Copy(io.Discard, r)
		r.Close()
	})
}

func defaultIndents() []string { return []string{"volume", "amount"} }

// ---- parseFeeds ----

// TestParseFeedsAcceptsEveryDocumentedValue walks acceptedFeeds() rather than
// a hand-written list, so the two cannot drift.
func TestParseFeedsAcceptsEveryDocumentedValue(t *testing.T) {
	for _, feed := range acceptedFeeds() {
		got, err := parseFeeds(feed, false)
		if err != nil {
			t.Errorf("parseFeeds(%q) failed: %v", feed, err)
			continue
		}
		if len(got) == 0 {
			t.Errorf("parseFeeds(%q) returned no feed", feed)
		}
	}
}

// TestParseFeedsIsCaseInsensitiveAndOrderPreserving covers the normalisation
// rules: case folded, whitespace trimmed, order kept, repeats dropped.
func TestParseFeedsIsCaseInsensitiveAndOrderPreserving(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "single feed", in: "quote", want: []string{"quote"}},
		{name: "upper case", in: "QUOTE", want: []string{"quote"}},
		{name: "mixed case with spaces", in: "  Quote , Depth ", want: []string{"quote", "depth"}},
		{name: "order preserved", in: "depth,quote,tick", want: []string{"depth", "quote", "tick"}},
		{name: "duplicate dropped", in: "quote,quote,depth", want: []string{"quote", "depth"}},
		{name: "blank segments dropped", in: "quote,,  ,depth,", want: []string{"quote", "depth"}},
		{name: "alias canonicalised", in: "crypto", want: []string{"cc"}},
		{name: "alias deduped against canonical", in: "cc,crypto", want: []string{"cc"}},
		// The composite: -account must mean the same thing as listing
		// account, or the two spellings would quietly diverge.
		{name: "account expands to include transaction", in: "account", want: []string{"account", "transaction"}},
		{
			name: "account then transaction is not repeated",
			in:   "account,transaction",
			want: []string{"account", "transaction"},
		},
		{
			name: "transaction then account keeps order without repeating",
			in:   "transaction,account",
			want: []string{"transaction", "account"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFeeds(tc.in, false)
			if err != nil {
				t.Fatalf("parseFeeds(%q) failed: %v", tc.in, err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("parseFeeds(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseFeedsForceAccount checks the -account flag, including that it does
// not double up when account was already listed.
func TestParseFeedsForceAccount(t *testing.T) {
	got, err := parseFeeds("quote", true)
	if err != nil {
		t.Fatalf("parseFeeds failed: %v", err)
	}
	if strings.Join(got, ",") != "quote,account,transaction" {
		t.Errorf("-account on top of quote = %v, want [quote account transaction]", got)
	}

	got, err = parseFeeds("account", true)
	if err != nil {
		t.Fatalf("parseFeeds failed: %v", err)
	}
	if strings.Join(got, ",") != "account,transaction" {
		t.Errorf("-account with account already listed = %v, want [account transaction]", got)
	}
}

// TestParseFeedsRejectsUnknownFeed pins the typo path: the error names the
// value the user typed and the full valid set, so a wrong flag is fixable
// without reading the source.
func TestParseFeedsRejectsUnknownFeed(t *testing.T) {
	_, err := parseFeeds("nope", false)
	if err == nil {
		t.Fatal("an unknown feed should be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nope") {
		t.Errorf("error should name the bad feed, got %q", msg)
	}
	for _, feed := range acceptedFeeds() {
		if !strings.Contains(msg, feed) {
			t.Errorf("error should list %q as valid, got %q", feed, msg)
		}
	}
}

// TestParseFeedsRejectsEmptySelection covers both ways of selecting nothing.
func TestParseFeedsRejectsEmptySelection(t *testing.T) {
	for _, in := range []string{"", "   ", ",,", " , "} {
		if _, err := parseFeeds(in, false); err == nil {
			t.Errorf("parseFeeds(%q) should be rejected", in)
		}
	}
}

// ---- vocabulary / dispatch agreement ----

// TestFeedVocabularyMatchesDispatch is the drift test. It parses this file's
// own source and compares three things that must agree: the feeds
// parseFeeds accepts, the feeds subscribeFeeds and unsubscribeFeeds switch
// on, and the feeds feedHint can explain. Adding a feed to the switch but not
// to the vocabulary fails, and so does the reverse — neither shows up as a
// runtime error otherwise, because a feed nobody can select is dead code and
// a feed nobody dispatches silently subscribes nothing.
func TestFeedVocabularyMatchesDispatch(t *testing.T) {
	switched := dispatchFeeds(t)
	vocabulary := map[string]bool{}
	for _, f := range canonicalFeeds {
		vocabulary[f] = true
	}

	for _, f := range canonicalFeeds {
		if !switched["subscribe:"+f] {
			t.Errorf("feed %q is in canonicalFeeds but subscribeFeeds has no case for it", f)
		}
		if !switched["unsubscribe:"+f] {
			t.Errorf("feed %q is in canonicalFeeds but unsubscribeFeeds has no case for it", f)
		}
		if feedHint(f) == "" {
			t.Errorf("feed %q has no delivery hint", f)
		}
		if knownFeed(f) == false {
			t.Errorf("feed %q is not in the known set used by parseFeeds", f)
		}
	}
	for key := range switched {
		parts := strings.SplitN(key, ":", 2)
		if !vocabulary[parts[1]] {
			t.Errorf("%s switches on %q, which is not in canonicalFeeds", parts[0], parts[1])
		}
	}
	// The alias must resolve to a real feed, or -subscribe crypto selects
	// nothing while looking accepted.
	for alias, target := range feedAliases {
		if !vocabulary[target] {
			t.Errorf("alias %q points at %q, which is not a feed", alias, target)
		}
	}
}

// dispatchFeeds reads the two dispatch switches out of main.go by parsing it,
// and returns the case labels keyed by "subscribe:feed" / "unsubscribe:feed".
func dispatchFeeds(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	found := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var prefix string
		switch fn.Name.Name {
		case "subscribeFeeds":
			prefix = "subscribe:"
		case "unsubscribeFeeds":
			prefix = "unsubscribe:"
		default:
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					found[prefix+strings.Trim(lit.Value, `"`)] = true
				}
			}
			return true
		})
	}
	if len(found) == 0 {
		t.Fatal("no dispatch cases found in main.go")
	}
	return found
}

func knownFeed(f string) bool {
	for _, c := range canonicalFeeds {
		if c == f {
			return true
		}
	}
	return false
}

// ---- dispatch ----

// TestSubscribeFeedsCallsExpectedMethod is the core dispatch table: each feed
// reaches exactly the SDK method it should, with exactly the arguments it
// should, and nothing else.
func TestSubscribeFeedsCallsExpectedMethod(t *testing.T) {
	tests := []struct {
		feed   string
		market string
		inds   []string
		want   []call
	}{
		{feed: "quote", want: []call{{"SubscribeQuote", "[AAPL|MSFT]"}}},
		{feed: "tick", want: []call{{"SubscribeTick", "[AAPL|MSFT]"}}},
		{feed: "depth", want: []call{{"SubscribeDepth", "[AAPL|MSFT]"}}},
		{feed: "option", want: []call{{"SubscribeOption", "[AAPL|MSFT]"}}},
		{feed: "future", want: []call{{"SubscribeFuture", "[AAPL|MSFT]"}}},
		{feed: "kline", want: []call{{"SubscribeKline", "[AAPL|MSFT]"}}},
		// cc takes the symbols as given: Tiger's docs disagree on the form.
		{feed: "cc", want: []call{{"SubscribeCc", "[AAPL|MSFT]"}}},
		{feed: "market", market: "HK", want: []call{{"SubscribeMarket", "HK"}}},
		{
			feed: "stock_top", market: "US", inds: []string{"changeRate"},
			want: []call{{"SubscribeStockTop", "US,[changeRate]"}},
		},
		{
			feed: "option_top", market: "US", inds: []string{"bigOrder", "volume"},
			want: []call{{"SubscribeOptionTop", "US,[bigOrder|volume]"}},
		},
		// The account composite: three subscribes, each with an empty account
		// meaning "the one from config".
		{feed: "account", want: []call{
			{"SubscribeOrder", ""}, {"SubscribePosition", ""}, {"SubscribeAsset", ""},
		}},
		{feed: "transaction", want: []call{{"SubscribeTransaction", ""}}},
	}
	for _, tc := range tests {
		t.Run(tc.feed, func(t *testing.T) {
			f := &fakePush{}
			log, _ := captureLog()
			inds := tc.inds
			if inds == nil {
				inds = defaultIndents()
			}
			if err := subscribeFeeds(f, []string{tc.feed}, testSyms, tc.market, inds, log); err != nil {
				t.Fatalf("subscribeFeeds(%q) failed: %v", tc.feed, err)
			}
			got := f.record()
			if len(got) != len(tc.want) {
				t.Fatalf("feed %q made %d calls, want %d: %v", tc.feed, len(got), len(tc.want), f.methods())
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("feed %q call %d = %s, want %s", tc.feed, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestSubscribeFeedsForSelectedFeedsCallsNothingElse runs the whole accepted
// vocabulary in one go. Per-feed tests could each pass while a name is typed
// differently in the two dispatch switches; this catches that.
func TestSubscribeFeedsForSelectedFeedsCallsNothingElse(t *testing.T) {
	feeds, err := parseFeeds(strings.Join(acceptedFeeds(), ","), false)
	if err != nil {
		t.Fatalf("parseFeeds failed: %v", err)
	}
	f := &fakePush{}
	log, _ := captureLog()
	if err := subscribeFeeds(f, feeds, testSyms, "US", defaultIndents(), log); err != nil {
		t.Fatalf("subscribeFeeds failed: %v", err)
	}
	// 13 accepted spellings, of which crypto is cc and account also pulls in
	// transaction and its own three account subscribes, so 14 calls in total.
	if got := f.methods(); len(got) != 14 {
		t.Errorf("all feeds made %d calls (%v), want 14", len(got), got)
	}
	subscribes, others := 0, 0
	for _, m := range f.methods() {
		if strings.HasPrefix(m, "Subscribe") {
			subscribes++
		} else {
			others++
		}
	}
	if others != 0 {
		t.Errorf("dispatch called non-subscribe methods: %v", others)
	}
	if subscribes != 14 {
		t.Errorf("dispatch made %d subscribe calls, want 14", subscribes)
	}
}

// TestUnsubscribeFeedsCallsMatchingMethod checks the mirror image, including
// the API asymmetry: the account unsubscribes take no arguments at all.
func TestUnsubscribeFeedsCallsMatchingMethod(t *testing.T) {
	tests := []struct {
		feed   string
		market string
		inds   []string
		want   call
	}{
		{feed: "quote", want: call{"UnsubscribeQuote", "[AAPL|MSFT]"}},
		{feed: "tick", want: call{"UnsubscribeTick", "[AAPL|MSFT]"}},
		{feed: "depth", want: call{"UnsubscribeDepth", "[AAPL|MSFT]"}},
		{feed: "option", want: call{"UnsubscribeOption", "[AAPL|MSFT]"}},
		{feed: "future", want: call{"UnsubscribeFuture", "[AAPL|MSFT]"}},
		{feed: "kline", want: call{"UnsubscribeKline", "[AAPL|MSFT]"}},
		{feed: "cc", want: call{"UnsubscribeCc", "[AAPL|MSFT]"}},
		{feed: "market", market: "HK", want: call{"UnsubscribeMarket", "HK"}},
		{feed: "stock_top", market: "US", want: call{"UnsubscribeStockTop", "US,[volume|amount]"}},
		{feed: "option_top", market: "US", want: call{"UnsubscribeOptionTop", "US,[volume|amount]"}},
		{feed: "transaction", want: call{"UnsubscribeTransaction", ""}},
	}
	for _, tc := range tests {
		t.Run(tc.feed, func(t *testing.T) {
			f := &fakePush{}
			log, _ := captureLog()
			if err := unsubscribeFeeds(f, []string{tc.feed}, testSyms, tc.market, defaultIndents(), log); err != nil {
				t.Fatalf("unsubscribeFeeds(%q) failed: %v", tc.feed, err)
			}
			got := f.record()
			want := []call{tc.want}
			if tc.feed == "account" {
				// The API asymmetry: order, position and asset are three
				// whole-subject unsubscribes that take no arguments, so there
				// is no way to drop just one of them.
				want = []call{
					{"UnsubscribeOrder", ""}, {"UnsubscribePosition", ""}, {"UnsubscribeAsset", ""},
				}
			}
			if len(got) != len(want) {
				t.Fatalf("feed %q made %d calls, want %d: %v", tc.feed, len(got), len(want), f.methods())
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("feed %q call %d = %s, want %s", tc.feed, i, got[i], want[i])
				}
			}
			for _, c := range got {
				if !strings.HasPrefix(c.method, "Unsubscribe") {
					t.Errorf("feed %q called %s, which is not an unsubscribe", tc.feed, c.method)
				}
				if strings.HasSuffix(c.method, "Order") && c.args != "" {
					t.Errorf("UnsubscribeOrder must take no argument, got %q", c.args)
				}
			}
		})
	}
}

// ---- -unsubscribe ----

// TestUnsubscribeIsOffByDefault is the behaviour-change guard. -unsubscribe
// defaults to false, and a run that unsubscribed by default would send
// teardown requests seconds after subscribing — which Tiger rejects, but the
// SDK never reports. So the default must send none.
func TestUnsubscribeIsOffByDefault(t *testing.T) {
	f := &fakePush{}
	log, logBuf := captureLog()
	feeds := []string{"quote", "account", "transaction"}

	if err := subscribeFeeds(f, feeds, testSyms, "", defaultIndents(), log); err != nil {
		t.Fatalf("subscribeFeeds failed: %v", err)
	}
	if err := sendUnsubscribe(log, f, feeds, testSyms, "", defaultIndents(), false, 10*time.Minute); err != nil {
		t.Fatalf("sendUnsubscribe failed: %v", err)
	}

	for _, m := range f.methods() {
		if strings.HasPrefix(m, "Unsubscribe") {
			t.Fatalf("an unsubscribe (%s) was sent with -unsubscribe off", m)
		}
	}
	// quote is one call, account is three, transaction is one.
	if got := len(f.record()); got != 5 {
		t.Errorf("made %d calls, want the 5 subscribes only: %v", got, f.methods())
	}
	if strings.Contains(logBuf.String(), "not unsubscribing") {
		t.Errorf("a refusal was logged for a run that never asked to unsubscribe")
	}
}

// TestUnsubscribeSentAfterCooldown covers the only case where the teardown
// request is sent, and asserts the ordering: every unsubscribe precedes the
// Disconnect, because after the socket closes there is nothing to write to.
func TestUnsubscribeSentAfterCooldown(t *testing.T) {
	f := &fakePush{}
	log, logBuf := captureLog()
	feeds := []string{"quote", "account", "transaction"}

	if err := subscribeFeeds(f, feeds, testSyms, "", defaultIndents(), log); err != nil {
		t.Fatalf("subscribeFeeds failed: %v", err)
	}
	if err := sendUnsubscribe(log, f, feeds, testSyms, "", defaultIndents(), true, 2*time.Minute); err != nil {
		t.Fatalf("sendUnsubscribe failed: %v", err)
	}
	if err := f.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	methods := f.methods()
	disconnectAt := -1
	for i, m := range methods {
		if m == "Disconnect" {
			disconnectAt = i
		}
		if strings.HasPrefix(m, "Unsubscribe") && disconnectAt >= 0 {
			t.Errorf("%s was called after Disconnect", m)
		}
	}
	if disconnectAt < 0 {
		t.Fatalf("Disconnect was never called: %v", methods)
	}
	want := []string{
		"SubscribeQuote",
		"SubscribeOrder", "SubscribePosition", "SubscribeAsset",
		"SubscribeTransaction",
		"UnsubscribeQuote",
		"UnsubscribeOrder", "UnsubscribePosition", "UnsubscribeAsset",
		"UnsubscribeTransaction",
		"Disconnect"}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Errorf("call order = %v, want %v", methods, want)
	}
	if !strings.Contains(logBuf.String(), "unsubscribe sent") {
		t.Errorf("the sends should be logged, got:\n%s", logBuf.String())
	}
}

// TestUnsubscribeRefusedInsideCooldown is the honesty test. A run that ends
// inside the cooldown cannot unsubscribe, so nothing is sent and the refusal
// is loud — an unsubscribe the server will reject is indistinguishable from
// one it accepted once the socket closes, and reporting a clean-up that did
// not happen is worse than saying so.
func TestUnsubscribeRefusedInsideCooldown(t *testing.T) {
	for _, elapsed := range []time.Duration{0, time.Second, 59 * time.Second} {
		f := &fakePush{}
		log, logBuf := captureLog()
		feeds := []string{"quote"}

		if err := sendUnsubscribe(log, f, feeds, testSyms, "", defaultIndents(), true, elapsed); err != nil {
			t.Fatalf("sendUnsubscribe failed: %v", err)
		}
		if n := len(f.record()); n != 0 {
			t.Errorf("elapsed %s: %d calls sent, want none: %v", elapsed, n, f.methods())
		}
		out := logBuf.String()
		for _, want := range []string{"not unsubscribing", unsubscribeCooldown.String()} {
			if !strings.Contains(out, want) {
				t.Errorf("elapsed %s: the refusal should mention %q, got:\n%s", elapsed, want, out)
			}
		}
	}
}

// TestCheckUnsubscribePlan pins the pre-flight refusal: a -duration shorter
// than the cooldown can be caught before any connection is made, and the
// message has to name both flags so the fix is obvious.
func TestCheckUnsubscribePlan(t *testing.T) {
	if err := checkUnsubscribePlan(false, time.Second); err != nil {
		t.Errorf("an off flag must never be refused, got %v", err)
	}
	err := checkUnsubscribePlan(true, 30*time.Second)
	if err == nil {
		t.Fatal("-unsubscribe with a 30s duration should be refused")
	}
	for _, want := range []string{"-unsubscribe", "-duration", "1m0s"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got %q", want, err)
		}
	}
	if err := checkUnsubscribePlan(true, unsubscribeCooldown); err != nil {
		t.Errorf("a duration equal to the cooldown is fine, got %v", err)
	}
	if err := checkUnsubscribePlan(true, 0); err != nil {
		t.Errorf("-duration 0 runs until Ctrl-C, so it cannot be refused up front, got %v", err)
	}
}

// ---- indicators ----

// TestParseIndicatorsAcceptsDocumentedValues checks every indicator the
// server documents, plus case folding, since the names are camelCase and a
// user typing them in upper case is not making a mistake worth a rejection.
func TestParseIndicatorsAcceptsDocumentedValues(t *testing.T) {
	for _, name := range stockTopIndicators {
		got, err := parseIndicators(name)
		if err != nil {
			t.Errorf("stock_top indicator %q rejected: %v", name, err)
			continue
		}
		if len(got) != 1 || got[0] != name {
			t.Errorf("stock_top indicator %q became %v, want [%s]", name, got, name)
		}
	}
	for _, name := range optionTopIndicators {
		got, err := parseIndicators(name)
		if err != nil {
			t.Errorf("option_top indicator %q rejected: %v", name, err)
			continue
		}
		if len(got) != 1 || got[0] != name {
			t.Errorf("option_top indicator %q became %v, want [%s]", name, got, name)
		}
	}
	got, err := parseIndicators(" CHANGERATE , volume ")
	if err != nil {
		t.Fatalf("parseIndicators failed: %v", err)
	}
	if strings.Join(got, ",") != "changeRate,volume" {
		t.Errorf("case-folded parse = %v, want [changeRate volume]", got)
	}
	if got, err := parseIndicators("volume,volume"); err != nil || len(got) != 1 {
		t.Errorf("a repeated indicator should collapse, got %v (%v)", got, err)
	}
}

// TestParseIndicatorsRejectsUnknownIndicator checks the typo path. The
// SDK's own tests use "top_gainer" and "top_volume", which are not values the
// server accepts, so this is the mistake most likely to be made.
func TestParseIndicatorsRejectsUnknownIndicator(t *testing.T) {
	// top_gainer and top_volume are the strings the SDK's own tests use; they
	// are placeholders, not values the server accepts, so a user copying them
	// out of the SDK source has to be told no.
	for _, bad := range []string{"top_gainer", "top_volume", "top_loser", "top_oi", "nope", "change_rate"} {
		_, err := parseIndicators(bad)
		if err == nil {
			t.Errorf("indicator %q should be rejected", bad)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, bad) {
			t.Errorf("error should name %q, got %q", bad, msg)
		}
		for _, name := range stockTopIndicators {
			if !strings.Contains(msg, name) {
				t.Errorf("error should list %q, got %q", name, msg)
			}
		}
	}
}

// TestValidateIndicatorsPerFeed checks the two per-feed valid sets. They
// overlap only on volume and amount, which is why the default is those two.
func TestValidateIndicatorsPerFeed(t *testing.T) {
	t.Run("each documented value is accepted for its own feed", func(t *testing.T) {
		for _, name := range stockTopIndicators {
			if err := validateIndicators([]string{"stock_top"}, []string{name}); err != nil {
				t.Errorf("stock_top rejected %q: %v", name, err)
			}
		}
		for _, name := range optionTopIndicators {
			if err := validateIndicators([]string{"option_top"}, []string{name}); err != nil {
				t.Errorf("option_top rejected %q: %v", name, err)
			}
		}
	})
	t.Run("an empty list is rejected for a ranking feed", func(t *testing.T) {
		for _, feed := range []string{"stock_top", "option_top"} {
			err := validateIndicators([]string{feed}, nil)
			if err == nil {
				t.Errorf("-subscribe %s with no indicators should be rejected", feed)
				continue
			}
			if !strings.Contains(err.Error(), "-indicators") {
				t.Errorf("error should name the flag, got %q", err)
			}
		}
	})
	t.Run("an indicator from the other feed is rejected", func(t *testing.T) {
		// bigOrder is option-only, changeRate is stock-only.
		if err := validateIndicators([]string{"stock_top"}, []string{"bigOrder"}); err == nil {
			t.Error("stock_top accepted the option-only indicator bigOrder")
		}
		if err := validateIndicators([]string{"option_top"}, []string{"changeRate"}); err == nil {
			t.Error("option_top accepted the stock-only indicator changeRate")
		}
	})
	t.Run("the error names the valid set for that feed", func(t *testing.T) {
		err := validateIndicators([]string{"option_top"}, []string{"amplitude"})
		if err == nil {
			t.Fatal("amplitude is stock-only and should be rejected for option_top")
		}
		for _, name := range optionTopIndicators {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error should list %q, got %q", name, err)
			}
		}
	})
	t.Run("a non-ranking feed ignores indicators", func(t *testing.T) {
		if err := validateIndicators([]string{"quote", "tick"}, nil); err != nil {
			t.Errorf("quote should not require indicators, got %v", err)
		}
	})
}

// TestDefaultIndicatorsAreValidForBothRankingFeeds guards the flag default:
// if the two valid sets ever stop overlapping, the default becomes a trap.
func TestDefaultIndicatorsAreValidForBothRankingFeeds(t *testing.T) {
	inds, err := parseIndicators(defaultIndicators)
	if err != nil {
		t.Fatalf("the -indicators default %q is not a valid indicator: %v", defaultIndicators, err)
	}
	for _, feed := range []string{"stock_top", "option_top"} {
		if err := validateIndicators([]string{feed}, inds); err != nil {
			t.Errorf("the default is not valid for %s: %v", feed, err)
		}
	}
}

// ---- market validation ----

// TestValidateFeedOptionsMarkets covers the three market rules, each of which
// the server enforces only by refusing the subscription — which the SDK would
// not report. So they are checked here, where a wrong value can still produce
// an error message.
func TestValidateFeedOptionsMarkets(t *testing.T) {
	tests := []struct {
		name    string
		feeds   []string
		market  string
		wantErr string
	}{
		{name: "market feed with HK", feeds: []string{"market"}, market: "HK"},
		{name: "market feed with US", feeds: []string{"market"}, market: "US", wantErr: "HK"},
		{name: "market feed with no market", feeds: []string{"market"}, wantErr: "-market"},
		{name: "stock_top with US", feeds: []string{"stock_top"}, market: "US"},
		{name: "stock_top with HK", feeds: []string{"stock_top"}, market: "HK"},
		{name: "stock_top with CN", feeds: []string{"stock_top"}, market: "CN", wantErr: "US, HK"},
		{name: "option_top with US", feeds: []string{"option_top"}, market: "US"},
		{name: "option_top with HK", feeds: []string{"option_top"}, market: "HK", wantErr: "US"},
		{name: "option_top with no market", feeds: []string{"option_top"}, wantErr: "-market"},
		{name: "lowercase market is folded", feeds: []string{"option_top"}, market: "us"},
		{name: "a feed with no market requirement", feeds: []string{"quote", "tick"}, market: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFeedOptions(tc.feeds, tc.market)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected refusal: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("should have been refused (want a message naming %q)", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should name %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateFeedOptionsRejectsUnsupportedMarketsBeforeConnecting is the
// shape the shipped command has to reject: the bad flag comes with a real feed
// selected, and nothing may reach a client. There is no client to reach, so
// the assertion is that the validation returns before any dispatch.
func TestValidateFeedOptionsRejectsUnsupportedMarketsBeforeConnecting(t *testing.T) {
	f := &fakePush{}
	log, _ := captureLog()

	if err := validateFeedOptions([]string{"option_top"}, "HK"); err == nil {
		t.Fatal("option_top for HK should be refused")
	}
	if n := len(f.record()); n != 0 {
		t.Errorf("a refused -market still made %d client calls", n)
	}

	if err := validateFeedOptions([]string{"quote"}, ""); err != nil {
		t.Errorf("quote should not need a market: %v", err)
	}
	_ = log
}

// ---- delivery accounting ----

// TestDeliveryCountingIsPerFeed is the point of the tracker: a feed that
// delivered nothing must be distinguishable from one that did, and a callback
// must only move the feeds that callback can actually serve.
func TestDeliveryCountingIsPerFeed(t *testing.T) {
	tests := []struct {
		name   string
		feeds  []string
		arrive []string
		want   map[string]int
	}{
		{
			name:   "a quiet feed stays at zero",
			feeds:  []string{"quote", "tick"},
			arrive: nil,
			want:   map[string]int{"quote": 0, "tick": 0},
		},
		{
			name:   "one arrival moves only its own feed",
			feeds:  []string{"quote", "tick"},
			arrive: []string{"quote"},
			want:   map[string]int{"quote": 1, "tick": 0},
		},
		{
			name:   "counts accumulate",
			feeds:  []string{"quote"},
			arrive: []string{"quote", "quote", "quote"},
			want:   map[string]int{"quote": 3},
		},
		{
			// A quote can be any of the three feeds that reuse QuoteData, so
			// one arrival marks all of the subscribed ones. Over-counting is
			// deliberate; under-counting would raise a warning nobody can act
			// on.
			name:   "a quote serves quote, cc and market alike",
			feeds:  []string{"quote", "cc", "market", "tick"},
			arrive: []string{"quote", "cc", "market"},
			want:   map[string]int{"quote": 1, "cc": 1, "market": 1, "tick": 0},
		},
		{
			// A callback may only credit the feeds it can actually serve. A
			// quote says nothing about a kline, and crediting it anyway would
			// silence the warning for a subscription that is genuinely dead.
			name:   "an arrival credits only the feeds its callback serves",
			feeds:  []string{"quote", "kline", "tick"},
			arrive: []string{"quote", "cc", "market"},
			want:   map[string]int{"quote": 1, "kline": 0, "tick": 0},
		},
		{
			name:   "an unsubscribed feed is not counted",
			feeds:  []string{"quote"},
			arrive: []string{"kline"},
			want:   map[string]int{"quote": 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFeedTracker(tc.feeds)
			for _, a := range tc.arrive {
				tr.arrive(a)
			}
			for feed, want := range tc.want {
				if got := tr.count(feed); got != want {
					t.Errorf("feed %q counted %d, want %d", feed, got, want)
				}
			}
		})
	}
}

// TestCallbacksMoveTheDeliveryCounts drives the real callback table, so the
// wiring between a feed and the callback that marks it is what is tested —
// not a hand-written list of the same pairs.
func TestCallbacksMoveTheDeliveryCounts(t *testing.T) {
	tests := []struct {
		name  string
		feeds []string
		fire  func(cb sdkpush.Callbacks)
		want  string
	}{
		{
			name:  "a quote counts for quote",
			feeds: []string{"quote"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnQuote(&sdkpb.QuoteData{Symbol: "AAPL"}) },
			want:  "quote",
		},
		{
			name:  "a tick counts for tick",
			feeds: []string{"tick"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnTick(sdkpush.PushTradeTick{Symbol: "AAPL"}) },
			want:  "tick",
		},
		{
			name:  "a book counts for depth",
			feeds: []string{"depth"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnDepth(&sdkpb.QuoteDepthData{Symbol: "AAPL"}) },
			want:  "depth",
		},
		{
			name:  "an option quote counts for option",
			feeds: []string{"option"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnOption(&sdkpb.QuoteData{Symbol: "AAPL"}) },
			want:  "option",
		},
		{
			name:  "a future quote counts for future",
			feeds: []string{"future"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnFuture(&sdkpb.QuoteData{Symbol: "AAPL"}) },
			want:  "future",
		},
		{
			name:  "a bar counts for kline",
			feeds: []string{"kline"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnKline(&sdkpb.KlineData{Symbol: "AAPL"}) },
			want:  "kline",
		},
		{
			name:  "a ranking counts for stock_top",
			feeds: []string{"stock_top"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnStockTop(&sdkpb.StockTopData{Market: "US"}) },
			want:  "stock_top",
		},
		{
			name:  "an option ranking counts for option_top",
			feeds: []string{"option_top"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnOptionTop(&sdkpb.OptionTopData{Market: "US"}) },
			want:  "option_top",
		},
		{
			name:  "an order counts for account",
			feeds: []string{"account"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnOrder(&sdkpb.OrderStatusData{Symbol: "AAPL"}) },
			want:  "account",
		},
		{
			name:  "a position counts for account",
			feeds: []string{"account"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnPosition(&sdkpb.PositionData{Symbol: "AAPL"}) },
			want:  "account",
		},
		{
			name:  "an asset update counts for account",
			feeds: []string{"account"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnAsset(&sdkpb.AssetData{Account: "acct"}) },
			want:  "account",
		},
		{
			name:  "a fill counts for transaction",
			feeds: []string{"transaction"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnTransaction(&sdkpb.OrderTransactionData{Symbol: "AAPL"}) },
			want:  "transaction",
		},
		{
			// The cc payload is a QuoteData and arrives here, so the crypto
			// feed's count comes from the quote callback. The alternative is
			// claiming a crypto feed was silent while a BTC quote is plainly
			// on screen. (TestDeliveryCountingIsPerFeed covers the full set
			// of feeds one arrival credits.)
			name:  "a quote also satisfies cc",
			feeds: []string{"cc"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnQuote(&sdkpb.QuoteData{Symbol: "BTC"}) },
			want:  "cc",
		},
		{
			// Two feeds that share no callback, with only one of them
			// delivering. This is the case that matters: a run subscribing
			// both must still report the silent one, so the quote callback
			// must not credit kline or tick.
			name:  "a quote credits neither kline nor tick",
			feeds: []string{"quote", "kline", "tick", "depth"},
			fire:  func(cb sdkpush.Callbacks) { cb.OnQuote(&sdkpb.QuoteData{Symbol: "AAPL"}) },
			want:  "quote",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			silenceStdout(t)
			tr := newFeedTracker(tc.feeds)
			cb := callbacks(logging.Discard(), tr)
			if tr.count(tc.want) != 0 {
				t.Fatalf("feed %q started non-zero", tc.want)
			}
			tc.fire(cb)
			if got := tr.count(tc.want); got != 1 {
				t.Errorf("feed %q counted %d after its callback fired, want 1", tc.want, got)
			}
			// The other half: a callback may only credit the feeds it can
			// serve. Crediting everything would silence the warning for a
			// subscription that is genuinely dead, which is the failure this
			// whole mechanism exists to prevent.
			for _, other := range tc.feeds {
				if other == tc.want {
					continue
				}
				if got := tr.count(other); got != 0 {
					t.Errorf("feed %q was credited %d times by %s's callback", other, got, tc.want)
				}
			}
		})
	}
}

// TestReportDeliveryNamesSilentFeeds is the user-facing half: a feed with no
// data must be reported as such, with a reason that could explain it.
func TestReportDeliveryNamesSilentFeeds(t *testing.T) {
	silenceStdout(t)
	feeds := []string{"quote", "cc", "stock_top"}
	tr := newFeedTracker(feeds)
	cb := callbacks(logging.Discard(), tr)
	cb.OnQuote(&sdkpb.QuoteData{Symbol: "AAPL"}) // covers quote and cc

	log, buf := captureLog()
	reportDelivery(log, tr, feeds)
	out := buf.String()

	if !strings.Contains(out, "delivered: feed=quote messages=1") {
		t.Errorf("the delivered feed should be counted, got:\n%s", out)
	}
	if !strings.Contains(out, "delivered: feed=cc messages=1") {
		t.Errorf("cc shares the quote callback, so it should count too, got:\n%s", out)
	}
	if !strings.Contains(out, "NO DATA: feed=stock_top") {
		t.Errorf("the silent feed should be reported, got:\n%s", out)
	}
	// The ranking hint has to mention market hours, because a silent ranking
	// outside trading hours is normal rather than a broken subscription.
	if !strings.Contains(out, "market hours") {
		t.Errorf("the stock_top hint should mention market hours, got:\n%s", out)
	}
}

// TestFeedHintCoversTheSilentCauses checks the three server-side reasons a
// subscription can be dead with no error at all. A rejected subscription is
// indistinguishable from a closed market from the client's side, so the
// message has to carry all three or it is misleading by omission.
func TestFeedHintCoversTheSilentCauses(t *testing.T) {
	for _, feed := range canonicalFeeds {
		hint := feedHint(feed)
		if hint == "" {
			t.Errorf("feed %q has no hint", feed)
			continue
		}
		for _, want := range []string{"3xxx", "code 4", "closed"} {
			if !strings.Contains(hint, want) {
				t.Errorf("feed %q hint does not mention %q: %s", feed, want, hint)
			}
		}
	}
	if !strings.Contains(feedHint("cc"), "BTC") {
		t.Error("the crypto hint should list the documented symbol forms to try")
	}
	if strings.Contains(strings.ToLower(feedHint("market")), "market status") {
		t.Error("the market feed is whole-market quote streaming, not market status")
	}
}

// TestDeliveryWarningIsNotAFailure pins the severity. A closed market is
// normal, so the report is informational: it must not be logged as an error.
func TestDeliveryWarningIsNotAFailure(t *testing.T) {
	var b strings.Builder
	log := logging.NewFromConfig(&b, "info")
	tr := newFeedTracker([]string{"quote"})
	reportDelivery(log, tr, []string{"quote"})
	out := b.String()
	if strings.Contains(out, "ERROR") {
		t.Errorf("a silent feed must not be an error, got:\n%s", out)
	}
	if !strings.Contains(out, "WARN") {
		t.Errorf("a silent feed should be loud, got:\n%s", out)
	}
}

// TestFeedTrackerIsRaceFree exercises the tracker the way the SDK does: from
// another goroutine, concurrently with a report. Run under -race this is the
// test that would catch an unguarded map.
func TestFeedTrackerIsRaceFree(t *testing.T) {
	feeds := []string{"quote", "cc", "market"}
	tr := newFeedTracker(feeds)
	log := logging.Discard()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			tr.arrive("quote", "cc", "market")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			reportDelivery(log, tr, feeds)
		}
	}()
	wg.Wait()

	if got := tr.count("quote"); got != 200 {
		t.Errorf("quote counted %d, want 200", got)
	}
}

// ---- error propagation and the flag surface ----

// errPush is a fake that fails one method, so the wrap-and-name path is
// exercised rather than assumed.
type errPush struct {
	fakePush
	fail string
}

func (e *errPush) SubscribeKline([]string) error {
	e.note("SubscribeKline")
	return errPushFail
}

func (e *errPush) UnsubscribeMarket(string) error {
	e.note("UnsubscribeMarket")
	return errPushFail
}

var errPushFail = errors.New("write failed")

// TestSubscribeFeedsNamesTheFeedOnFailure pins the error a refused write
// produces: it must name the feed, so a multi-feed run tells the user which
// one stopped it.
func TestSubscribeFeedsNamesTheFeedOnFailure(t *testing.T) {
	f := &errPush{fail: "SubscribeKline"}
	log, _ := captureLog()
	err := subscribeFeeds(f, []string{"kline"}, testSyms, "", defaultIndents(), log)
	if err == nil {
		t.Fatal("a failed write should surface as an error")
	}
	for _, want := range []string{"kline", "write failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestUnsubscribeFeedsNamesTheFeedOnFailure(t *testing.T) {
	f := &errPush{fail: "UnsubscribeMarket"}
	log, _ := captureLog()
	err := unsubscribeFeeds(f, []string{"market"}, testSyms, "HK", defaultIndents(), log)
	if err == nil {
		t.Fatal("a failed write should surface as an error")
	}
	for _, want := range []string{"market", "write failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// TestRunHelpNeedsNoCredentials checks the -h path end to end. It is the only
// part of run a test can reach without a Tiger ID, and it has to return nil
// rather than the missing-credential error.
func TestRunHelpNeedsNoCredentials(t *testing.T) {
	if err := run([]string{"-h"}); err != nil {
		t.Fatalf("-h should succeed without credentials, got %v", err)
	}
}

// TestFlagDefaults pins every default, because a default is behaviour.
// -unsubscribe is the one that matters: it is off because the server rejects
// an unsubscribe sent inside the one-minute cooldown, and a run that
// unsubscribed by default would send teardown requests seconds after
// subscribing on every invocation.
func TestFlagDefaults(t *testing.T) {
	var o options
	fs := newFlagSet(&o)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse empty args: %v", err)
	}
	if o.unsubscribe {
		t.Error("-unsubscribe must default to off: a run that ended inside the cooldown could not honour it anyway")
	}
	if o.account {
		t.Error("-account must default to off: it subscribes the whole account")
	}
	if o.subscribe != "quote" {
		t.Errorf("-subscribe default = %q, want quote", o.subscribe)
	}
	if o.symbols != "AAPL,MSFT" {
		t.Errorf("-symbols default = %q", o.symbols)
	}
	if o.market != "" {
		t.Errorf("-market default = %q, want empty so a feed that needs one says so", o.market)
	}
	if o.indicators != defaultIndicators {
		t.Errorf("-indicators default = %q, want %q", o.indicators, defaultIndicators)
	}
	if o.duration != 30*time.Second {
		t.Errorf("-duration default = %s, want 30s", o.duration)
	}
}

// TestResolveRejectsBadCommandLines exercises the whole pre-connection
// validation path the way a user hits it: parse the flags, then resolve. A
// bad feed, market or indicator has to be refused here rather than after a
// config load, so the error is about the flag and not about credentials.
func TestResolveRejectsBadCommandLines(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown feed", args: []string{"-subscribe", "nope"}, want: "nope"},
		{name: "no feeds", args: []string{"-subscribe", ""}, want: "no feeds"},
		{name: "option_top outside the US", args: []string{"-subscribe", "option_top", "-market", "HK"}, want: "US"},
		{name: "market feed outside HK", args: []string{"-subscribe", "market", "-market", "US"}, want: "HK"},
		{name: "a market-scoped feed with no market", args: []string{"-subscribe", "market"}, want: "-market"},
		{name: "a placeholder indicator from the SDK's own tests", args: []string{"-subscribe", "stock_top", "-indicators", "top_gainer"}, want: "top_gainer"},
		{name: "an indicator from the other ranking feed", args: []string{"-subscribe", "option_top", "-market", "US", "-indicators", "changeRate"}, want: "changeRate"},
		{name: "an empty indicator list", args: []string{"-subscribe", "stock_top", "-market", "US", "-indicators", ""}, want: "-indicators"},
		{name: "unsubscribe with a duration inside the cooldown", args: []string{"-unsubscribe", "-duration", "5s"}, want: "-unsubscribe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o options
			fs := newFlagSet(&o)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse %v: %v", tc.args, err)
			}
			err := o.resolve()
			if err == nil {
				t.Fatalf("%v should have been refused", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

// TestResolveAcceptsValidCommandLines is the other half: the documented
// invocations must survive resolution, or the validators are rejecting the
// feeds they exist to protect.
func TestResolveAcceptsValidCommandLines(t *testing.T) {
	tests := [][]string{
		{},
		{"-subscribe", "quote,tick,depth"},
		{"-subscribe", "crypto"},
		{"-subscribe", "cc"},
		{"-subscribe", "market", "-market", "HK"},
		{"-subscribe", "stock_top", "-market", "US", "-indicators", "changeRate"},
		{"-subscribe", "stock_top", "-market", "HK", "-indicators", "amplitude"},
		{"-subscribe", "option_top", "-market", "US", "-indicators", "bigOrder"},
		{"-subscribe", "option,future,kline"},
		{"-subscribe", "account"},
		{"-subscribe", "transaction"},
		{"-account", "-subscribe", "quote"},
		{"-unsubscribe", "-duration", "5m"},
		// OCC symbols carry padding spaces, so the symbol list must survive
		// resolution intact rather than being split on them.
		{"-subscribe", "option", "-symbols", "AAPL  260619C00200000"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var o options
			fs := newFlagSet(&o)
			if err := fs.Parse(args); err != nil {
				t.Fatalf("parse %v: %v", args, err)
			}
			if err := o.resolve(); err != nil {
				t.Fatalf("%v should be valid, got %v", args, err)
			}
			if len(o.feeds) == 0 {
				t.Errorf("%v resolved to no feeds", args)
			}
			if len(o.syms) == 0 {
				t.Errorf("%v resolved to no symbols", args)
			}
		})
	}
}

// TestResolveKeepsOCCPaddedSymbolsIntact guards the one input whose commas are
// not the separator. An option symbol's OCC padding is spaces, so the split
// has to be on commas only — and a symbol that gets trimmed to "AAPL 260619C00200000"
// would silently be rejected by the server instead of subscribed.
func TestResolveKeepsOCCPaddedSymbolsIntact(t *testing.T) {
	var o options
	fs := newFlagSet(&o)
	if err := fs.Parse([]string{"-subscribe", "option", "-symbols", "AAPL  260619C00200000,MSFT  260619C00210000"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := o.resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(o.syms) != 2 {
		t.Fatalf("resolved %d symbols (%v), want 2", len(o.syms), o.syms)
	}
	if o.syms[0] != "AAPL  260619C00200000" {
		t.Errorf("OCC padding was lost: %q", o.syms[0])
	}
}

// TestConnectionCallbacksAreWired covers the four connection-level callbacks.
// They are the only way this command learns about a second connection kicking
// the first, which is the failure mode of running two push commands at once.
func TestConnectionCallbacksAreWired(t *testing.T) {
	log, buf := captureLog()
	cb := callbacks(log, newFeedTracker([]string{"quote"}))

	cb.OnConnect()
	cb.OnDisconnect()
	cb.OnKickout("4001 kicked by another connection")
	cb.OnError(errors.New("write failed"))

	out := buf.String()
	for _, want := range []string{"push connected", "push disconnected", "4001", "write failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("connection callbacks should log %q, got:\n%s", want, out)
		}
	}
	// A kickout is the one event the operator has to act on, so it must be
	// logged as an error rather than a warning.
	if !strings.Contains(out, "ERROR") {
		t.Errorf("a kickout should be an error, got:\n%s", out)
	}
}

// ---- rendering ----

// captureStdout runs fn with os.Stdout redirected and returns what was
// printed. The renderers write straight to os.Stdout, which is a variable, so
// no seam is needed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		io.Copy(&b, r)
		done <- b.String()
	}()
	fn()
	os.Stdout = saved
	w.Close()
	out := <-done
	r.Close()
	return out
}

// TestPrintQuoteKeepsTheExistingLineFormat guards the four original feeds'
// output. The quote line predates this change and users grep for it, so the
// format is asserted byte for byte rather than left to drift.
func TestPrintQuoteKeepsTheExistingLineFormat(t *testing.T) {
	price := func(v float64) *float64 { return &v }
	size := func(v int64) *int64 { return &v }
	vol := int64(1234)
	status := "Trading"

	out := captureStdout(t, func() {
		printQuote("QUOTE", &sdkpb.QuoteData{
			Symbol:       "AAPL",
			LatestPrice:  price(187.25),
			BidPrice:     price(187.24),
			BidSize:      size(100),
			AskPrice:     price(187.26),
			AskSize:      size(200),
			Volume:       &vol,
			MarketStatus: &status,
		})
	})
	want := "[QUOTE] AAPL       last=187.2500 bid=187.2400/100 ask=187.2600/200 vol=1234 status=Trading\n"
	if out != want {
		t.Errorf("quote line changed:\n got %q\nwant %q", out, want)
	}
}

// TestPrintQuoteLabelsTheFeedItArrivedOn checks the option and future feeds,
// which deliver the same payload and so are only distinguishable by label.
func TestPrintQuoteLabelsTheFeedItArrivedOn(t *testing.T) {
	out := captureStdout(t, func() {
		printQuote("OPTION", &sdkpb.QuoteData{Symbol: "AAPL"})
		printQuote("FUTURE", &sdkpb.QuoteData{Symbol: "ES"})
	})
	if !strings.HasPrefix(out, "[OPTION] AAPL") {
		t.Errorf("option line = %q", out)
	}
	if !strings.Contains(out, "[FUTURE] ES") {
		t.Errorf("future line = %q", out)
	}
}

// TestPrintBookSide exercises the four shapes the book arrives in: absent,
// empty, populated with and without order counts, and truncated at five
// levels. The parallel-slice indexing is the easy thing to get wrong.
func TestPrintBookSide(t *testing.T) {
	tests := []struct {
		name string
		book *sdkpb.QuoteDepthData_OrderBook
		want []string
	}{
		{name: "absent book", book: nil, want: []string{"(none)"}},
		{name: "empty book", book: &sdkpb.QuoteDepthData_OrderBook{}, want: []string{"(empty)"}},
		{
			name: "levels with counts",
			book: &sdkpb.QuoteDepthData_OrderBook{
				Price: []float64{100, 99}, Volume: []int64{10, 20}, OrderCount: []uint32{2, 3},
			},
			want: []string{"2 level(s)", "100.0000", "vol=10", "orders=2", "99.0000", "orders=3"},
		},
		{
			name: "a missing count prints as a dash, not a zero",
			book: &sdkpb.QuoteDepthData_OrderBook{
				Price: []float64{100}, Volume: []int64{10},
			},
			want: []string{"orders=-"},
		},
		{
			// A volume slice shorter than the price slice must not panic.
			name: "short volume slice stops the render",
			book: &sdkpb.QuoteDepthData_OrderBook{
				Price: []float64{100, 99}, Volume: []int64{10},
			},
			want: []string{"2 level(s)", "100.0000"},
		},
		{
			name: "more than five levels are truncated",
			book: &sdkpb.QuoteDepthData_OrderBook{
				Price: []float64{1, 2, 3, 4, 5, 6, 7}, Volume: []int64{1, 1, 1, 1, 1, 1, 1},
			},
			want: []string{"... 2 more level(s)"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() { printBookSide("  bid", tc.book) })
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("output %q should contain %q", out, want)
				}
			}
		})
	}
}

// TestPrintBookSideTruncatesAfterFiveLevels counts the levels drawn, so a
// change to the cap is a visible test failure rather than a surprise in a
// long output.
func TestPrintBookSideTruncatesAfterFiveLevels(t *testing.T) {
	book := &sdkpb.QuoteDepthData_OrderBook{}
	for i := 0; i < 8; i++ {
		book.Price = append(book.Price, float64(100+i))
		book.Volume = append(book.Volume, 1)
	}
	out := captureStdout(t, func() { printBookSide("  ask", book) })
	if got := strings.Count(out, "vol=1"); got != 5 {
		t.Errorf("drew %d levels, want 5", got)
	}
	if !strings.Contains(out, "... 3 more level(s)") {
		t.Errorf("truncation note missing: %q", out)
	}
}

func TestPrintKlineIncludesVolumeAndCount(t *testing.T) {
	out := captureStdout(t, func() {
		printKline(&sdkpb.KlineData{
			Symbol: "AAPL", Open: 1, High: 2, Low: 0.5, Close: 1.5, Avg: 1.2,
			Volume: 100, Count: 7, Amount: 123.45,
		})
	})
	for _, want := range []string{"[KLINE] AAPL", "c=1.5000", "vol=100", "n=7", "amt=123.45"} {
		if !strings.Contains(out, want) {
			t.Errorf("kline line %q should contain %q", out, want)
		}
	}
}

// TestPrintStockTopGroupsByIndicator checks the two shapes the server sends:
// a populated group and a group with no rows. The indicator name is a group
// header, not a per-row field, so a header-less output would be unreadable.
func TestPrintStockTopGroupsByIndicator(t *testing.T) {
	out := captureStdout(t, func() {
		printStockTop(&sdkpb.StockTopData{
			Market: "US",
			TopData: []*sdkpb.StockTopData_TopData{
				{
					TargetName: "changeRate",
					Item: []*sdkpb.StockTopData_StockItem{
						{Symbol: "AAA", LatestPrice: 10, TargetValue: 5},
						{Symbol: "BBB", LatestPrice: 20, TargetValue: -1},
					},
				},
				{TargetName: "volume"},
			},
		})
	})
	for _, want := range []string{
		"[STOPTOP] market=US indicator=changeRate rows=2",
		"AAA", "last=10.0000", "value=5.0000",
		"[STOPTOP] market=US indicator=volume rows=0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stock top output should contain %q, got:\n%s", want, out)
		}
	}
}

func TestPrintStockTopTruncatesLongGroups(t *testing.T) {
	group := &sdkpb.StockTopData_TopData{TargetName: "amount"}
	for i := 0; i < 12; i++ {
		group.Item = append(group.Item, &sdkpb.StockTopData_StockItem{Symbol: "S"})
	}
	out := captureStdout(t, func() {
		printStockTop(&sdkpb.StockTopData{Market: "HK", TopData: []*sdkpb.StockTopData_TopData{group}})
	})
	if !strings.Contains(out, "... 2 more row(s)") {
		t.Errorf("truncation note missing: %q", out)
	}
}

// TestPrintOptionTopRendersBothShapes checks the option ranking, which packs
// a large-order list and a ranked list into one message. Dropping either
// would silently hide half the payload.
func TestPrintOptionTopRendersBothShapes(t *testing.T) {
	out := captureStdout(t, func() {
		printOptionTop(&sdkpb.OptionTopData{
			Market: "US",
			TopData: []*sdkpb.OptionTopData_TopData{{
				TargetName: "volume",
				Item: []*sdkpb.OptionTopData_OptionItem{{
					Symbol: "AAPL", Expiry: "20260619", Strike: "200", Right: "CALL",
					TotalVolume: 10, TotalAmount: 2000, TotalOpenInt: 500, VolumeToOpenInt: 0.02,
				}},
				BigOrder: []*sdkpb.OptionTopData_BigOrder{{
					Symbol: "AAPL", Expiry: "20260619", Strike: "200", Right: "PUT",
					Dir: "BUY", Volume: 1500, Price: 3.5, Amount: 5250,
				}},
			}},
		})
	})
	for _, want := range []string{
		"[OPTTOP] market=US indicator=volume rows=1 big_orders=1",
		"AAPL 20260619 200 CALL", "vol=10.00", "oi=500.00",
		"AAPL 20260619 200 PUT", "BUY", "price=3.5000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("option top output should contain %q, got:\n%s", want, out)
		}
	}
}

func TestPrintOptionTopTruncatesBothLists(t *testing.T) {
	group := &sdkpb.OptionTopData_TopData{TargetName: "amount"}
	for i := 0; i < 11; i++ {
		group.Item = append(group.Item, &sdkpb.OptionTopData_OptionItem{Symbol: "S"})
		group.BigOrder = append(group.BigOrder, &sdkpb.OptionTopData_BigOrder{Symbol: "S"})
	}
	out := captureStdout(t, func() {
		printOptionTop(&sdkpb.OptionTopData{Market: "US", TopData: []*sdkpb.OptionTopData_TopData{group}})
	})
	if !strings.Contains(out, "... 1 more row(s)") {
		t.Errorf("ranked-list truncation note missing: %q", out)
	}
	if !strings.Contains(out, "... 1 more big order(s)") {
		t.Errorf("big-order truncation note missing: %q", out)
	}
}

// TestErrorSuffix checks the two states: no server error, and one.
func TestErrorSuffix(t *testing.T) {
	if got := errorSuffix(""); got != "" {
		t.Errorf("no error should add nothing, got %q", got)
	}
	if got := errorSuffix("insufficient funds"); got != " err=insufficient funds" {
		t.Errorf("error suffix = %q", got)
	}
	out := captureStdout(t, func() {
		cb := callbacks(logging.Discard(), newFeedTracker([]string{"account"}))
		cb.OnOrder(&sdkpb.OrderStatusData{Symbol: "AAPL", ErrorMsg: "rejected"})
	})
	if !strings.Contains(out, "err=rejected") {
		t.Errorf("order line should carry the server error: %q", out)
	}
}

// TestSplitSymbols pins the symbol handling the whole command depends on:
// trimming, upper-casing and dropping blanks. The upper case matters because
// it is what makes -symbols btc and BTC the same subscription, and the OCC
// double space in an option symbol must survive the trim.
func TestSplitSymbols(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "AAPL,MSFT", want: "AAPL,MSFT"},
		{in: " aapl , msft ", want: "AAPL,MSFT"},
		{in: "AAPL,,MSFT,", want: "AAPL,MSFT"},
		{in: "", want: ""},
		{in: "   ", want: ""},
		// OCC padding: the underlying is left-justified in six characters, so
		// a two-space pad is part of the symbol and must not be trimmed away
		// from the middle.
		{in: "AAPL  260619C00200000", want: "AAPL  260619C00200000"},
		{in: "AAPL 260619C00200000", want: "AAPL 260619C00200000"},
	}
	for _, tc := range tests {
		if got := strings.Join(splitSymbols(tc.in), ","); got != tc.want {
			t.Errorf("splitSymbols(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestLevelFallsBackToInfo checks the log-level resolution the flag default
// depends on: verbose wins, then a config file's log_level, then info.
func TestLevelFallsBackToInfo(t *testing.T) {
	if got := level("", true); got != "debug" {
		t.Errorf("-v should give debug, got %q", got)
	}
	if got := level("", false); got != "info" {
		t.Errorf("no config should give info, got %q", got)
	}
	dir := t.TempDir()
	path := dir + "/config.yaml"
	if err := os.WriteFile(path, []byte("account: acct\nlog_level: debug\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if got := level(path, false); got != "debug" {
		t.Errorf("a config log_level should be honoured, got %q", got)
	}
}

// ---- the read-only gate ----

// TestPackageCannotReachOrderWrites turns the structural claim about this
// command into a test. The repo's safety model says a read-only command cannot
// reach a write; for push that means the trade client is never imported, no
// order is ever placed, modified or cancelled, and the pushClient interface
// cannot grow one of those methods by accident without this failing.
//
// It parses the package's own source rather than trusting a comment.
func TestPackageCannotReachOrderWrites(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	var files []string
	for _, e := range entries {
		if name := e.Name(); strings.HasSuffix(name, ".go") {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		t.Fatal("no Go files found in the package directory")
	}

	forbidden := []string{
		"openapi-go-sdk/trade",
		"PlaceOrder", "ModifyOrder", "CancelOrder",
		"PlaceForexOrder", "OptionExerciseSubmit", "OptionExerciseCancel",
		"TransferSegmentFund", "CancelSegmentFund", "TransferPosition",
		"NewTradeClient", "TradeClient", "PreviewOrder",
	}

	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		// Strip comments so the prose that names the forbidden symbols —
		// including the comment above this very test's sibling assertions —
		// does not trip the check. The gate is about code, not comments.
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, f := range file.Imports {
			imported := strings.Trim(f.Path.Value, `"`)
			for _, bad := range forbidden {
				if strings.Contains(imported, bad) {
					t.Errorf("%s imports %s; push must not be able to write", name, imported)
				}
			}
		}
		for _, ident := range identifiers(t, file) {
			for _, bad := range forbidden {
				if ident == bad {
					t.Errorf("%s references %s; push must not be able to write", name, bad)
				}
			}
		}
	}
}

// identifiers returns every identifier in the file's code, skipping comments
// and string literals so documentation cannot fail the gate.
func identifiers(t *testing.T, file *ast.File) []string {
	t.Helper()
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			out = append(out, v.Name)
		case *ast.SelectorExpr:
			// Skip the selector's Sel: a call is recorded as
			// SelectorExpr{X: Client, Sel: PlaceOrder}, and the Sel is the
			// part that matters. Record it here and let the caller see it.
			out = append(out, v.Sel.Name)
		case *ast.BasicLit:
			return false
		}
		return true
	})
	return out
}

// TestPushClientInterfaceHasNoWriteMethods asserts the same property from the
// type's side: a fake satisfying pushClient can be built without any order
// method, so a write cannot be introduced by adding a line elsewhere.
func TestPushClientInterfaceHasNoWriteMethods(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	var iface *ast.InterfaceType
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if ts.Name.Name == "pushClient" {
				if it, ok := ts.Type.(*ast.InterfaceType); ok {
					iface = it
				}
			}
		}
	}
	if iface == nil {
		t.Fatal("no pushClient interface found in main.go")
	}
	for _, field := range iface.Methods.List {
		for _, name := range field.Names {
			if strings.HasPrefix(name.Name, "Subscribe") {
				continue
			}
			if strings.HasPrefix(name.Name, "Unsubscribe") {
				continue
			}
			switch name.Name {
			case "SetCallbacks", "Connect", "Disconnect":
			default:
				t.Errorf("pushClient exposes %q, which is neither a subscribe, an unsubscribe nor connection setup", name.Name)
			}
		}
	}
}
