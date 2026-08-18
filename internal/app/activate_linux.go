package app

// activateWindow can't bring another process's window to front on Linux —
// no cross-desktop-environment API bish can rely on. The caller falls back
// to spawning a new window, same "no Dock menu" precedent dock_linux.go
// already sets for this platform.
func activateWindow(pid int) bool {
	return false
}
