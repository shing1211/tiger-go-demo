// Package tigersdk builds SDK clients from our validated configuration.
//
// It exists to solve one specific upstream hazard: the Tiger SDK auto-discovers
// ./tiger_openapi_config.properties (and ~/.tigeropen/...) and will silently
// override credentials you passed explicitly. Every ClientConfig we hand out is
// re-asserted after construction so that a stray properties file can never
// change which account is being traded.
package tigersdk

import (
	"errors"
	"fmt"
	"os"
	"time"

	sdkclient "github.com/tigerfintech/openapi-go-sdk/client"
	sdkconfig "github.com/tigerfintech/openapi-go-sdk/config"
	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"
	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"
	sdktrade "github.com/tigerfintech/openapi-go-sdk/trade"

	"github.com/tchan/tiger-go-demo/internal/config"
	"github.com/tchan/tiger-go-demo/internal/logging"
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
		sdkconfig.WithQuoteServerURL(cfg.QuoteServerURL),
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
	sc.QuoteServerURL = cfg.QuoteServerURL
	if cfg.DeviceID != "" {
		sc.DeviceID = cfg.DeviceID
	}
	return sc, nil
}

// Session bundles the HTTP client shared by the quote and trade clients.
type Session struct {
	Config *config.Config
	SDK    *sdkconfig.ClientConfig
	HTTP   *sdkclient.HttpClient
}

// NewSession builds the SDK config and HTTP client.
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
	return &Session{Config: cfg, SDK: sc, HTTP: hc}, nil
}

// Close releases the HTTP client's resources.
func (s *Session) Close() {
	if s != nil && s.HTTP != nil {
		s.HTTP.Close()
	}
}

// Quote returns a market-data client.
func (s *Session) Quote() *sdkquote.QuoteClient {
	return sdkquote.NewQuoteClient(s.HTTP)
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

// WarnStrayProperties prints a warning if the SDK's auto-discovery file is
// present in dir. We neutralise it, but a user who edited it deserves to know
// why it has no effect.
func WarnStrayProperties(dir string, w *os.File) {
	p, found := config.WarnIfStrayPropertiesFile(dir)
	if !found {
		return
	}
	fmt.Fprintf(w, "warning: found %s\n"+
		"         It is being IGNORED — this project loads credentials from env/YAML only,\n"+
		"         so that a stray file can never redirect your orders. Delete it.\n", p)
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
