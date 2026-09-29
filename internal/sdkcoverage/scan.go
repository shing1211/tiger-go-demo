// Package sdkcoverage checks which of the SDK's client methods this project
// references.
//
// The check is a static reference search. It proves a method is named by
// command or internal code; it does not prove the method was ever exercised
// against a live Tiger account, and no artifact may state that it was. The
// figure this package produces is published in README.md and in
// openspec/specs/sdk-coverage, so a change to it is a claim about the SDK or
// about this project and needs a reason written down, not a new expected
// value.
package sdkcoverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// sdkModule is the library whose client surface is covered.
const sdkModule = "github.com/tigerfintech/openapi-go-sdk"

// ModuleDir returns the SDK's directory in the local module cache.
//
// go list -m -f {{.Dir}} resolves the path itself, which is what makes this
// portable. Hand-assembling the path from GOMODCACHE plus a version string is
// the kind of thing that works on one machine and returns a path that does not
// exist on another; the module cache layout also differs by GOOS.
func ModuleDir() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", sdkModule).Output()
	if err != nil {
		return "", fmt.Errorf("locating %s: %w", sdkModule, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("%s is not in the module cache; run `go mod download %s`",
			sdkModule, sdkModule)
	}
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("%s is not present at %s: %w", sdkModule, dir, err)
	}
	return dir, nil
}

// ClientMethods returns every exported method declared on an SDK *Client type,
// sorted and de-duplicated.
//
// Three properties of this function are load-bearing, and each is silent when
// broken — the run reports success either way.
//
// It matches the receiver TYPE, never the receiver variable's name. The
// previous implementation was a grep for a literal "(c \*", and pinning a
// receiver name is a latent bug with no failure attached: the day the SDK
// renames one receiver, the pattern matches nothing for that type, the whole
// type drops out of the denominator, and the run is simply smaller and greener.
// A pinned name is not a smaller number that someone notices; it is one that
// looks better than the truth.
//
// It accepts a value receiver as well as a pointer receiver. None of the four
// client types declares a value receiver today, so this changes no current
// count, and that is exactly why it must stay: forbidding value receivers
// outright would reintroduce the same silent-shrink failure for a future SDK
// that adds one. Value receivers do exist elsewhere in the module (model,
// logger), so the shape is a real distinction and not a hypothetical.
//
// It walks declarations, which is what excludes push/pb structurally. That
// package declares 32 receiver types over 43 declared types, 278 of them field
// accessors — hundreds of methods no SDK caller would ever write. None of its
// receiver types ends in "Client", and the string "Client" does not appear in
// the package at all, so a walk keyed on client-typed declarations cannot reach
// it. The old --exclude-dir=pb flag was honest documentation rather than a
// guard, and there is no flag here to drop.
func ClientMethods(sdkDir string) ([]string, error) {
	info, err := os.Stat(sdkDir)
	if err != nil {
		return nil, fmt.Errorf("SDK directory %s: %w", sdkDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("SDK path %s is not a directory", sdkDir)
	}

	seen := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(sdkDir, func(path string, d os.DirEntry, err error) error {
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
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || !fn.Name.IsExported() {
				continue
			}
			if isClientType(fn.Recv.List[0].Type) {
				seen[fn.Name.Name] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// isClientType reports whether a receiver type is named *Client, accepting
// either a pointer or a value receiver.
func isClientType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return isClientType(t.X)
	case *ast.Ident:
		return strings.HasSuffix(t.Name, "Client")
	}
	return false
}
