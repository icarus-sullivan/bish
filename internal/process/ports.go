package process

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DetectPorts returns listening ports for the given PID using lsof.
func DetectPorts(pid int) []int {
	out, err := exec.Command("lsof", "-i", "-n", "-P", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var ports []int
	for _, line := range bytes.Split(out, []byte("\n")) {
		s := string(line)
		if !strings.Contains(s, "LISTEN") {
			continue
		}
		// Format: ...:PORT (LISTEN)
		fields := strings.Fields(s)
		if len(fields) < 9 {
			continue
		}
		addr := fields[8]
		idx := strings.LastIndex(addr, ":")
		if idx < 0 {
			continue
		}
		p, err := strconv.Atoi(addr[idx+1:])
		if err != nil || seen[p] {
			continue
		}
		seen[p] = true
		ports = append(ports, p)
	}
	return ports
}
