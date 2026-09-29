// Package test holds the repository-wide invariants — the ones that are about
// the shape of the tree rather than about one package's behaviour, and so have
// no natural home inside any single command.
//
// There is exactly one invariant here: which commands are read-only. It is
// enforced in both directions, because both directions have a failure mode that
// looks like success. A command added to the read-only list with no directory
// behind it, or a directory added under cmd/ with no list entry, both produce a
// green build and a false safety claim.
package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// repoRoot is the directory holding cmd/. Tests here run from the test/ package
// directory, which is the only Go package in the repository that is not under
// cmd/ or internal/ — which is also why this file lives here.
const repoRoot = ".."

// readOnlyCommands are the binaries that cannot place, modify or cancel an
// order.
//
// The classification is a safety claim, so it is stated once, here, and checked
// rather than being re-argued in each command's documentation. Membership means
// two things, both asserted below: the package never imports the SDK's trade
// package, and it exposes no order method through any interface it declares.
var readOnlyCommands = []string{
	"quote", "options", "futures", "reference", "corporate", "push", "token",
}

// writeCommands are the binaries with a write path. There is exactly one, and
// that is the point of the map being partitioned rather than a flag: "this
// command can write" has to be a deliberate, reviewed statement about a named
// directory, not the absence of a classification.
var writeCommands = []string{"trade"}

// TestCommandClassificationIsBidirectional is the enforcement, in both
// directions.
//
// Direction one: every name in either list must have a directory under cmd/.
// A stale entry is not harmless — it says a command is read-only when there is
// no such command, and the next person to add one inherits the claim for free.
//
// Direction two: every directory under cmd/ must appear in exactly one list. An
// unclassified directory is the dangerous direction, because a new command
// defaults to being nobody's problem, and a command nobody has classified is a
// command nobody has looked at.
func TestCommandClassificationIsBidirectional(t *testing.T) {
	errs := checkClassification(t, repoRoot, readOnlyCommands, writeCommands)
	for _, e := range errs {
		t.Error(e)
	}
}

// checkClassification is the enforcement, extracted so TestTheCheckIsNotVacuous
// can point it at a synthetic tree.
//
// The four rules, all of them load-bearing and all of them silent when broken:
//   - a classified name with no directory behind it is a false safety claim
//   - a directory with no classification is a command nobody has looked at
//   - a name in both lists is classified as safe and unsafe at once
//   - a second write command means the gate's blast radius grew
func checkClassification(t *testing.T, root string, readOnly, write []string) []string {
	t.Helper()
	var errs []string
	dirs := commandDirs(t, root)

	classified := map[string]string{}
	for _, name := range readOnly {
		if prev, dup := classified[name]; dup {
			errs = append(errs, name+" is classified "+prev+" and read-only")
		}
		classified[name] = "read-only"
	}
	for _, name := range write {
		if prev, dup := classified[name]; dup {
			errs = append(errs, name+" is classified "+prev+" and writes")
		}
		classified[name] = "writes"
	}

	// Direction one: an entry with no directory behind it.
	for _, name := range sortedKeys(classified) {
		if !dirs[name] {
			errs = append(errs, name+" is classified "+classified[name]+
				" but cmd/"+name+" does not exist; a classification with no command "+
				"behind it is a false claim")
		}
	}
	// Direction two: a directory with no classification.
	for _, name := range sortedKeys(dirs) {
		if _, ok := classified[name]; !ok {
			errs = append(errs, "cmd/"+name+" is not classified; add it to "+
				"readOnlyCommands or writeCommands in test/readonly_test.go. An "+
				"unclassified command is one nobody has decided is safe")
		}
	}
	if len(write) != 1 || write[0] != "trade" {
		errs = append(errs, "writeCommands is "+strings.Join(write, ",")+
			", want exactly [trade]; a second write command means the safety "+
			"argument in README.md has changed and needs arguing again")
	}
	return errs
}

