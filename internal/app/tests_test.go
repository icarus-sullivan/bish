package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileGoTestsFindsOnlyRealTestFuncs(t *testing.T) {
	dir := t.TempDir()
	src := `package pkg

import "testing"

func TestFoo(t *testing.T) {}

func TestBar(t *testing.T) {
	t.Run("sub", func(t *testing.T) {})
}

// not a test: wrong signature
func TestHelper(x int) {}

// not a test: method, not a func
type S struct{}
func (s S) TestMethod(t *testing.T) {}

func helperTestish(t *testing.T) {}
`
	path := filepath.Join(dir, "foo_test.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	got := fileGoTests(path)
	want := map[string]int{"TestFoo": 5, "TestBar": 7}
	if len(got) != len(want) {
		t.Fatalf("got %d tests, want %d: %+v", len(got), len(want), got)
	}
	for _, g := range got {
		line, ok := want[g.Name]
		if !ok {
			t.Errorf("unexpected test found: %s", g.Name)
			continue
		}
		if g.Line != line {
			t.Errorf("%s: line = %d, want %d", g.Name, g.Line, line)
		}
		if g.Pkg != dir {
			t.Errorf("%s: pkg = %q, want %q", g.Name, g.Pkg, dir)
		}
	}
}

func TestFileTestsRejectsNonTestFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	os.WriteFile(path, []byte("package pkg\nfunc TestFoo(t *testing.T){}\n"), 0o644) //nolint

	a := &App{}
	if got := a.FileTests(path); got != nil {
		t.Fatalf("FileTests on a non-_test.go file returned %+v, want nil", got)
	}
}
