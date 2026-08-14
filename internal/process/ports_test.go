package process

import (
	"reflect"
	"sort"
	"testing"
)

// Regression test for a real bug found while building this feature: BSD/
// macOS lsof silently drops the -p filter entirely (dumping every socket on
// the system, exit 0) when the target PID has already exited — reproduced
// live with `lsof -i -n -P -p 99999999`. parseLsofListenPorts must ignore
// any row whose own PID column doesn't match, or a single dead child in a
// process tree would attribute the whole machine's open ports to it.
func TestParseLsofListenPortsIgnoresRowsForOtherPIDs(t *testing.T) {
	// realistic shape of what lsof prints when it ignores -p and dumps
	// everything: our target pid (4242) mixed in with unrelated processes.
	out := []byte(`COMMAND     PID      USER   FD   TYPE             DEVICE SIZE/OFF      NODE NAME
ollama      590 csullivan    3u  IPv4 0xd8fba1f515240812      0t0       TCP 127.0.0.1:11434 (LISTEN)
Brave\x20  1081 csullivan   31u  IPv6 0x914f8e8b68d701a3      0t0       TCP [::1]:49403->[::1]:5173 (ESTABLISHED)
node       4242 csullivan   20u  IPv6 0xdc40baa861aa7a54      0t0       TCP [::1]:5173 (LISTEN)
node       4242 csullivan   36u  IPv6 0xb621af85cb17e1b9      0t0       TCP [::1]:5173->[::1]:49403 (ESTABLISHED)
OrbStack    671 csullivan  107u  IPv4 0x6613df0648188b92      0t0       TCP 127.0.0.1:32222 (LISTEN)
`)
	got := parseLsofListenPorts(out, 4242)
	want := []int{5173}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLsofListenPorts(pid=4242) = %v, want %v (ollama:11434 and OrbStack:32222 belong to other PIDs and must be excluded)", got, want)
	}
}

func TestParseLsofListenPortsNoMatchingRows(t *testing.T) {
	out := []byte(`COMMAND     PID      USER   FD   TYPE             DEVICE SIZE/OFF      NODE NAME
ollama      590 csullivan    3u  IPv4 0xd8fba1f515240812      0t0       TCP 127.0.0.1:11434 (LISTEN)
`)
	if got := parseLsofListenPorts(out, 4242); got != nil {
		t.Fatalf("parseLsofListenPorts(pid=4242) = %v, want nil", got)
	}
}

func TestPidTreeWalksAllDescendants(t *testing.T) {
	// 1 -> 2 -> 4
	//   -> 3
	// 5 (unrelated, must not appear)
	children := map[int][]int{
		1: {2, 3},
		2: {4},
		5: {6},
	}
	got := pidTree(1, children)
	sort.Ints(got)
	want := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pidTree(1) = %v, want %v", got, want)
	}
}

func TestPidTreeHandlesCycleWithoutHanging(t *testing.T) {
	// pathological/corrupt ps snapshot: 1 -> 2 -> 1
	children := map[int][]int{
		1: {2},
		2: {1},
	}
	got := pidTree(1, children)
	sort.Ints(got)
	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pidTree(1) = %v, want %v", got, want)
	}
}

func TestPidTreeLeafOnly(t *testing.T) {
	got := pidTree(42, map[int][]int{})
	want := []int{42}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pidTree(42) = %v, want %v", got, want)
	}
}
