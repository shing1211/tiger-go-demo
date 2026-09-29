package tigersdk

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/tigerfintech/openapi-go-sdk/client"
	sdkconfig "github.com/tigerfintech/openapi-go-sdk/config"
	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
)

// TestMain silences the SDK's advisory std-log output, exactly as every binary
// in this project does. Left on, each NewClientConfig that finds no token file
// prints a Chinese-language notice about it, which buries real failures.
func TestMain(m *testing.M) {
	logging.SilenceSDKNoise()
	os.Exit(m.Run())
}

// The properties-file defence is the reason this package exists, so most of
// these tests are about it. The hazard: sdkconfig.NewClientConfig applies
// ./tiger_openapi_config.properties (and $HOME/.tigeropen/...) AFTER the options
// we pass, and then applies $TIGEROPEN_* on top of everything. Either can point
// the client at a different account. NewClientConfig re-asserts every field
// afterwards so the values we resolved are the ones that survive.
//
// The token file is a second, independent discovery path (a different file,
// found by a different mechanism, feeding a different field), so it needs its
// own tests rather than being folded into the ones above.

// strayProperties is a properties file that tries to redirect the client at a
// different account. Every value differs from the ones the test config uses.
const strayProperties = `# left behind by someone experimenting locally
tiger_id=99999999
private_key=-----BEGIN RSA PRIVATE KEY-----\nSTRAY\n-----END RSA PRIVATE KEY-----
account=DU9999999
secret_key=stray-secret
license=TBSG
language=zh_TN
timezone=Asia/Hong_Kong
server_url=https://stray.example.invalid/gateway
quote_server_url=https://stray.example.invalid/quote
device_id=11:22:33:44:55:66
`

// testConfig is a config that passes Validate. The private key is a dummy: the
// SDK only checks that it is non-empty until a request is actually signed, and
// no test here sends one.
func testConfig() *config.Config {
	return &config.Config{
		TigerID:        "12345",
		PrivateKey:     "-----BEGIN RSA PRIVATE KEY-----\nreal\n-----END RSA PRIVATE KEY-----",
		Account:        "DU1234567",
		SecretKey:      "our-secret",
		License:        config.LicenseTBNZ,
		Language:       "en_US",
		Timezone:       "US/Eastern",
		DeviceID:       "aa:bb:cc:dd:ee:ff",
		ServerURL:      config.DefaultServerURL,
		QuoteServerURL: config.DefaultServerURL,
		PushURL:        config.DefaultPushURL,
		Timeout:        15 * 1000000000, // 15s
	}
}

// chdirTemp moves the test into an empty directory and points $HOME at another
// empty one, so neither the package's own directory nor the developer's real
// home directory can contribute a properties file to the SDK's auto-discovery.
//
// It also clears $TIGEROPEN_* and $TIGEROPEN_TOKEN_FILE, which the SDK consults
// with the same (or higher) priority.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	for _, k := range []string{
		"TIGEROPEN_TIGER_ID", "TIGEROPEN_PRIVATE_KEY", "TIGEROPEN_ACCOUNT",
		"TIGEROPEN_SECRET_KEY", "TIGEROPEN_TOKEN", "TIGEROPEN_TOKEN_FILE",
		"TIGER_CONFIG",
	} {
		t.Setenv(k, "")
	}
	return dir
}

// writeStrayProperties plants the redirecting file in dir.
func writeStrayProperties(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "tiger_openapi_config.properties")
	if err := os.WriteFile(p, []byte(strayProperties), 0o600); err != nil {
		t.Fatalf("write stray properties: %v", err)
	}
	return p
}

// TestNewClientConfigBeatsStrayPropertiesFile is the core test: with a
// ./tiger_openapi_config.properties in the working directory that names a
// different account, every field of the returned ClientConfig must still be the
// one from *config.Config.
func TestNewClientConfigBeatsStrayPropertiesFile(t *testing.T) {
	dir := chdirTemp(t)
	writeStrayProperties(t, dir)
	cfg := testConfig()

	sc, err := NewClientConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}

	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"TigerID", sc.TigerID, cfg.TigerID},
		{"PrivateKey", sc.PrivateKey, cfg.PrivateKey},
		{"Account", sc.Account, cfg.Account},
		{"SecretKey", sc.SecretKey, cfg.SecretKey},
		{"License", sc.License, cfg.License},
		{"Language", sc.Language, cfg.Language},
		{"Timezone", sc.Timezone, cfg.Timezone},
		{"DeviceID", sc.DeviceID, cfg.DeviceID},
		{"ServerURL", sc.ServerURL, cfg.ServerURL},
		{"QuoteServerURL", sc.QuoteServerURL, cfg.QuoteServerURL},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q (the value from *config.Config)", tc.field, tc.got, tc.want)
		}
	}
	// The whole point is the account, so say so loudly if it moved.
	if sc.Account != cfg.Account {
		t.Errorf("ACCOUNT REDIRECTED: orders would go to %q instead of %q", sc.Account, cfg.Account)
	}
}

