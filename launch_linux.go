package main

// observeLaunch returns nil on Linux — there are no native openFile events to
// wait for; launcher paths arrive as CLI args, so startup resolves at once.
func observeLaunch() <-chan struct{} { return nil }
