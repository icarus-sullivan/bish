package main

// installQuitIntercept is a no-op on Windows — there's no AppDelegate-style
// hook to distinguish Quit from closing a single window.
func installQuitIntercept() {}
