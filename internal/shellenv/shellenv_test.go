package shellenv

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultShell(t *testing.T) {
	if DefaultShell() == "" {
		t.Fatal("DefaultShell returned empty")
	}
	t.Setenv("SHELL", "")
	if s := DefaultShell(); s == "" {
		t.Fatalf("DefaultShell with no $SHELL returned empty")
	}
}

func TestLoadLoginEnv(t *testing.T) {
	// Simulate launchd's minimal GUI PATH, then recover via login shell.
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	LoadLoginEnv(DefaultShell())
	p := os.Getenv("PATH")
	if !strings.Contains(p, "/usr/bin") {
		t.Fatalf("PATH lost /usr/bin: %q", p)
	}
	if p == "/usr/bin:/bin:/usr/sbin:/sbin" {
		t.Fatalf("PATH not enriched by login shell (rc files may be empty on this machine): %q", p)
	}
}
