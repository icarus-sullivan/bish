package remote

import (
	"reflect"
	"testing"
)

func TestParseDest(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantDest string
		wantArgs []string
	}{
		{
			name:     "plain user@host",
			raw:      "root@216.243.220.223",
			wantDest: "root@216.243.220.223",
			wantArgs: nil,
		},
		{
			name:     "ssh config alias",
			raw:      "lora-box",
			wantDest: "lora-box",
			wantArgs: nil,
		},
		{
			name:     "pasted ssh command with port and identity file",
			raw:      "ssh root@216.243.220.223 -p 16338 -i ~/.ssh/id_ed25519",
			wantDest: "root@216.243.220.223",
			wantArgs: []string{"-p", "16338", "-i", "~/.ssh/id_ed25519"},
		},
		{
			name:     "pasted command without leading ssh",
			raw:      "root@216.243.220.223 -p 16338 -i ~/.ssh/id_ed25519",
			wantDest: "root@216.243.220.223",
			wantArgs: []string{"-p", "16338", "-i", "~/.ssh/id_ed25519"},
		},
		{
			name:     "flags before the host",
			raw:      "ssh -p 16338 -i ~/.ssh/id_ed25519 root@216.243.220.223",
			wantDest: "root@216.243.220.223",
			wantArgs: []string{"-p", "16338", "-i", "~/.ssh/id_ed25519"},
		},
		{
			name:     "runpod ssh.runpod.io style with -i only",
			raw:      "ssh kkchorqq219jv6-64411fef@ssh.runpod.io -i ~/.ssh/id_ed25519",
			wantDest: "kkchorqq219jv6-64411fef@ssh.runpod.io",
			wantArgs: []string{"-i", "~/.ssh/id_ed25519"},
		},
		{
			name:     "empty",
			raw:      "",
			wantDest: "",
			wantArgs: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest, args := ParseDest(c.raw)
			if dest != c.wantDest {
				t.Errorf("dest = %q, want %q", dest, c.wantDest)
			}
			if !reflect.DeepEqual(args, c.wantArgs) {
				t.Errorf("args = %v, want %v", args, c.wantArgs)
			}
		})
	}
}

func TestShortDest(t *testing.T) {
	got := ShortDest("ssh root@216.243.220.223 -p 16338 -i ~/.ssh/id_ed25519")
	want := "root@216.243.220.223"
	if got != want {
		t.Errorf("ShortDest = %q, want %q", got, want)
	}
}
