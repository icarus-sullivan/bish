// Package search's SQLite-FTS5-trigram index: a per-project-root cache that
// makes repeat searches on large repos index-bound instead of walk-bound.
// SQLite only narrows candidates (a fast trigram MATCH); every candidate
// line is still re-verified in Go against the exact same BuildMatcher
// pattern the brute-force path uses, so the two paths can never disagree on
// what counts as a match — see indexSearch in sqlite_query.go.
//
// modernc.org/sqlite (not mattn/go-sqlite3) deliberately: pure Go, no cgo,
// no WASM runtime, a small and stable transitive dependency graph, verified
// directly before adopting it (an earlier attempt at this feature used
// sourcegraph/zoekt, which turned out to statically link Sentry, Zap,
// cockroachdb/errors, and a full WASM JIT runtime regardless of config —
// reverted before it shipped).
package search

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// staleAfter is how long an on-disk index can sit unused before
// PruneStaleIndexes removes it.
const staleAfter = 5 * 24 * time.Hour

const schema = `
CREATE TABLE IF NOT EXISTS files (
	id   INTEGER PRIMARY KEY,
	path TEXT UNIQUE NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);

CREATE TABLE IF NOT EXISTS file_lines (
	id      INTEGER PRIMARY KEY,
	file_id INTEGER NOT NULL,
	lineno  INTEGER NOT NULL,
	text    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_lines_file ON file_lines(file_id);

CREATE VIRTUAL TABLE IF NOT EXISTS lines_fts USING fts5(
	text,
	content='file_lines',
	content_rowid='id',
	tokenize='trigram case_sensitive 0'
);
`

var (
	indexMu sync.Mutex
	indexes = map[string]*rootIndex{}
)

// rootIndex is the live index (and its supporting watcher) for one search
// root. Search consults it via indexSearch; a root that's never been
// searched, or whose initial build hasn't finished, has no live rootIndex
// (or one with ready == false) and Search transparently falls back to the
// brute-force walk.
type rootIndex struct {
	root   string
	dbPath string

	buildOnce sync.Once
	ready     atomic.Bool
	db        *sql.DB // set once ready

	watcher *rootWatcher
}

// searchIndexRoot returns the directory holding all per-project index
// caches, mirroring the ~/.config/bish/... convention used elsewhere
// (internal/config, internal/project).
func searchIndexRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bish-search-index")
	}
	return filepath.Join(home, ".config", "bish", "search-index")
}

// indexDirFor maps an absolute root path to its on-disk index directory.
// Hashing the resolved path (not the project name) avoids collisions between
// same-named projects in different locations.
func indexDirFor(absRoot string) string {
	h := sha1.Sum([]byte(absRoot)) //nolint:gosec // content-addressing a path, not security-sensitive
	return filepath.Join(searchIndexRoot(), hex.EncodeToString(h[:]))
}

// ensureIndex returns the rootIndex for dir, lazily starting an async full
// build on the first call for that root. Always returns a non-nil handle;
// callers must still check ready.Load() before trusting it for results.
func ensureIndex(dir string) *rootIndex {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	abs = filepath.Clean(abs)

	indexMu.Lock()
	idx, ok := indexes[abs]
	if !ok {
		idx = &rootIndex{root: abs, dbPath: filepath.Join(indexDirFor(abs), "index.db")}
		indexes[abs] = idx
	}
	indexMu.Unlock()

	idx.buildOnce.Do(func() {
		go idx.buildFull()
	})
	return idx
}

// ReconcileIndexRoots closes and unregisters any index whose root isn't in
// desiredRoots — called from internal/app on project open/switch/close so
// watcher goroutines and fds don't accumulate across a long session. Not
// required for correctness: a root dropped here just gets a fresh lazy build
// the next time it's searched.
func ReconcileIndexRoots(desiredRoots []string) {
	want := make(map[string]struct{}, len(desiredRoots))
	for _, r := range desiredRoots {
		if abs, err := filepath.Abs(r); err == nil {
			want[filepath.Clean(abs)] = struct{}{}
		}
	}

	indexMu.Lock()
	var drop []*rootIndex
	for root, idx := range indexes {
		if _, ok := want[root]; !ok {
			drop = append(drop, idx)
			delete(indexes, root)
		}
	}
	indexMu.Unlock()

	for _, idx := range drop {
		idx.close()
	}
}

func (idx *rootIndex) close() {
	if idx.watcher != nil {
		idx.watcher.stop()
	}
	if idx.db != nil {
		idx.db.Close() //nolint
	}
}

