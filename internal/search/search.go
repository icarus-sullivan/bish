// Package search implements the plain/regex/whole-word file search and
// find-replace walk shared by the Files panel (via internal/app) and the
// assistant tool-calling loop (via internal/assistant) — extracted here so
// both can call it without internal/assistant importing internal/app.
package search

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/csullivan/bish/internal/tree"
)

// maxFileSize skips huge files in search/replace; raise if it bites.
const maxFileSize = 2 * 1024 * 1024

var skipDirs = tree.SkipDirs

// DefaultMaxWalkDepth is MaxWalkDepth's value until Settings overrides it.
const DefaultMaxWalkDepth = 40

// MaxWalkDepth bounds recursion in Search/Replace/AllFiles (mainly against
// symlink cycles now that dirIsDir follows symlinks). A package var rather
// than a parameter on every call — Settings UI and the assistant's
// list_files/search_files tools (internal/assistant/tools.go) share the one
// knob rather than threading it through every signature. Exported so
// internal/app can apply config.Config.SearchMaxDepth to it on startup and
// on every SaveConfig.
var MaxWalkDepth = DefaultMaxWalkDepth

// dirIsDir reports whether entry e (at fullPath) should be walked as a
// directory. os.DirEntry.IsDir() reflects the dirent's own type and is false
// for a symlink even when it points at a directory, which silently dropped
// symlinked dirs from search (opened as a "file", then Scan() failed with
// EISDIR and produced zero matches) while they still showed up fine in the
// file tree, whose loadNode re-os.Stats every child. Resolve explicitly so
// both walkers agree.
func dirIsDir(e os.DirEntry, fullPath string) bool {
	if e.Type()&os.ModeSymlink == 0 {
		return e.IsDir()
	}
	info, err := os.Stat(fullPath)
	return err == nil && info.IsDir()
}

type Result struct {
	File string
	Line int
	Col  int
	Text string
}

// globToRegex converts a shell glob (supporting *, ?, and ** for
// cross-directory matches) into an anchored regexp matched against a
// forward-slash relative path.
func globToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// compileGlobs parses a comma-separated list of glob patterns. A pattern
// with no '/' is treated as matching at any depth (like "**/" + pattern),
// mirroring the common VS Code include/exclude convention.
func compileGlobs(raw string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			p = "**/" + p
		}
		if re, err := globToRegex(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

func matchesAny(patterns []*regexp.Regexp, relPath string) bool {
	for _, re := range patterns {
		if re.MatchString(relPath) {
			return true
		}
	}
	return false
}

func BuildMatcher(query string, caseSensitive, wholeWord, useRegex bool) (*regexp.Regexp, string, error) {
	if useRegex || wholeWord {
		pattern := query
		if !useRegex {
			pattern = regexp.QuoteMeta(query)
		}
		if wholeWord {
			pattern = `\b` + pattern + `\b`
		}
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		return re, "", err
	}
	plain := query
	if !caseSensitive {
		plain = strings.ToLower(query)
	}
	return nil, plain, nil
}

func Search(dir, query string, caseSensitive, wholeWord, useRegex bool, include, exclude string) []Result {
	if query == "" {
		return nil
	}
	re, plain, err := BuildMatcher(query, caseSensitive, wholeWord, useRegex)
	if err != nil {
		return nil
	}
	includeRe := compileGlobs(include)
	excludeRe := compileGlobs(exclude)
	var results []Result
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		if depth > MaxWalkDepth || len(results) >= 500 {
			return
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(d, name)
			rel := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(fullPath, dir), "/"))
			if dirIsDir(e, fullPath) {
				if skipDirs[name] || matchesAny(excludeRe, rel) {
					continue
				}
				walk(fullPath, depth+1)
			} else {
				if len(excludeRe) > 0 && matchesAny(excludeRe, rel) {
					continue
				}
				if len(includeRe) > 0 && !matchesAny(includeRe, rel) {
					continue
				}
				if info, err := e.Info(); err != nil || info.Size() > maxFileSize {
					continue
				}
				f, err := os.Open(fullPath)
				if err != nil {
					continue
				}
				scanner := bufio.NewScanner(f)
				// default 64KB line cap silently aborts files with long
				// (minified) lines — raise it so matches after them aren't lost
				scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
				lineNum := 0
				for scanner.Scan() {
					lineNum++
					raw := scanner.Text()
					if strings.ContainsRune(raw, 0) {
						break
					}
					var col int
					if re != nil {
						loc := re.FindStringIndex(raw)
						if loc == nil {
							continue
						}
						col = loc[0]
					} else {
						haystack := raw
						if !caseSensitive {
							haystack = strings.ToLower(raw)
						}
						col = strings.Index(haystack, plain)
						if col < 0 {
							continue
						}
					}
					results = append(results, Result{File: fullPath, Line: lineNum, Col: col, Text: raw})
					if len(results) >= 500 {
						f.Close()
						return
					}
				}
				f.Close()
			}
		}
	}
	walk(dir, 0)
	return results
}

func Replace(dir, query, replacement string, caseSensitive, wholeWord, useRegex bool, include, exclude string) (int, error) {
	if query == "" {
		return 0, nil
	}
	re, plain, err := BuildMatcher(query, caseSensitive, wholeWord, useRegex)
	if err != nil {
		return 0, err
	}
	includeRe := compileGlobs(include)
	excludeRe := compileGlobs(exclude)
	changed := 0
	var walk func(d string, depth int) error
	walk = func(d string, depth int) error {
		if depth > MaxWalkDepth {
			return nil
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(d, name)
			rel := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(fullPath, dir), "/"))
			if dirIsDir(e, fullPath) {
				if skipDirs[name] || matchesAny(excludeRe, rel) {
					continue
				}
				walk(fullPath, depth+1) //nolint
			} else {
				if len(excludeRe) > 0 && matchesAny(excludeRe, rel) {
					continue
				}
				if len(includeRe) > 0 && !matchesAny(includeRe, rel) {
					continue
				}
				if info, err := e.Info(); err != nil || info.Size() > maxFileSize {
					continue
				}
				content, err := os.ReadFile(fullPath)
				if err != nil {
					continue
				}
				if strings.ContainsRune(string(content), 0) {
					continue
				}
				var newContent string
				if re != nil {
					newContent = re.ReplaceAllString(string(content), replacement)
				} else if caseSensitive {
					newContent = strings.ReplaceAll(string(content), query, replacement)
				} else {
					s := string(content)
					lower := strings.ToLower(s)
					lq := plain
					var b strings.Builder
					for {
						idx := strings.Index(lower, lq)
						if idx < 0 {
							b.WriteString(s)
							break
						}
						b.WriteString(s[:idx])
						b.WriteString(replacement)
						s = s[idx+len(query):]
						lower = lower[idx+len(query):]
					}
					newContent = b.String()
				}
				if newContent != string(content) {
					if err := os.WriteFile(fullPath, []byte(newContent), 0o644); err != nil {
						return fmt.Errorf("write %s: %w", fullPath, err)
					}
					changed++
				}
			}
		}
		return nil
	}
	err = walk(dir, 0)
	return changed, err
}

// AllFiles recursively lists files under root (mirrors the Files panel's
// flat-list view), skipping dotfiles and the same heavy dirs as Search.
func AllFiles(root string) []string {
	var files []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > MaxWalkDepth {
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
			if dirIsDir(e, fullPath) {
				if skipDirs[name] {
					continue
				}
				walk(fullPath, depth+1)
			} else {
				files = append(files, fullPath)
			}
		}
	}
	walk(root, 0)
	return files
}
