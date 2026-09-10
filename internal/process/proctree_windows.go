//go:build windows

package process

import (
	"os/exec"
	"strconv"
)

// setProcAttrs is a no-op on Windows — killGroup uses taskkill's process-tree
// flag instead of a POSIX process-group signal.
func setProcAttrs(cmd *exec.Cmd) {}

// killGroup kills pid and its full descendant tree via taskkill, since
// Windows has no equivalent of POSIX's negative-pid group kill.
func killGroup(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
