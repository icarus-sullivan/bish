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

// Entry is one directory listing result — the seam between Tree's walking
// logic and where the bytes actually come from (local os.ReadDir, or SSH for
// a remote project).
type Entry struct {
	Name  string
	IsDir bool
}

// FS is implemented by whatever backs a Tree's filesystem. nil (the zero
// value on Tree.FS) means local disk — see localFS below.
type FS interface {
	Stat(path string) (isDir bool, err error)
	ReadDir(path string) ([]Entry, error)
}

type localFS struct{}

func (localFS) Stat(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (localFS) ReadDir(path string) ([]Entry, error) {
	des, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(des))
	for i, d := range des {
		out[i] = Entry{Name: d.Name(), IsDir: d.IsDir()}
	}
	return out, nil
}

type Tree struct {
	Root     *Node
	Flat     []*Node // visible nodes in order
	Selected int
	FS       FS // nil = local disk; set to a remote.TreeFS for a remote project
}

func (t *Tree) fs() FS {
	if t.FS != nil {
		return t.FS
	}
	return localFS{}
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
	t.Root = t.loadNode(root, 0, 2)
	t.Root.Expanded = true
	t.flatten()
	t.Selected = 0
}

func (t *Tree) loadNode(path string, depth, maxDepth int) *Node {
	isDir, err := t.fs().Stat(path)
	if err != nil {
		return &Node{Name: filepath.Base(path), Path: path}
	}
	n := &Node{
		Name:  filepath.Base(path),
		Path:  path,
		IsDir: isDir,
		Depth: depth,
	}
	if !isDir || depth >= maxDepth {
		return n
	}
	entries, err := t.fs().ReadDir(path)
	if err != nil {
		return n
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	for _, e := range entries {
		if hiddenNames[e.Name] {
			continue
		}
		md := maxDepth
		if e.IsDir && SkipDirs[e.Name] {
			md = depth + 1 // show collapsed; children load on expand
		}
		child := t.loadNode(filepath.Join(path, e.Name), depth+1, md)
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
					loaded := t.loadNode(n.Path, n.Depth, n.Depth+2)
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

// ExpandToPath expands every ancestor directory of path (root-to-leaf, so
// each expandPath call can find nodes loaded by the previous one) so a
// target file becomes visible in Flat — used to reveal a Cmd+P selection
// even when its parent folders are collapsed.
func (t *Tree) ExpandToPath(path string) {
	if t.Root == nil {
		return
	}
	var ancestors []string
	for dir := filepath.Dir(path); dir != t.Root.Path && dir != "." && dir != string(filepath.Separator); {
		ancestors = append(ancestors, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		t.expandPath(ancestors[i])
	}
	t.flatten()
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
		children := t.loadNode(n.Path, n.Depth, n.Depth+2)
		n.Children = children.Children
	}
	t.flatten()
}
