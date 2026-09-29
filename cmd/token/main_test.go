package main

import (
	"bytes"
	"errors"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"strings"
	"testing"

	sdkconfig "github.com/tigerfintech/openapi-go-sdk/config"
	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"
)

// sentinel is a value no real credential could be, so "the token was printed"
// is a substring check rather than a judgement call. Every output test runs a
// -set through with this value and asserts it never reaches either stream.
const sentinel = "SENTINEL-TOKEN-a1b2c3d4"

// fakeTokenClient records the calls made against it, in order.
//
// The refresh argument is recorded rather than ignored: passing nil is the whole
// reason -refresh does not write a file, and a fake that swallowed the argument
// would let that property rot without any test noticing.
type fakeTokenClient struct {
	calls []string
	// sawManager records whether RefreshToken received a non-nil token manager,
	// which would mean the token was being written to a file.
	sawManager bool
	setValue   string
	refreshErr error
}

func (f *fakeTokenClient) RefreshToken(m *sdkconfig.TokenManager) error {
	f.calls = append(f.calls, "RefreshToken")
	if m != nil {
		f.sawManager = true
	}
	return f.refreshErr
}

func (f *fakeTokenClient) SetCurrentToken(token string) {
	f.calls = append(f.calls, "SetCurrentToken")
	f.setValue = token
}

// fakeSubClient returns whatever the test hands it. The real getter returns the
// keys of a map in a randomised order, so the tests below also pass a reversed
// slice to prove the renderer sorts rather than echoing what it was given.
type fakeSubClient struct {
	subs  []sdkpush.SubjectType
	calls int
}

func (f *fakeSubClient) GetAccountSubscriptions() []sdkpush.SubjectType {
	f.calls++
	return f.subs
}

// parse runs the real flag set and resolve() without building a session, so a
// validation test costs nothing and needs no credentials.
//
// It is split out from run() because run() deliberately builds real clients, and
// the thing under test here is what happens before that. The flag set itself is
// run()'s, not a copy, so a default cannot drift between the two.
func parse(t *testing.T, args ...string) (*options, error) {
	t.Helper()
	var o options
	fs := newFlagSet(&o, io.Discard)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if err := o.resolve(given); err != nil {
		return nil, err
	}
	return &o, nil
}

// ---- validation ----

// TestNoActionFlagIsAUsageError covers the "do nothing quietly" case. A bare
// invocation is the shape that hides a typo, and a silent success would be the
// worst possible reading of a command whose whole job is to be explicit.
func TestNoActionFlagIsAUsageError(t *testing.T) {
	o, err := parse(t)
	if err == nil {
		t.Fatal("a bare invocation should be an error, not a silent no-op")
	}
	if o != nil {
		t.Errorf("no options should be resolved on failure, got %+v", o)
	}
	for _, want := range []string{"-refresh", "-set", "-show"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should name %s as an option, got:\n%v", want, err)
		}
	}
}

// TestSetEmptyIsRefused is the trap the -set/--set distinction exists for. An
// unset shell variable expands to nothing, and SetCurrentToken("") would clear
// the token — the exact opposite of what the operator asked for, and silent.
func TestSetEmptyIsRefused(t *testing.T) {
	if _, err := parse(t, "-set", ""); err == nil {
		t.Fatal(`-set "" should be refused rather than applied`)
	} else if !strings.Contains(err.Error(), "-set") {
		t.Errorf("the error should name the flag, got:\n%v", err)
	}
}

// TestSetWhitespaceOnlyIsRefused covers the same mistake reached through a file:
// $(cat token.txt) on a file with a trailing blank line, or a paste of spaces.
func TestSetWhitespaceOnlyIsRefused(t *testing.T) {
	if _, err := parse(t, "-set", "  \n\t "); err == nil {
		t.Fatal("a whitespace-only -set should be refused")
	}
}

