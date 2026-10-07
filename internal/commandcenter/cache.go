package commandcenter

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/csullivan/bish/internal/envdetect"
)

// A step-cache hit means the step's declared inputs haven't changed since
// it last succeeded — NOT that its effect is still present. That's why only
// install/codegen steps the user explicitly gave cacheInputs are ever
// cached, and destructive or stateful ones never are.

// cacheable reports whether st may ever be skipped on a cache hit.
func cacheable(st *Step) bool {
	if len(st.CacheInputs) == 0 || st.Destructive || st.Stateful {
		return false
	}
	return st.Kind == "install" || st.Kind == "codegen"
}

type stepCacheEntry struct {
	Hash     string    `json:"hash"`
	At       time.Time `json:"at"`
	ExitCode int       `json:"exitCode"`
}

// stepCacher fingerprints and records steps for one checkout directory.
type stepCacher struct {
	dir       string
	toolchain string
	hashes    map[string]string // step name -> hash computed before the run
}

func newStepCacher(dir string) *stepCacher {
	return &stepCacher{dir: dir, toolchain: envdetect.Toolchain(dir), hashes: map[string]string{}}
}

func stepCacheDir(repoPath string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "cache", "steps", fmt.Sprintf("%x", md5.Sum([]byte(repoPath))))
}

func (c *stepCacher) file(st *Step) string {
	return filepath.Join(stepCacheDir(c.dir), safeName.ReplaceAllString(st.Name, "-")+".json")
}

// hit computes st's current fingerprint (remembered for record) and reports
// whether the last recorded run had the same one and succeeded.
func (c *stepCacher) hit(st *Step) bool {
	h := c.hash(st)
	c.hashes[st.Name] = h
	b, err := os.ReadFile(c.file(st))
	if err != nil {
		return false
	}
	var e stepCacheEntry
	if json.Unmarshal(b, &e) != nil {
		return false
	}
	return e.Hash == h && e.ExitCode == 0
}

// record stores the pre-run fingerprint of a step that just succeeded.
func (c *stepCacher) record(st *Step) {
	h, ok := c.hashes[st.Name]
	if !ok {
		return
	}
	b, _ := json.Marshal(stepCacheEntry{Hash: h, At: time.Now(), ExitCode: 0})
	if os.MkdirAll(filepath.Dir(c.file(st)), 0o755) == nil {
		os.WriteFile(c.file(st), b, 0o644) //nolint
	}
}

// hash is sha256 over the step's name, command, the toolchain, and each
// matched input's (relpath, size, mtime) — stat, not content, and never a
// walk into node_modules or .git.
func (c *stepCacher) hash(st *Step) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", st.Name, st.Cmd, c.toolchain)
	var lines []string
	for _, rel := range matchInputs(c.dir, st.CacheInputs) {
		fi, err := os.Stat(filepath.Join(c.dir, rel))
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\x00%d\x00%d", rel, fi.Size(), fi.ModTime().UnixNano()))
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Fprintln(h, l)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// matchInputs expands globs relative to dir. A "**/" prefix matches at any
// depth, skipping node_modules, .git and other dot-dirs.
func matchInputs(dir string, globs []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	for _, g := range globs {
		if rest, ok := strings.CutPrefix(g, "**/"); ok {
			filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error { //nolint
				if err != nil {
					return nil
				}
				if d.IsDir() {
					n := d.Name()
					if p != dir && (n == "node_modules" || strings.HasPrefix(n, ".") || n == "dist" || n == "target" || n == "vendor") {
						return filepath.SkipDir
					}
					return nil
				}
				if ok, _ := filepath.Match(rest, d.Name()); ok {
					if rel, err := filepath.Rel(dir, p); err == nil {
						add(rel)
					}
				}
				return nil
			})
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, g))
		for _, p := range matches {
			if rel, err := filepath.Rel(dir, p); err == nil {
				add(rel)
			}
		}
	}
	return out
}

// ClearStepCache forgets every recorded step run for the repo's checkouts
// in every env, forcing a full prestart next start.
func (m *Manager) ClearStepCache(repoID string) error {
	m.mu.Lock()
	r := m.def.repo(repoID)
	var dirs []string
	if r != nil {
		dirs = append(dirs, r.Path)
		for _, e := range m.state.Envs {
			if t := e.Targets[repoID]; t != nil && t.Path != "" {
				dirs = append(dirs, t.Path)
			}
		}
	}
	m.mu.Unlock()
	if r == nil {
		return fmt.Errorf("repo %s not found", repoID)
	}
	for _, d := range dirs {
		if err := os.RemoveAll(stepCacheDir(d)); err != nil {
			return err
		}
	}
	return nil
}
