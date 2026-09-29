package main

// The renderers live here rather than in main.go so apply() stays a list of
// decisions and every string this command can print is in one file. That is what
// makes "no token value is ever printed" a property a test can check by reading
// one place: config.Redact is the only path a value could take to stdout, and it
// is called from exactly three lines below.

import (
	"fmt"
	"io"
	"sort"

	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"

	"github.com/shing1211/tiger-go-demo/internal/config"
)

// reportSet confirms a token was installed, by length only.
//
// It deliberately does not echo the value, not even truncated. The rule is the
// same one config.Redact exists for, and the same one the -set warning states
// out loud: a value that is on a command line is already compromised, and
// printing it again adds a second copy to every log that captures stdout.
func reportSet(stdout io.Writer, token string) {
	fmt.Fprintf(stdout, "token  set from the command line: %s (in memory only; not written to any file)\n",
		config.Redact(token))
}

// reportRefreshed confirms a rotation, and is precise about the one thing a
// caller might otherwise assume wrongly: the token in effect afterwards is the
// gateway's, not the one that was in effect before.
//
// The new value is not printed, and its length is not printed either, because
// the SDK's RefreshToken returns only an error. Printing a number this function
// cannot know would be inventing it. The SDK's own logger emits the length
// transition at debug level, and that is where the lengths are, if you need
// them: run with -v.
func reportRefreshed(stdout io.Writer, o *options) {
	fmt.Fprintf(stdout, "token  refreshed from the gateway using the %s token; the new token replaced it\n",
		describePriorToken(o))
	fmt.Fprintln(stdout, "       in memory only; not written to any file, and gone when this process exits")
}

// describePriorToken says what the refresh request was authenticated with.
//
// With no -set, that is nothing: this project starts every client with an empty
// token, because NewClientConfig clears the field the SDK would otherwise have
// filled from a file. Saying "no token" is the honest report and it is also the
// surprising one, so it is said rather than left for the reader to work out.
func describePriorToken(o *options) string {
	if !o.setGiven {
		return "empty"
	}
	return config.Redact(o.setToken)
}

// reportRefreshFailed says what did NOT change.
//
// A failed refresh returns before the SDK stores anything, so the token in
// memory is untouched. Without this line a failed -refresh is indistinguishable
// from a successful one that produced no visible effect, and the natural reading
// of the output is that the token was cleared.
func reportRefreshFailed(stdout io.Writer, o *options, err error) {
	fmt.Fprintf(stdout, "token  NOT refreshed: %v\n", err)
	fmt.Fprintf(stdout, "       the %s token is unchanged and still in effect\n", describePriorToken(o))
	fmt.Fprintln(stdout, "       nothing was written to any file")
}

// reportSubscriptions prints the SDK's account-level subscription record.
//
// The banner is unconditional, is printed BEFORE the list, and says LOCAL and
// says the server was not asked. That placement is the point: an empty list
// printed with no context is the exact output that reads like "the server says
// this account has no subscriptions", which is the one wrong conclusion
// available from this data. The getter is a local read of the push client's own
// map, so there is nothing in it that could support that conclusion.
func reportSubscriptions(stdout io.Writer, subs []sdkpush.SubjectType) {
	fmt.Fprintln(stdout, "\nsubscriptions  LOCAL — this process's own in-memory record")
	fmt.Fprintln(stdout, "              the server was not asked; no request of any kind was made")
	if len(subs) == 0 {
		fmt.Fprintln(stdout, "  (none — a subscription is recorded only by a Subscribe* call, and this")
		fmt.Fprintln(stdout, "   command makes none and never connects, so an empty list is the expected")
		fmt.Fprintln(stdout, "   result, not an answer about the account.)")
		fmt.Fprintln(stdout, "  cmd/push is where subscriptions are actually made.")
		return
	}
	// The SDK returns the keys of a map and Go randomises map iteration order,
	// so the list is sorted here. An unsorted set is a set a test cannot assert
	// on and a diff cannot read.
	names := make([]string, 0, len(subs))
	for _, s := range subs {
		names = append(names, string(s))
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(stdout, "  %s\n", n)
	}
	fmt.Fprintln(stdout, "  (these are this process's records, not the server's)")
}