// TestSetTrimsWhitespace is the other half of that trap: a real token with a
// stray newline round it must be trimmed, because an untrimmed one
// authenticates as the wrong thing and fails in a way that looks like the
// server's fault.
func TestSetTrimsWhitespace(t *testing.T) {
	o, err := parse(t, "-set", "  "+sentinel+"\n")
	if err != nil {
		t.Fatalf("-set with surrounding whitespace should be accepted: %v", err)
	}
	if o.setToken != sentinel {
		t.Errorf("setToken = %q, want it trimmed to %q", o.setToken, sentinel)
	}
}

// TestSetPrivateKeyIsRefused keeps the worst available outcome off a command
// line. The error must also not quote the value, or the refusal would itself be
// the leak it exists to prevent.
func TestSetPrivateKeyIsRefused(t *testing.T) {
	key := "-----BEGIN RSA PRIVATE KEY-----\nMIIEow==\n-----END RSA PRIVATE KEY-----"
	_, err := parse(t, "-set", key)
	if err == nil {
		t.Fatal("a private key should be refused")
	}
	if !strings.Contains(err.Error(), "private key") {
		t.Errorf("the error should name what it thought it saw, got:\n%v", err)
	}
	if strings.Contains(err.Error(), "MIIEow") {
		t.Error("the refusal must not quote the value it refused")
	}
}

// TestSetWithoutTheFlagIsNotTreatedAsSet is the other direction: the default
// empty string must not read as "-set ”", which resolve() would refuse.
func TestSetWithoutTheFlagIsNotTreatedAsSet(t *testing.T) {
	o, err := parse(t, "-show")
	if err != nil {
		t.Fatalf("-show alone should be accepted: %v", err)
	}
	if o.setGiven {
		t.Error("setGiven should be false when -set was not passed")
	}
	if o.setToken != "" {
		t.Errorf("setToken = %q, want empty", o.setToken)
	}
}

// TestEverySingleActionIsAccepted guards the allow-list of one-flag invocations
// against resolve() tightening by accident.
func TestEverySingleActionIsAccepted(t *testing.T) {
	for _, args := range [][]string{{"-show"}, {"-refresh"}, {"-set", sentinel}} {
		if _, err := parse(t, args...); err != nil {
			t.Errorf("%v should be accepted, got: %v", args, err)
		}
	}
}

// TestEveryCombinationIsAccepted pins the combinations the usage text promises,
// so the two cannot drift apart.
func TestEveryCombinationIsAccepted(t *testing.T) {
	for _, args := range [][]string{
		{"-refresh", "-show"},
		{"-set", sentinel, "-show"},
		{"-set", sentinel, "-refresh"},
		{"-set", sentinel, "-refresh", "-show"},
	} {
		if _, err := parse(t, args...); err != nil {
			t.Errorf("%v should be accepted, got: %v", args, err)
		}
	}
}

// ---- dispatch and ordering ----

func TestSetAloneSetsAndDoesNothingElse(t *testing.T) {
	hc, sc := &fakeTokenClient{}, &fakeSubClient{}
	o, err := parse(t, "-set", sentinel)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := apply(o, hc, sc, &out); err != nil {
		t.Fatal(err)
	}
	if hc.setValue != sentinel {
		t.Errorf("setValue = %q, want %q", hc.setValue, sentinel)
	}
	if len(hc.calls) != 1 || hc.calls[0] != "SetCurrentToken" {
		t.Errorf("calls = %v, want exactly [SetCurrentToken]", hc.calls)
	}
	if sc.calls != 0 {
		t.Error("-show was not asked for, so the push client should not have been touched")
	}
}

func TestRefreshAloneRefreshesAndDoesNothingElse(t *testing.T) {
	hc, sc := &fakeTokenClient{}, &fakeSubClient{}
	o, err := parse(t, "-refresh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := apply(o, hc, sc, &out); err != nil {
		t.Fatal(err)
	}
	if len(hc.calls) != 1 || hc.calls[0] != "RefreshToken" {
		t.Errorf("calls = %v, want exactly [RefreshToken]", hc.calls)
	}
	if hc.setValue != "" {
		t.Error("-set was not asked for, so no token should have been set")
	}
}

