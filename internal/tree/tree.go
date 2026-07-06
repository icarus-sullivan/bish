package tree

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Node struct {
	Name     string
	Path     string
	IsDir    bool
	Depth    int
	Expanded bool
	Children []*Node
}

type Tree struct {
	Root     *Node
	Flat     []*Node // visible nodes in order
	Selected int
}

func New(root string) *Tree {
	t := &Tree{}
	t.Load(root)
	return t
}

func (t *Tree) Load(root string) {
	t.Root = loadNode(root, 0, 2)
	t.Root.Expanded = true
	t.flatten()
	t.Selected = 0
}

func loadNode(path string, depth, maxDepth int) *Node {
	info, err := os.Stat(path)
	if err != nil {
		return &Node{Name: filepath.Base(path), Path: path}
	}
	n := &Node{
		Name:  filepath.Base(path),
		Path:  path,
		IsDir: info.IsDir(),
		Depth: depth,
	}
	if !info.IsDir() || depth >= maxDepth {
		return n
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return n
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := loadNode(filepath.Join(path, e.Name()), depth+1, maxDepth)
		n.Children = append(n.Children, child)
	}
	return n
}

func (t *Tree) flatten() {
	t.Flat = nil
	var walk func(n *Node)
	walk = func(n *Node) {
		t.Flat = append(t.Flat, n)
		if n.IsDir && n.Expanded {
			for _, c := range n.Children {
				walk(c)
			}
		}
	}
	if t.Root != nil {
		walk(t.Root)
	}
}

// ExpandedPaths returns the absolute paths of all currently-expanded dirs
// (excluding the root itself, which is always expanded).
func (t *Tree) ExpandedPaths() []string {
	var paths []string
	var walk func(*Node)
	walk = func(n *Node) {
		if n.IsDir && n.Expanded && n.Depth > 0 {
			paths = append(paths, n.Path)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if t.Root != nil {
		walk(t.Root)
	}
	return paths
}

// RestoreExpanded marks the given paths as expanded, loading children on demand.
// Call after Load() to re-apply a saved or captured expansion set.
func (t *Tree) RestoreExpanded(paths []string) {
	for _, p := range paths {
		t.expandPath(p)
	}
	t.flatten()
}

func (t *Tree) expandPath(path string) {
	var walk func(*Node) bool
	walk = func(n *Node) bool {
		if n.Path == path {
			if n.IsDir {
				n.Expanded = true
				if len(n.Children) == 0 {
					loaded := loadNode(n.Path, n.Depth, n.Depth+2)
					n.Children = loaded.Children
				}
			}
			return true
		}
		for _, c := range n.Children {
			if walk(c) {
				return true
			}
		}
		return false
	}
	if t.Root != nil {
		walk(t.Root)
	}
}

// CollapseAll collapses all expanded directories (keeping root expanded).
func (t *Tree) CollapseAll() {
	var walk func(*Node)
	walk = func(n *Node) {
		if n.IsDir && n.Depth > 0 {
			n.Expanded = false
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if t.Root != nil {
		walk(t.Root)
	}
	t.flatten()
}

func (t *Tree) Toggle() {
	if t.Selected >= len(t.Flat) {
		return
	}
	n := t.Flat[t.Selected]
	if !n.IsDir {
		return
	}
	n.Expanded = !n.Expanded
	if n.Expanded && len(n.Children) == 0 {
		// load one more level
		children := loadNode(n.Path, n.Depth, n.Depth+2)
		n.Children = children.Children
	}
	t.flatten()
}

func (t *Tree) MoveUp() {
	if t.Selected > 0 {
		t.Selected--
	}
}

func (t *Tree) MoveDown() {
	if t.Selected < len(t.Flat)-1 {
		t.Selected++
	}
}

func (t *Tree) SelectedPath() string {
	if t.Selected >= len(t.Flat) || len(t.Flat) == 0 {
		return ""
	}
	return t.Flat[t.Selected].Path
}

func (t *Tree) Prefix(n *Node) string {
	if n.Depth == 0 {
		return ""
	}
	indent := strings.Repeat("  ", n.Depth-1)
	if n.IsDir {
		if n.Expanded {
			return indent + "▾ "
		}
		return indent + "▸ "
	}
	return indent + "  "
}
