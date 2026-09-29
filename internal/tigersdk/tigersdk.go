// Package tigersdk builds SDK clients from our validated configuration.
//
// It exists to solve one specific upstream hazard: the Tiger SDK auto-discovers
// ./tiger_openapi_config.properties, ~/.tigeropen/... and
// ./tiger_openapi_token.properties, and will silently override credentials you
// passed explicitly. Every ClientConfig we hand out is re-asserted after
// construction so that a stray properties file can never change which account
// is being traded.
package tigersdk

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdkclient "github.com/tigerfintech/openapi-go-sdk/client"
	sdkconfig "github.com/tigerfintech/openapi-go-sdk/config"
	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"
	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"
	sdktrade "github.com/tigerfintech/openapi-go-sdk/trade"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
)

// NewClientConfig converts our config into an SDK ClientConfig.
//
// Every field is passed explicitly and then re-asserted, so the SDK's
// properties-file auto-discovery has nothing left to fill in.
func NewClientConfig(cfg *config.Config) (*sdkconfig.ClientConfig, error) {
	if cfg == nil {
		return nil, errors.New("nil config")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	opts := []sdkconfig.Option{
		sdkconfig.WithTigerID(cfg.TigerID),
		sdkconfig.WithPrivateKey(cfg.PrivateKey),
		sdkconfig.WithAccount(cfg.Account),
		sdkconfig.WithLicense(cfg.License),
		sdkconfig.WithLanguage(cfg.Language),
		sdkconfig.WithTimezone(cfg.Timezone),
		sdkconfig.WithTimeout(cfg.Timeout),
		sdkconfig.WithServerURL(cfg.ServerURL),
		sdkconfig.WithQuoteServerURL(quoteServerURL(cfg)),
		// Dynamic domain lookup is a network call; the explicit URL is honoured
		// as-is so behaviour is predictable and offline-testable.
		sdkconfig.WithEnableDynamicDomain(false),
	}
	if cfg.DeviceID != "" {
		opts = append(opts, sdkconfig.WithDeviceID(cfg.DeviceID))
	}
	// NOTE: the SDK's own env vars (TIGEROPEN_*) are deliberately NOT used.
	// Our loader is the single source of truth, and the SDK's NewClientConfig
	// would apply TIGEROPEN_* on top of what we resolved here.
	if cfg.SecretKey != "" {
		opts = append(opts, sdkconfig.WithSecretKey(cfg.SecretKey))
	}

	sc, err := sdkconfig.NewClientConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("build SDK client config: %w", err)
	}

	// Re-assert: defeat auto-discovery of ./tiger_openapi_config.properties.
	sc.TigerID = cfg.TigerID
	sc.PrivateKey = cfg.PrivateKey
	sc.Account = cfg.Account
	sc.SecretKey = cfg.SecretKey
	sc.License = cfg.License
	sc.Language = cfg.Language
	sc.Timezone = cfg.Timezone
	sc.ServerURL = cfg.ServerURL
	sc.QuoteServerURL = quoteServerURL(cfg)
	if cfg.DeviceID != "" {
		sc.DeviceID = cfg.DeviceID
	}
	// Token is the one field the SDK discovers from a *second* file,
	// ./tiger_openapi_token.properties (or $TIGEROPEN_TOKEN / $TIGEROPEN_TOKEN_FILE),
	// independently of the config file above. It is not a credential we ever
	// set, and that is exactly why it has to be cleared: NewHttpClient copies it
	// into the Authorization header of every request, so a file nobody in this
	// project wrote can authenticate the session as somebody else's account.
	// Clearing costs nothing because no flow here uses a bearer token.
	sc.Token = ""
	return sc, nil
}

// quoteServerURL resolves the endpoint market data goes to.
//
// The README documents the default as "= server_url", and config.Load already
// applies it, but a Config built by hand (a test, a library caller) can still
// leave it empty. The SDK also falls back to ServerURL inside
// NewQuoteHttpClient, so an empty value is not a crash — it is just implicit.
// Resolving it here keeps the field meaning one thing: the quote client is
// always built from a URL this project chose.
func quoteServerURL(cfg *config.Config) string {
	if cfg.QuoteServerURL != "" {
		return cfg.QuoteServerURL
	}
	return cfg.ServerURL
}

