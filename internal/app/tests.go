package app

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// GoTest is one `func TestXxx(t *testing.T)` declaration — Go only for this
// first cut (matches the debugger's own scoping call); JS/Python test
// frameworks are a follow-up, not a silent gap.
type GoTest struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"` // 1-based
	Pkg  string `json:"pkg"`  // directory `go test -run` should run in
}

// FileTests returns the Go tests declared directly in path (for the editor
// gutter — only _test.go files get one, everything else is nil).
func (a *App) FileTests(path string) []GoTest {
	if !strings.HasSuffix(path, "_test.go") {
		return nil
	}
	return fileGoTests(path)
}

// GetGoTests walks root for every Go test, for the Tests sidebar panel.
// Mirrors GetProjectSymbols' walk (skipDirs, hidden-dir skip) but restricted
// to *_test.go files — no per-file cache like symbols.go's since this is a
// user-triggered refresh, not typed-in-real-time.
func (a *App) GetGoTests(root string) []GoTest {
	var out []GoTest
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 10 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			if e.IsDir() {
				if skipDirs[name] {
					continue
				}
				walk(full, depth+1)
				continue
			}
			if strings.HasSuffix(name, "_test.go") {
				out = append(out, fileGoTests(full)...)
			}
		}
	}
	walk(root, 0)
	return out
}

func fileGoTests(path string) []GoTest {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(path)
	var out []GoTest
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || !isTestSignature(fn) {
			continue
		}
		out = append(out, GoTest{
			Name: fn.Name.Name,
			File: path,
			Line: fset.Position(fn.Pos()).Line,
			Pkg:  dir,
		})
	}
	return out
}

// isTestSignature checks for a single `*testing.T` (or bare `*T`, covering
// a dot-imported testing package) parameter — good enough without full
// type-checking, since that's the only sane source of such an argument to
// a func named TestXxx.
func isTestSignature(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch t := star.X.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name == "T"
	case *ast.Ident:
		return t.Name == "T"
	}
	return false
}

// RunGoTest runs a single test via the process manager — same mechanism as
// Task Runner, so pass/fail shows up as the existing running/stopped/crashed
// dot in the Process List with zero new plumbing. Returns the spawned
// process's ID so the frontend can correlate status transitions back to
// this specific test.
func (a *App) RunGoTest(pkg, name string) (string, error) {
	if pkg == "" || name == "" {
		return "", fmt.Errorf("missing test package/name")
	}
	cmd := fmt.Sprintf("go test -run '^%s$' -v .", name)
	p, err := a.mgr.Add(cmd, pkg, name)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}