// TestRefreshPassesNilTokenManager is the assertion that keeps the "no file is
// written" promise honest. nil is the only argument that stops the SDK
// persisting the token, and a fake that ignored its argument would let that
// change silently.
func TestRefreshPassesNilTokenManager(t *testing.T) {
	hc := &fakeTokenClient{}
	o, _ := parse(t, "-refresh")
	if err := apply(o, hc, &fakeSubClient{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if hc.sawManager {
		t.Error("RefreshToken was given a token manager, so the token would have been written to a file")
	}
}

// TestSetRunsBeforeRefresh pins the documented order, and the reasoning behind
// it: a refresh is authenticated with the token in hand, so -set has to seed it
// or the two flags are unrelated. The reverse order would make -set a no-op.
func TestSetRunsBeforeRefresh(t *testing.T) {
	hc := &fakeTokenClient{}
	o, err := parse(t, "-set", sentinel, "-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(o, hc, &fakeSubClient{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := []string{"SetCurrentToken", "RefreshToken"}
	if strings.Join(hc.calls, ",") != strings.Join(want, ",") {
		t.Errorf("call order = %v, want %v", hc.calls, want)
	}
}

func TestShowMakesNoTokenCall(t *testing.T) {
	hc, sc := &fakeTokenClient{}, &fakeSubClient{}
	o, err := parse(t, "-show")
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(o, hc, sc, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(hc.calls) != 0 {
		t.Errorf("-show should not touch the token client, got %v", hc.calls)
	}
	if sc.calls != 1 {
		t.Errorf("the subscription record should be read once, got %d", sc.calls)
	}
}

// TestAllThreeRunInTheDocumentedOrder covers the full combination in one place,
// so the order of all three is asserted rather than inferred from two tests.
func TestAllThreeRunInTheDocumentedOrder(t *testing.T) {
	hc, sc := &fakeTokenClient{}, &fakeSubClient{}
	o, err := parse(t, "-set", sentinel, "-refresh", "-show")
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(o, hc, sc, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := []string{"SetCurrentToken", "RefreshToken"}
	if strings.Join(hc.calls, ",") != strings.Join(want, ",") {
		t.Errorf("call order = %v, want %v", hc.calls, want)
	}
	if sc.calls != 1 {
		t.Errorf("the subscription record should be read once, got %d", sc.calls)
	}
}

// TestRefreshFailureStopsBeforeShow checks that a failed refresh does not go on
// to print a subscription record as though the run had completed.
func TestRefreshFailureStopsBeforeShow(t *testing.T) {
	boom := errors.New("server said no")
	hc, sc := &fakeTokenClient{refreshErr: boom}, &fakeSubClient{}
	o, err := parse(t, "-set", sentinel, "-refresh", "-show")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runErr := apply(o, hc, sc, &out)
	if runErr == nil {
		t.Fatal("a failed refresh should be an error")
	}
	if !errors.Is(runErr, boom) {
		t.Errorf("the SDK error should be wrapped, not replaced, got %v", runErr)
	}
	if sc.calls != 0 {
		t.Error("a failed refresh should stop the run before -show")
	}
	// The set token is still in memory: the SDK only stores a new token once
	// the request succeeds, so saying so is the difference between "unchanged"
	// and a reader guessing.
	if !strings.Contains(out.String(), "unchanged") {
		t.Errorf("a failed refresh should say the token is unchanged, got:\n%s", out.String())
	}
}

// ---- output honesty ----

// TestTheTokenValueIsNeverPrinted is the load-bearing output test. It runs a
// recognisable value through every combination and asserts it reaches neither
// stream, not whole and not in part. The length is deliberately not checked
// here: a length is what Redact is for, and it is what the spec permits.
func TestTheTokenValueIsNeverPrinted(t *testing.T) {
	combos := [][]string{
		{"-set", sentinel},
		{"-set", sentinel, "-refresh"},
		{"-set", sentinel, "-show"},
		{"-set", sentinel, "-refresh", "-show"},
	}
	for _, args := range combos {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			o, err := parse(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := apply(o, &fakeTokenClient{}, &fakeSubClient{}, &out); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "SENTINEL") {
				t.Errorf("the token value reached the output:\n%s", out.String())
			}
			if !strings.Contains(out.String(), "redacted") && strings.Contains(out.String(), "set from") {
				t.Error("a set token should be reported through the redacting helper, not verbatim")
			}
		})
	}
}

// TestTheTokenValueIsNeverPrintedOnTheErrorPath closes the gap the test above
// leaves: a failing refresh is the one place an SDK error string could carry
// something we did not choose.
func TestTheTokenValueIsNeverPrintedOnTheErrorPath(t *testing.T) {
	o, err := parse(t, "-set", sentinel, "-refresh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	hc := &fakeTokenClient{refreshErr: errors.New("token 刷新请求失败")}
	// A non-ASCII error string on purpose: the SDK wraps its failures in
	// Chinese, so a test that only ever saw ASCII errors would not prove the
	// check survives a multi-byte string landing in the middle of the output.
	if err := apply(o, hc, &fakeSubClient{}, &out); err == nil {
		t.Fatal("the refresh was supposed to fail")
	}
	if strings.Contains(out.String(), "SENTINEL") {
		t.Errorf("the token value reached the failure output:\n%s", out.String())
	}
}

// TestSetWarningNamesAllThreeCosts pins the warning to the three things a user
// needs to know before the value is used: the defence is bypassed, and the
// value is in shell history and in ps.
func TestSetWarningNamesAllThreeCosts(t *testing.T) {
	for _, want := range []string{"shell history", "ps", "deliberately bypassed"} {
		if !strings.Contains(setWarning, want) {
			t.Errorf("the -set warning should mention %q, got:\n%s", want, setWarning)
		}
	}
	if strings.Contains(setWarning, sentinel) {
		t.Error("the warning is a constant and must not carry a value")
	}
}

// TestSubscriptionsOutputIsUnambiguouslyLocal is the honesty test for -show.
//
// The requirement is not that the output mentions "local" somewhere. It is that
// the output cannot be read as a server answer, which means the claim has to
// come BEFORE the list, and the empty case has to say why it is empty rather
// than leaving an empty section that looks like an answer.
func TestSubscriptionsOutputIsUnambiguouslyLocal(t *testing.T) {
	var out bytes.Buffer
	reportSubscriptions(&out, nil)
	got := out.String()

	localAt := strings.Index(got, "LOCAL")
	notAskedAt := strings.Index(got, "server was not asked")
	if localAt < 0 || notAskedAt < 0 {
		t.Fatalf("the banner must say LOCAL and that the server was not asked, got:\n%s", got)
	}
	if localAt > notAskedAt {
		t.Error("both claims belong in the banner, before anything that could be read as a result")
	}
	if !strings.Contains(got, "expected") {
		t.Errorf("an empty list should say it is the expected result, got:\n%s", got)
	}
	if !strings.Contains(got, "cmd/push") {
		t.Errorf("an empty list should point at the command that does subscribe, got:\n%s", got)
	}
	for _, forbidden := range []string{"server says", "your subscriptions are", "account has no"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("the output must not read as a server answer (%q), got:\n%s", forbidden, got)
		}
	}
}

// TestSubscriptionsAreSorted pins the sort. The real getter returns map keys,
// and Go randomises map iteration order, so an unsorted renderer would produce
// output that differs between two identical runs.
func TestSubscriptionsAreSorted(t *testing.T) {
	// Deliberately in the worst order: the renderer must not echo it.
	subs := []sdkpush.SubjectType{"transaction", "asset", "position", "order"}
	var out bytes.Buffer
	reportSubscriptions(&out, subs)
	prev := -1
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
			continue
		}
		name := strings.TrimSpace(line)
		at := strings.Index(out.String(), name)
		if at < prev {
			t.Errorf("subscriptions are not sorted, got:\n%s", out.String())
		}
		prev = at
	}
	if prev < 0 {
		t.Errorf("no subscription line was printed, got:\n%s", out.String())
	}
}

// TestNonEmptySubscriptionsStillSayLocal proves the banner is not conditional on
// the interesting case. If the "local" wording only appeared on the empty path,
// a future change that made the list non-empty would silently drop the caveat.
func TestNonEmptySubscriptionsStillSayLocal(t *testing.T) {
	var out bytes.Buffer
	reportSubscriptions(&out, []sdkpush.SubjectType{sdkpush.SubjectOrder})
	got := out.String()
	if !strings.Contains(got, "LOCAL") || !strings.Contains(got, "server was not asked") {
		t.Errorf("a non-empty list needs the same banner, got:\n%s", got)
	}
	if !strings.Contains(got, "not the server's") {
		t.Errorf("a non-empty list must not be left to speak for itself, got:\n%s", got)
	}
}

// TestRefreshedOutputSaysWhoseTokenWon is the -set + -refresh honesty test. The
// single most misleading thing this command could print is a line that implies
// the value passed to -set is still in effect afterwards.
func TestRefreshedOutputSaysWhoseTokenWon(t *testing.T) {
	o, _ := parse(t, "-set", sentinel, "-refresh")
	var out bytes.Buffer
	reportRefreshed(&out, o)
	got := out.String()
	if !strings.Contains(got, "replaced it") {
		t.Errorf("the output should say the new token replaced the old, got:\n%s", got)
	}
	if !strings.Contains(got, "gone when this process exits") {
		t.Errorf("the output should say the token does not survive the process, got:\n%s", got)
	}
	if !strings.Contains(got, "not written to any file") {
		t.Errorf("the output should say nothing was written, got:\n%s", got)
	}
}

// TestRefreshedOutputNamesAnEmptyPriorToken covers the -refresh-alone case,
// which is the surprising one: this project starts every client with an empty
// token, so "the request was authenticated with nothing" is the true statement
// and leaving it implicit would read as a bug.
func TestRefreshedOutputNamesAnEmptyPriorToken(t *testing.T) {
	o, _ := parse(t, "-refresh")
	var out bytes.Buffer
	reportRefreshed(&out, o)
	if !strings.Contains(out.String(), "empty token") {
		t.Errorf("-refresh alone should say the prior token was empty, got:\n%s", out.String())
	}
}

// ---- flags, help, and the read-only assertion ----

// TestFlagDefaults pins the defaults, because the defaults are behaviour: three
// actions that all default to off means a bare invocation must be an error, and
// that is only true while none of them defaults to on.
func TestFlagDefaults(t *testing.T) {
	var o options
	if err := newFlagSet(&o, io.Discard).Parse(nil); err != nil {
		t.Fatal(err)
	}
	if o.setToken != "" || o.refresh || o.show || o.verbose {
		t.Errorf("defaults = %+v, want every action flag off", o)
	}
}

// TestUnexpectedPositionalArgumentIsRejected keeps a stray word from being
// silently ignored, which is how a mistyped flag name turns into a no-op.
func TestUnexpectedPositionalArgumentIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"refrsh"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("a positional argument should be an error")
	}
	if !strings.Contains(err.Error(), "refrsh") {
		t.Errorf("the error should quote what it did not understand, got: %v", err)
	}
}

