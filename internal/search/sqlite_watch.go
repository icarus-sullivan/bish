package search

import (
	"bufio"
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// rootWatcher watches every directory under a rootIndex's root — fsnotify is
// non-recursive, so every directory needs its own Add, unlike
// internal/app/watch.go's watcher (root + only the UI's *expanded* dirs,
// which is the wrong shape for an index that must see the whole tree) — and
// folds bursts of changes into debounced re-indexes. Same
// 300ms-settle/2s-max-wait shape as watch.go, so a burst of edits (a build, a
// branch switch) collapses into one transaction instead of one per file.
//
// Directories that walkTree would skip (skipDirs, gitignored, hidden when
// IncludeHidden is off) are never Add()'ed in the first place, so events
// from inside them structurally can't arrive here — no separate check
// needed for those. Known gap: editing .gitignore mid-session doesn't
// retroactively re-evaluate already-indexed files; not worth chasing for an
// edge case this narrow.
type rootWatcher struct {
	idx  *rootIndex
	w    *fsnotify.Watcher
	done chan struct{}
}

func newRootWatcher(idx *rootIndex) *rootWatcher {
	return &rootWatcher{idx: idx}
}

func (rw *rootWatcher) start() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return // no watcher — index just goes stale until the next full rebuild (process restart)
	}
	rw.w = w
	rw.done = make(chan struct{})

	_ = walkTree(rw.idx.root, nil, nil, func(dir string) error {
		w.Add(dir) //nolint
		return nil
	}, nil)
	w.Add(rw.idx.root) //nolint

	go rw.loop()
}

func (rw *rootWatcher) stop() {
	if rw.w == nil {
		return
	}
	close(rw.done)
	rw.w.Close() //nolint
}

func (rw *rootWatcher) loop() {
	pending := map[string]struct{}{}
	var debounce, maxWait *time.Timer
	for {
		var debounceC, maxWaitC <-chan time.Time
		if debounce != nil {
			debounceC = debounce.C
		}
		if maxWait != nil {
			maxWaitC = maxWait.C
		}
		select {
		case <-rw.done:
			return
		case ev, ok := <-rw.w.Events:
			if !ok {
				return
			}
			if ev.Op == fsnotify.Chmod {
				continue
			}
			if isNewDir(ev) {
				rw.w.Add(ev.Name) //nolint
			}
			pending[ev.Name] = struct{}{}
			if debounce == nil {
				debounce = time.NewTimer(300 * time.Millisecond)
			} else {
				debounce.Reset(300 * time.Millisecond)
			}
			if maxWait == nil {
				maxWait = time.NewTimer(2 * time.Second)
			}
		case <-debounceC:
			debounce, maxWait = nil, nil
			rw.flush(pending)
			pending = map[string]struct{}{}
		case <-maxWaitC:
			debounce, maxWait = nil, nil
			rw.flush(pending)
			pending = map[string]struct{}{}
		case _, ok := <-rw.w.Errors:
			if !ok {
				return
			}
		}
	}
}

// isNewDir reports whether ev is a newly created directory, which needs its
// own fsnotify.Add before its contents can be watched.
func isNewDir(ev fsnotify.Event) bool {
	if ev.Op&fsnotify.Create == 0 {
		return false
	}
	info, err := os.Stat(ev.Name)
	return err == nil && info.IsDir()
}

// flush re-indexes every path in changed within one transaction: delete
// whatever rows that file previously had, then (if the file still exists and
// is still index-eligible) insert its current lines fresh. Plain
// delete-then-reinsert rather than any shard/tombstone bookkeeping — SQLite
// makes that the easy, obviously-correct option here.
func (rw *rootWatcher) flush(changed map[string]struct{}) {
	if len(changed) == 0 || rw.idx.db == nil {
		return
	}
	tx, err := rw.idx.db.Begin()
	if err != nil {
		return
	}
	for full := range changed {
		rel, err := filepath.Rel(rw.idx.root, full)
		if err != nil {
			continue
		}
		reindexOneFile(tx, filepath.ToSlash(rel), full)
	}
	_ = tx.Commit()
}

// reindexOneFile deletes rel's existing rows (if any) and, if full still
// exists on disk and is index-eligible, inserts its current lines. A path
// that fsnotify reported but that's no longer eligible (deleted, grown past
// maxFileSize, now hidden with IncludeHidden off) just ends up deleted and
// not reinserted, which is correct either way.
func reindexOneFile(tx *sql.Tx, rel, full string) {
	var fileID int64
	if err := tx.QueryRow(`SELECT id FROM files WHERE path = ?`, rel).Scan(&fileID); err == nil {
		tx.Exec(`DELETE FROM lines_fts WHERE rowid IN (SELECT id FROM file_lines WHERE file_id = ?)`, fileID) //nolint
		tx.Exec(`DELETE FROM file_lines WHERE file_id = ?`, fileID)                                           //nolint
		tx.Exec(`DELETE FROM files WHERE id = ?`, fileID)                                                     //nolint
	}

	if !IncludeHidden {
		for _, part := range strings.Split(rel, "/") {
			if strings.HasPrefix(part, ".") {
				return
			}
		}
	}

	var content []byte
	if err := withRetry(3, func() error {
		var readErr error
		content, readErr = os.ReadFile(full)
		return readErr
	}); err != nil || bytes.IndexByte(content, 0) >= 0 || int64(len(content)) > maxFileSize {
		return
	}

	res, err := tx.Exec(`INSERT INTO files(path) VALUES (?)`, rel)
	if err != nil {
		return
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		lr, err := tx.Exec(`INSERT INTO file_lines(file_id, lineno, text) VALUES (?, ?, ?)`, newID, lineno, line)
		if err != nil {
			continue
		}
		rowID, err := lr.LastInsertId()
		if err != nil {
			continue
		}
		tx.Exec(`INSERT INTO lines_fts(rowid, text) VALUES (?, ?)`, rowID, line) //nolint
	}
}
