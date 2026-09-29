// Command token reports on, and deliberately overrides, the bearer token the
// SDK holds in memory, and prints the SDK's own local record of which
// account-level push subjects this process has subscribed to.
//
// It is read-only with respect to the account. It authenticates and it reports;
// it never places, modifies or cancels an order, and it does not import the
// SDK's trade package at all. The classification is asserted, not asserted-in-
// prose: test/readonly_test.go walks every directory under cmd/ and fails if a
// read-only one names the trade package or any order method.
//
// # -set is the sharp edge, and it is deliberate
//
// internal/tigersdk.NewClientConfig clears ClientConfig.Token after the SDK has
// built it, because NewHttpClient copies that field into the Authorization
// header of every request. That clearing is what stops a
// ./tiger_openapi_token.properties this project never wrote — or a value from
// $TIGEROPEN_TOKEN or $TIGEROPEN_TOKEN_FILE — from authenticating the whole
// session as somebody else's account.
//
// That clearing is UNCHANGED, and it still runs for every command in this
// project, this one included. -set does not undo it, and does not route around
// it. -set is a separate input, applied after construction, to a different
// field (the client's atomic token rather than the shared ClientConfig.Token),
// only in this one process, and only when a human typed it on the command line.
// No file this project did not write can reach it, and WarnStrayProperties still
// reports every such file as IGNORED — the file defence and -set are not in
// tension, because they read different fields.
//
// What -set does do is let one operator, on purpose, put a token they already
// hold into one process. The cost is stated on stderr before the value is used:
// the value came from a command line, so it is in that user's shell history and
// readable by any other user on the machine through `ps`. The project does not
// pretend otherwise, and it never prints the value — only its length, through
// the same config.Redact helper that renders the private key and the app secret.
//
// # -set and -refresh together: -set first, then -refresh
//
// The order is fixed and it is not arbitrary. A token refresh is a request
// authenticated with the token currently in hand, so -set seeds that token and
// -refresh then rotates it: the value from -set is the bearer the refresh
// request is signed and sent with, and the token left in memory afterwards is
// the one the gateway issued. The reverse order would make -set a no-op, because
// the refresh would overwrite it immediately.
//
// The consequence, stated plainly because it is the kind of thing that reads as
// a bug when you hit it: the value you passed to -set is NOT the token in effect
// when the process exits. The output says so at the point where it stops being
// true.
//
// # -refresh is the only flag here that makes a network call
//
// It asks the real gateway for a new token, and this project has never made a
// request with valid credentials, so its behaviour is unverified beyond "it
// compiles, it is wired, and the error path is tested against a fake". It is
// called with a nil token manager on purpose: that is what keeps the new token
// in memory. Passing a manager would write it to a file, which is the one thing
// this project refuses to do behind the operator's back.
//
// The SDK's own docstring warns that a refresh updates only the client it was
// called on and not the shared config field, so a token obtained this way is
// gone when the process exits. There is nothing to persist it to.
//
// # -show reports LOCAL state, and the output says so on every run
//
// The push client keeps its own in-memory map of account-level subjects. The
// getter takes a read lock over that map and returns its keys. It opens no
// connection, sends nothing, and knows nothing about any other process.
//
// So -show cannot tell you what the server thinks you are subscribed to, and its
// output is written to make that impossible to miss: the banner says LOCAL and
// says the server was not asked, before any list is printed. An empty list
// printed with no context is exactly the output that reads like "the account has
// no subscriptions", which is the one wrong conclusion available here.
//
// This command makes no subscribe call and never connects, so the list is empty
// by construction and the output says that too. cmd/push is where subscriptions
// are actually made.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	sdkclient "github.com/tigerfintech/openapi-go-sdk/client"
	sdkconfig "github.com/tigerfintech/openapi-go-sdk/config"
	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
	"github.com/shing1211/tiger-go-demo/internal/tigersdk"
)

// tokenClient is the subset of the SDK's HTTP client this command uses, kept as
// an interface so a fake can stand in for it. Same reasoning as cmd/push's
// pushClient: the set of SDK methods this command may call is a small, reviewable
// list, and it should be a list you can read at the type rather than discover
// from a call site.
//
// The trade client and the order methods are absent from this file entirely, so
// a write would need a new import and a new line here, and
// TestPackageCannotReachOrderWrites checks that neither exists.
type tokenClient interface {
	// RefreshToken is the SDK's token rotation. The *config.TokenManager
	// parameter is a concrete SDK type, not an interface, which looks like it
	// makes this seam untestable. It does not: this command only ever passes
	// the untyped nil literal, so a fake never has to construct one. The test
	// asserts the argument really arrives nil, which is the property that
	// matters — nil is what keeps the new token out of any file.
	RefreshToken(*sdkconfig.TokenManager) error
	// SetCurrentToken is the SDK's own escape hatch for supplying a token from
	// outside, which is exactly what -set is.
	SetCurrentToken(string)
}

