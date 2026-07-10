package tree

import (
	"os"
	"path/filepath"
	"testing"
)

// Skip dirs must appear collapsed with no eagerly-loaded children, and still
// load their children on explicit expand.
func TestLoadSkipsHeavyDirsButExpandsOnDemand(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, root, "src")
	mustMkdir(t, root, "src", "sub")
	mustMkdir(t, root, "node_modules", "pkg", "lib")

	tr := &Tree{}
	tr.Load(root)

	nm := findChild(tr.Root, "node_modules")
	if nm == nil {
		t.Fatal("node_modules missing from tree — should be shown collapsed, not hidden")
	}
	if len(nm.Children) != 0 {
		t.Fatalf("node_modules eagerly loaded %d children, want 0", len(nm.Children))
	}
	src := findChild(tr.Root, "src")
	if src == nil || len(src.Children) != 1 {
		t.Fatalf("normal dir src should have 1 eagerly-loaded child, got %+v", src)
	}

	// expand node_modules via Toggle (selects it in Flat first)
	for i, n := range tr.Flat {
		if n.Name == "node_modules" {
			tr.Selected = i
		}
	}
	tr.Toggle()
	if len(nm.Children) != 1 || nm.Children[0].Name != "pkg" {
		t.Fatalf("expanding node_modules should load pkg, got %+v", nm.Children)
	}
}

func mustMkdir(t *testing.T, parts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(parts...), 0o755); err != nil {
		t.Fatal(err)
	}
}

func findChild(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}