// Session bundles the HTTP clients the quote and trade calls are made through.
//
// The two are separate objects. The SDK's NewQuoteHttpClient clones the config
// and substitutes QuoteServerURL for ServerURL on the copy, so a shared client
// would be locked to whichever URL it was built with; two clients means the
// trade gateway and the quote gateway are each honoured as configured.
type Session struct {
	Config *config.Config
	SDK    *sdkconfig.ClientConfig
	// HTTP talks to ServerURL (trade, account, corporate actions).
	HTTP *sdkclient.HttpClient
	// QuoteHTTP talks to QuoteServerURL. Never nil for a session built by
	// NewSession.
	QuoteHTTP *sdkclient.HttpClient
}

// NewSession builds the SDK config and the HTTP clients.
func NewSession(cfg *config.Config, log *logging.Logger) (*Session, error) {
	sc, err := NewClientConfig(cfg)
	if err != nil {
		return nil, err
	}
	var opts []sdkclient.ClientOption
	if log != nil {
		opts = append(opts, sdkclient.WithLogger(log))
	}
	hc := sdkclient.NewHttpClient(sc, opts...)

	// The quote client needs its own HttpClient: QuoteServerURL is substituted
	// for ServerURL inside NewQuoteHttpClient, and only there. Handing Quote()
	// the client above would post every market-data request to the trade
	// gateway and leave QuoteServerURL unread — a knob that looks live and is
	// not. NewQuoteHttpClient also zeroes TokenRefreshDuration, so this second
	// client starts no refresh goroutine of its own; it borrows the first
	// client's token storage instead, so a token, if one were ever set, cannot
	// differ between the two.
	quoteHC := newQuoteHttpClient(sc,
		append([]sdkclient.ClientOption{sdkclient.WithSharedTokenFrom(hc)}, opts...)...)
	return &Session{Config: cfg, SDK: sc, HTTP: hc, QuoteHTTP: quoteHC}, nil
}

// newQuoteHttpClient is sdkclient.NewQuoteHttpClient behind a variable, so a
// test can see the ClientConfig the quote client is built from. HttpClient
// keeps that config unexported and exports no accessor for it, so the endpoint
// quote traffic will actually use is otherwise invisible — which is how a knob
// that does nothing survives a change like this one.
var newQuoteHttpClient = sdkclient.NewQuoteHttpClient

// Close releases the HTTP clients' resources.
//
// Both are closed, and they are two closes of two objects rather than two
// closes of one: NewQuoteHttpClient builds a fresh client, so the trade
// gateway's Close says nothing about the quote gateway's. Neither client owns a
// token-refresh goroutine today (TokenRefreshDuration is 0, and
// NewQuoteHttpClient zeroes it again), which makes the second close a no-op —
// but a quote client nobody closes is a leak the day that stops being true.
// The guards keep a second call, and a nil or half-built session, safe: the
// commands defer Close and also call it on some paths.
func (s *Session) Close() {
	if s == nil {
		return
	}
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	if s.QuoteHTTP != nil {
		s.QuoteHTTP.Close()
	}
}

// Quote returns a market-data client bound to QuoteServerURL.
func (s *Session) Quote() *sdkquote.QuoteClient {
	return sdkquote.NewQuoteClient(s.QuoteHTTP)
}

// Trade returns a trading client bound to the configured account.
func (s *Session) Trade() *sdktrade.TradeClient {
	return sdktrade.NewTradeClient(s.HTTP, s.Config.Account)
}

// PushOptions configures the push client.
type PushOptions struct {
	// Log receives connection and protocol diagnostics.
	Log *logging.Logger
	// HeartbeatInterval defaults to 30s when zero.
	HeartbeatInterval time.Duration
	// ReconnectInterval defaults to 10s when zero.
	ReconnectInterval time.Duration
	// ConnectTimeout defaults to 15s when zero.
	ConnectTimeout time.Duration
}

