// Package search implements the plain/regex/whole-word file search and
// find-replace walk shared by the Files panel (via internal/app) and the
// assistant tool-calling loop (via internal/assistant) — extracted here so
// both can call it without internal/assistant importing internal/app.
package search

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	ignore "github.com/sabhiram/go-gitignore"

	"github.com/csullivan/bish/internal/tree"
)

// withRetry runs fn up to attempts times, with a short backoff between
// tries, before giving up and returning the last error. Corporate AV/EDR
// real-time scanners commonly lock a file for a few dozen milliseconds while
// they inspect it; without a retry that reads as a permanent I/O error and
// the file (or, in the indexer, the whole index) silently drops the content
// forever instead of just being briefly delayed.
func withRetry(attempts int, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(time.Duration(i+1) * 40 * time.Millisecond)
		}
	}
	return err
}

// maxFileSize skips huge files in search/replace; raise if it bites.
const maxFileSize = 2 * 1024 * 1024

// maxResults caps Search's result count. Was 500, which on large repos let
// the walk hit the cap inside an early (alphabetically-sorted) directory and
// stop, silently dropping real matches that lived in later directories; then
// 5000, before the frontend switched to chunked/virtualized rendering
// (GlobalSearch.svelte no longer mounts one DOM node per result up front, so
// a much bigger in-memory result set no longer means a frozen tab).
const maxResults = 50000

var skipDirs = tree.SkipDirs

// IncludeGitignored, when true, makes Search/Replace ignore .gitignore rules
// entirely (and skip the cost of reading/compiling them). false (default) =
// paths matched by an in-scope .gitignore are excluded. A package var rather
// than a parameter on every call — set from config.Config.SearchIncludeGitignored
// by internal/app's applySearchConfig, on startup and on every SaveConfig.
var IncludeGitignored = false

// IncludeHidden, when true (default), makes Search/Replace walk into
// dotfiles/dot-dirs instead of skipping them outright. Set from
// config.Config.SearchHiddenEnabled() the same way as IncludeGitignored.
var IncludeHidden = true

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

// gitignoreLayer is a compiled .gitignore rooted at base — matched against
// paths relative to base, not the search root, since gitignore patterns are
// relative to the directory containing the file that defines them.
type gitignoreLayer struct {
	base string
	gi   *ignore.GitIgnore
}

// extendGitignoreStack checks dir for a .gitignore file and, if present,
// returns stack with a new layer appended; otherwise returns stack
// unchanged. Callers only invoke this when IncludeGitignored is false.
func extendGitignoreStack(stack []gitignoreLayer, dir string) []gitignoreLayer {
	gi, err := ignore.CompileIgnoreFile(filepath.Join(dir, ".gitignore"))
	if err != nil || gi == nil {
		return stack
	}
	return append(stack, gitignoreLayer{base: dir, gi: gi})
}

