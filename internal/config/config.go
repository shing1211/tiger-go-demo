// Package config loads, validates and redacts Tiger OpenAPI credentials.
//
// Precedence (highest wins): environment variables > YAML file > built-in defaults.
//
// The loader is deliberately strict: it never lets a value come from an
// implicit source, and it never returns a partially-valid Config. Callers get
// a single actionable error listing exactly which variables are missing and
// where in the Tiger developer portal to generate them.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultServerURL is the Tiger OpenAPI gateway. The SDK ships the same value;
// we set it explicitly so a stray ./tiger_openapi_config.properties in the
// working directory can never take effect (see ClientConfig in internal/tigersdk).
const DefaultServerURL = "https://openapi.tigerfintech.com/gateway"

// DefaultPushURL is the Tiger push server (TCP + TLS + Protobuf).
const DefaultPushURL = "openapi.tigerfintech.com:8887"

// EnvVar names. Every credential the SDK understands is listed here.
const (
	EnvTigerID       = "TIGER_ID"
	EnvPrivateKey    = "TIGER_PRIVATE_KEY"
	EnvPrivateKeyFil = "TIGER_PRIVATE_KEY_FILE"
	EnvAccount       = "TIGER_ACCOUNT"
	EnvSecretKey     = "TIGER_SECRET_KEY"
	EnvLicense       = "TIGER_LICENSE"
	EnvLanguage      = "TIGER_LANGUAGE"
	EnvTimezone      = "TIGER_TIMEZONE"
	EnvDeviceID      = "TIGER_DEVICE_ID"
	EnvServerURL     = "TIGER_SERVER_URL"
	EnvQuoteSrvURL   = "TIGER_QUOTE_SERVER_URL"
	EnvPushURL       = "TIGER_PUSH_URL"
	EnvTimeout       = "TIGER_TIMEOUT"
	EnvDryRun        = "TIGER_DRY_RUN"
	EnvLogLevel      = "TIGER_LOG_LEVEL"
	EnvConfigPath    = "TIGER_CONFIG"
)

// PortalURL is the Tiger OpenAPI developer portal where credentials are issued.
const PortalURL = "https://quant.itigerup.com/openapi/en/"

// License values that the SDK understands.
const (
	LicenseTBNZ = "TBNZ" // Tiger Brokers (New Zealand)
	LicenseTBSG = "TBSG" // Tiger Brokers (Singapore)
	LicenseTBAU = "TBAU" // Tiger Brokers (Australia)
	LicenseTBIH = "TBIH" // Tiger Brokers (Hong Kong)
	LicenseTBHK = "TBHK" // Tiger Brokers (Hong Kong) token-based
)

// Config is the fully-resolved, validated configuration for all three binaries.
//
// Secrets are stored here and must never be printed. Use String or Redacted to
// obtain a log-safe representation.
type Config struct {
	TigerID    string
	PrivateKey string
	Account    string
	SecretKey  string // App Secret; only used for institutional accounts.
	License    string
	Language   string
	Timezone   string
	DeviceID   string

	ServerURL      string
	QuoteServerURL string
	PushURL        string
	Timeout        time.Duration

	// DryRun is the master kill switch for every order write. It defaults to
	// true. When true, no write request is ever sent to Tiger.
	DryRun bool

	LogLevel string

	// sources records where each non-default value came from, for logging.
	sources map[string]string
}

// Source reports where a field's value came from ("env", "yaml:<path>" or "default").
func (c *Config) Source(field string) string {
	if s, ok := c.sources[field]; ok {
		return s
	}
	return "default"
}

// Getenv is the environment lookup function, injectable for tests.
type Getenv func(string) string

// Options controls Load.
type Options struct {
	// Getenv defaults to os.Getenv.
	Getenv Getenv
	// ConfigPath is the YAML file to read. Empty falls back to $TIGER_CONFIG.
	// A path that is set but unreadable is a hard error — silently ignoring
	// it would run against the wrong environment.
	ConfigPath string
}