// TestHelpNeedsNoCredentials is the only invocation a user can try before
// configuring anything, so it has to work with no environment and no config.
func TestHelpNeedsNoCredentials(t *testing.T) {
	t.Setenv("TIGER_ID", "")
	t.Setenv("TIGER_PRIVATE_KEY", "")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-h"}, &stdout, &stderr); err != nil {
		t.Fatalf("-h should succeed with no credentials, got: %v", err)
	}
}

func TestHelpDocumentsEveryFlag(t *testing.T) {
	for _, flagName := range []string{"-refresh", "-set", "-show"} {
		if !strings.Contains(usage, flagName) {
			t.Errorf("the usage text should document %s, got:\n%s", flagName, usage)
		}
	}
	// The combinations are the part a user cannot guess, so they are pinned
	// here rather than left to the flag defaults to imply.
	for _, want := range []string{"-set TOKEN -refresh", "local", "server is not asked"} {
		if !strings.Contains(usage, want) {
			t.Errorf("the usage text should cover %q, got:\n%s", want, usage)
		}
	}
}

// TestUsageStatesTheCombinationOrder pins the -set/-refresh order in the text a
// user reads, not only in the code. These two drifted independently in plenty of
// CLIs, and the drift is invisible until someone depends on the wrong one.
func TestUsageStatesTheCombinationOrder(t *testing.T) {
	at := strings.Index(usage, "-set TOKEN -refresh")
	if at < 0 {
		t.Fatal("the usage text should show the -set/-refresh combination")
	}
	if !strings.Contains(usage[at:], "-set first") {
		t.Error("the usage text should say -set runs first")
	}
	if !strings.Contains(usage[at:], "NOT the token in effect") {
		t.Error("the usage text should say the -set value does not survive the refresh")
	}
}

