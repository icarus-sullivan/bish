package langext

import (
	"os"
	"os/exec"
	"path/filepath"
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
		if out, err := cmd.Output(); err == nil {
			s := string(out)
			start := strings.Index(s, startMarker)
			end := strings.Index(s, endMarker)
			if start != -1 && end != -1 && end > start {
				if shellPath := strings.TrimSpace(s[start+len(startMarker) : end]); shellPath != "" {
					os.Setenv("PATH", mergePath(os.Getenv("PATH"), shellPath))
				}
			}
		}
		ensurePnpmHome()
	})
}

// ensurePnpmHome sets PNPM_HOME (and adds it to PATH) when pnpm has never
// had its global bin dir configured — the common case when pnpm was
// installed via corepack/nvm/homebrew but the user never ran `pnpm setup`.
// Without this, every "pnpm add -g" (the installer for the JS/TS/Svelte/Bash
// language servers) fails with "Run \"pnpm setup\" to create it
// automatically...". Only kicks in when nothing is already configured, so a
// deliberate PNPM_HOME or `pnpm config` setting is never overridden.
func ensurePnpmHome() {
	if os.Getenv("PNPM_HOME") != "" {
		return
	}
	if _, err := exec.LookPath("pnpm"); err != nil {
		return
	}
	if out, err := exec.Command("pnpm", "config", "get", "global-bin-dir").Output(); err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" && dir != "undefined" {
			return
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".local", "share", "pnpm")
	if runtime.GOOS == "darwin" {
		dir = filepath.Join(home, "Library", "pnpm")
	}
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	os.Setenv("PNPM_HOME", dir)
	os.Setenv("PATH", mergePath(os.Getenv("PATH"), dir))
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