// yamlConfig mirrors config.example.yaml. Pointers distinguish "absent" from
// "explicitly set to the zero value" (notably for dry_run).
type yamlConfig struct {
	TigerID    *string `yaml:"tiger_id"`
	PrivateKey *string `yaml:"private_key"`
	Account    *string `yaml:"account"`
	SecretKey  *string `yaml:"secret_key"`
	License    *string `yaml:"license"`
	Language   *string `yaml:"language"`
	Timezone   *string `yaml:"timezone"`
	DeviceID   *string `yaml:"device_id"`

	ServerURL      *string `yaml:"server_url"`
	QuoteServerURL *string `yaml:"quote_server_url"`
	PushURL        *string `yaml:"push_url"`
	Timeout        *string `yaml:"timeout"`
	DryRun         *bool   `yaml:"dry_run"`
	LogLevel       *string `yaml:"log_level"`
}

// MissingCredentialError reports every absent required credential at once,
// rather than failing on the first one.
type MissingCredentialError struct {
	Missing []string
	// HasAccount is false when a trading account is absent.
	HasAccount bool
}

func (e *MissingCredentialError) Error() string {
	var b strings.Builder
	b.WriteString("incomplete Tiger OpenAPI configuration\n\n")
	b.WriteString("Missing required setting(s):\n")
	for _, m := range e.Missing {
		b.WriteString("  - " + m + "\n")
	}
	b.WriteString("\nSet them in any of these ways:\n")
	b.WriteString("  export TIGER_ID=...            # or put them in a .env / shell profile\n")
	b.WriteString("  cp config.example.yaml config.local.yaml   # then edit it\n")
	b.WriteString("  go run ./cmd/quote --config config.local.yaml\n\n")
	b.WriteString("Where each credential comes from (" + PortalURL + "):\n")
	b.WriteString("  TIGER_ID            Developer ID (App Key)   -> portal: My OpenAPI / App management\n")
	b.WriteString("  TIGER_PRIVATE_KEY   RSA private key (PEM)    -> portal: App management -> Download private key\n")
	b.WriteString("  TIGER_ACCOUNT       Funded trading account   -> your Tiger statement / portal account page\n")
	b.WriteString("  TIGER_SECRET_KEY    App Secret, institutional accounts only -> portal: App management\n\n")
	b.WriteString("Note: a Tiger SIMULATED account will NOT authenticate against OpenAPI.\n")
	b.WriteString("You need a real, funded account with OpenAPI access enabled.\n")
	return b.String()
}

// MissingCredentialError reports whether err is a configuration error.
func MissingCredentialErrorIs(err error) bool {
	var e *MissingCredentialError
	return errors.As(err, &e)
}