// TestNewClientConfigBeatsHomePropertiesFile covers the second discovery path,
// $HOME/.tigeropen/tiger_openapi_config.properties.
func TestNewClientConfigBeatsHomePropertiesFile(t *testing.T) {
	chdirTemp(t)
	home := os.Getenv("HOME")
	dir := filepath.Join(home, ".tigeropen")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tiger_openapi_config.properties"),
		[]byte(strayProperties), 0o600); err != nil {
		t.Fatalf("write home properties: %v", err)
	}

	cfg := testConfig()
	sc, err := NewClientConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.Account != cfg.Account || sc.TigerID != cfg.TigerID {
		t.Errorf("home properties file overrode the config: account=%q tiger_id=%q",
			sc.Account, sc.TigerID)
	}
}

// TestStrayPropertiesFileIsActuallyReachable proves the test above is not
// vacuous: with no options at all, the SDK really does adopt the file on disk.
// If this stops holding, the file-discovery hazard is gone upstream and the
// re-assertion is no longer load-bearing.
func TestStrayPropertiesFileIsActuallyReachable(t *testing.T) {
	dir := chdirTemp(t)
	writeStrayProperties(t, dir)

	sc, err := sdkconfig.NewClientConfig()
	if err != nil {
		t.Fatalf("sdk NewClientConfig with only a properties file: %v", err)
	}
	if sc.TigerID != "99999999" || sc.Account != "DU9999999" {
		t.Fatalf("the SDK did not pick up the stray file (tiger_id=%q account=%q); "+
			"the re-assertion test is no longer proving anything", sc.TigerID, sc.Account)
	}
}

// TestNewClientConfigBeatsSDKEnvVars covers the other override path. The SDK
// applies $TIGEROPEN_TIGER_ID and friends with the highest priority of all,
// above even an explicit option, so only the re-assertion stops them.
func TestNewClientConfigBeatsSDKEnvVars(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TIGEROPEN_TIGER_ID", "99999999")
	t.Setenv("TIGEROPEN_ACCOUNT", "DU9999999")
	t.Setenv("TIGEROPEN_SECRET_KEY", "env-secret")
	t.Setenv("TIGEROPEN_PRIVATE_KEY", "env-private-key")

	cfg := testConfig()
	sc, err := NewClientConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.TigerID != cfg.TigerID {
		t.Errorf("TigerID = %q, want %q ($TIGEROPEN_TIGER_ID leaked through)", sc.TigerID, cfg.TigerID)
	}
	if sc.Account != cfg.Account {
		t.Errorf("ACCOUNT REDIRECTED by $TIGEROPEN_ACCOUNT: %q instead of %q", sc.Account, cfg.Account)
	}
	if sc.PrivateKey != cfg.PrivateKey {
		t.Errorf("PrivateKey = %q, want the config's key", sc.PrivateKey)
	}
	if sc.SecretKey != cfg.SecretKey {
		t.Errorf("SecretKey = %q, want the config's secret", sc.SecretKey)
	}
}

// TestNewClientConfigBeatsStrayFileWhenFieldsAreUnset is where the re-assertion
// is genuinely load-bearing.
//
// sdkconfig.applyProperties only fills a field that is still empty, so a stray
// file cannot overwrite a credential we supplied. But our own config legitimately
// leaves some fields empty — Account is not required for a read-only command —
// and an empty field is exactly the one the file gets to fill. Without the
// re-assertion a stray file would supply an account the user never configured.
func TestNewClientConfigBeatsStrayFileWhenFieldsAreUnset(t *testing.T) {
	dir := chdirTemp(t)
	writeStrayProperties(t, dir)

	cfg := testConfig()
	cfg.Account = ""   // a read-only command needs no account
	cfg.SecretKey = "" // institutional-only field
	cfg.DeviceID = ""  // the SDK would otherwise auto-detect a MAC

	sc, err := NewClientConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.Account != "" {
		t.Errorf("ACCOUNT REDIRECTED: a stray file supplied %q for an unset account", sc.Account)
	}
	if sc.SecretKey != "" {
		t.Errorf("SecretKey = %q, want empty; a stray file supplied it", sc.SecretKey)
	}
	// DeviceID is the one field the code deliberately does not re-assert when
	// empty, so the SDK's own auto-detection stands. That is safe for two
	// reasons: the SDK's applyProperties has no device_id key at all, so a
	// properties file cannot set it (this file's device_id line is ignored), and
	// a MAC address is an identifier rather than a credential.
	if sc.DeviceID == "11:22:33:44:55:66" {
		t.Errorf("DeviceID = %q came from the stray file", sc.DeviceID)
	}
}

// TestNewClientConfigEmptyFieldsSurviveTheSDKWithoutAFile is the control for the
// test above: with no file on disk the same unset fields stay unset, so the
// previous test is measuring the re-assertion and not the SDK.
func TestNewClientConfigEmptyFieldsSurviveTheSDKWithoutAFile(t *testing.T) {
	chdirTemp(t)
	cfg := testConfig()
	cfg.Account = ""
	cfg.SecretKey = ""

	sc, err := NewClientConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.Account != "" || sc.SecretKey != "" {
		t.Errorf("unset fields should stay unset with no properties file: account=%q secret=%q",
			sc.Account, sc.SecretKey)
	}
}