// TestPackageCannotReachOrderWrites is cmd/push's structural test, applied to
// this package. It parses the source rather than trusting the doc comment, so
// the claim "this command is read-only" is checkable rather than asserted.
func TestPackageCannotReachOrderWrites(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Fatal("no Go files found in the package directory")
	}

	forbidden := []string{
		"openapi-go-sdk/trade",
		"PlaceOrder", "ModifyOrder", "CancelOrder",
		"PlaceForexOrder", "OptionExerciseSubmit", "OptionExerciseCancel",
		"TransferSegmentFund", "CancelSegmentFund", "TransferPosition",
		"NewTradeClient", "TradeClient", "PreviewOrder",
	}
	for _, name := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, f := range file.Imports {
			imported := strings.Trim(f.Path.Value, `"`)
			for _, bad := range forbidden {
				if strings.Contains(imported, bad) {
					t.Errorf("%s imports %s; token must not be able to write", name, imported)
				}
			}
		}
		for _, ident := range identifiers(t, file) {
			for _, bad := range forbidden {
				if ident == bad {
					t.Errorf("%s references %s; token must not be able to write", name, bad)
				}
			}
		}
	}
}

// identifiers returns every identifier in the file's code, skipping comments and
// string literals so documentation cannot fail the gate.
func identifiers(t *testing.T, file *ast.File) []string {
	t.Helper()
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			out = append(out, v.Name)
		case *ast.SelectorExpr:
			out = append(out, v.Sel.Name)
		case *ast.BasicLit:
			return false
		}
		return true
	})
	return out
}