var _ tokenClient = (*sdkclient.HttpClient)(nil)

// subClient is the subset of the SDK's push client this command uses.
type subClient interface {
	GetAccountSubscriptions() []sdkpush.SubjectType
}

var _ subClient = (*sdkpush.PushClient)(nil)

// options is the command line, already parsed.
type options struct {
	setToken   string
	refresh    bool
	show       bool
	configPath string
	verbose    bool

	// setGiven distinguishes "-set was not passed" from "-set was passed with
	// an empty value". The two are not the same thing and must not be folded
	// together: the first means the token in memory is whatever the session
	// starts with, and the second means the operator asked for something
	// specific and got nothing, which is a mistake worth refusing (see
	// resolve).
	setGiven bool
}

// resolve validates what can be decided from the command line alone, which is
// everything this command does before it touches a credential. It runs before
// the config is loaded so a typo reads as a typo and not as a missing
// credential.
//
// given is the set of flag names the user actually passed, from flag.Visit. The
// default for -set is the empty string, and "empty because unset" and "empty
// because the shell expanded an unset variable" are different bugs.
func (o *options) resolve(given map[string]bool) error {
	o.setGiven = given["set"]
	// A token read with $(cat file) or a copy-paste routinely carries a trailing
	// newline, and a token with a stray byte in it authenticates as the wrong
	// thing and fails in a way that looks like a server problem.
	o.setToken = strings.TrimSpace(o.setToken)

	switch {
	case o.setGiven && o.setToken == "":
		return errors.New("-set was given an empty value. If you meant to pass the\n" +
			"contents of an environment variable that is unset, the shell expanded it to\n" +
			"nothing — check it before running this. An empty token is refused rather\n" +
			"than applied, because applying it would silently clear the token instead of\n" +
			"setting one.")
	case o.setGiven && looksLikePrivateKey(o.setToken):
		// The worst available outcome in this command is a private key on a
		// command line, and the most likely way to get there is a copy-paste of
		// the wrong credential. The message names the KIND of value and never
		// the value itself, so the refusal does not itself put it in a log.
		return errors.New("-set looks like an RSA private key, not a bearer token.\n" +
			"A private key on a command line is in your shell history and readable by\n" +
			"any other user on this machine via `ps`. Refused, and the value has not\n" +
			"been printed anywhere.")
	case !o.setGiven && !o.refresh && !o.show:
		return errors.New("nothing to do: pass at least one of -refresh, -set or -show.\n" +
			"  -refresh   fetch a new token from the gateway, in memory only\n" +
			"  -set       put a token you already hold into this process, in memory only\n" +
			"  -show      print this process's LOCAL account-subscription record\n" +
			"The combinations are documented in the -h output.")
	}
	return nil
}

// looksLikePrivateKey reports whether a value is an RSA private key rather than
// a bearer token. A bearer token is opaque and this project cannot validate one;
// this is the one shape it can recognise, and it is the one that must never
// reach a command line.
func looksLikePrivateKey(v string) bool {
	return strings.Contains(v, "PRIVATE KEY")
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		if config.MissingCredentialErrorIs(err) {
			os.Exit(2) // configuration problem
		}
		os.Exit(1) // runtime problem, including a usage error
	}
}

// newFlagSet declares every flag, with its default, and binds it to o.
//
// It is a separate function so the defaults can be read by a test without
// credentials and without the rest of run. The defaults are behaviour: all
// three actions default to off, and that is the only reason a bare invocation
// can be a usage error rather than a silent no-op.
func newFlagSet(o *options, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.StringVar(&o.setToken, "set", "", "use this bearer token for this process, in memory only (never printed; write -set=VALUE if it starts with '-')")
	fs.BoolVar(&o.refresh, "refresh", false, "fetch a new bearer token from the gateway and use it in memory only (no file is written)")
	fs.BoolVar(&o.show, "show", false, "print this process's LOCAL account-subscription record (the server is not asked)")
	fs.StringVar(&o.configPath, "config", "", "path to a YAML config file (also $TIGER_CONFIG)")
	fs.BoolVar(&o.verbose, "v", false, "verbose logging")

	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	return fs
}

