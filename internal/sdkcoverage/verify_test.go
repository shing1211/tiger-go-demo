package sdkcoverage

import (
	"path/filepath"
	"strings"
	"testing"
)

// These tests are the control tests for the reason derivation. A derivation
// that returns nothing is indistinguishable from a derivation that is correct
// and finds nothing, so each one points UnsupportedReasons at a synthetic module
// tree built to be wrong in exactly one way.

// TestUnsupportedReasonsFlagsInternalWithNoLibraryCaller is the case the README
// argues at length and the Makefile asserted in a comment: a reason that claims
// the library calls the method when the module has no such call at all.
func TestUnsupportedReasonsFlagsInternalWithNoLibraryCaller(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "client", "http_client.go"), `package client

type HttpClient struct{}

func (c *HttpClient) Whatever() {}
`)

	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	for _, m := range []string{"Execute", "QueryToken", "SecretKey", "StartTokenAutoRefresh"} {
		if !containsLine(got, m) {
			t.Errorf("%s claims 'internal' but the module has no library caller; "+
				"it should be flagged.\ngot:\n%s", m, joined)
		}
	}
}

// TestUnsupportedReasonsAcceptsALibraryCaller is the positive case: a caller in
// a package the SDK ships for its callers to import does earn the reason.
//
// The fixtures below contain CALLS, not declarations, and the difference is not
// incidental. moduleCallers looks for a *ast.CallExpr whose callee is a
// selector, so `func (c *HttpClient) Execute() {}` — a declaration — earns
// nothing. Declaring the method is what ClientMethods counts; calling it is what
// earns the reason, and a fixture that conflates the two would pass for the
// wrong reason.
func TestUnsupportedReasonsAcceptsALibraryCaller(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "client", "http_client.go"), `package client

type HttpClient struct{}

func (c *HttpClient) execute() {
	c.Execute()
	c.QueryToken()
	c.StartTokenAutoRefresh()
}
`)
	write(t, filepath.Join(dir, "trade", "trade_client.go"), `package trade

type TradeClient struct{}

func (c *TradeClient) key() string {
	_ = c.SecretKey()
	return ""
}
`)

	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("all four are called from a library package, so none should be "+
			"flagged; got:\n%s", strings.Join(got, "\n"))
	}
}

// TestUnsupportedReasonsRejectsAnExampleOnlyCaller is the distinction the whole
// rule turns on. examples/manual_test/ ships inside the module, so a naive
// "does the module call it" search would say yes. It is a program for a human to
// run, and nothing in the library's own call graph depends on it.
func TestUnsupportedReasonsRejectsAnExampleOnlyCaller(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "examples", "manual_test", "demo.go"), `package manual_test

type HttpClient struct{}

func Demo(h *HttpClient) { h.Execute() }
`)

	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(got, "Execute") {
		t.Errorf("a caller in examples/ must not earn 'internal'; got:\n%s",
			strings.Join(got, "\n"))
	}
	// And the message has to say where the caller actually was, or a reader
	// cannot tell which file to look at.
	if !strings.Contains(strings.Join(got, "\n"), "examples") {
		t.Errorf("the message should name the directory the caller was found in; got:\n%s",
			strings.Join(got, "\n"))
	}
}

// TestUnsupportedReasonsRejectsACommandOnlyCaller covers the other program
// directory, and the other reason: ExecuteRaw claims "not-used", which is earned
// by there being no caller at all.
func TestUnsupportedReasonsRejectsACommandOnlyCaller(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "cmd", "integ", "main.go"), `package main

type HttpClient struct{}

func run(h *HttpClient) { h.ExecuteRaw("x", "{}") }
`)

	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(got, "ExecuteRaw") {
		t.Errorf("ExecuteRaw claims 'not-used' but something in the module calls it; got:\n%s",
			strings.Join(got, "\n"))
	}
}

// TestUnsupportedReasonsIgnoresTestFiles: a test-only caller is not a library
// call either, and it must not silently satisfy "not-used" either.
func TestUnsupportedReasonsIgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "client", "http_client_test.go"), `package client

type HttpClient struct{}

func (c *HttpClient) Execute() {}
`)

	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(got, "Execute") {
		t.Errorf("a call only from a _test.go file is not a library call; got:\n%s",
			strings.Join(got, "\n"))
	}
}

// TestUnsupportedReasonsSaysNothingWhenNothingIsWrong is the other half: a
// derivation that flags everything is no more trustworthy than one that flags
// nothing, and the real module is the case that matters.
func TestUnsupportedReasonsSaysNothingWhenNothingIsWrong(t *testing.T) {
	dir, err := ModuleDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnsupportedReasons(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("against the real module every reason should be supported, but these "+
			"are not:\n%s", strings.Join(got, "\n"))
	}
}

// containsLine reports whether any entry names the given method.
func containsLine(list []string, method string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, method+":") {
			return true
		}
	}
	return false
}