// TestTokenClientInterfaceHasNoWriteMethods asserts the same property from the
// type's side: widening tokenClient is the only way a write could appear here,
// so the interface is checked to be exactly the two methods the command needs.
func TestTokenClientInterfaceHasNoWriteMethods(t *testing.T) {
	iface := findInterface(t, "tokenClient")
	var got []string
	for _, field := range iface.Methods.List {
		for _, name := range field.Names {
			got = append(got, name.Name)
		}
	}
	if strings.Join(got, ",") != "RefreshToken,SetCurrentToken" {
		t.Errorf("tokenClient exposes %v, want exactly [RefreshToken SetCurrentToken]", got)
	}
}

// TestSubClientInterfaceHasNoWriteMethods does the same for the push side, and
// in particular asserts there is no Connect or Subscribe: -show is specified as
// a purely local read, and a Connect on this interface would quietly make it a
// network call.
func TestSubClientInterfaceHasNoWriteMethods(t *testing.T) {
	iface := findInterface(t, "subClient")
	var got []string
	for _, field := range iface.Methods.List {
		for _, name := range field.Names {
			got = append(got, name.Name)
		}
	}
	if strings.Join(got, ",") != "GetAccountSubscriptions" {
		t.Errorf("subClient exposes %v, want exactly [GetAccountSubscriptions]", got)
	}
}

func findInterface(t *testing.T, name string) *ast.InterfaceType {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}
			if it, ok := ts.Type.(*ast.InterfaceType); ok {
				return it
			}
		}
	}
	t.Fatalf("no %s interface found in main.go", name)
	return nil
}