// ignoredByStack reports whether fullPath is excluded by any layer in stack,
// each checked relative to its own base directory. This approximates git's
// real precedence (a closer .gitignore should be able to override a farther
// one, including via negation) by simply OR-ing every layer — a deliberate
// simplification that covers the common single- or few-.gitignore case
// without needing to reconcile cross-file negation.
func ignoredByStack(stack []gitignoreLayer, fullPath string) bool {
	for _, layer := range stack {
		rel, err := filepath.Rel(layer.base, fullPath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if layer.gi.MatchesPath(rel) {
			return true
		}
	}
	return false
}

// realDir resolves e (known to be a symlink) to its target's real path, for
// symlink-cycle detection now that recursion has no depth cap.
func realDir(fullPath string) (string, bool) {
	real, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return real, true
}

// errStopWalk, returned from a walkFiles visit callback, ends the walk early
// without being treated as a failure (e.g. a result cap was hit).
var errStopWalk = errors.New("stop walk")

// walkTree walks dir, calling onDir for every eligible directory (skipDirs,
// hidden, gitignore, and exclude-glob checked — include-glob doesn't apply to
// directories) before descending into it, and onFile for every eligible
// regular file (skipDirs/hidden/gitignore/include/exclude/maxFileSize
// checked). Either callback may be nil. A callback returning errStopWalk ends
// the walk early without being treated as a failure; any other non-nil error
// aborts and propagates to the caller.
func walkTree(dir string, includeRe, excludeRe []*regexp.Regexp, onDir, onFile func(fullPath string) error) error {
	visited := map[string]struct{}{}
	var walk func(d string, stack []gitignoreLayer) error
	walk = func(d string, stack []gitignoreLayer) error {
		if !IncludeGitignored {
			stack = extendGitignoreStack(stack, d)
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			name := e.Name()
			if !IncludeHidden && strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(d, name)
			rel := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(fullPath, dir), "/"))
			if !IncludeGitignored && ignoredByStack(stack, fullPath) {
				continue
			}
			if dirIsDir(e, fullPath) {
				if skipDirs[name] || matchesAny(excludeRe, rel) {
					continue
				}
				if e.Type()&os.ModeSymlink != 0 {
					real, ok := realDir(fullPath)
					if !ok {
						continue
					}
					if _, seen := visited[real]; seen {
						continue
					}
					visited[real] = struct{}{}
				}
				if onDir != nil {
					if err := onDir(fullPath); err != nil {
						return err
					}
				}
				if err := walk(fullPath, stack); err != nil {
					return err
				}
			} else {
				if onFile == nil {
					continue
				}
				if len(excludeRe) > 0 && matchesAny(excludeRe, rel) {
					continue
				}
				if len(includeRe) > 0 && !matchesAny(includeRe, rel) {
					continue
				}
				if info, err := e.Info(); err != nil || info.Size() > maxFileSize {
					continue
				}
				if err := onFile(fullPath); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(dir, nil); err != nil && err != errStopWalk {
		return err
	}
	return nil
}

// walkFiles is walkTree with only a file callback — the common case for
// Search, Replace, and the zoekt indexer's full-build pass.
func walkFiles(dir string, includeRe, excludeRe []*regexp.Regexp, visit func(fullPath string) error) error {
	return walkTree(dir, includeRe, excludeRe, nil, visit)
}

// regexPattern returns query's regex pattern with whole-word/regex wrapping
// applied, before any case-insensitivity is layered on — shared by
// BuildMatcher (which adds the (?i) prefix itself) and buildZoektQuery
// (zoekt_query.go), which expresses case-sensitivity via
// query.Regexp.CaseSensitive instead of an inline flag.
func regexPattern(query string, wholeWord, useRegex bool) string {
	pattern := query
	if !useRegex {
		pattern = regexp.QuoteMeta(query)
	}
	if wholeWord {
		pattern = `\b` + pattern + `\b`
	}
	return pattern
}

func BuildMatcher(query string, caseSensitive, wholeWord, useRegex bool) (*regexp.Regexp, string, error) {
	if useRegex || wholeWord {
		pattern := regexPattern(query, wholeWord, useRegex)
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
	if results, ok := indexSearch(dir, query, caseSensitive, wholeWord, useRegex, include, exclude); ok {
		return results
	}
	includeRe := compileGlobs(include)
	excludeRe := compileGlobs(exclude)
	var results []Result
	_ = walkFiles(dir, includeRe, excludeRe, func(fullPath string) error {
		var f *os.File
		if err := withRetry(3, func() error {
			var openErr error
			f, openErr = os.Open(fullPath)
			return openErr
		}); err != nil {
			return nil
		}
		defer f.Close()
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
			if len(results) >= maxResults {
				return errStopWalk
			}
		}
		return nil
	})
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
	err = walkFiles(dir, includeRe, excludeRe, func(fullPath string) error {
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil
		}
		if strings.ContainsRune(string(content), 0) {
			return nil
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
		return nil
	})
	return changed, err
}

// AllFiles recursively lists files under root (mirrors the Files panel's
// flat-list view), skipping dotfiles and the same heavy dirs as Search.
func AllFiles(root string) []string {
	var files []string
	visited := map[string]struct{}{}
	var walk func(dir string)
	walk = func(dir string) {
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
				if e.Type()&os.ModeSymlink != 0 {
					real, ok := realDir(fullPath)
					if !ok {
						continue
					}
					if _, seen := visited[real]; seen {
						continue
					}
					visited[real] = struct{}{}
				}
				walk(fullPath)
			} else {
				files = append(files, fullPath)
			}
		}
	}
	walk(root)
	return files
}