// TestNewClientConfigRejectsBadConfig covers the guards in front of the SDK.
func TestNewClientConfigRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantMissing string
	}{
		{"nil config", nil, ""},
		{"no tiger id", &config.Config{PrivateKey: "k"}, config.EnvTigerID},
		{"no private key", &config.Config{TigerID: "1"}, config.EnvPrivateKey},
		{"neither", &config.Config{}, config.EnvTigerID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			sc, err := NewClientConfig(tc.cfg)
			if err == nil {
				t.Fatalf("want an error, got config %+v", sc)
			}
			if sc != nil {
				t.Errorf("a rejected config must not return a ClientConfig, got %+v", sc)
			}
			if tc.wantMissing == "" {
				if !strings.Contains(err.Error(), "nil config") {
					t.Errorf("nil config error = %v", err)
				}
				return
			}
			if !config.MissingCredentialErrorIs(err) {
				t.Errorf("want a MissingCredentialError so the command exits 2, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tc.wantMissing) {
				t.Errorf("error should name %s, got %v", tc.wantMissing, err)
			}
		})
	}
}

// TestNewClientConfigNeedsNoNetwork pins the property the rest of the project
// relies on: building the SDK config is offline and deterministic, so it can
// run in a test and a command can fail fast on bad credentials before dialling.
func TestNewClientConfigNeedsNoNetwork(t *testing.T) {
	chdirTemp(t)
	// No ServerURL, so the SDK would have to look a domain up if dynamic lookup
	// were enabled. We pass an explicit URL and disable the lookup, so nothing
	// leaves the process.
	cfg := testConfig()
	sc, err := NewClientConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.EnableDynamicDomain {
		t.Error("dynamic domain lookup should be off, so no request is made at construction")
	}
	if sc.ServerURL == "" || sc.QuoteServerURL == "" {
		t.Errorf("both URLs must be resolved locally, got server=%q quote=%q",
			sc.ServerURL, sc.QuoteServerURL)
	}
	if sc.Timeout != cfg.Timeout {
		t.Errorf("Timeout = %v, want %v", sc.Timeout, cfg.Timeout)
	}
}

// TestNewSessionWithLogger covers the non-nil logger branch: the logger is
// handed to the SDK so SDK internals share our format and threshold.
func TestNewSessionWithLogger(t *testing.T) {
	chdirTemp(t)
	var buf bytes.Buffer
	s, err := NewSession(testConfig(), logging.New(&buf, logging.ParseLevel("info")))
	if err != nil {
		t.Fatalf("NewSession with a logger: %v", err)
	}
	if s.HTTP == nil {
		t.Fatal("Session.HTTP is nil")
	}
	// A nil writer would still have to be safe; the point is only that the
	// option path is taken and does not disturb construction.
}

// TestNewSession covers construction, the two client accessors and Close.
// Nothing here opens a socket: NewHttpClient only builds an http.Client and
// parses the public key, and the token auto-refresh goroutine only starts when
// TokenRefreshDuration is set, which it never is.
func TestNewSession(t *testing.T) {
	chdirTemp(t)
	cfg := testConfig()

	s, err := NewSession(cfg, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	if s.Config != cfg {
		t.Error("Session.Config should be the config we passed")
	}
	if s.SDK == nil {
		t.Fatal("Session.SDK is nil")
	}
	if s.HTTP == nil {
		t.Fatal("Session.HTTP is nil")
	}
	if s.QuoteHTTP == nil {
		t.Fatal("Session.QuoteHTTP is nil")
	}
	if s.QuoteHTTP == s.HTTP {
		t.Error("the quote client must not share the trade HttpClient: " +
			"one client can only honour one server URL")
	}
	if s.Quote() == nil {
		t.Error("Quote() returned nil")
	}
	if s.Trade() == nil {
		t.Error("Trade() returned nil")
	}
	// The trade client is bound to the configured account, which is the whole
	// point of the re-assertion above.
	if got := s.Trade(); got == nil {
		t.Error("Trade() should be constructible from the session")
	}
	// Building the quote client must leave the trade gateway alone: the SDK
	// substitutes QuoteServerURL onto its own copy of the config, and if that
	// copy were shared, ServerURL here would come back as the quote endpoint.
	if s.SDK.ServerURL != cfg.ServerURL {
		t.Errorf("Session.SDK.ServerURL = %q, want the trade gateway %q",
			s.SDK.ServerURL, cfg.ServerURL)
	}
	s.Close()
}

// captureQuoteConfig swaps the SDK's quote-client constructor for one that
// records the ClientConfig it is handed, and returns a getter for it. This is
// how the quote endpoint is observed at all: HttpClient holds its config in an
// unexported field and exports nothing that reveals it.
func captureQuoteConfig(t *testing.T) func() *sdkconfig.ClientConfig {
	t.Helper()
	var captured *sdkconfig.ClientConfig
	orig := newQuoteHttpClient
	newQuoteHttpClient = func(cfg *sdkconfig.ClientConfig, opts ...sdkclient.ClientOption) *sdkclient.HttpClient {
		captured = cfg
		return orig(cfg, opts...)
	}
	t.Cleanup(func() { newQuoteHttpClient = orig })
	return func() *sdkconfig.ClientConfig { return captured }
}

// TestQuoteServerURLIsHonoured is the guard for the quote-gateway fix.
//
// The SDK substitutes QuoteServerURL for ServerURL in exactly one place,
// client.NewQuoteHttpClient. Before, Quote() handed that constructor the
// session's single HttpClient, so every market-data request went to the trade
// gateway and a configured quote endpoint was never read — the documented knob
// did nothing at all.
func TestQuoteServerURLIsHonoured(t *testing.T) {
	const quoteURL = "https://quote.example.invalid/gateway"
	tests := []struct {
		name     string
		quoteURL string
		want     string
	}{
		{
			name:     "a distinct quote endpoint is used as configured",
			quoteURL: quoteURL,
			want:     quoteURL,
		},
		{
			// The behaviour everyone actually relies on: config.Load defaults
			// quote_server_url to server_url, and so must a Config built by
			// hand. This is the case that would break first if the quote client
			// were wired to the wrong HttpClient.
			name:     "an unset quote endpoint falls back to the trade gateway",
			quoteURL: "",
			want:     config.DefaultServerURL,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			quoteConfig := captureQuoteConfig(t)

			cfg := testConfig()
			cfg.QuoteServerURL = tc.quoteURL

			s, err := NewSession(cfg, nil)
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			t.Cleanup(s.Close)

			qc := quoteConfig()
			if qc == nil {
				t.Fatal("the quote client was not built with NewQuoteHttpClient, " +
					"so QuoteServerURL has no way to be honoured")
			}
			if qc.QuoteServerURL != tc.want {
				t.Errorf("quote client built with QuoteServerURL = %q, want %q",
					qc.QuoteServerURL, tc.want)
			}
			// The same resolution has to be in the SDK config we hand out, not
			// just in the quote client: it is what a caller reading
			// Session.SDK would (reasonably) believe is in effect.
			if s.SDK.QuoteServerURL != tc.want {
				t.Errorf("Session.SDK.QuoteServerURL = %q, want %q",
					s.SDK.QuoteServerURL, tc.want)
			}
			// And the fallback must not disturb the trade gateway.
			if s.SDK.ServerURL != cfg.ServerURL {
				t.Errorf("Session.SDK.ServerURL = %q, want %q", s.SDK.ServerURL, cfg.ServerURL)
			}
		})
	}
}

