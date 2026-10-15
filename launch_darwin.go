package main

/*
// Forward declaration only — definition lives in launch_impl_darwin.go
void bishObserveLaunchC(void);
*/
import "C"

import "sync"

var (
	launchSettled     = make(chan struct{})
	launchSettledOnce sync.Once
)

//export goLaunchSettled
func goLaunchSettled() {
	launchSettledOnce.Do(func() { close(launchSettled) })
}

// observeLaunch returns a channel that closes once NSApplication finishes
// launching. AppKit delivers cold-start application:openFile: events (Dock
// Recent items, Finder Open With) before didFinishLaunching, so App waits on
// this before deciding between the requested path and session restore. Must
// be called before wails.Run so the observer exists when AppKit posts it.
func observeLaunch() <-chan struct{} {
	C.bishObserveLaunchC()
	return launchSettled
}
