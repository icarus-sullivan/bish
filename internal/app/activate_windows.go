package app

// activateWindow can't bring another process's window to front on Windows —
// no cross-process activation API bish relies on here, same as Linux. The
// caller falls back to spawning a new window.
func activateWindow(pid int) bool {
	return false
}
