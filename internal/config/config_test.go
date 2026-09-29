package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envFrom(m map[string]string) Getenv {
	return func(k string) string { return m[k] }
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return p
}

func TestLoadMissingCredentialsReportsAllAtOnce(t *testing.T) {
	_, err := Load(Options{Getenv: envFrom(nil)})
	if err == nil {
		t.Fatal("want error with no credentials, got nil")
	}
	if !MissingCredentialErrorIs(err) {
		t.Fatalf("want MissingCredentialError, got %T", err)
	}
	msg := err.Error()
	// Both required credentials must be named, not just the first one found.
	for _, want := range []string{EnvTigerID, EnvPrivateKey} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message does not mention %s:\n%s", want, msg)
		}
	}
	// It must point at the developer portal.
	if !strings.Contains(msg, PortalURL) {
		t.Errorf("error message does not link the portal:\n%s", msg)
	}
	// It must warn that a simulated account will not work.
	if !strings.Contains(msg, "SIMULATED") {
		t.Errorf("error message does not warn about simulated accounts:\n%s", msg)
	}
}

func TestLoadValidateForTradingRequiresAccount(t *testing.T) {
	env := map[string]string{
		EnvTigerID:    "12345",
		EnvPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----",
	}
	cfg, err := Load(Options{Getenv: envFrom(env)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate should pass without an account: %v", err)
	}
	err = cfg.ValidateForTrading()
	if err == nil {
		t.Fatal("want error for missing account")
	}
	if !strings.Contains(err.Error(), EnvAccount) {
		t.Errorf("error should name %s, got: %s", EnvAccount, err)
	}
}

func TestDryRunDefaultsTrue(t *testing.T) {
	cfg, err := Load(Options{Getenv: envFrom(goodEnv())})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.DryRun {
		t.Fatal("dry_run must default to true")
	}
}

func goodEnv() map[string]string {
	return map[string]string{
		EnvTigerID:    "12345",
		EnvPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----",
		EnvAccount:    "DU1234567",
	}
}

func TestWritableGate(t *testing.T) {
	base := func(dryRun, confirmed bool) *Config {
		return &Config{DryRun: dryRun}
	}
	tests := []struct {
		name      string
		dryRun    bool
		confirmed bool
		wantErr   error
	}{
		{"dry run blocks even with confirm", true, true, &DryRunError{}},
		{"dry run blocks without confirm", true, false, &DryRunError{}},
		{"missing confirm blocks", false, false, &NotConfirmedError{}},
		{"both satisfied allows write", false, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := base(tt.dryRun, tt.confirmed).Writable(tt.confirmed)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want %T, got nil", tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) && !IsSafetyError(err) {
				t.Fatalf("want a safety error, got %T: %v", err, err)
			}
			// The refusal text must state that nothing was sent.
			if !strings.Contains(err.Error(), "No order was sent") {
				t.Errorf("refusal should say nothing was sent, got: %s", err)
			}
		})
	}
}

func TestRedactNeverLeaks(t *testing.T) {
	const secret = "-----BEGIN RSA PRIVATE KEY-----SUPERSECRET"
	r := Redact(secret)
	if strings.Contains(r, "SUPERSECRET") {
		t.Fatalf("Redact leaked the secret: %q", r)
	}
	if r == Redact("") {
		t.Fatal("unset and set must be distinguishable")
	}
}