// Load resolves configuration from the environment and an optional YAML file.
//
// It returns *MissingCredentialError when tiger_id or private_key (or, when
// requireAccount is set, account) is absent.
func Load(opts Options) (*Config, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	cfg := &Config{
		License:   LicenseTBNZ,
		Language:  "en_US",
		Timezone:  "US/Eastern",
		ServerURL: DefaultServerURL,
		PushURL:   DefaultPushURL,
		Timeout:   15 * time.Second,
		DryRun:    true, // SAFE DEFAULT. Never flip this without a very good reason.
		LogLevel:  "info",
		sources:   map[string]string{},
	}

	// 1. YAML file (lowest precedence above defaults).
	path := opts.ConfigPath
	if path == "" {
		path = getenv(EnvConfigPath)
	}
	if path != "" {
		if err := cfg.applyYAML(path); err != nil {
			return nil, err
		}
	}

	// 2. Environment variables (highest precedence).
	type envBinding struct {
		env   string
		field string
		set   func(string)
	}
	bindings := []envBinding{
		{EnvTigerID, "tiger_id", func(v string) { cfg.TigerID = v }},
		{EnvPrivateKey, "private_key", func(v string) { cfg.PrivateKey = v }},
		{EnvAccount, "account", func(v string) { cfg.Account = v }},
		{EnvSecretKey, "secret_key", func(v string) { cfg.SecretKey = v }},
		{EnvLicense, "license", func(v string) { cfg.License = v }},
		{EnvLanguage, "language", func(v string) { cfg.Language = v }},
		{EnvTimezone, "timezone", func(v string) { cfg.Timezone = v }},
		{EnvDeviceID, "device_id", func(v string) { cfg.DeviceID = v }},
		{EnvServerURL, "server_url", func(v string) { cfg.ServerURL = v }},
		{EnvQuoteSrvURL, "quote_server_url", func(v string) { cfg.QuoteServerURL = v }},
		{EnvPushURL, "push_url", func(v string) { cfg.PushURL = v }},
		{EnvLogLevel, "log_level", func(v string) { cfg.LogLevel = v }},
	}
	for _, b := range bindings {
		if v := strings.TrimSpace(getenv(b.env)); v != "" {
			b.set(v)
			cfg.sources[b.field] = "env:" + b.env
		}
	}

	// Private key from a file, so the secret never sits in a shell history.
	if cfg.PrivateKey == "" {
		if f := strings.TrimSpace(getenv(EnvPrivateKeyFil)); f != "" {
			raw, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("read %s (%s): %w", EnvPrivateKeyFil, f, err)
			}
			cfg.PrivateKey = strings.TrimSpace(string(raw))
			cfg.sources["private_key"] = "env:" + EnvPrivateKeyFil
		}
	}

	if v := strings.TrimSpace(getenv(EnvTimeout)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s=%q: %w", EnvTimeout, v, err)
		}
		cfg.Timeout = d
		cfg.sources["timeout"] = "env:" + EnvTimeout
	}

	if v := strings.TrimSpace(getenv(EnvDryRun)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s=%q: want true/false/1/0", EnvDryRun, v)
		}
		cfg.DryRun = b
		cfg.sources["dry_run"] = "env:" + EnvDryRun
	}

	// Quote server defaults to the trade gateway when unset.
	if cfg.QuoteServerURL == "" {
		cfg.QuoteServerURL = cfg.ServerURL
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyYAML merges the YAML file into cfg, recording the source of each value.
func (c *Config) applyYAML(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	var yc yamlConfig
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true) // typo in a YAML key is an error, not silence
	if err := dec.Decode(&yc); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	src := "yaml:" + path

	str := func(p *string, dst *string, field string) {
		if p != nil && strings.TrimSpace(*p) != "" {
			*dst = strings.TrimSpace(*p)
			c.sources[field] = src
		}
	}
	str(yc.TigerID, &c.TigerID, "tiger_id")
	str(yc.PrivateKey, &c.PrivateKey, "private_key")
	str(yc.Account, &c.Account, "account")
	str(yc.SecretKey, &c.SecretKey, "secret_key")
	str(yc.License, &c.License, "license")
	str(yc.Language, &c.Language, "language")
	str(yc.Timezone, &c.Timezone, "timezone")
	str(yc.DeviceID, &c.DeviceID, "device_id")
	str(yc.ServerURL, &c.ServerURL, "server_url")
	str(yc.QuoteServerURL, &c.QuoteServerURL, "quote_server_url")
	str(yc.PushURL, &c.PushURL, "push_url")
	str(yc.LogLevel, &c.LogLevel, "log_level")

	if yc.Timeout != nil {
		d, err := time.ParseDuration(*yc.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout %q in %s: %w", *yc.Timeout, path, err)
		}
		c.Timeout = d
		c.sources["timeout"] = src
	}
	if yc.DryRun != nil {
		c.DryRun = *yc.DryRun
		c.sources["dry_run"] = src
	}
	return nil
}

// Validate reports every missing required credential at once.
func (c *Config) Validate() error {
	var missing []string
	if c.TigerID == "" {
		missing = append(missing, EnvTigerID+"  (developer id / App Key)")
	}
	if c.PrivateKey == "" {
		missing = append(missing, EnvPrivateKey+" or "+EnvPrivateKeyFil+"  (RSA private key)")
	}
	if len(missing) > 0 {
		return &MissingCredentialError{Missing: missing, HasAccount: c.Account != ""}
	}
	return nil
}

// ValidateForTrading additionally requires a trading account.
func (c *Config) ValidateForTrading() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Account == "" {
		return &MissingCredentialError{
			Missing:    []string{EnvAccount + "  (trading account, required for order operations)"},
			HasAccount: false,
		}
	}
	return nil
}

