//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// setProcAttrs puts cmd in its own process group so killGroup can take down
// everything it spawns — a shell running `npm run dev`, `make start`, etc.
// commonly forks children (bundlers, dev servers) that survive the shell
// itself getting killed and keep holding their port.
func setProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup kills pid's whole process group (pid is the group leader, set up
// by setProcAttrs at spawn time), not just pid itself.
func killGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
