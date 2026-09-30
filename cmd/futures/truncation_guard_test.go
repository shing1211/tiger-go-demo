package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// This test is a structural net, not a behavioural one.
//
// A renderer that stops early because of -limit is only useful if it says how
// many rows it hid. The call that does that is rocli.Truncate. Once, an entire
// package lost every one of those calls: the `if i >= limit { break }` guards
// all survived, so every limit test still passed -- they only asserted that
// rows past the limit were ABSENT, which a bare `break` also satisfies. The
// "... N more row(s) not shown" line silently disappeared from all of them.
//
// Writing one behavioural subtest per renderer would not have caught that
// either, because each would have had to be remembered separately. This walks
// the source instead, so it covers every capped loop in the package, including
// any added later, and fails the moment a guard loses its Truncate call.
func TestEveryLimitGuardReportsTruncation(t *testing.T) {
	src, err := os.ReadFile("output.go")
	if err != nil {
		t.Fatalf("read output.go: %v", err)
	}
	lines := strings.Split(string(src), "\n")

	guard := regexp.MustCompile(`^\s*if ([ij]) >= limit \{$`)
	var current string
	checked, reported := 0, 0

	for n, line := range lines {
		if m := regexp.MustCompile(`^func (print\w+)\(`).FindStringSubmatch(line); m != nil {
			current = m[1]
		}
		m := guard.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		checked++
		// The guard body runs to the first line that closes the block.
		body := ""
		for _, next := range lines[n+1:] {
			if strings.TrimSpace(next) == "}" {
				break
			}
			body += next
		}
		if !strings.Contains(body, "rocli.Truncate") {
			t.Errorf("%s caps a loop at line %d but never calls rocli.Truncate, so a "+
				"truncated result would not say how many rows were hidden",
				current, n+1)
			continue
		}
		if !regexp.MustCompile(`rocli\.Truncate\(\w+, ` + m[1] + `,`).MatchString(body) {
			t.Errorf("%s at line %d truncates on %q but passes a different index to "+
				"rocli.Truncate; the hidden-row count would be wrong",
				current, n+1, m[1])
			continue
		}
		reported++
	}

	if checked == 0 {
		t.Fatal("no limit guards found in output.go; this test is not looking at the right file")
	}
	t.Logf("verified %d/%d capped loops report truncation", reported, checked)
}
