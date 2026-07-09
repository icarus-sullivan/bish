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
	"sync"
	"time"
)

type Symbol struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`       // func | type | var | const | class | default
	File       string `json:"file"`
	ImportPath string `json:"importPath"` // Go: module path + rel dir; else ""
	Pkg        string `json:"pkg"`        // Go package name; else ""
}

var symbolExts = map[string]bool{
	".go": true, ".js": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".jsx": true, ".py": true,
	".svelte": true,
}

const maxSymbolFileSize = 512 * 1024 // ponytail: size cap, raise if someone hits it

// ponytail: one global cache keyed by path; per-project maps if contention shows up
var (
	symCacheMu sync.Mutex
	symCache   = map[string]symCacheEntry{}
	modCache   = map[string]string{} // root -> go.mod module path
)

type symCacheEntry struct {
	mtime time.Time
	syms  []Symbol
}

func (a *App) GetProjectSymbols(root string) []Symbol {
	var out []Symbol
	modPath := goModulePath(root)
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
			fullPath := filepath.Join(dir, name)
			if e.IsDir() {
				if skipDirs[name] {
					continue
				}
				walk(fullPath, depth+1)
				continue
			}
			if !symbolExts[filepath.Ext(name)] {
				continue
			}
			out = append(out, fileSymbols(fullPath, root, modPath)...)
		}
	}
	walk(root, 0)
	return out
}

func fileSymbols(path, root, modPath string) []Symbol {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxSymbolFileSize {
		return nil
	}
	symCacheMu.Lock()
	entry, ok := symCache[path]
	symCacheMu.Unlock()
	if ok && entry.mtime.Equal(info.ModTime()) {
		return entry.syms
	}
	var syms []Symbol
	switch filepath.Ext(path) {
	case ".go":
		syms = goSymbols(path, root, modPath)
	case ".py":
		syms = pySymbols(path)
	case ".svelte":
		// a component's one export is itself: default import named by filename
		syms = []Symbol{{Name: strings.TrimSuffix(filepath.Base(path), ".svelte"), Kind: "default", File: path}}
	default:
		syms = jsSymbols(path)
	}
	symCacheMu.Lock()
	symCache[path] = symCacheEntry{mtime: info.ModTime(), syms: syms}
	symCacheMu.Unlock()
	return syms
}

func goModulePath(root string) string {
	symCacheMu.Lock()
	if p, ok := modCache[root]; ok {
		symCacheMu.Unlock()
		return p
	}
	symCacheMu.Unlock()
	mod := ""
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "module ") {
				mod = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}
	symCacheMu.Lock()
	modCache[root] = mod
	symCacheMu.Unlock()
	return mod
}

func goSymbols(path, root, modPath string) []Symbol {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	importPath := ""
	if modPath != "" {
		if rel, err := filepath.Rel(root, filepath.Dir(path)); err == nil {
			if rel == "." {
				importPath = modPath
			} else {
				importPath = modPath + "/" + filepath.ToSlash(rel)
			}
		}
	}
	var syms []Symbol
	add := func(name, kind string) {
		if ast.IsExported(name) {
			syms = append(syms, Symbol{Name: name, Kind: kind, File: path, ImportPath: importPath, Pkg: f.Name.Name})
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil { // skip methods — not import targets
				add(d.Name.Name, "func")
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					add(s.Name.Name, "type")
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, n := range s.Names {
						add(n.Name, kind)
					}
				}
			}
		}
	}
	return syms
}

var (
	jsExportDecl    = regexp.MustCompile(`^export\s+(?:declare\s+)?(?:async\s+)?(?:function\*?|const|let|var|class|interface|type|enum)\s+([A-Za-z_$][\w$]*)`)
	jsExportBrace   = regexp.MustCompile(`^export\s*\{([^}]*)\}`)
	jsExportDefault = regexp.MustCompile(`^export\s+default\s+(?:async\s+)?(?:function\*?|class)\s+([A-Za-z_$][\w$]*)`)
)

func jsSymbols(path string) []Symbol {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var syms []Symbol
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.ContainsRune(line, 0) {
			return nil // binary
		}
		if m := jsExportDefault.FindStringSubmatch(line); m != nil {
			syms = append(syms, Symbol{Name: m[1], Kind: "default", File: path})
		} else if m := jsExportDecl.FindStringSubmatch(line); m != nil {
			kind := "var"
			if strings.Contains(line, "class") {
				kind = "class"
			} else if strings.Contains(line, "function") {
				kind = "func"
			} else if strings.Contains(line, "interface") || strings.Contains(line, "type ") || strings.Contains(line, "enum") {
				kind = "type"
			}
			syms = append(syms, Symbol{Name: m[1], Kind: kind, File: path})
		} else if m := jsExportBrace.FindStringSubmatch(line); m != nil {
			for _, part := range strings.Split(m[1], ",") {
				name := strings.TrimSpace(part)
				if idx := strings.LastIndex(name, " as "); idx != -1 {
					name = strings.TrimSpace(name[idx+4:])
				}
				if name != "" && name != "default" {
					syms = append(syms, Symbol{Name: name, Kind: "var", File: path})
				}
			}
		}
	}
	return syms
}

var pyDef = regexp.MustCompile(`^(def|class)\s+(\w+)`)

func pySymbols(path string) []Symbol {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var syms []Symbol
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.ContainsRune(line, 0) {
			return nil
		}
		if m := pyDef.FindStringSubmatch(line); m != nil && !strings.HasPrefix(m[2], "_") {
			kind := "func"
			if m[1] == "class" {
				kind = "class"
			}
			syms = append(syms, Symbol{Name: m[2], Kind: kind, File: path})
		}
	}
	return syms
}
