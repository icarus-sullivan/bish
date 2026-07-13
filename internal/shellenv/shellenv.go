// Package shellenv resolves the user's default shell and bootstraps the
// process environment from a login shell. GUI-launched apps (Finder/Dock)
// inherit launchd's minimal env; capturing `shell -l -i -c env` once at
// startup gives every child process the user's real PATH etc.
package shellenv

import (
	"context"
	"fmt"
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
// it to this process. If the interactive shell fails or times out (slow
// zshrc, nvm, etc.) it falls back to a plain login shell so .zprofile PATH
// (brew shellenv etc.) is still recovered. On total failure the current env
// is kept and an error is returned.
func LoadLoginEnv(shell string) error {
	// -l loads zprofile/bash_profile, -i loads zshrc/bashrc.
	// A timed-out capture silently strands the whole session on launchd's
	// minimal PATH, so be generous: interactive zsh startup is 1-2s warm.
	out, ierr := captureEnv(shell, 15*time.Second, "-l", "-i", "-c", "command env -0")
	if ierr != nil {
		var lerr error
		out, lerr = captureEnv(shell, 5*time.Second, "-l", "-c", "command env -0")
		if lerr != nil {
			return fmt.Errorf("interactive shell: %w; login shell: %w", ierr, lerr)
		}
	}

	// env -0 separates entries with NUL, so multiline values parse intact.
	for _, line := range strings.Split(string(out), "\x00") {
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
	if ierr != nil {
		return fmt.Errorf("interactive shell failed, used login-only env: %w", ierr)
	}
	return nil
}

func captureEnv(shell string, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, shell, args...).Output()
}
