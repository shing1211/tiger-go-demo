package rocli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// This file holds the small presentation helpers the read-only commands share.
// Two rules apply everywhere:
//
//   - Never print a raw SDK error. Wrap it with the endpoint it came from so a
//     failure names the call that failed.
//   - Never print more than -limit rows, so a broad query cannot flood a
//     terminal. Say how many were truncated.

// Section writes a heading for one endpoint's output.
func Section(w io.Writer, name string, args ...interface{}) {
	fmt.Fprintf(w, "\n== %s ==\n", fmt.Sprintf(name, args...))
}

// JSON writes v as indented JSON. Many Tiger endpoints return open-ended maps
// (stock fundamental data, for example), so a faithful dump beats a table we
// would have to invent column names for.
func JSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// MSFmt renders an epoch-millisecond timestamp the way the SDK models them.
func MSFmt(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

// Dash renders an empty string as "-", so table columns stay aligned.
func Dash(s string) string { return DashOr(s, "-") }

// DashOr renders a blank string as the given fallback, so a column can say
// "any" or "server default" instead of a dash.
//
// This is Dash with a parameter, and the parameter is why both exist. The call
// sites use seven distinct fallbacks — "sdk default", "server default", "all",
// "unset", "today", "open", "any" — and each of them tells the reader something
// a dash does not: that the value is genuinely unset, or that the server chose
// it, are different facts. Collapsing them to one dash would lose all seven.
//
// Trimming is deliberately not done: a value that is present but padded is
// present, and silently trimming it would change what a caller asked for.
func DashOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// requiredFlagError is the error RequiredFlag returns. It is a distinct type so
// a caller can recognise "you forgot a flag" separately from "the server
// refused", which are different problems with different fixes.
type requiredFlagError struct {
	name, example string
}

func (e *requiredFlagError) Error() string {
	flag := strings.TrimLeft(e.name, "-")
	return fmt.Sprintf("%s is required for this endpoint, e.g. -%s %s", flag, flag, e.example)
}

// RequiredFlag returns the error a command gives when a flag it needs was not
// supplied, naming the flag and a value that would work.
//
// It is raised locally, before any connection, so a missing flag reads as a typo
// rather than as a credential or network problem.
func RequiredFlag(name, example string) error {
	return &requiredFlagError{name: name, example: example}
}

// FirstSymbol returns the first entry of a comma-separated symbol list, or ""
// when the list holds none. Upper-cased and blank-trimmed like List, so
// "  aapl , MSFT" yields "AAPL" rather than a leading space.
func FirstSymbol(list string) string {
	if l := List(list); len(l) > 0 {
		return l[0]
	}
	return ""
}

// DateMillis parses YYYY-MM-DD into epoch milliseconds for the endpoints that
// take a date range.
//
// It returns nil for an absent or unparseable date rather than a zero, so the
// field is omitted from the request instead of carrying 1970-01-01, which Tiger
// would read as a real date. A silently wrong date is worse than an absent one:
// the user is asking for a range and would get one, just not the one they meant.
func DateMillis(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

// Px renders a price to four decimal places, so price columns line up and a
// fractional cent is visible rather than rounded away silently.
func Px(v float64) string { return fmt.Sprintf("%.4f", v) }

// List splits a comma-separated flag value, trimming and dropping blanks.
func List(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

// ListRaw is List without upper-casing, for values like currencies or option
// identifiers where Tiger's own casing matters.
func ListRaw(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Float parses a numeric flag, rejecting junk rather than silently sending 0
// to Tiger and getting a confusing error back.
func Float(name, s string) (float64, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid -%s %q: want a number", name, s)
	}
	return v, nil
}

// Int parses an integer flag with the same contract as Float.
func Int(name, s string) (int, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid -%s %q: want an integer", name, s)
	}
	return v, nil
}

// Bools parses a comma-separated boolean flag list.
func Bools(s string) ([]bool, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []bool
	for _, p := range ListRaw(s) {
		v, err := strconv.ParseBool(p)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean %q: want true/false", p)
		}
		out = append(out, v)
	}
	return out, nil
}

// JSONFlag decodes a flag value that holds a JSON object or array, for the
// endpoints whose filters are open-ended (market scanner, option chain filters).
func JSONFlag(name, s string, out interface{}) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), out); err != nil {
		return fmt.Errorf("invalid -%s: want JSON (%v)", name, err)
	}
	return nil
}

// Truncate reports how many of n rows were printed given the limit, and prints
// a note when rows were dropped.
func Truncate(w io.Writer, shown, total, limit int) {
	if limit > 0 && total > shown {
		fmt.Fprintf(w, "  ... %d more row(s) not shown (-limit %d)\n", total-shown, limit)
	}
}

// SortedKeys returns a map's keys in a stable order, for deterministic output.
func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
