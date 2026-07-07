package app

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func names(syms []Symbol) map[string]string {
	out := map[string]string{}
	for _, s := range syms {
		out[s.Name] = s.Kind
	}
	return out
}

func TestGoSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/m\n")
	p := writeFile(t, dir, "a.go", `package a

func Exported() {}
func unexported() {}
func (x *T) Method() {}

type T struct{}

const MaxSize = 1
var Global, hidden = 1, 2
`)
	got := names(goSymbols(p, dir, goModulePath(dir)))
	want := map[string]string{"Exported": "func", "T": "type", "MaxSize": "const", "Global": "var"}
	for n, k := range want {
		if got[n] != k {
			t.Errorf("Go: want %s=%s, got %q (all: %v)", n, k, got[n], got)
		}
	}
	if _, ok := got["unexported"]; ok {
		t.Error("Go: unexported leaked")
	}
	if _, ok := got["Method"]; ok {
		t.Error("Go: method leaked")
	}
	if len(goSymbols(p, dir, "")) == 0 {
		t.Error("Go: no symbols without module path")
	}
	syms := goSymbols(p, dir, "example.com/m")
	if syms[0].ImportPath != "example.com/m" || syms[0].Pkg != "a" {
		t.Errorf("Go: importPath/pkg wrong: %+v", syms[0])
	}
}

func TestJSSymbols(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.ts", `import { x } from './b'
export function foo() {}
export async function bar() {}
export const baz = 1
export class Qux {}
export interface Shape {}
export type Alias = string
export default function main() {}
export { one, two as three }
const internal = 1
`)
	got := names(jsSymbols(p))
	for n, k := range map[string]string{
		"foo": "func", "bar": "func", "baz": "var", "Qux": "class",
		"Shape": "type", "Alias": "type", "main": "default",
		"one": "var", "three": "var",
	} {
		if got[n] != k {
			t.Errorf("JS: want %s=%s, got %q", n, k, got[n])
		}
	}
	if _, ok := got["internal"]; ok {
		t.Error("JS: non-export leaked")
	}
	if _, ok := got["two"]; ok {
		t.Error("JS: pre-alias name leaked")
	}
}

func TestPySymbols(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.py", `def public(): pass
def _private(): pass
class Thing: pass
    def method(self): pass
`)
	got := names(pySymbols(p))
	if got["public"] != "func" || got["Thing"] != "class" {
		t.Errorf("Py: got %v", got)
	}
	if _, ok := got["_private"]; ok {
		t.Error("Py: underscore leaked")
	}
	if _, ok := got["method"]; ok {
		t.Error("Py: indented def leaked")
	}
}

func TestSymbolCacheByMtime(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.py", "def first(): pass\n")
	if _, ok := names(fileSymbols(p, dir, ""))["first"]; !ok {
		t.Fatal("miss on first read")
	}
	// same mtime → cached result even if content differs on disk
	info, _ := os.Stat(p)
	os.WriteFile(p, []byte("def second(): pass\n"), 0o644)
	os.Chtimes(p, info.ModTime(), info.ModTime())
	if _, ok := names(fileSymbols(p, dir, ""))["first"]; !ok {
		t.Error("cache not used for unchanged mtime")
	}
}