// TestQuoteClientDoesNotDisturbTheTradeClient: the two gateways are configured
// independently, so pointing one at a proxy must not move the other.
func TestQuoteClientDoesNotDisturbTheTradeClient(t *testing.T) {
	chdirTemp(t)
	cfg := testConfig()
	cfg.ServerURL = "https://trade.example.invalid/gateway"
	cfg.QuoteServerURL = "https://quote.example.invalid/gateway"

	s, err := NewSession(cfg, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(s.Close)

	if s.SDK.ServerURL != cfg.ServerURL {
		t.Errorf("trade gateway = %q, want %q", s.SDK.ServerURL, cfg.ServerURL)
	}
	if s.SDK.QuoteServerURL != cfg.QuoteServerURL {
		t.Errorf("quote gateway = %q, want %q", s.SDK.QuoteServerURL, cfg.QuoteServerURL)
	}
}

// TestSessionCloseIsIdempotent: the commands defer Close and also call it on
// some paths, and Close must tolerate being reached twice.
//
// The session now owns two HTTP clients and Close has to reach both — a quote
// client that nobody closes is the leak this guards. They are distinct
// objects, so closing each one is not a double close of a single client; that
// is asserted above and is what makes the second Close harmless.
func TestSessionCloseIsIdempotent(t *testing.T) {
	chdirTemp(t)
	s, err := NewSession(testConfig(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s.HTTP == nil || s.QuoteHTTP == nil {
		t.Fatal("both clients must exist for Close to have anything to release")
	}
	if s.HTTP == s.QuoteHTTP {
		t.Fatal("the two clients are the same object; Close would stop one goroutine twice")
	}
	s.Close()
	s.Close() // must not panic

	// Closing the SDK's own quote client twice is safe independently of the
	// session: NewQuoteHttpClient zeroes TokenRefreshDuration, so it owns no
	// refresh goroutine and Close has nothing to stop.
	quote := sdkclient.NewQuoteHttpClient(s.SDK)
	quote.Close()
	quote.Close()
}

// TestSessionCloseStopsBothRefreshLoops makes "Close releases both clients"
// observable instead of assumed.
//
// HttpClient.Close only stops a background token-refresh goroutine, and the
// clients NewSession builds own none — TokenRefreshDuration is 0 everywhere and
// NewQuoteHttpClient zeroes it again — so closing them is a no-op today and no
// assertion could tell a leaked quote client from a released one. The goroutine
// is the only resource either client has, so the test gives each one and counts
// them. Their 24h check interval and zero refresh threshold mean the loop never
// reaches the network.
//
// A client owns a refresh loop only when NewHttpClient sees a non-zero
// TokenRefreshDuration: StartTokenAutoRefresh is exported and starts one, but
// it does not record the manager on the client, and Close reads it from there.
func TestSessionCloseStopsBothRefreshLoops(t *testing.T) {
	chdirTemp(t)
	// Counted relative to whatever else is running, so an unrelated goroutine
	// cannot make the exact-count assertions below flaky.
	base := countRefreshLoops(t)
	raw := &sdkconfig.ClientConfig{
		TigerID:              "1",
		PrivateKey:           "k",
		Timeout:              15 * time.Second,
		TokenRefreshDuration: time.Hour,
		TokenCheckInterval:   24 * time.Hour,
	}
	s := &Session{
		HTTP:      sdkclient.NewHttpClient(raw),
		QuoteHTTP: sdkclient.NewHttpClient(raw),
	}
	t.Cleanup(s.Close)
	waitForRefreshLoops(t, base+2)

	s.Close()
	waitForRefreshLoops(t, base)

	s.Close() // twice is safe, and releases nothing twice
	waitForRefreshLoops(t, base)
}

// TestNewSessionOwnsNoGoroutines: the clients NewSession really builds start no
// token-refresh goroutine, so a session that is never closed leaks nothing on
// the goroutine front. It is the reason the test above has to work for its
// observation, and it is the shape every command actually uses.
func TestNewSessionOwnsNoGoroutines(t *testing.T) {
	chdirTemp(t)
	base := countRefreshLoops(t)
	s, err := NewSession(testConfig(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(s.Close)
	if got := countRefreshLoops(t); got != base {
		t.Errorf("a new session started %d goroutines, want none", got-base)
	}
}

// countRefreshLoops counts the SDK's token-refresh goroutines. Matching the
// function name in a stack dump counts one specific thing, rather than
// comparing a total that other tests' goroutines would make unreliable.
func countRefreshLoops(t *testing.T) int {
	t.Helper()
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), "refreshLoopWith")
		}
		buf = make([]byte, 2*len(buf))
	}
}

// waitForRefreshLoops gives goroutines a bounded window to start or wind down.
// The window is generous because Close signals a goroutine rather than killing
// it: it has to be scheduled before the count changes, and a scheduling delay
// is not a leak.
func waitForRefreshLoops(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := countRefreshLoops(t)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("found %d token-refresh goroutines, want %d (a client was leaked, or closed twice)", got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSessionNilIsSafe covers the nil guards on Close, so a partially built
// session can always be cleaned up.
func TestSessionNilIsSafe(t *testing.T) {
	var s *Session
	s.Close() // must not panic

	empty := &Session{}
	empty.Close() // must not panic

	// A half-built session: one client constructed, the other never reached
	// because construction failed. Close must not fall over the missing one.
	partial := &Session{HTTP: sdkclient.NewHttpClient(&sdkconfig.ClientConfig{
		TigerID: "1", Timeout: 15 * 1000000000,
	})}
	partial.Close()
}

// TestNewSessionRejectsBadConfig: a command with no credentials must fail here,
// before it can reach a client.
func TestNewSessionRejectsBadConfig(t *testing.T) {
	chdirTemp(t)
	s, err := NewSession(&config.Config{}, nil)
	if err == nil {
		t.Fatal("want an error for an empty config")
	}
	if s != nil {
		t.Errorf("no session should be returned, got %+v", s)
	}
	if !config.MissingCredentialErrorIs(err) {
		t.Errorf("want a MissingCredentialError (exit code 2), got %T: %v", err, err)
	}
}

// TestSDKConfigErrorIsUnreachable explains the one uncovered branch in
// NewClientConfig: the error return from sdkconfig.NewClientConfig. That
// function only fails when tiger_id or private_key is empty, and Validate above
// has already rejected exactly that case, so the branch cannot be reached
// through this API. It is defensive against a future SDK change that adds a new
// validation rule, and it is left untested rather than worked around.
func TestSDKConfigErrorIsUnreachable(t *testing.T) {
	chdirTemp(t)
	// Validate accepts these, so the SDK accepts them too.
	cfg := &config.Config{TigerID: "1", PrivateKey: "k"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate should accept a minimal config: %v", err)
	}
	if _, err := NewClientConfig(cfg); err != nil {
		t.Errorf("a validated config should always build, got %v", err)
	}
}

// TestPushReachableWithoutNetwork: Push builds a client and returns; the
// connection only happens on Connect, which no test calls.
func TestPushReachableWithoutNetwork(t *testing.T) {
	chdirTemp(t)
	c, err := Push(testConfig(), PushOptions{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if c == nil {
		t.Fatal("Push returned nil client")
	}
	if c.State() != sdkpush.StateDisconnected {
		t.Errorf("State() = %v, want StateDisconnected (no socket should be opened)", c.State())
	}
	if subs := c.GetSubscriptions(); len(subs) != 0 {
		t.Errorf("a fresh client should have no subscriptions, got %v", subs)
	}
}

// TestPushBeatsStrayPropertiesFile: the push client authenticates with the
// account too, so it needs the same re-assertion. There is no public accessor
// for the account, so the check is that construction succeeds and the config it
// was built from is the one we passed — a stray file must not make Push fail or
// resolve a different gateway.
func TestPushBeatsStrayPropertiesFile(t *testing.T) {
	dir := chdirTemp(t)
	writeStrayProperties(t, dir)

	cfg := testConfig()
	c, err := Push(cfg, PushOptions{})
	if err != nil {
		t.Fatalf("Push with a stray properties file present: %v", err)
	}
	if c.State() != sdkpush.StateDisconnected {
		t.Errorf("State() = %v, want StateDisconnected", c.State())
	}
}

// TestPushRejectsBadConfig mirrors NewSession: no credentials, no client.
func TestPushRejectsBadConfig(t *testing.T) {
	chdirTemp(t)
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{"nil config", nil},
		{"empty config", &config.Config{}},
		{"no private key", &config.Config{TigerID: "1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			c, err := Push(tc.cfg, PushOptions{})
			if err == nil {
				t.Fatalf("want an error, got a client in state %v", c.State())
			}
			if c != nil {
				t.Errorf("no client should be returned, got %+v", c)
			}
			if tc.cfg != nil && !config.MissingCredentialErrorIs(err) {
				t.Errorf("want a MissingCredentialError, got %T: %v", err, err)
			}
		})
	}
}

// TestPushOptionsAcceptsAnyValues records what PushOptions actually does: there
// is no validation, each non-zero interval is passed to the SDK, and the zero
// value means "let the SDK decide". Negative values are passed through as-is,
// which is a property of the SDK, not a check we make.
func TestPushOptionsAcceptsAnyValues(t *testing.T) {
	tests := []struct {
		name string
		opts PushOptions
	}{
		{"zero values", PushOptions{}},
		{"all set", PushOptions{
			HeartbeatInterval: 30 * 1000000000,
			ReconnectInterval: 10 * 1000000000,
			ConnectTimeout:    15 * 1000000000,
		}},
		{"negative values are not rejected", PushOptions{
			HeartbeatInterval: -1,
			ReconnectInterval: -2,
			ConnectTimeout:    -3,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			c, err := Push(testConfig(), tc.opts)
			if err != nil {
				t.Fatalf("Push: %v", err)
			}
			if c.State() != sdkpush.StateDisconnected {
				t.Errorf("State() = %v, want StateDisconnected", c.State())
			}
		})
	}
}

// TestWarnStrayProperties covers both branches: a file present must be named
// and called out as ignored, and its absence must be completely silent.
func TestWarnStrayProperties(t *testing.T) {
	tests := []struct {
		name     string
		plant    bool
		wantOut  bool
		wantText []string
	}{
		{
			name:     "file present warns and names it",
			plant:    true,
			wantOut:  true,
			wantText: []string{"tiger_openapi_config.properties", "IGNORED"},
		},
		{
			name:    "file absent stays silent",
			plant:   false,
			wantOut: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := chdirTemp(t)
			if tc.plant {
				writeStrayProperties(t, dir)
			}
			var buf strings.Builder
			WarnStrayProperties(dir, &buf)
			got := buf.String()
			if tc.wantOut {
				if got == "" {
					t.Fatal("a stray properties file must produce a warning")
				}
				for _, want := range tc.wantText {
					if !strings.Contains(got, want) {
						t.Errorf("warning should mention %q, got:\n%s", want, got)
					}
				}
				// It must be obvious that the file is not in effect, and that
				// deleting it is the fix.
				if !strings.Contains(got, "redirect your orders") {
					t.Errorf("warning should explain the consequence, got:\n%s", got)
				}
			} else if got != "" {
				t.Errorf("nothing should be written when no file exists, got:\n%s", got)
			}
		})
	}
}

// TestWarnStrayPropertiesNamesTheFile checks the warning identifies the file
// that was found, and records an accurate detail about the path: the commands
// pass ".", and filepath.Join(".", name) normalises away the "./" prefix, so the
// warning names the file without a directory. That is adequate here because the
// only directory ever checked is the working directory, and the message says so
// by being about "the" file — but it means the warning is not a usable path for
// a script to delete.
func TestWarnStrayPropertiesNamesTheFile(t *testing.T) {
	dir := chdirTemp(t)
	p := writeStrayProperties(t, dir)

	var relative strings.Builder
	WarnStrayProperties(".", &relative)
	if !strings.Contains(relative.String(), "tiger_openapi_config.properties") {
		t.Errorf("warning should name the file, got:\n%s", relative.String())
	}
	if filepath.IsAbs(relative.String()) || strings.Contains(relative.String(), p) {
		t.Logf("warning embeds the path %q", p)
	}

	// An absolute directory yields the absolute path, which is the useful form.
	var absolute strings.Builder
	WarnStrayProperties(dir, &absolute)
	if !strings.Contains(absolute.String(), p) {
		t.Errorf("with an absolute dir the warning should contain %q, got:\n%s", p, absolute.String())
	}
}

// TestWarnStrayPropertiesMissingDirectory: a directory that does not exist must
// not produce a warning and must not panic. The commands pass ".", but a
// library caller could pass anything.
func TestWarnStrayPropertiesMissingDirectory(t *testing.T) {
	chdirTemp(t)
	var buf strings.Builder
	WarnStrayProperties(filepath.Join(t.TempDir(), "nope"), &buf)
	if buf.String() != "" {
		t.Errorf("a missing directory should be silent, got:\n%s", buf.String())
	}
}

// TestDescribeError covers the three cases a caller has to tell apart: no error,
// an error from our own code, and a structured error from Tiger that carries a
// code and a category.
func TestDescribeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		want     string
		contains []string
	}{
		{
			name: "nil",
			err:  nil,
			want: "<nil>",
		},
		{
			name:     "plain error passes through",
			err:      errors.New("context deadline exceeded"),
			want:     "context deadline exceeded",
			contains: []string{"context deadline exceeded"},
		},
		{
			name:     "tiger error renders code and category",
			err:      sdkclient.NewTigerError(1042, "参数不合法"),
			contains: []string{"tiger API error", "参数不合法", "code 1042", "biz_param_error"},
		},
		{
			name:     "tiger error with a permission code",
			err:      sdkclient.NewTigerError(4001, "no permission"),
			contains: []string{"tiger API error", "no permission", "code 4001", "permission_error"},
		},
		{
			name:     "tiger error with a rate-limit code",
			err:      sdkclient.NewTigerError(5, "too many requests"),
			contains: []string{"code 5", "rate_limit"},
		},
		{
			name:     "wrapped tiger error is still recognised",
			err:      fmt.Errorf("get addon entitlement: %w", sdkclient.NewTigerError(1010, "bad param")),
			contains: []string{"tiger API error", "bad param", "code 1010"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DescribeError(tc.err)
			if tc.want != "" && got != tc.want {
				t.Errorf("DescribeError = %q, want exactly %q", got, tc.want)
			}
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("DescribeError = %q, should contain %q", got, want)
				}
			}
		})
	}
}

