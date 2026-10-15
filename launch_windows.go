package main

// observeLaunch returns nil on Windows — paths arrive as CLI args, so
// startup resolves at once.
func observeLaunch() <-chan struct{} { return nil }
