package app

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// raiseSignal asks another bish window (a separate process) to raise itself.
// There's no cross-desktop-environment API for raising another process's
// window, so the target does it from the inside (see installRaiseHandler).
// SIGUSR1 is already taken by Dock-representation promotion
// (promote_signal_unix.go in package main).
const raiseSignal = syscall.SIGUSR2

// activateWindow signals pid's window to come to the front — used by
// OpenRecentProject/launcher opens to focus an already-open project instead
// of spawning a duplicate window for it. Best effort: Wayland compositors may
// only flash/urgent-hint the window rather than focus it.
func activateWindow(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(raiseSignal) == nil
}

// installRaiseHandler raises this window when another bish process sends
// raiseSignal. The always-on-top toggle nudges window managers that ignore a
// plain show/present request from a background app.
func (a *App) installRaiseHandler() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, raiseSignal)
	go func() {
		for range ch {
			runtime.WindowUnminimise(a.ctx)
			runtime.WindowShow(a.ctx)
			runtime.WindowSetAlwaysOnTop(a.ctx, true)
			time.Sleep(100 * time.Millisecond)
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
		}
	}()
}
