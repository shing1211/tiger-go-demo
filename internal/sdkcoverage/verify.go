package sdkcoverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// libraryDirs are the SDK directories it ships for its callers to import. A call
// site in one of these is what earns ReasonInternal.
var libraryDirs = map[string]bool{
	"client": true, "quote": true, "trade": true, "push": true,
	"config": true, "model": true, "signer": true, "logger": true,
}

// programDirs ship for a human to run, not for import. A call site here does
// NOT earn ReasonInternal: these are programs the SDK's authors wrote to
// demonstrate or integrate, and nothing in the library's own call graph depends
// on them. This distinction is the entire content of the sdk-coverage rule on
// the internal reason, and it used to be four paragraphs of README prose and one
// Makefile comment.
var programDirs = map[string]bool{
	"examples": true, "cmd": true, "integtest": true, "scripts": true,
}

// UnsupportedReasons returns allow-list entries whose reason the module's own
// source does not support, as "Method:reason — why" lines, sorted.
//
// It checks the two claims that are cheap to verify and expensive to argue:
//
//   - ReasonInternal is earned only by a call site in a library package, never
//     by one in a program the SDK ships for a human to run.
//   - ReasonNotUsed is earned by there being no call site anywhere in the
//     module outside test files, including the library's own code.
//
// The remaining reasons are claims about this project rather than about the SDK
// — deprecated, mutating, not-a-call — and cannot be checked from the module, so
// they are not checked here.
func UnsupportedReasons(sdkDir string) ([]string, error) {
	callers, err := moduleCallers(sdkDir)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, e := range AllowList() {
		switch e.Reason {
		case ReasonInternal:
			if len(callers.library[e.Method]) == 0 {
				bad = append(bad, fmt.Sprintf("%s:%s — no call site in a library package (found only in %s)",
					e.Method, e.Reason, describeDirs(callers.program[e.Method])))
			}
		case ReasonNotUsed:
			if all := append(append([]string{}, callers.library[e.Method]...),
				callers.program[e.Method]...); len(all) > 0 {
				bad = append(bad, fmt.Sprintf("%s:%s — the module does call it, from %s",
					e.Method, e.Reason, describeDirs(all)))
			}
		}
	}
	sort.Strings(bad)
	return bad, nil
}

// callerIndex records, per tracked method, which library and which program
// directories call it.
type callerIndex struct {
	library map[string][]string
	program map[string][]string
}

// moduleCallers indexes every non-test call of an allow-listed method by the
// top-level directory it appears in.
func moduleCallers(sdkDir string) (callerIndex, error) {
	tracked := map[string]bool{}
	for _, e := range AllowList() {
		tracked[e.Method] = true
	}
	idx := callerIndex{library: map[string][]string{}, program: map[string][]string{}}

	fset := token.NewFileSet()
	err := filepath.WalkDir(sdkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		top := topLevelDir(sdkDir, path)
		inLibrary := libraryDirs[top]
		if !inLibrary && !programDirs[top] {
			// A directory in neither list is neither a known library package
			// nor a known shipped program. Count it as library code, because
			// the reason is earned by the SDK's own importable code and an
			// unlisted directory is far more likely to be that than a demo.
			// Erring this way costs a lenient check; erring the other way would
			// flag a correct entry, which is the failure that teaches people to
			// ignore the check.
			inLibrary = true
		}

		seen := map[string]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if !tracked[sel.Sel.Name] || seen[sel.Sel.Name] {
				return true
			}
			seen[sel.Sel.Name] = true
			if inLibrary {
				idx.library[sel.Sel.Name] = append(idx.library[sel.Sel.Name], top)
			} else {
				idx.program[sel.Sel.Name] = append(idx.program[sel.Sel.Name], top)
			}
			return true
		})
		return nil
	})
	return idx, err
}

// topLevelDir is the first path segment of path relative to root.
func topLevelDir(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func describeDirs(dirs []string) string {
	if len(dirs) == 0 {
		return "nowhere outside test files"
	}
	out := append([]string{}, dirs...)
	sort.Strings(out)
	return strings.Join(out, ", ")
}
