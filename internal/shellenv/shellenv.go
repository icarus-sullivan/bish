// Package shellenv resolves the user's default shell and bootstraps the
// process environment from a login shell. GUI-launched apps (Finder/Dock)
// inherit launchd's minimal env; capturing `shell -l -i -c env` once at
// startup gives every child process the user's real PATH etc.
package shellenv

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// DefaultShell returns $SHELL, then the user's dscl UserShell (macOS),
// then /bin/zsh.
func DefaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if u, err := user.Current(); err == nil {
		out, err := exec.Command("dscl", ".", "-read", "/Users/"+u.Username, "UserShell").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if rest, ok := strings.CutPrefix(line, "UserShell:"); ok {
					if s := strings.TrimSpace(rest); s != "" {
						return s
					}
				}
			}
		}
	}
	return "/bin/zsh"
}

// LoadLoginEnv runs shell as login+interactive, captures its env, and applies
// it to this process. On error or timeout the current env is kept.
func LoadLoginEnv(shell string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// -l loads zprofile/bash_profile, -i loads zshrc/bashrc
	out, err := exec.CommandContext(ctx, shell, "-l", "-i", "-c", "command env").Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		switch key {
		case "TERM", "PWD", "OLDPWD", "SHLVL", "_":
			continue
		}
		if strings.HasPrefix(key, "BISH_") {
			continue
		}
		os.Setenv(key, val) //nolint
	}
}
