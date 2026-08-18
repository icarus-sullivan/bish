package main

import "github.com/csullivan/bish/internal/app"

// promoteToRegular is a no-op on Windows — there's no Dock/activation-policy
// concept to promote into.
func promoteToRegular() {}

// installPromoteHandler and sendPromoteSignal are no-ops on Windows — the
// Dock-icon handoff they implement on macOS (promote_signal_unix.go) has no
// Windows equivalent: windows aren't collapsed under one icon there, so
// there's nothing to hand off.
func installPromoteHandler(a *app.App) {}

func sendPromoteSignal(pid int) {}
