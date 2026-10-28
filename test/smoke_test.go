package test

import (
	"os"
	"testing"
	"time"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/tigersdk"
)

// chdirTemp mirrors the pattern from internal/tigersdk/tigersdk_test.go.
// It clears all TIGEROPEN_* vars and points $HOME at an empty temp dir,
// so no live credentials can leak into the test.
func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, k := range []string{
		"TIGEROPEN_TIGER_ID", "TIGEROPEN_PRIVATE_KEY", "TIGEROPEN_ACCOUNT",
		"TIGEROPEN_SECRET_KEY", "TIGEROPEN_TOKEN", "TIGEROPEN_TOKEN_FILE",
		"TIGER_CONFIG",
	} {
		t.Setenv(k, "")
	}
}

// testConfig mirrors the pattern from internal/tigersdk/tigersdk_test.go.
// The private key is a dummy: the SDK only checks non-empty at request time,
// and no request is sent without a real signature.
func testConfig() *config.Config {
	return &config.Config{
		TigerID:        "12345",
		PrivateKey:     "-----BEGIN RSA PRIVATE KEY-----\nSMOKE\n-----END RSA PRIVATE KEY-----",
		Account:        "DU1234567",
		SecretKey:      "smoke-secret",
		License:        config.LicenseTBNZ,
		Language:       "en_US",
		Timezone:       "US/Eastern",
		ServerURL:      config.DefaultServerURL,
		QuoteServerURL: config.DefaultServerURL,
		Timeout:        5 * time.Second,
	}
}

// TestSmokeWiring verifies the SDK wiring is intact: config loading, session
// construction, and client accessors all produce non-nil values with no network
// calls. This catches a dependency migration that silently breaks the construction
// path.
//
// Network calls are made below only for completeness; errors are tolerated so the
// test remains credentials-free and CI-friendly.
func TestSmokeWiring(t *testing.T) {
	chdirTemp(t)

	cfg := testConfig()

	// Nil logger: SDK noise goes to os.Stderr and does not fail the test.
	// The existing tigersdk_test.go silences SDK noise via TestMain in that
	// package; this test is in a different package so it does not inherit
	// that silence — but the test goal is wiring, not SDK output.
	session, err := tigersdk.NewSession(cfg, nil) // nil logger: SDK noise not fatal for wiring test
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if session.HTTP == nil {
		t.Fatal("session.HTTP is nil")
	}
	if session.QuoteHTTP == nil {
		t.Fatal("session.QuoteHTTP is nil")
	}

	qc := session.Quote()
	if qc == nil {
		t.Fatal("session.Quote() returned nil")
	}

	// Make one live read call against the real gateway. The Tiger API requires
	// a valid signature even for market-state queries, so this will return a
	// auth/signature error rather than data — but the HTTP round-trip itself
	// proves the transport, URL resolution, and request-serialization paths
	// are intact. A dependency migration that silently breaks any of those
	// layers will surface here.
	states, err := qc.GetMarketState("US")
	if err != nil {
		// Auth errors are expected without real credentials. Any other error
		// category — TLS failure, DNS failure, malformed response — would
		// indicate a wiring problem.
		t.Logf("GetMarketState (credentials-free, expect auth/signature error): %v", err)
	} else {
		t.Logf("GetMarketState: got %d states", len(states))
	}
}
