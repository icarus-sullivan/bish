package langext

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var fixPathOnce sync.Once

// FixPath augments the process PATH with the user's login-shell PATH. A
// GUI-launched app (double-clicked .app, no parent terminal) inherits
// launchd's bare PATH; nvm/pnpm/goenv/rustup shims that only land on PATH
// once .zshrc/.zprofile run are otherwise invisible to exec.LookPath and to
// every spawned command (server/formatter binaries and their installers
// alike). Best-effort and cached for the process lifetime; any failure just
// leaves the inherited PATH in place. Shared by every langext-driven spawn
// (lsp.Manager and FormatterManager both call this) so there's one PATH-fix
// implementation, not one per spawner.
func FixPath() {
	fixPathOnce.Do(func() {
		if runtime.GOOS == "windows" {
			return
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		const startMarker, endMarker = "__BISH_PATH_START__", "__BISH_PATH_END__"
		cmd := exec.Command(shell, "-ilc", "echo "+startMarker+"$PATH"+endMarker)
		out, err := cmd.Output()
		if err != nil {
			return
		}
		s := string(out)
		start := strings.Index(s, startMarker)
		end := strings.Index(s, endMarker)
		if start == -1 || end == -1 || end < start {
			return
		}
		shellPath := strings.TrimSpace(s[start+len(startMarker) : end])
		if shellPath == "" {
			return
		}
		os.Setenv("PATH", mergePath(os.Getenv("PATH"), shellPath))
	})
}

// mergePath unions two PATH strings, preserving first-seen order and
// dropping duplicates (current PATH wins ties over the shell-derived one).
func mergePath(current, extra string) string {
	seen := make(map[string]bool)
	var parts []string
	for _, p := range append(strings.Split(current, ":"), strings.Split(extra, ":")...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		parts = append(parts, p)
	}
	return strings.Join(parts, ":")
}
