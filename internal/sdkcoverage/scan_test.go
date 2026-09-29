package sdkcoverage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientMethodsFindsEveryDeclaredClientType(t *testing.T) {
	dir := t.TempDir()
	// Helper is named deliberately: a type whose name does not end in
	// "Client" is not a client type, and neither is an unexported method on
	// one. The suffix is the SDK's only signal for "this is a client" — there
	// is no interface and no registry — so it is worth pinning what the suffix
	// does and does not admit.
	write(t, filepath.Join(dir, "quote", "q.go"), `package quote

type QuoteClient struct{}

func (c *QuoteClient) GetKline() {}

func (c *QuoteClient) hidden() {}

type Helper struct{}

func (n Helper) Nope() {}
`)
	// push/pb is generated protobuf. None of its receiver types ends in
	// "Client", so walking declarations cannot reach it — that is the whole
	// reason the old --exclude-dir=pb flag was documented as non-load-bearing.
	write(t, filepath.Join(dir, "push", "pb", "gen.go"), `package pb

type QuoteData struct{}

func (d *QuoteData) Reset() {}

func (d *QuoteData) String() string { return "" }
`)

	got, err := ClientMethods(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"GetKline"}; !equal(got, want) {
		t.Fatalf("ClientMethods = %v, want %v", got, want)
	}
}

func TestClientMethodsIgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "client", "c.go"), `package client

type HttpClient struct{}

func (c *HttpClient) Execute() {}
`)
	write(t, filepath.Join(dir, "client", "c_test.go"), `package client

func (c *HttpClient) HelperOnlyInTests() {}
`)

	got, err := ClientMethods(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(got, []string{"Execute"}) {
		t.Fatalf("ClientMethods = %v, want [Execute]; a test helper is not coverage", got)
	}
}

// TestClientMethodsDoesNotPinTheReceiverName is the control test for the
// failure that produces no error at all. A matcher that hardcodes the receiver
// variable "c" stops matching the day the SDK renames one receiver, that whole
// client type drops out of the denominator, and the run reports a smaller,
// greener number. Nothing fails. This asserts the scan is a function of the
// declaration's shape and not of a name.
func TestClientMethodsDoesNotPinTheReceiverName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "trade", "t.go"), `package trade

type TradeClient struct{}

func (this TradeClient) PlaceOrder() {}

func (x *TradeClient) CancelOrder() {}
`)

	got, err := ClientMethods(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"CancelOrder", "PlaceOrder"}; !equal(got, want) {
		t.Fatalf("ClientMethods = %v, want %v — a pinned receiver name would "+
			"silently drop PlaceOrder rather than fail", got, want)
	}
}

func TestClientMethodsFailsLoudlyOnAnAbsentModule(t *testing.T) {
	if _, err := ClientMethods(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("ClientMethods on a missing directory should fail loudly; an empty " +
			"result would report every method as uncovered and read as a real finding")
	}
}

// TestCallSitesCountsRealCallsOnly is the test that earns the move off text
// matching. The previous implementation was grep -E "\.Name(", which cannot
// tell a call from prose: a method named in a comment, or inside a string
// literal, counted as covered. Both are ways for the check to report a method
// as referenced when nothing in the program ever calls it.
func TestCallSitesCountsRealCallsOnly(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "cmd", "x", "main.go"), `package main

import "example"

func run() {
	example.GetKline()
	example.CancelOrder()
	_ = example.Execute
}
`)
	write(t, filepath.Join(root, "cmd", "x", "notes.go"), `package main

// example.GetBrief() is deprecated and is only named in this comment.
var doc = "example.SecretKey is a string in this literal"
`)

	got, err := CallSites(root, []string{"GetKline", "CancelOrder", "Execute", "GetBrief", "SecretKey"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GetKline", "CancelOrder"} {
		if !got[want] {
			t.Errorf("%s is called and should be a call site", want)
		}
	}
	// A comment, a string literal, and a method used only as a value. The
	// first two are what grep counted; the third is stricter than either and
	// is the intended direction, because the check claims call sites.
	for _, unwanted := range []string{"GetBrief", "SecretKey", "Execute"} {
		if got[unwanted] {
			t.Errorf("%s must not count: it is %s", unwanted, whatItIs(unwanted))
		}
	}
}

func whatItIs(method string) string {
	switch method {
	case "GetBrief":
		return "named only in a comment"
	case "SecretKey":
		return "named only in a string literal"
	default:
		return "used as a value, not called"
	}
}

func TestCallSitesIncludesTestFiles(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "internal", "t", "t.go"), `package t

func run(c *Client) {}
`)
	write(t, filepath.Join(root, "internal", "t", "t_test.go"), `package t

func check(c *Client) { c.GetSubscriptions() }
`)

	got, err := CallSites(root, []string{"GetSubscriptions"})
	if err != nil {
		t.Fatal(err)
	}
	// Test files are deliberately in scope. GetSubscriptions and State are
	// referenced only from internal/tigersdk/tigersdk_test.go, and excluding
	// them would move the published figure from 140/159 to 138/159. That
	// exclusion is a defensible reading, but it is a change to a published
	// number and belongs in README.md as a decision, not in a silent scope
	// change here.
	if !got["GetSubscriptions"] {
		t.Error("a call from a _test.go file is in scope by decision; excluding it " +
			"would change the published figure and must be an explicit one")
	}
}

func TestCallSitesFailsOnUnparseableSource(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "cmd", "x", "main.go"), "package main\nfunc (")
	if _, err := CallSites(root, []string{"GetKline"}); err == nil {
		t.Fatal("a parse error must be reported; swallowing it would silently turn " +
			"every method in that file into a false gap")
	}
}

func TestCallSitesToleratesARootWithoutInternal(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "cmd", "x", "main.go"), "package main\n\nfunc run() { c.GetKline() }\n")
	got, err := CallSites(root, []string{"GetKline"})
	if err != nil {
		t.Fatalf("a root with no internal/ is still a valid root: %v", err)
	}
	if !got["GetKline"] {
		t.Error("GetKline should be found under cmd/")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
