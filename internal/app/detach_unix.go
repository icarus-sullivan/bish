//go:build !windows

package app

import "syscall"

// detachAttr gives a spawned child window its own session, so it isn't tied
// to the parent's controlling terminal/process group. Without this, closing
// the first-launched window (often the session/group leader) sends SIGHUP to
// sibling windows sharing that terminal, killing them too.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