// TestNewClientConfigBeatsStrayTokenFile covers the second auto-discovered
// file, ./tiger_openapi_token.properties, which supplies the bearer token that
// NewHttpClient copies into the Authorization header of every request. Nothing
// in this project sets a token, so a file in the working directory must not be
// able to authenticate the session as somebody else's account.
func TestNewClientConfigBeatsStrayTokenFile(t *testing.T) {
	dir := chdirTemp(t)
	const stray = "STRAY-TOKEN-VALUE"
	writeStrayTokenFile(t, dir, stray)

	sc, err := NewClientConfig(testConfig())
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.Token != "" {
		t.Errorf("Token = %q, want empty; a stray ./%s authenticated the session",
			sc.Token, sdkTokenFileName)
	}

	// The file is also named in the warning, on its own: the two are discovered
	// by separate mechanisms, so finding one says nothing about the other.
	var buf strings.Builder
	WarnStrayProperties(dir, &buf)
	got := buf.String()
	if !strings.Contains(got, sdkTokenFileName) {
		t.Errorf("warning should name %s, got:\n%s", sdkTokenFileName, got)
	}
	if !strings.Contains(got, "IGNORED") {
		t.Errorf("warning should say the file is ignored, got:\n%s", got)
	}
}

// TestStrayTokenFileIsActuallyReachable proves the test above is not vacuous:
// the SDK really does read ./tiger_openapi_token.properties on its own and put
// the value in ClientConfig.Token. If this stops holding, the token-file hazard
// is gone upstream and clearing Token is no longer load-bearing.
func TestStrayTokenFileIsActuallyReachable(t *testing.T) {
	dir := chdirTemp(t)
	const stray = "STRAY-TOKEN-VALUE"
	writeStrayTokenFile(t, dir, stray)

	// No options at all would fail validation on tiger_id/private_key, and the
	// token block runs before validation, so credentials are supplied explicitly
	// to isolate the token file as the only discovered input.
	sc, err := sdkconfig.NewClientConfig(
		sdkconfig.WithTigerID("1"),
		sdkconfig.WithPrivateKey("k"),
	)
	if err != nil {
		t.Fatalf("sdk NewClientConfig: %v", err)
	}
	if sc.Token != stray {
		t.Fatalf("Token = %q, want the stray value %q; the SDK did not read the token "+
			"file, so TestNewClientConfigBeatsStrayTokenFile is no longer proving anything",
			sc.Token, stray)
	}
}