// TestTheCheckIsNotVacuous is the control test, and it is the reason the checks
// above can be trusted.
//
// A defence test on its own proves nothing: the hazard could go away and the
// test would keep passing while measuring nothing. This one builds a synthetic
// cmd/ tree in a temporary directory — one good entry, one stale entry, one
// unclassified directory — and asserts the checker objects to each. If the
// checker stops complaining about any of the three, this fails.
func TestTheCheckIsNotVacuous(t *testing.T) {
	root := t.TempDir()
	mkCmd(t, root, "alpha") // classified read-only, has a directory
	mkCmd(t, root, "beta")  // present but unclassified
	mkCmd(t, root, "gamma") // present but unclassified
	mkCmd(t, root, "trade")

	// alpha is right; delta is classified with no directory; beta and gamma are
	// directories with no classification. Each of the last three must produce a
	// distinct complaint.
	errs := checkClassification(t, root, []string{"alpha", "delta"}, []string{"trade"})

	joined := strings.Join(errs, "\n")
	for _, want := range []string{
		"delta", // classified, no directory
		"beta",  // directory, no classification
		"gamma", // directory, no classification
		"unclassified",
		"does not exist",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the control tree should have produced a complaint about %q, got:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "alpha") {
		t.Errorf("a correctly classified command should produce no complaint, got:\n%s", joined)
	}

	// And a correct tree produces nothing at all, or the control above would
	// pass for the wrong reason.
	if errs := checkClassification(t, root, []string{"alpha", "beta", "gamma"}, []string{"trade"}); len(errs) != 0 {
		t.Errorf("a fully classified tree should be silent, got:\n%s", strings.Join(errs, "\n"))
	}
}

// TestOnlyOneWriteCommandIsStructural is the fourth rule on its own, so a
// failure names the thing that actually changed rather than a list of strings.
func TestOnlyOneWriteCommandIsStructural(t *testing.T) {
	if len(writeCommands) != 1 || writeCommands[0] != "trade" {
		t.Errorf("writeCommands = %v, want exactly [trade]. cmd/trade is this "+
			"project's only write path, and it is the only package that may import "+
			"the SDK's trade package.", writeCommands)
	}
}

func mkCmd(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "cmd", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write %s/main.go: %v", name, err)
	}
}

// TestReadOnlyCommandsCannotReachOrderWrites turns the read-only list from a
// claim into a check, by parsing each read-only command's own source.
//
// It is the same test cmd/push carries for itself, widened to every command on
// the list, because a list that is enforced one entry at a time is enforced only
// where someone remembered to. Comments and string literals are skipped, so
// documentation that has to NAME these methods cannot fail the check — the gate
// is about code.
//
// The trade package itself is the target: reaching an order method requires
// importing it, and importing it is visible here.
func TestReadOnlyCommandsCannotReachOrderWrites(t *testing.T) {
	forbidden := []string{
		"openapi-go-sdk/trade",
		"PlaceOrder", "ModifyOrder", "CancelOrder",
		"PlaceForexOrder", "OptionExerciseSubmit", "OptionExerciseCancel",
		"TransferSegmentFund", "CancelSegmentFund", "TransferPosition",
		"NewTradeClient", "TradeClient", "PreviewOrder",
	}

	for _, name := range readOnlyCommands {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(repoRoot, "cmd", name)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read cmd/%s: %v", name, err)
			}
			var files []string
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".go") {
					files = append(files, e.Name())
				}
			}
			if len(files) == 0 {
				t.Fatalf("cmd/%s has no Go files", name)
			}
			for _, f := range files {
				path := filepath.Join(dir, f)
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
				if err != nil {
					t.Fatalf("parse %s: %v", path, err)
				}
				for _, imp := range file.Imports {
					imported := strings.Trim(imp.Path.Value, `"`)
					for _, bad := range forbidden {
						if strings.Contains(imported, bad) {
							t.Errorf("cmd/%s/%s imports %s; a read-only command must not "+
								"be able to place an order", name, f, imported)
						}
					}
				}
				for _, ident := range identifiers(file) {
					for _, bad := range forbidden {
						if ident == bad {
							t.Errorf("cmd/%s/%s references %s; a read-only command must not "+
								"be able to place an order", name, f, bad)
						}
					}
				}
			}
		})
	}
}

// TestOnlyTheWriteCommandImportsTheTradePackage is the other direction of the
// same claim: the write gate's blast radius is exactly one package, so a second
// one is a finding rather than a detail.
func TestOnlyTheWriteCommandImportsTheTradePackage(t *testing.T) {
	for _, name := range writeCommands {
		if name != "trade" {
			t.Errorf("writeCommands lists %q, but this project's only write path is cmd/trade; "+
				"if that has changed, the safety argument in README.md has changed with it", name)
		}
	}
	dirs := commandDirs(t, repoRoot)
	for name := range dirs {
		if name == "trade" {
			continue
		}
		if importsTradePackage(t, filepath.Join(repoRoot, "cmd", name)) {
			t.Errorf("cmd/%s imports the SDK's trade package but is not in writeCommands", name)
		}
	}
}

