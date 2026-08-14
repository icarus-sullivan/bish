package process

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// DetectPorts returns listening ports for pid and every descendant of it
// (children, grandchildren, ...). Every bish-managed process is spawned as
// `<shell> -l -c <cmdStr>` (see spawnLocked) — the tracked PID is the login
// shell, and dev-server tools routinely fork again on top of that (npm's
// actual vite/webpack child, a Makefile's sub-make, ...), so the listening
// socket is almost never held by the tracked PID itself. Walking the tree
// is the fix, not a nice-to-have.
func DetectPorts(pid int) []int {
	return detectPorts(pid, processChildren())
}

// detectPorts queries lsof once per PID in the tree. Verified the hard way:
// BSD/macOS lsof silently DROPS THE `-p` FILTER ENTIRELY — dumping every
// socket on the whole system, exit code 0, no error — the moment that PID
// has already exited by the time lsof actually runs (true even for a single
// PID, not just a comma list; a real race for any short-lived child of the
// tracked process). The defense isn't trying to win that race — it's not
// trusting `-p` to have filtered at all: lsofListenPorts cross-checks each
// output row's own PID column against what we asked for and drops anything
// that doesn't match, so a dead PID just contributes nothing instead of
// contributing every process's ports on the machine.
func detectPorts(pid int, children map[int][]int) []int {
	seen := map[int]bool{}
	var ports []int
	for _, p := range pidTree(pid, children) {
		for _, port := range lsofListenPorts(p) {
			if !seen[port] {
				seen[port] = true
				ports = append(ports, port)
			}
		}
	}
	return ports
}

func lsofListenPorts(pid int) []int {
	out, err := exec.Command("lsof", "-i", "-n", "-P", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return nil
	}
	return parseLsofListenPorts(out, pid)
}

// parseLsofListenPorts extracts LISTEN-ing ports for pid from raw `lsof -i
// -n -P -p <pid>` output, cross-checking each row's own PID column against
// pid — see detectPorts' comment for why that check matters (lsof may have
// ignored -p entirely and dumped the whole system).
func parseLsofListenPorts(out []byte, pid int) []int {
	pidStr := strconv.Itoa(pid)
	var ports []int
	for _, line := range bytes.Split(out, []byte("\n")) {
		s := string(line)
		if !strings.Contains(s, "LISTEN") {
			continue
		}
		// Format: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE ...:PORT (LISTEN)
		fields := strings.Fields(s)
		if len(fields) < 9 || fields[1] != pidStr {
			continue // not our PID — lsof ignored -p and dumped everything
		}
		addr := fields[8]
		idx := strings.LastIndex(addr, ":")
		if idx < 0 {
			continue
		}
		if p, err := strconv.Atoi(addr[idx+1:]); err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

// processChildren snapshots the whole system's pid->ppid mapping once,
// cheap enough (one `ps` call) to do it once per Refresh tick — the
// alternative, a per-process recursive `pgrep -P`, means N syscalls per
// tick instead of 1 once there's more than a couple of tracked processes.
func processChildren() map[int][]int {
	out, err := exec.Command("ps", "-Ao", "pid,ppid").Output()
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		lines = lines[1:] // header row
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	return children
}

// pidTree returns root plus every descendant of it, per the children map.
func pidTree(root int, children map[int][]int) []int {
	seen := map[int]bool{}
	var out []int
	var walk func(int)
	walk = func(pid int) {
		if seen[pid] {
			return
		}
		seen[pid] = true
		out = append(out, pid)
		for _, c := range children[pid] {
			walk(c)
		}
	}
	walk(root)
	return out
}
