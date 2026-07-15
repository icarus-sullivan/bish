package tree

import (
	"os"
	"path/filepath"
	"sort"
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

// hiddenNames are the only entries the tree hides outright — dotfiles like
// .env / .gitignore stay visible; dot-dirs like .svelte-kit show collapsed
// via SkipDirs
var hiddenNames = map[string]bool{".git": true, ".DS_Store": true}

// SkipDirs are heavy directories the walker shows but never descends into
// eagerly; children load only on explicit expand. Shared with search/replace
// and the fs watcher, which skip them outright.
var SkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "__pycache__": true, ".next": true,
	"target": true, ".cache": true, ".svelte-kit": true,
	".build": true, "build": false, // "build" kept — user might want it
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
		if hiddenNames[e.Name()] {
			continue
		}
		md := maxDepth
		if e.IsDir() && SkipDirs[e.Name()] {
			md = depth + 1 // show collapsed; children load on expand
		}
		child := loadNode(filepath.Join(path, e.Name()), depth+1, md)
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

