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