// buildFull walks idx.root, inserts every eligible file's lines into a fresh
// SQLite+FTS5 index, then opens it for search and starts the change watcher.
// Runs once per root (via buildOnce); on any failure it simply leaves ready
// false forever for this process — Search keeps using the brute-force
// fallback, which is always correct, just not accelerated.
func (idx *rootIndex) buildFull() {
	dir := filepath.Dir(idx.dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	// Open + schema creation retried together: a corporate AV/EDR real-time
	// scanner locking the freshly-created file mid-write is the likeliest
	// way this ends up corrupted ("file is not a database") rather than
	// just failing to open — a plain open failure wouldn't explain that.
	// Removing and starting over on each attempt discards any such half
	// written file instead of leaving it behind for the next process to
	// trip over.
	var db *sql.DB
	err := withRetry(3, func() error {
		os.Remove(idx.dbPath) //nolint // stale/half-written index; rebuilding fresh is simpler than reconciling
		var openErr error
		db, openErr = openIndexDB(idx.dbPath)
		if openErr != nil {
			return openErr
		}
		if _, execErr := db.Exec(schema); execErr != nil {
			db.Close() //nolint
			db = nil
			return execErr
		}
		return nil
	})
	if err != nil {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		db.Close() //nolint
		return
	}
	insertFile, err := tx.Prepare(`INSERT INTO files(path) VALUES (?)`)
	if err != nil {
		tx.Rollback() //nolint
		db.Close()    //nolint
		return
	}
	insertLine, err := tx.Prepare(`INSERT INTO file_lines(file_id, lineno, text) VALUES (?, ?, ?)`)
	if err != nil {
		insertFile.Close() //nolint
		tx.Rollback()      //nolint
		db.Close()         //nolint
		return
	}
	insertFTS, err := tx.Prepare(`INSERT INTO lines_fts(rowid, text) VALUES (?, ?)`)
	if err != nil {
		insertFile.Close() //nolint
		insertLine.Close() //nolint
		tx.Rollback()      //nolint
		db.Close()         //nolint
		return
	}

	_ = walkFiles(idx.root, nil, nil, func(fullPath string) error {
		rel, err := filepath.Rel(idx.root, fullPath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		indexFileLines(insertFile, insertLine, insertFTS, rel, fullPath)
		return nil
	})

	insertFile.Close() //nolint
	insertLine.Close() //nolint
	insertFTS.Close()  //nolint

	if err := tx.Commit(); err != nil {
		db.Close() //nolint
		return
	}

	idx.db = db
	idx.ready.Store(true)

	idx.watcher = newRootWatcher(idx)
	idx.watcher.start()
}

// indexFileLines reads fullPath and inserts its lines into the
// files/file_lines/lines_fts tables via the given prepared statements
// (buildFull's bulk-insert path). Best effort: an unreadable or binary file
// is silently skipped — same rule walkFiles/Search/Replace already apply.
// Uses the same bufio.Scanner line-splitting (and buffer sizing, for long
// minified lines) as Search's brute-force scan, so index and fallback can
// never disagree about where a line starts or ends.
func indexFileLines(insertFile, insertLine, insertFTS *sql.Stmt, rel, fullPath string) {
	var content []byte
	if err := withRetry(3, func() error {
		var readErr error
		content, readErr = os.ReadFile(fullPath)
		return readErr
	}); err != nil || bytes.IndexByte(content, 0) >= 0 {
		return
	}
	res, err := insertFile.Exec(rel)
	if err != nil {
		return
	}
	fileID, err := res.LastInsertId()
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		lr, err := insertLine.Exec(fileID, lineno, line)
		if err != nil {
			continue
		}
		rowID, err := lr.LastInsertId()
		if err != nil {
			continue
		}
		_, _ = insertFTS.Exec(rowID, line)
	}
}

// PruneStaleIndexes removes on-disk index directories under
// searchIndexRoot() that are both (a) not currently registered as an active
// root in this process and (b) untouched for staleAfter. Call once on
// startup — bish is a desktop app, not a daemon, so there's no long-lived
// cron to hang a periodic check off of; a startup sweep gets the same
// practical effect.
func PruneStaleIndexes() {
	root := searchIndexRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	indexMu.Lock()
	active := make(map[string]struct{}, len(indexes))
	for _, idx := range indexes {
		active[filepath.Base(filepath.Dir(idx.dbPath))] = struct{}{}
	}
	indexMu.Unlock()

	cutoff := time.Now().Add(-staleAfter)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := active[e.Name()]; ok {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if newestModTime(dir).Before(cutoff) {
			os.RemoveAll(dir) //nolint
		}
	}
}

// newestModTime returns the most recent modification time of any entry
// directly inside dir (not recursive — an index dir just holds index.db and
// its WAL/SHM siblings).
func newestModTime(dir string) time.Time {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// openIndexDB opens path with the settings every rootIndex/watcher write
// through: WAL for crash resilience, and a single connection so SQLite never
// sees concurrent writers (this app's read/write volume doesn't need a
// pool — simplicity and correctness win over squeezing out concurrency here).
func openIndexDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close() //nolint
		return nil, err
	}
	return db, nil
}