// TestNewClientConfigBeatsSDKTokenEnvVars covers the two env vars that feed the
// same field. $TIGEROPEN_TOKEN supplies the token outright, and
// $TIGEROPEN_TOKEN_FILE redirects the SDK to any file on disk — so it can name a
// path this project never looks at. Both are defeated by the same clearing.
func TestNewClientConfigBeatsSDKTokenEnvVars(t *testing.T) {
	t.Run("$TIGEROPEN_TOKEN supplies the token", func(t *testing.T) {
		chdirTemp(t)
		t.Setenv("TIGEROPEN_TOKEN", "env-token")

		sc, err := NewClientConfig(testConfig())
		if err != nil {
			t.Fatalf("NewClientConfig: %v", err)
		}
		if sc.Token != "" {
			t.Errorf("Token = %q, want empty ($TIGEROPEN_TOKEN leaked through)", sc.Token)
		}
	})

	// A token file somewhere the SDK is told to look, outside the directory
	// WarnStrayProperties inspects. It wins over the default file name, so it can
	// name a path this project never looks at.
	t.Run("$TIGEROPEN_TOKEN_FILE redirects to another path", func(t *testing.T) {
		chdirTemp(t)
		elsewhere := filepath.Join(t.TempDir(), "elsewhere.properties")
		if err := os.WriteFile(elsewhere, []byte("token=redirected-token\n"), 0o600); err != nil {
			t.Fatalf("write redirected token file: %v", err)
		}
		t.Setenv("TIGEROPEN_TOKEN_FILE", elsewhere)

		sc, err := NewClientConfig(testConfig())
		if err != nil {
			t.Fatalf("NewClientConfig: %v", err)
		}
		if sc.Token != "" {
			t.Errorf("Token = %q, want empty ($TIGEROPEN_TOKEN_FILE redirected the SDK to %s)",
				sc.Token, elsewhere)
		}
	})
}