// Redacted returns a log-safe view of the configuration. Secrets are never
// included, only whether they are present and how long they are.
func (c *Config) Redacted() string {
	fields := []struct {
		k, v string
	}{
		{"tiger_id", c.TigerID},
		{"account", c.Account},
		{"secret_key", Redact(c.SecretKey)},
		{"private_key", Redact(c.PrivateKey)},
		{"license", c.License},
		{"language", c.Language},
		{"timezone", c.Timezone},
		{"device_id", c.DeviceID},
		{"server_url", c.ServerURL},
		{"quote_server_url", c.QuoteServerURL},
		{"push_url", c.PushURL},
		{"timeout", c.Timeout.String()},
		{"dry_run", strconv.FormatBool(c.DryRun)},
		{"log_level", c.LogLevel},
	}
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  %-18s %s", f.k, f.v)
	}
	return b.String()
}

// Redact masks a secret, revealing only its length so that "unset" and
// "loaded" can be told apart in logs.
func Redact(s string) string {
	if s == "" {
		return "<unset>"
	}
	return fmt.Sprintf("<redacted:%d bytes>", len(s))
}

// String implements fmt.Stringer with the redacted view, so that an accidental
// fmt.Printf("%v", cfg) can never leak a key.
func (c *Config) String() string { return "tiger.Config{\n" + c.Redacted() + "\n}" }

// Writable reports whether a live write is permitted.
//
// Both conditions must hold: the config must have dry_run disabled, and the
// caller must have passed the explicit --confirm-live flag. This is the single
// choke point for order placement, modification and cancellation.
func (c *Config) Writable(confirmed bool) error {
	if c.DryRun {
		return &DryRunError{}
	}
	if !confirmed {
		return &NotConfirmedError{}
	}
	return nil
}

// DryRunError is returned when TIGER_DRY_RUN is still on.
type DryRunError struct{}

func (e *DryRunError) Error() string {
	return "REFUSED: dry run is enabled (TIGER_DRY_RUN=true, the default).\n" +
		"No order was sent to Tiger. Set TIGER_DRY_RUN=false AND pass --confirm-live to submit one."
}

// NotConfirmedError is returned when --confirm-live was not supplied.
type NotConfirmedError struct{}

func (e *NotConfirmedError) Error() string {
	return "REFUSED: --confirm-live was not supplied.\n" +
		"No order was sent to Tiger. Re-run with --confirm-live (and TIGER_DRY_RUN=false) to submit it."
}

// IsSafetyError reports whether err is one of the two write-path refusals, so
// callers can exit with a distinct status.
func IsSafetyError(err error) bool {
	var d *DryRunError
	var n *NotConfirmedError
	return errors.As(err, &d) || errors.As(err, &n)
}

// ResolveConfigPath returns the YAML path in effect for flags/env, for help text.
func ResolveConfigPath(flagValue string, getenv Getenv) string {
	if flagValue != "" {
		return flagValue
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	return getenv(EnvConfigPath)
}

// StrayPropertiesFileName is the file the SDK auto-discovers for credentials.
const StrayPropertiesFileName = "tiger_openapi_config.properties"

// sdkHomeDir is the SDK's unexported config home directory
// (config/client_config.go:229). It is repeated here because the SDK exposes no
// name for it, and warning about a file we cannot name is not worth much.
const sdkHomeDir = ".tigeropen"

// WarnIfStrayPropertiesFile reports a tiger_openapi_config.properties in the
// working directory. The SDK would normally auto-discover it and silently
// override credentials; internal/tigersdk defends against that, and this helper
// tells the user why their edits appear to be ignored.
func WarnIfStrayPropertiesFile(dir string) (string, bool) {
	p := filepath.Join(dir, StrayPropertiesFileName)
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

// WarnIfStrayHomePropertiesFile reports a tiger_openapi_config.properties in
// $HOME/.tigeropen, the second location the SDK auto-discovers and one that
// does not depend on the working directory at all. A run from any directory
// picks it up, which makes it the more confusing of the two to discover.
//
// A system with no home directory is not an error: the file cannot exist there,
// and failing a command over a warning is the wrong trade.
func WarnIfStrayHomePropertiesFile() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return WarnIfStrayPropertiesFile(filepath.Join(home, sdkHomeDir))
}