func run(args []string, stdout, stderr io.Writer) error {
	var o options
	fs := newFlagSet(&o, stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // -h is a success, and needs no credentials
		}
		return err
	}
	if extra := fs.Args(); len(extra) > 0 {
		return fmt.Errorf("unexpected argument %q (this command takes flags only)", extra[0])
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if err := o.resolve(given); err != nil {
		return err
	}

	// Before the config is loaded, so the warning is seen even by a run that
	// then fails for an unrelated reason — the value is already in the user's
	// history whether or not this process gets as far as using it.
	if o.setGiven {
		fmt.Fprint(stderr, setWarning)
	}

	logging.SilenceSDKNoise()
	log := logging.NewFromConfig(stderr, level(o.configPath, o.verbose))

	cfg, err := config.Load(config.Options{ConfigPath: o.configPath})
	if err != nil {
		return err
	}
	tigersdk.WarnStrayProperties(".", stderr)
	log.Info("config loaded:\n%s", cfg.Redacted())

	// Both clients are built up front, before any action is dispatched, so that
	// a credential problem surfaces before a token is put anywhere and the
	// order of the steps below is not a function of what is needed when.
	session, err := tigersdk.NewSession(cfg, log)
	if err != nil {
		return err
	}
	defer session.Close()

	// A push client that never connects holds no connection, so there is
	// nothing to release here — which is also why the empty subscription list
	// -show prints is empty by construction rather than by accident.
	push, err := tigersdk.Push(cfg, tigersdk.PushOptions{Log: log})
	if err != nil {
		return err
	}

	// The quote client shares the HTTP client's token storage, so a token set
	// here cannot diverge between the two. Nothing in this command uses the
	// quote client; the note is here because the next person to add a flag
	// should not have to re-derive it.
	return apply(&o, tokenClient(session.HTTP), subClient(push), stdout)
}

// apply runs the requested actions in the fixed order documented on the package
// comment: -set first, then -refresh, then -show. The order is data, not a
// sequence of statements in a branch, so a test can assert it.
func apply(o *options, hc tokenClient, sc subClient, stdout io.Writer) error {
	if o.setGiven {
		hc.SetCurrentToken(o.setToken)
		reportSet(stdout, o.setToken)
	}
	if o.refresh {
		// nil is the whole point of this call: with a nil token manager the
		// SDK updates the client's in-memory token and writes nothing to disk.
		// Anything else would persist a credential behind the operator's back,
		// which is the thing this project does not do anywhere else.
		if err := hc.RefreshToken(nil); err != nil {
			// The SDK only stores the new token once the request has succeeded,
			// so a failure here leaves whatever was in memory untouched. Say so
			// rather than letting a failed refresh look like a cleared token.
			reportRefreshFailed(stdout, o, err)
			return fmt.Errorf("refresh token: %w", err)
		}
		reportRefreshed(stdout, o)
	}
	if o.show {
		reportSubscriptions(stdout, sc.GetAccountSubscriptions())
	}
	return nil
}

// level resolves the log level, so a config problem is still reported at the
// level the user asked for.
func level(configPath string, verbose bool) string {
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

const setWarning = `warning: -set puts a bearer token into this process's memory, taken from the command line.
         This is the one place in this project where the token-file defence is
         deliberately bypassed, and it is bypassed only for this process: no
         file this project did not write can supply a token to any command,
         this one included, and ./tiger_openapi_token.properties is still
         reported as ignored.
         The value came from a command line, so it is in your shell history and
         readable by any other user on this machine through 'ps'. Only its
         length is ever printed; the value is not.
`

const usage = `token — Tiger OpenAPI bearer token, and local push subscription state (read-only)

Reports and deliberately overrides the bearer token the SDK holds in memory, and
prints the SDK's own record of which account-level push subjects THIS process has
subscribed to. It never places, modifies or cancels an order.

Combinations:
  -refresh                 ask the gateway for a new token; in memory only
  -set TOKEN               use a token you already hold; in memory only
  -refresh -show           refresh, then print the local subscription record
  -set TOKEN -refresh      -set first: your token is the bearer the refresh
                           request is sent with, and the token left in memory is
                           the one the gateway issued. What you passed to -set is
                           NOT the token in effect when the process exits.
  -set TOKEN -show         set, then print the local subscription record
  all three                both of the above, in that order
  (no action flag)         a usage error, not a silent no-op

What is not written anywhere:
  no token is ever written to a file. The refresh is called with a nil token
  manager for exactly that reason, and the token this process ends up holding is
  gone when it exits.

What is never printed:
  a token value, in full or in part. Only its length, through the same redaction
  the private key and the app secret get.

-local, always:
  -show reads the push client's own in-memory map. It opens no connection and
  sends nothing, so the server is not asked. This command never subscribes and
  never connects, so the list is empty by construction; cmd/push is where
  subscriptions are actually made.

-traps worth knowing:
  -set with an empty value is refused rather than applied, because applying it
  would clear the token instead of setting one
  -set with a value that looks like an RSA private key is refused
  if your token starts with '-', write -set=TOKEN so the shell hands it over
  -set puts the value in your shell history and in 'ps' output for every user
  on this machine

No credentials are needed for -h.

Examples:
  go run ./cmd/token -show
  go run ./cmd/token -refresh
  go run ./cmd/token -set "$TOKEN" -refresh
  go run ./cmd/token -set="$TOKEN" -refresh -show

Flags:
`