// writeStrayTokenFile plants a token file in dir.
func writeStrayTokenFile(t *testing.T, dir, token string) string {
	t.Helper()
	p := filepath.Join(dir, sdkTokenFileName)
	if err := os.WriteFile(p, []byte("token="+token+"\n"), 0o600); err != nil {
		t.Fatalf("write stray token file: %v", err)
	}
	return p
}

// writeStrayHomeProperties plants the redirecting file in $HOME/.tigeropen, the
// location the SDK auto-discovers regardless of the working directory.
func writeStrayHomeProperties(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".tigeropen")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	p := filepath.Join(dir, config.StrayPropertiesFileName)
	if err := os.WriteFile(p, []byte(strayProperties), 0o600); err != nil {
		t.Fatalf("write home properties: %v", err)
	}
	return p
}

// TestWarnStrayPropertiesNamesTheHomeFile is the second half of the home-file
// defence.
//
// The SDK reads ~/.tigeropen/tiger_openapi_config.properties no matter which
// directory a command runs from, so a user can plant a file in their home
// directory, run a command from anywhere, watch it have no effect, and get no
// explanation: every call site passes "." to WarnStrayProperties, so the file
// was never stat'd. Our values still win — the re-assertion above guarantees
// that, and this test checks it too — but the discovery itself has to be
// visible, or the "my edits do nothing, why?" experience is back.
func TestWarnStrayPropertiesNamesTheHomeFile(t *testing.T) {
	chdirTemp(t) // HOME points at an empty directory; the working dir is empty too
	p := writeStrayHomeProperties(t)

	var buf strings.Builder
	WarnStrayProperties(".", &buf)
	got := buf.String()
	if !strings.Contains(got, p) {
		t.Errorf("warning should name the home file %q, got:\n%s", p, got)
	}
	if !strings.Contains(got, "IGNORED") || !strings.Contains(got, "redirect your orders") {
		t.Errorf("warning should say the file is ignored and inert, got:\n%s", got)
	}
	// Only the home file exists, so it is the only one to name: a warning that
	// also pointed at a working-directory file would send the user to delete
	// something that is not there.
	if n := strings.Count(got, config.StrayPropertiesFileName); n != 1 {
		t.Errorf("expected the home file to be named exactly once, saw %d mentions:\n%s", n, got)
	}

	// And the defence itself: the file is discovered and overridden, and our
	// values are the ones that survive.
	sc, err := NewClientConfig(testConfig())
	if err != nil {
		t.Fatalf("NewClientConfig: %v", err)
	}
	if sc.Account != "DU1234567" || sc.TigerID != "12345" {
		t.Errorf("the home file redirected the client: account=%q tiger_id=%q",
			sc.Account, sc.TigerID)
	}
	if sc.ServerURL != config.DefaultServerURL {
		t.Errorf("ServerURL = %q, want the configured gateway %q",
			sc.ServerURL, config.DefaultServerURL)
	}
}

