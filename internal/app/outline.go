package app

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// OutlineSym is one entry in a file's symbol outline (all top-level defs, with
// line numbers — unlike Symbol, which is exports-only for import completion).
type OutlineSym struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"` // func | method | type | var | const | class | interface
	Line  int    `json:"line"` // 1-based
	Depth int    `json:"depth"`
}

// FileOutline returns the symbol outline for path, or nil for unsupported /
// oversized / unparseable files.
func (a *App) FileOutline(path string) []OutlineSym {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxSymbolFileSize {
		return nil
	}
	switch filepath.Ext(path) {
	case ".go":
		return goOutline(path)
	case ".py":
		return pyOutline(path)
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".svelte":
		return jsOutline(path)
	}
	return nil
}

func goOutline(path string) []OutlineSym {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	ln := func(p token.Pos) int { return fset.Position(p).Line }
	var out []OutlineSym
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name, kind := d.Name.Name, "func"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
				name = recvTypeName(d.Recv.List[0].Type) + "." + name
			}
			out = append(out, OutlineSym{name, kind, ln(d.Pos()), 0})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					out = append(out, OutlineSym{s.Name.Name, "type", ln(s.Pos()), 0})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, n := range s.Names {
						out = append(out, OutlineSym{n.Name, kind, ln(n.Pos()), 0})
					}
				}
			}
		}
	}
	return out
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

var pyDefOutline = regexp.MustCompile(`^(\s*)(def|class)\s+(\w+)`)

func pyOutline(path string) []OutlineSym {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []OutlineSym
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		t := sc.Text()
		if strings.ContainsRune(t, 0) {
			return nil
		}
		if m := pyDefOutline.FindStringSubmatch(t); m != nil {
			kind := "func"
			if m[2] == "class" {
				kind = "class"
			}
			out = append(out, OutlineSym{m[3], kind, line, indentDepth(m[1])})
		}
	}
	return out
}

var (
	jsDeclOutline  = regexp.MustCompile(`^(\s*)(?:export\s+)?(?:default\s+)?(?:async\s+)?(function\*?|class|interface|type|enum)\s+([A-Za-z_$][\w$]*)`)
	jsArrowOutline = regexp.MustCompile(`^(\s*)(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)
)

func jsOutline(path string) []OutlineSym {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []OutlineSym
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		t := sc.Text()
		if strings.ContainsRune(t, 0) {
			return nil
		}
		if m := jsDeclOutline.FindStringSubmatch(t); m != nil {
			kind := m[2]
			if kind == "function" || kind == "function*" {
				kind = "func"
			}
			out = append(out, OutlineSym{m[3], kind, line, indentDepth(m[1])})
		} else if m := jsArrowOutline.FindStringSubmatch(t); m != nil {
			out = append(out, OutlineSym{m[2], "func", line, indentDepth(m[1])})
		}
	}
	return out
}

// indentDepth maps leading whitespace to a nesting level (tab or 2 spaces = 1).
func indentDepth(ws string) int {
	n := 0
	for _, c := range ws {
		if c == '\t' {
			n += 2
		} else {
			n++
		}
	}
	return n / 2
}
