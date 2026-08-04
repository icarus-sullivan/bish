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
		if depth > 10 || len(results) >= 500 {
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
			if e.IsDir() {
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
		if depth > 10 {
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
			if e.IsDir() {
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
			} else {
				files = append(files, fullPath)
			}
		}
	}
	walk(root, 0)
	return files
}
