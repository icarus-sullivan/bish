package app

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// External FS changes (git checkout, builds, other editors) refresh the tree;
// the Git panel rides the same tree:update event. fsnotify is non-recursive,
// so the watch set is root + currently-expanded dirs, re-armed after every
// tree reload or toggle.
func (a *App) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return // no watcher — manual refresh still works
	}
	a.fsw = w
	go func() {
		// debounce bursts (builds touch many files at once) but cap the wait
		// with maxWait — a sustained stream of events under 300ms apart
		// (e.g. rsync copying many small files) would otherwise keep
		// resetting debounce forever and never actually reload.
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
			case <-a.ctx.Done():
				w.Close() //nolint
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op == fsnotify.Chmod {
					continue
				}
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
				a.reloadTree()
			case <-maxWaitC:
				debounce, maxWait = nil, nil
				a.reloadTree()
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	a.rearmWatcher()
}

// rearmWatcher replaces the watch set with root + expanded dirs.
func (a *App) rearmWatcher() {
	if a.fsw == nil {
		return
	}
	a.projectMu.Lock()
	root := a.projectRoot
	remote := a.remoteDest != ""
	a.projectMu.Unlock()
	if remote {
		return // no local inotify equivalent over SSH — manual refresh only
	}
	if root == "" {
		root = a.cwd
	}
	a.treeMu.Lock()
	dirs := append([]string{root}, a.fileTree.ExpandedPaths()...)
	// extra workspace roots: watch each root dir itself (top-level changes
	// only) — expanded-subdir tracking for them is a v1 cut, manual refresh
	// covers it via the "Collapse All" / re-expand path
	dirs = append(dirs, a.extraRoots...)
	a.treeMu.Unlock()
	for _, p := range a.fsw.WatchList() {
		a.fsw.Remove(p) //nolint
	}
	for _, d := range dirs {
		if skipDirs[filepath.Base(d)] {
			continue
		}
		a.fsw.Add(d) //nolint
	}
}