func TestRedactedViewHidesSecrets(t *testing.T) {
	cfg, err := Load(Options{Getenv: envFrom(map[string]string{
		EnvTigerID:    "12345",
		EnvPrivateKey: "-----BEGIN RSA PRIVATE KEY-----TOPSECRET",
		EnvSecretKey:  "APPSECRETVALUE",
	})})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	view := cfg.Redacted()
	if strings.Contains(view, "TOPSECRET") || strings.Contains(view, "APPSECRETVALUE") {
		t.Fatalf("redacted view leaked a secret:\n%s", view)
	}
	// String() must be safe too, since it is reachable via fmt %v.
	if s := cfg.String(); strings.Contains(s, "TOPSECRET") {
		t.Fatalf("String() leaked the private key:\n%s", s)
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	p := writeYAML(t, "tiger_id: from-yaml\naccount: ACC-YAML\ndry_run: true\n")
	env := map[string]string{EnvTigerID: "from-env", EnvPrivateKey: "pk", EnvAccount: "ACC-ENV"}
	cfg, err := Load(Options{ConfigPath: p, Getenv: envFrom(env)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TigerID != "from-env" {
		t.Errorf("env should win: got %q", cfg.TigerID)
	}
	if cfg.Account != "ACC-ENV" {
		t.Errorf("env should win: got %q", cfg.Account)
	}
	if cfg.Source("tiger_id") != "env:"+EnvTigerID {
		t.Errorf("Source should report env, got %q", cfg.Source("tiger_id"))
	}
}

func TestYAMLOnly(t *testing.T) {
	p := writeYAML(t, "tiger_id: yaml-id\nprivate_key: yaml-pk\naccount: yaml-acc\nlicense: TBSG\n")
	cfg, err := Load(Options{ConfigPath: p, Getenv: envFrom(nil)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TigerID != "yaml-id" || cfg.License != "TBSG" {
		t.Errorf("yaml not applied: %+v", cfg.Redacted())
	}
}

func TestYAMLUnknownKeyIsAnError(t *testing.T) {
	p := writeYAML(t, "tiger_idd: typo\n")
	_, err := Load(Options{ConfigPath: p, Getenv: envFrom(nil)})
	if err == nil {
		t.Fatal("a typo'd YAML key must be an error, not silence")
	}
}

func TestMissingConfigFileIsAnError(t *testing.T) {
	_, err := Load(Options{ConfigPath: filepath.Join(t.TempDir(), "nope.yaml"), Getenv: envFrom(nil)})
	if err == nil {
		t.Fatal("an explicitly requested but missing config file must be an error")
	}
	if MissingCredentialErrorIs(err) {
		t.Errorf("should be a file error, not a credentials error: %v", err)
	}
}

func TestInvalidTimeoutRejected(t *testing.T) {
	env := goodEnv()
	env[EnvTimeout] = "not-a-duration"
	_, err := Load(Options{Getenv: envFrom(env)})
	if err == nil {
		t.Fatal("want error for bad timeout")
	}
	if !strings.Contains(err.Error(), EnvTimeout) {
		t.Errorf("error should name %s: %v", EnvTimeout, err)
	}
}

func TestInvalidDryRunRejected(t *testing.T) {
	env := goodEnv()
	env[EnvDryRun] = "maybe"
	_, err := Load(Options{Getenv: envFrom(env)})
	if err == nil {
		t.Fatal("want error for bad dry_run")
	}
}

func TestDryRunCanBeDisabled(t *testing.T) {
	env := goodEnv()
	env[EnvDryRun] = "false"
	cfg, err := Load(Options{Getenv: envFrom(env)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DryRun {
		t.Fatal("TIGER_DRY_RUN=false should disable dry run")
	}
	// ...but it must still require --confirm-live.
	if err := cfg.Writable(false); err == nil {
		t.Fatal("dry_run=false must not by itself permit a write")
	}
}

func TestPrivateKeyFromFile(t *testing.T) {
	dir := t.TempDir()
	kp := filepath.Join(dir, "tiger.pem")
	if err := os.WriteFile(kp, []byte("-----BEGIN RSA PRIVATE KEY-----\nFROMFILE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{EnvTigerID: "id", EnvPrivateKeyFil: kp}
	cfg, err := Load(Options{Getenv: envFrom(env)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !strings.Contains(cfg.PrivateKey, "FROMFILE") {
		t.Errorf("private key not read from file")
	}
	if cfg.Source("private_key") != "env:"+EnvPrivateKeyFil {
		t.Errorf("unexpected source %q", cfg.Source("private_key"))
	}
}

func TestQuoteServerDefaultsToServer(t *testing.T) {
	cfg, err := Load(Options{Getenv: envFrom(goodEnv())})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.QuoteServerURL != cfg.ServerURL {
		t.Errorf("quote server should default to the trade gateway, got %q", cfg.QuoteServerURL)
	}
}

func TestTimeoutDefault(t *testing.T) {
	cfg, err := Load(Options{Getenv: envFrom(goodEnv())})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Timeout != 15*time.Second {
		t.Errorf("default timeout = %v, want 15s", cfg.Timeout)
	}
}

func TestWarnIfStrayPropertiesFile(t *testing.T) {
	dir := t.TempDir()
	if _, found := WarnIfStrayPropertiesFile(dir); found {
		t.Error("no file should be reported in an empty dir")
	}
	if err := os.WriteFile(filepath.Join(dir, "tiger_openapi_config.properties"), []byte("tiger_id=x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found := WarnIfStrayPropertiesFile(dir); !found {
		t.Error("the stray properties file should be detected")
	}
}

// isolateHome points $HOME at an empty directory for the duration of the test,
// so a stray file in the developer's real home directory cannot make these
// assertions pass or fail. os.UserHomeDir reads $HOME on Unix and %USERPROFILE%
// on Windows; both are set.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestWarnIfStrayHomePropertiesFile(t *testing.T) {
	home := isolateHome(t)
	if _, found := WarnIfStrayHomePropertiesFile(); found {
		t.Error("no file should be reported in an empty home directory")
	}

	dir := filepath.Join(home, ".tigeropen")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, StrayPropertiesFileName)
	if err := os.WriteFile(p, []byte("tiger_id=x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, found := WarnIfStrayHomePropertiesFile()
	if !found {
		t.Fatal("the home properties file should be detected")
	}
	// The path has to be usable, not merely present: a warning naming a path
	// the user cannot delete is a warning they cannot act on.
	if got != p {
		t.Errorf("reported %q, want %q", got, p)
	}
}

func TestWarnIfStrayHomePropertiesFileMissingHome(t *testing.T) {
	isolateHome(t)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "no-such-home"))
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if p, found := WarnIfStrayHomePropertiesFile(); found {
		t.Errorf("a home directory that does not exist cannot hold the file, got %q", p)
	}

	// A system with no home directory at all is not an error either. os.UserHomeDir
	// fails when $HOME is empty or unset, and that must degrade to silence.
	t.Setenv("HOME", "")
	if p, found := WarnIfStrayHomePropertiesFile(); found {
		t.Errorf("no home directory cannot hold the file, got %q", p)
	}
}

// TestQuoteServerFallbackIsExplicit pins the default the README documents
// (quote_server_url = server_url) at the point it is applied, including for a
// Config assembled by hand rather than through Load.
func TestQuoteServerFallbackIsExplicit(t *testing.T) {
	p := writeYAML(t, "tiger_id: yaml-id\nprivate_key: pk\nserver_url: https://trade.example.invalid/gw\n")
	cfg, err := Load(Options{ConfigPath: p, Getenv: envFrom(nil)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.QuoteServerURL != cfg.ServerURL {
		t.Errorf("quote server should follow the trade gateway, got %q vs %q",
			cfg.QuoteServerURL, cfg.ServerURL)
	}

	// An explicit quote endpoint is not overwritten by the fallback.
	p = writeYAML(t, "tiger_id: yaml-id\nprivate_key: pk\nquote_server_url: https://quote.example.invalid/gw\n")
	cfg, err = Load(Options{ConfigPath: p, Getenv: envFrom(nil)})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.QuoteServerURL != "https://quote.example.invalid/gw" {
		t.Errorf("explicit quote server should be kept, got %q", cfg.QuoteServerURL)
	}
}
