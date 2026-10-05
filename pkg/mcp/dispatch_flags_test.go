//nolint:testpackage // exercises unexported argv builders
package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// cliPackageDir is the cobra command package whose flag registrations are the
// source of truth for what a container-dispatched `yap` argv may contain.
// pkg/mcp imports that package, and its cobra commands are unexported, so the
// registrations are read from source instead of instantiating the commands.
const cliPackageDir = "../../cmd/yap/command"

var flagRegisterRe = regexp.MustCompile(`^(Bool|String|StringSlice|StringArray|Int)Var(P)?$`)

// cliStringConsts collects package-level string constants (e.g. flagSkipSync).
func cliStringConsts(files []*ast.File) map[string]string {
	consts := map[string]string{}

	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}

			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != len(vs.Values) {
					continue
				}

				for i, v := range vs.Values {
					if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						s, _ := strconv.Unquote(lit.Value)
						consts[vs.Names[i].Name] = s
					}
				}
			}
		}
	}

	return consts
}

// cliFlagNames returns, per cobra command variable (buildCmd, prepareCmd,
// rootCmd...), the long flag names registered on it.
func cliFlagNames(t *testing.T) map[string]map[string]bool {
	t.Helper()

	fset := token.NewFileSet()

	entries, err := os.ReadDir(cliPackageDir)
	if err != nil {
		t.Fatalf("read %s: %v", cliPackageDir, err)
	}

	var files []*ast.File

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
			strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(cliPackageDir, e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}

		files = append(files, f)
	}

	consts := cliStringConsts(files)
	out := map[string]map[string]bool{}

	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			recv, name := flagRegistration(call, consts)
			if recv != "" {
				if out[recv] == nil {
					out[recv] = map[string]bool{}
				}

				out[recv][name] = true
			}

			return true
		})
	}

	return out
}

// flagRegistration recognises `<cmd>.Flags().XxxVar[P](&v, "name", ...)` and
// `<cmd>.PersistentFlags()...` calls and returns the command variable and flag.
func flagRegistration(call *ast.CallExpr, consts map[string]string) (cmdVar, name string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !flagRegisterRe.MatchString(sel.Sel.Name) || len(call.Args) < 2 {
		return "", ""
	}

	inner, ok := sel.X.(*ast.CallExpr)
	if !ok {
		return "", ""
	}

	getter, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || (getter.Sel.Name != "Flags" && getter.Sel.Name != "PersistentFlags") {
		return "", ""
	}

	id, ok := getter.X.(*ast.Ident)
	if !ok {
		return "", ""
	}

	switch arg := call.Args[1].(type) {
	case *ast.BasicLit:
		s, _ := strconv.Unquote(arg.Value)
		return id.Name, s
	case *ast.Ident:
		if s, found := consts[arg.Name]; found {
			return id.Name, s
		}
	}

	return "", ""
}

// fillAllFields sets every field of a buildArgs to a non-zero value so the
// generated argv exercises every flag the dispatcher can emit.
func fillAllFields(t *testing.T) *buildArgs {
	t.Helper()

	args := &buildArgs{}
	v := reflect.ValueOf(args).Elem()

	for _, f := range v.Fields() {
		switch f.Kind() { //nolint:exhaustive // only the kinds buildArgs uses
		case reflect.Bool:
			f.SetBool(true)
		case reflect.String:
			f.SetString("x")
		case reflect.Slice:
			f.Set(reflect.ValueOf([]string{"x"}))
		default:
			t.Fatalf("unhandled buildArgs field kind %s", f.Kind())
		}
	}

	return args
}

// longFlags returns the distinct `--flag` tokens of an argv.
func longFlags(argv []string) []string {
	var out []string

	for _, a := range argv {
		if name, ok := strings.CutPrefix(a, "--"); ok {
			out = append(out, name)
		}
	}

	return out
}

// TestDispatchedBuildArgvUsesRealCLIFlags guards against the dispatcher
// emitting flag names the yap CLI does not define: cobra would reject the
// unknown flag inside the container and fail the whole build session.
func TestDispatchedBuildArgvUsesRealCLIFlags(t *testing.T) {
	names := cliFlagNames(t)

	known := map[string]bool{}

	for _, cmd := range []string{"buildCmd", "rootCmd"} {
		for n := range names[cmd] {
			known[n] = true
		}
	}

	if !known["skip-sync"] || !known["verbose"] {
		t.Fatalf("flag extraction looks broken, got %v", names)
	}

	argv := buildCLIArgsFromArgs(fillAllFields(t), "ubuntu-noble")

	for _, f := range longFlags(argv) {
		if !known[f] {
			t.Errorf("dispatched argv carries --%s, which `yap build` does not define", f)
		}
	}
}

// TestPrepareArgvUsesRealCLIFlags does the same for the chained prepare step.
func TestPrepareArgvUsesRealCLIFlags(t *testing.T) {
	names := cliFlagNames(t)

	known := map[string]bool{}

	for _, cmd := range []string{"prepareCmd", "rootCmd"} {
		for n := range names[cmd] {
			known[n] = true
		}
	}

	argv := prepareCLIArgs(fillAllFields(t), "ubuntu-noble")

	if len(argv) < 2 || argv[0] != "prepare" || argv[1] != "ubuntu-noble" {
		t.Fatalf("prepare argv head = %v", argv)
	}

	for _, f := range longFlags(argv) {
		if !known[f] {
			t.Errorf("prepare argv carries --%s, which `yap prepare` does not define", f)
		}
	}
}