// Push builds a push client for the configured account and push URL.
func Push(cfg *config.Config, opts PushOptions) (*sdkpush.PushClient, error) {
	sc, err := NewClientConfig(cfg)
	if err != nil {
		return nil, err
	}
	var popts []sdkpush.PushClientOption
	if cfg.PushURL != "" {
		popts = append(popts, sdkpush.WithPushURL(cfg.PushURL))
	}
	if opts.HeartbeatInterval > 0 {
		popts = append(popts, sdkpush.WithHeartbeatInterval(opts.HeartbeatInterval))
	}
	if opts.ReconnectInterval > 0 {
		popts = append(popts, sdkpush.WithReconnectInterval(opts.ReconnectInterval))
	}
	if opts.ConnectTimeout > 0 {
		popts = append(popts, sdkpush.WithConnectTimeout(opts.ConnectTimeout))
	}
	return sdkpush.NewPushClient(sc, popts...), nil
}

// sdkTokenFileName mirrors the SDK's unexported config.defaultTokenFileName
// (config/token_manager.go:16). It has to be repeated here: sdkconfig reads the
// file on its own, with no exported name that exposes the path.
const sdkTokenFileName = "tiger_openapi_token.properties"

// sdkTokenFileEnv is the SDK's env var for "read the token from this file
// instead" (config/client_config.go:272). It reaches the same field as the token
// file above, so it is neutralised by the same clearing — but it escapes both
// directory scans, since it can name a path anywhere on disk.
const sdkTokenFileEnv = "TIGEROPEN_TOKEN_FILE"

// WarnStrayProperties prints a warning for each of the files the SDK discovers
// on its own: the config and token files in dir, the config file in the user's
// home directory, and the file $TIGEROPEN_TOKEN_FILE points at. We neutralise
// them, but a user who edited one deserves to know why it has no effect.
//
// The bare env vars ($TIGEROPEN_TOKEN, $TIGEROPEN_TIGER_ID and friends) stay
// silent, as designed: a value the user exported is already visible to them in
// their own shell, and warnStrayFile prints a path, so it could never be handed
// a token value. $TIGEROPEN_TOKEN_FILE is the exception because it is
// file-shaped — the file it names is something they may not know exists, and it
// can sit outside both directories the scans above cover.
func WarnStrayProperties(dir string, w io.Writer) {
	// The files are discovered by separate mechanisms — the config files by
	// sdkconfig's auto-discovery list, the token file by the TokenManager it
	// wires up for the bearer token — so a user who removes one can still be
	// redirected by the other, and each is named on its own. The home-directory
	// copy is found from any working directory, so the caller's dir cannot
	// cover it and it is looked up separately.
	if p, found := config.WarnIfStrayPropertiesFile(dir); found {
		warnStrayFile(w, p)
	}
	if p, found := config.WarnIfStrayHomePropertiesFile(); found {
		warnStrayFile(w, p)
	}
	if p := filepath.Join(dir, sdkTokenFileName); fileExists(p) {
		warnStrayFile(w, p)
	}
	// $TIGEROPEN_TOKEN_FILE is the one discovered input that is file-shaped but
	// escapes both directory scans above: it can name a path anywhere on disk, so
	// "we tell you about files" is otherwise an untrue implication. It goes last
	// because it is also the least likely — a user has to have exported it
	// deliberately, where the three files above are things someone left lying
	// around. Its payload is the highest-consequence of the four (the bearer
	// token), so being last in the output costs nothing.
	//
	// The raw value is what gets named, not the trimmed one: the SDK passes the
	// raw os.Getenv result straight to the token manager, so the raw string is
	// what it would actually open and the only one the user can recognise.
	if p := os.Getenv(sdkTokenFileEnv); strings.TrimSpace(p) != "" && fileExists(p) {
		warnStrayFile(w, p)
	}
}

func warnStrayFile(w io.Writer, p string) {
	fmt.Fprintf(w, "warning: found %s\n"+
		"         It is being IGNORED — this project loads credentials from env/YAML only,\n"+
		"         so that a stray file can never redirect your orders. Delete it.\n", p)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// DescribeError renders a Tiger SDK error with its category, if it is one.
func DescribeError(err error) string {
	if err == nil {
		return "<nil>"
	}
	var te *sdkclient.TigerError
	if errors.As(err, &te) {
		return fmt.Sprintf("tiger API error: %s (code %d, category %s)", te.Message, te.Code, te.Category)
	}
	return err.Error()
}
