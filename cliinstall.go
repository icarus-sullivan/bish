package main

import (
	"os"
	"path/filepath"
)

// installCLICommand symlinks the running app's binary into /usr/local/bin so
// a plain `bish` works in Terminal, the same trick VS Code/Sublime use for
// `code`/`subl`. Runs on every launch, no-ops once linked. Best-effort and
// silent: /usr/local/bin is group-writable by "admin" on a normal single-user
// Mac, so this succeeds without a password prompt in the common case; if it
// can't write (non-admin account, read-only volume, etc.) it just skips —
// never elevates privileges or prompts.
func installCLICommand() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	const target = "/usr/local/bin/bish"
	switch link, err := os.Readlink(target); {
	case err == nil && link == exe:
		return // already installed
	case err == nil:
		os.Remove(target) //nolint // stale link from a previous app location
	case !os.IsNotExist(err):
		return // a real file/dir sits at target — don't clobber it
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return
	}
	os.Symlink(exe, target) //nolint
}