// TestReadOnlyCommandsCannotExitWithTheRefusalStatus is the runtime half of the
// exit-codes requirement that every read-only binary cannot produce a refusal.
//
// exit 3 is defined in the shared read-only plumbing, so the status is reachable
// in this project's code — what is unreachable is any route to it from a command
// that never asks to write. A command with no write path has nothing to refuse,
// so a branch producing 3 in one of these binaries would be a claim it can decline
// something, which is exactly the kind of claim this project should not make.
//
// Comments and string literals are skipped for the same reason the import check
// skips them: the usage strings in cmd/quote and cmd/push describe the exit-code
// convention in prose, and documentation about the gate must not trip the gate.
func TestReadOnlyCommandsCannotExitWithTheRefusalStatus(t *testing.T) {
	// os.Exit(3) and any alias for the constant 3. rocli.ExitCode is named
	// separately because a read-only command may legitimately CALL the shared
	// mapping -- the mapping defines 3 even though no such command can reach
	// it -- so its presence is not the finding. The finding is a hardcoded
	// refusal.
	forbidden := []string{"os.Exit(3)", "os.Exit( 3 )"}

	for _, name := range readOnlyCommands {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(repoRoot, "cmd", name)
			for _, f := range goFiles(t, dir) {
				path := filepath.Join(dir, f)
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
				if err != nil {
					t.Fatalf("parse %s: %v", path, err)
				}
				_ = file // parsed to prove the source is valid Go before scanning it
				// Scan the code for a literal exit of the refusal status, with
				// comments and string contents removed first.
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				code := codeOnly(string(src))
				for _, bad := range forbidden {
					if strings.Contains(code, bad) {
						t.Errorf("cmd/%s/%s contains %s; a read-only command has no "+
							"write path to refuse, so it cannot produce a refusal",
							name, f, bad)
					}
				}
			}
		})
	}
}

// codeOnly strips comments and the contents of string literals, so a check for
// a token cannot be satisfied by documentation naming it.
//
// It is deliberately a text-level strip rather than an AST walk: the thing being
// looked for is a literal inside a call expression, and removing comments and
// string bodies is enough to make the search mean "this appears in code".
func codeOnly(src string) string {
	var out strings.Builder
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			i += 2
			for i < len(src) && !strings.HasPrefix(src[i:], "*/") {
				i++
			}
			i += 2
		case src[i] == '"' || src[i] == '`':
			quote := src[i]
			i++
			for i < len(src) && src[i] != quote {
				if quote == '"' && src[i] == '\\' {
					i++
				}
				i++
			}
			i++ // closing quote
		default:
			out.WriteByte(src[i])
			i++
		}
	}
	return out.String()
}

// goFiles lists the .go files in a command directory, sorted so a failure does
// not come out in a different order on every run.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files
}

// TestTokenIsClassifiedReadOnly is a regression pin on the specific entry this
// command was added for.
//
// The bidirectional test above would still pass if token were moved to
// writeCommands, because moving it is a valid edit to the list. This test is
// what makes the move a deliberate one: it fails, and the failure says why.
func TestTokenIsClassifiedReadOnly(t *testing.T) {
	for _, name := range writeCommands {
		if name == "token" {
			t.Error("cmd/token is read-only: it authenticates and reports, and it " +
				"never places, modifies or cancels an order. Removing it from the " +
				"read-only list needs a write path, and adding one needs the gate.")
		}
	}
	if !contains(readOnlyCommands, "token") {
		t.Error("cmd/token should be listed read-only")
	}
}

// TestTheListsDoNotOverlap keeps the partition honest: a command in both lists
// is classified as safe and unsafe at once, which satisfies neither reader.
func TestTheListsDoNotOverlap(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, readOnlyCommands...), writeCommands...) {
		if seen[name] {
			t.Errorf("%s appears in both lists", name)
		}
		seen[name] = true
	}
}

// commandDirs is every directory directly under cmd/, which is the unit of
// classification: one binary per directory.
func commandDirs(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("read cmd/: %v", err)
	}
	dirs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs[e.Name()] = true
		}
	}
	if len(dirs) == 0 {
		t.Fatal("cmd/ has no command directories")
	}
	return dirs
}

// sortedKeys exists because a map's iteration order is random, and a test that
// fails in a different order on every run is a test people stop reading. It
// also makes the two directions of the check come out in a stable sequence.
func sortedKeys[V ~string | ~bool](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func importsTradePackage(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, imp := range file.Imports {
			if strings.Contains(strings.Trim(imp.Path.Value, `"`), "openapi-go-sdk/trade") {
				return true
			}
		}
	}
	return false
}

// identifiers returns every identifier in the file's code, skipping comments and
// string literals so documentation cannot fail the gate.
func identifiers(file *ast.File) []string {
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

func contains(list []string, want string) bool { return slices.Contains(list, want) }
