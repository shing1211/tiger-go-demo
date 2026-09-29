package sdkcoverage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Result is what one run of the coverage check found.
type Result struct {
	Total, Covered int
	// Uncovered is every method with no call site, sorted.
	Uncovered []string
	// NewGaps is every uncovered method with no allow-list entry, sorted.
	NewGaps []string
	// Stale is every allow-list entry that gained a call site, sorted.
	Stale []string
	// UnsupportedReasons is filled in by UnsupportedReasons in verify.go: the
	// allow-list entries whose reason is a claim about the SDK that the module
	// source does not support.
	UnsupportedReasons []string
}

// OK reports whether the check found nothing to say: the uncovered set is
// exactly the allow-list, in both directions, and every reason that is a claim
// about the SDK is supported by the module source.
func (r Result) OK() bool {
	return len(r.Stale) == 0 && len(r.NewGaps) == 0 && len(r.UnsupportedReasons) == 0
}

// FindRepoRoot walks up from the working directory to the directory holding
// go.mod.
//
// It is a walk rather than a fixed relative path because the working directory
// of a test is the package's own directory, and the check is invoked from two
// different packages: internal/sdkcoverage (where the repository root is
// "../..") and test/ (where it is ".."). A path relative to the source file
// works for one caller and silently finds nothing for the other — and finding
// nothing is the worst outcome available, because the check then reports every
// method as uncovered and looks like a real finding.
func FindRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("locating the working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %q; run this from inside the repository", dir)
		}
		dir = parent
	}
}

// Check runs the whole check against the repository root it discovers: find the
// SDK's client surface, find the call sites in this repository, compare the
// difference against the allow-list, and derive the reasons that are claims
// about the SDK rather than about this project.
//
// It makes no network request and needs no credentials. It does need the SDK in
// the local module cache, and fails loudly naming the path when it is absent,
// rather than reporting an empty and therefore vacuous result.
func Check() (Result, error) {
	root, err := FindRepoRoot()
	if err != nil {
		return Result{}, err
	}
	sdkDir, err := ModuleDir()
	if err != nil {
		return Result{}, err
	}
	methods, err := ClientMethods(sdkDir)
	if err != nil {
		return Result{}, err
	}
	covered, err := CallSites(root, methods)
	if err != nil {
		return Result{}, err
	}

	allowed := map[string]bool{}
	for _, e := range AllowList() {
		allowed[e.Method] = true
	}

	r := Result{Total: len(methods)}
	for _, m := range methods {
		if covered[m] {
			r.Covered++
			continue
		}
		r.Uncovered = append(r.Uncovered, m)
		if !allowed[m] {
			r.NewGaps = append(r.NewGaps, m)
		}
	}
	for _, e := range AllowList() {
		if covered[e.Method] {
			r.Stale = append(r.Stale, e.Method)
		}
	}
	sort.Strings(r.Uncovered)
	sort.Strings(r.NewGaps)
	sort.Strings(r.Stale)

	// Derive the reasons that are claims about the SDK, so a relabelling the
	// module source does not support fails the build instead of becoming a
	// comment nobody re-reads.
	bad, err := UnsupportedReasons(sdkDir)
	if err != nil {
		return Result{}, err
	}
	r.UnsupportedReasons = bad

	return r, nil
}

// Report writes the human-facing summary: the figure, then the allow-list with
// its reasons, then whatever needs attention.
//
// The reasons are printed even on a clean run. A coverage number on its own
// cannot distinguish a deliberate exclusion from an oversight, and the whole
// point of the table is that distinction.
func (r Result) Report(w io.Writer) {
	fmt.Fprintf(w, "sdk coverage: %d/%d methods covered, %d uncovered\n",
		r.Covered, r.Total, len(r.Uncovered))

	for _, line := range r.UnsupportedReasons {
		fmt.Fprintf(w, "  UNSUPPORTED REASON: %s\n", line)
	}
	if r.OK() {
		fmt.Fprintln(w, "uncovered set matches the allow-list:")
		for _, e := range AllowList() {
			fmt.Fprintf(w, "  %s:%s\n", e.Method, e.Reason)
		}
		return
	}
	for _, m := range r.NewGaps {
		fmt.Fprintf(w, "  UNCOVERED: %s — cover it, or justify it and add an Entry\n", m)
	}
	for _, m := range r.Stale {
		fmt.Fprintf(w, "  NOW COVERED: %s — delete its allow-list entry\n", m)
	}
}
