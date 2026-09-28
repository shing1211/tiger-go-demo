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
func Dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

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
