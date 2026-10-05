package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/csullivan/bish/internal/remote"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ErrFileConflict prefixes WriteFileChecked's error when the file changed on
// disk since the editor loaded it; the frontend matches on this prefix.
const ErrFileConflict = "conflict: file changed on disk"

// StatMtime returns path's modification time in Unix nanoseconds, or 0 when
// it can't be known (missing file, remote project). 0 disables the editor's
// conflict check rather than failing it.
func (a *App) StatMtime(path string) int64 {
	if a.remoteDest != "" {
		return 0
	}
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}

// WriteFileChecked saves content only if path's mtime still equals
// expectedMtime (skipped when expectedMtime is 0), so a save never silently
// clobbers a change made by git, a formatter, or another editor. Returns the
// new mtime on success.
func (a *App) WriteFileChecked(path, content string, expectedMtime int64) (int64, error) {
	if a.remoteDest != "" {
		return 0, remote.WriteFile(a.remoteDest, path, content)
	}
	if expectedMtime != 0 {
		if cur := a.StatMtime(path); cur != 0 && cur != expectedMtime {
			return 0, errors.New(ErrFileConflict)
		}
	}
	if err := writeFileAtomic(path, []byte(content)); err != nil {
		return 0, err
	}
	return a.StatMtime(path), nil
}

// ConfirmFileConflict asks how to resolve a save against a file that changed
// on disk. Returns "overwrite", "reload", or "cancel".
func (a *App) ConfirmFileConflict(name string) string {
	choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.WarningDialog,
		Title:         "File Changed on Disk",
		Message:       fmt.Sprintf("%s was changed outside the editor since you opened it.\n\nOverwrite it with your version, or reload and discard your edits?", name),
		Buttons:       []string{"Overwrite", "Reload", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return "cancel"
	}
	switch choice {
	case "Overwrite":
		return "overwrite"
	case "Reload":
		return "reload"
	}
	return "cancel"
}

// writeFileAtomic writes via a temp file + rename in the same directory, so a
// crash or full disk mid-save never leaves a truncated file. Symlinks are
// resolved first (the rename must replace the target, not the link) and the
// original permission bits are kept.
func writeFileAtomic(path string, data []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".bish-*")
	if err != nil {
		// read-only dir but writable file: fall back to an in-place write
		return os.WriteFile(path, data, mode)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp) //nolint
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close() //nolint
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close() //nolint
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
