//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/csullivan/bish/internal/app"
)

// promoteSignal is sent by an exiting Dock-icon-owning window to hand off
// Dock/menu-bar representation to another live bish window — see
// App.Shutdown (internal/app/app.go) and internal/project/instances.go.
const promoteSignal = syscall.SIGUSR1

// installPromoteHandler listens for promoteSignal and, on receipt, takes
// over Dock representation in place (native no-op on Linux — see
// promote_linux.go). Installed by every window, not just child ones: a
// window that was itself promoted earlier can be asked to hand off again
// later, so ChildWindow tracks current role rather than how this process
// was launched.
func installPromoteHandler(a *app.App) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, promoteSignal)
	go func() {
		for range ch {
			a.ChildWindow.Store(false)
			promoteToRegular()
		}
	}()
}

// sendPromoteSignal asks pid's window to take over Dock representation.
func sendPromoteSignal(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Signal(promoteSignal) //nolint
}
