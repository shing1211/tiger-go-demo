// Package rocli holds the plumbing shared by the read-only Tiger commands
// (options, futures, reference, corporate).
//
// Every one of those commands is a thin table of SDK calls over one shared
// quote client. The only thing they have in common is how they get there:
// parse flags, load credentials, open a session, print something readable and
// map an error onto an exit code. That lives here so the commands themselves
// stay a readable list of endpoints.
//
// Nothing in this package can place an order. It exposes no trade client, and
// it never calls config.Writable: the read-only commands have no write path at
// all, which is the strongest form of the project's safety property.
package rocli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	sdkquote "github.com/tigerfintech/openapi-go-sdk/quote"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
	"github.com/shing1211/tiger-go-demo/internal/tigersdk"
)

// Common holds the flags every read-only command shares. Commands embed it and
// register it with AddCommonFlags, then add their own endpoint-specific flags.
type Common struct {
	// Market filters requests that take one, e.g. -market US.
	Market string
	// Lang is the Tiger response language (en_US, zh_CN, zh_TW).
	Lang string
	// SecType is the security type for reference-data lookups (STK, OPT, FUT).
	SecType string
	// Limit caps the number of rows a command prints or requests.
	Limit int
	// Page and PageSize drive the paged endpoints.
	Page     int
	PageSize int
	// Begin and End bound time ranges. They are epoch milliseconds; zero means
	// "let Tiger choose its default".
	Begin int64
	End   int64
	// ConfigPath points at a YAML config file (also $TIGER_CONFIG).
	ConfigPath string
	// Verbose turns on debug logging.
	Verbose bool
}

// AddCommonFlags registers the shared flags on fs.
func AddCommonFlags(fs flagSet, c *Common) {
	fs.StringVar(&c.Market, "market", "US", "market filter: US, HK, SG, AU")
	fs.StringVar(&c.Lang, "lang", "en_US", "response language: en_US, zh_CN, zh_TW")
	fs.StringVar(&c.SecType, "sec-type", "STK", "security type: STK, OPT, FUT")
	fs.IntVar(&c.Limit, "limit", 20, "max rows to print / request")
	fs.IntVar(&c.Page, "page", 0, "page number for paged endpoints (0 = unset)")
	fs.IntVar(&c.PageSize, "page-size", 0, "page size for paged endpoints (0 = unset)")
	fs.Int64Var(&c.Begin, "begin", 0, "range start, epoch milliseconds (0 = server default)")
	fs.Int64Var(&c.End, "end", 0, "range end, epoch milliseconds (0 = server default)")
	fs.StringVar(&c.ConfigPath, "config", "", "path to a YAML config file (also $TIGER_CONFIG)")
	fs.BoolVar(&c.Verbose, "v", false, "verbose (debug) logging")
}

// flagSet is the subset of *flag.FlagSet we use, so tests can pass a plain
// FlagSet and callers do not have to import flag for the type.
type flagSet interface {
	StringVar(p *string, name, value, usage string)
	IntVar(p *int, name string, value int, usage string)
	Int64Var(p *int64, name string, value int64, usage string)
	BoolVar(p *bool, name string, value bool, usage string)
}

// Env is an opened, read-only session: configuration, a quote client and the
// logger they share.
type Env struct {
	Config  *config.Config
	Session *tigersdk.Session
	Log     *logging.Logger
}

// Quote returns the market-data client. Read-only commands use only this.
func (e *Env) Quote() *sdkquote.QuoteClient { return e.Session.Quote() }

// Open loads credentials and builds the SDK session. It returns the actionable
// MissingCredentialError when TIGER_ID or TIGER_PRIVATE_KEY is absent — the
// same error every other command in this project reports.
//
// A trading account is deliberately NOT required: none of these endpoints read
// account state, and demanding one would block a legitimate read-only use.
func Open(configPath string, verbose bool, stderr io.Writer) (*Env, error) {
	logging.SilenceSDKNoise()
	log := logging.NewFromConfig(stderr, LogLevel(configPath, verbose))
	cfg, err := config.Load(config.Options{ConfigPath: configPath})
	if err != nil {
		return nil, err
	}
	tigersdk.WarnStrayProperties(".", stderr)
	log.Info("config loaded:\n%s", cfg.Redacted())
	session, err := tigersdk.NewSession(cfg, log)
	if err != nil {
		return nil, err
	}
	return &Env{Config: cfg, Session: session, Log: log}, nil
}

// Close releases the HTTP client's resources.
func (e *Env) Close() {
	if e != nil {
		e.Session.Close()
	}
}

// Context returns a context bounded by the shared command timeout.
func Context(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

// LogLevel resolves the log level before validation has run, so that a config
// problem is still reported at the level the user asked for.
func LogLevel(configPath string, verbose bool) string {
	if verbose {
		return "debug"
	}
	if p := config.ResolveConfigPath(configPath, os.Getenv); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if t := strings.TrimSpace(line); strings.HasPrefix(t, "log_level:") {
					return strings.TrimSpace(strings.TrimPrefix(t, "log_level:"))
				}
			}
		}
	}
	return "info"
}

// ExitCode maps an error onto this project's exit-code convention:
// 2 for missing credentials, 3 for a safety refusal, 1 for anything else.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case config.MissingCredentialErrorIs(err):
		return 2
	case config.IsSafetyError(err):
		return 3
	default:
		return 1
	}
}

// Main is the shared entry point: run, print any error, exit with ExitCode.
//
// -h is handled inside run (it returns nil), so help works with no credentials
// and exits 0.
func Main(run func() error) {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(ExitCode(err))
	}
}
