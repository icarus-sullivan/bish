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
		var timer *time.Timer
		for {
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
				// debounce bursts (builds touch many files at once)
				if timer == nil {
					timer = time.AfterFunc(300*time.Millisecond, a.reloadTree)
				} else {
					timer.Reset(300 * time.Millisecond)
				}
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
	a.projectMu.Unlock()
	if root == "" {
		root = a.cwd
	}
	a.treeMu.Lock()
	dirs := append([]string{root}, a.fileTree.ExpandedPaths()...)
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
