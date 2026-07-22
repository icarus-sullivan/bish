package app

import "testing"

func TestGoOutline(t *testing.T) {
	a := &App{}
	syms := a.FileOutline("outline.go") // this file
	if syms == nil {
		t.Fatal("nil outline for outline.go")
	}
	// FileOutline has a receiver → listed as a method "App.FileOutline"
	want := map[string]bool{"App.FileOutline": false, "goOutline": false, "indentDepth": false}
	for _, s := range syms {
		if _, ok := want[s.Name]; ok {
			want[s.Name] = true
			if s.Line == 0 {
				t.Fatalf("%s has no line number", s.Name)
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("outline missing %s", name)
		}
	}
}