// TestHomeFileIsActuallyReachable proves the test above is not vacuous: the SDK
// really does adopt ~/.tigeropen/tiger_openapi_config.properties on its own.
// If this stops holding, the home file is no longer a discovery path and the
// warning is noise.
func TestHomeFileIsActuallyReachable(t *testing.T) {
	chdirTemp(t)
	writeStrayHomeProperties(t)

	sc, err := sdkconfig.NewClientConfig(
		sdkconfig.WithTigerID("1"),
		sdkconfig.WithPrivateKey("k"),
	)
	if err != nil {
		t.Fatalf("sdk NewClientConfig: %v", err)
	}
	if sc.Account != "DU9999999" {
		t.Fatalf("the SDK did not read the home properties file (account=%q); "+
			"the home-file warning is no longer describing a real discovery path",
			sc.Account)
	}
}

// TestWarnStrayPropertiesHomeFileAbsentIsSilent: the overwhelmingly common
// case. A home directory with no ~/.tigeropen must produce no output at all —
// a warning nobody can act on trains people to ignore warnings.
func TestWarnStrayPropertiesHomeFileAbsentIsSilent(t *testing.T) {
	chdirTemp(t) // HOME is an empty temporary directory

	var buf strings.Builder
	WarnStrayProperties(".", &buf)
	if got := buf.String(); got != "" {
		t.Errorf("no stray file exists, so nothing should be printed, got:\n%s", got)
	}
}

// TestWarnStrayPropertiesSurvivesAMissingHome covers the machines where there
// is no home directory to look in: $HOME unset (a bare container, a cron job
// started without a login shell) and $HOME pointing at a path that does not
// exist. Neither is an error worth surfacing — the file cannot be there — and
// neither may take a command down.
func TestWarnStrayPropertiesSurvivesAMissingHome(t *testing.T) {
	t.Run("HOME is unset", func(t *testing.T) {
		dir := chdirTemp(t)
		// Setenv registers the restore; unsetting on top of it leaves the
		// variable genuinely absent rather than set to an empty string.
		t.Setenv("HOME", "")
		if err := os.Unsetenv("HOME"); err != nil {
			t.Fatalf("unset HOME: %v", err)
		}
		// A working-directory file is still reported, so a broken $HOME
		// disables one detection rather than all of them.
		writeStrayProperties(t, dir)

		var buf strings.Builder
		WarnStrayProperties(".", &buf) // must not panic
		got := buf.String()
		if !strings.Contains(got, config.StrayPropertiesFileName) {
			t.Errorf("the working-directory file should still be named, got:\n%s", got)
		}
		if strings.Contains(got, ".tigeropen") {
			t.Errorf("no home file can exist, so none should be named, got:\n%s", got)
		}
	})

	t.Run("HOME points at a directory that does not exist", func(t *testing.T) {
		chdirTemp(t)
		missing := filepath.Join(t.TempDir(), "no-such-home")
		t.Setenv("HOME", missing)
		t.Setenv("USERPROFILE", missing) // os.UserHomeDir on Windows

		var buf strings.Builder
		WarnStrayProperties(".", &buf) // must not panic
		if got := buf.String(); got != "" {
			t.Errorf("no home file can exist, so nothing should be printed, got:\n%s", got)
		}
	})
}
