package search

import (
	"path/filepath"
	"regexp/syntax"
	"strings"
)

// maxCandidates bounds how many FTS-matched lines indexSearch pulls before
// re-verifying in Go. Larger than maxResults on purpose: a literal extracted
// from a bigger regex (see ftsLiteral) can match far more lines than the
// full pattern eventually keeps, so there needs to be room for the
// re-verification pass to filter down to maxResults real matches.
const maxCandidates = maxResults * 4

// indexSearch answers a search from dir's live index if one exists, has
// finished its initial build, and the query can be safely narrowed by the
// trigram index; ok is false whenever the caller should fall back to the
// brute-force walk. SQLite only narrows candidate lines — every candidate is
// re-verified here with the exact same BuildMatcher pattern the brute-force
// path uses, so the two paths can never disagree about what counts as a
// match.
func indexSearch(dir, query string, caseSensitive, wholeWord, useRegex bool, include, exclude string) ([]Result, bool) {
	idx := ensureIndex(dir)
	if idx == nil || !idx.ready.Load() || idx.db == nil {
		return nil, false
	}

	literal, ok := ftsLiteral(query, wholeWord, useRegex)
	if !ok {
		return nil, false
	}

	re, plain, err := BuildMatcher(query, caseSensitive, wholeWord, useRegex)
	if err != nil {
		return nil, false
	}

	rows, err := idx.db.Query(
		`SELECT f.path, l.lineno, l.text
		 FROM lines_fts
		 JOIN file_lines l ON l.id = lines_fts.rowid
		 JOIN files f ON f.id = l.file_id
		 WHERE lines_fts MATCH ?
		 LIMIT ?`,
		ftsPhraseQuery(literal), maxCandidates,
	)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	includeRe := compileGlobs(include)
	excludeRe := compileGlobs(exclude)
	var results []Result
	for rows.Next() {
		var path string
		var lineno int
		var text string
		if err := rows.Scan(&path, &lineno, &text); err != nil {
			continue
		}
		rel := filepath.ToSlash(path)
		if len(excludeRe) > 0 && matchesAny(excludeRe, rel) {
			continue
		}
		if len(includeRe) > 0 && !matchesAny(includeRe, rel) {
			continue
		}

		var col int
		if re != nil {
			loc := re.FindStringIndex(text)
			if loc == nil {
				// FTS over-approximated (case-insensitive superset, or a
				// literal pulled out of a larger regex) — this candidate
				// doesn't actually match; never trust the index over the
				// real pattern.
				continue
			}
			col = loc[0]
		} else {
			haystack := text
			if !caseSensitive {
				haystack = strings.ToLower(haystack)
			}
			col = strings.Index(haystack, plain)
			if col < 0 {
				continue
			}
		}

		results = append(results, Result{File: filepath.Join(dir, path), Line: lineno, Col: col, Text: text})
		if len(results) >= maxResults {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false
	}
	return results, true
}

// ftsLiteral returns the exact literal substring an index search can safely
// narrow query to, or ok=false when the caller must fall back to a
// brute-force scan for this particular search:
//   - plain query, or a whole-word-wrapped plain query: the query itself —
//     a whole-word match still requires that literal substring to appear
//     somewhere in the line.
//   - useRegex: only when the parsed pattern (regexPattern's output, so
//     whole-word wrapping/anchors are already folded in) is nothing but a
//     literal plus optional anchors/word-boundaries. Real regex structure
//     (alternation, character classes, quantifiers, wildcards) can't be
//     safely reduced to a required literal, so that search just isn't
//     accelerated — correct, not a bug, and the same limitation any
//     trigram-index tool has for regexes without an extractable literal.
//
// A literal under 3 characters can't usefully drive a trigram index either
// way, so that also falls back.
func ftsLiteral(query string, wholeWord, useRegex bool) (string, bool) {
	literal := query
	if useRegex {
		re, err := syntax.Parse(regexPattern(query, wholeWord, useRegex), syntax.Perl)
		if err != nil {
			return "", false
		}
		lit, ok := literalFromRegexAST(re)
		if !ok {
			return "", false
		}
		literal = lit
	}
	if len(literal) < 3 {
		return "", false
	}
	return literal, true
}

// literalFromRegexAST returns the exact literal re requires, if re's parsed
// form is nothing but that literal plus optional anchors/word boundaries.
func literalFromRegexAST(re *syntax.Regexp) (string, bool) {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune), true
	case syntax.OpBeginText, syntax.OpEndText, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return "", true
	case syntax.OpConcat:
		var b strings.Builder
		for _, sub := range re.Sub {
			lit, ok := literalFromRegexAST(sub)
			if !ok {
				return "", false
			}
			b.WriteString(lit)
		}
		if b.Len() == 0 {
			return "", false
		}
		return b.String(), true
	default:
		return "", false
	}
}

// ftsPhraseQuery wraps literal as an FTS5 phrase query, which — with the
// trigram tokenizer — means "substring match". Internal double quotes are
// doubled per FTS5's phrase-text escaping rule.
func ftsPhraseQuery(literal string) string {
	return `"` + strings.ReplaceAll(literal, `"`, `""`) + `"`
}
