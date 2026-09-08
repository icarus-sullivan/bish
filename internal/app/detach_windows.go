//go:build windows

package app

import "syscall"

// detachAttr is a no-op on Windows: processes aren't tied to a Unix-style
// controlling terminal/session in the way that causes the sibling-window
// SIGHUP issue on Linux/macOS.
func detachAttr() *syscall.SysProcAttr {
	return nil
}
