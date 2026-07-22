package pty

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/creack/pty"

	"github.com/csullivan/bish/internal/shellenv"
)

type PTY struct {
	f       *os.File
	cmd     *exec.Cmd
	cleanup func()
}

func New(shell, cwdFile, wFilePath, galleryFilePath string) (*PTY, error) {
	if shell == "" {
		shell = shellenv.DefaultShell()
	}

	pid := os.Getpid()
	initFile := fmt.Sprintf("/tmp/bish_init_%d.sh", pid)

	env := append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"BISH_CWD_FILE="+cwdFile,
		"BISH_W_FILE="+wFilePath,
		"BISH_GALLERY_FILE="+galleryFilePath,
	)

	cleanup := func() {
		os.Remove(initFile)
	}

	base := filepath.Base(shell)
	var cmd *exec.Cmd
	switch {
	case strings.Contains(base, "zsh"):
		// inject via ZDOTDIR so it loads silently before the prompt
		writeInitFile(initFile)
		zdotdir, cl := setupZshZDOTDIR(pid, initFile)
		if zdotdir != "" {
			env = append(env, "ZDOTDIR="+zdotdir)
			prev := cleanup
			cleanup = func() { prev(); cl() }
		}
		cmd = exec.Command(shell, "-l") // login shell, like Terminal.app/iTerm
	case strings.Contains(base, "bash"):
		// bash has no ZDOTDIR; use --rcfile. Login env is preloaded at app
		// startup (shellenv.LoadLoginEnv), so an interactive (non-login) shell
		// still has the user's PATH etc.
		writeBashInit(initFile)
		cmd = exec.Command(shell, "--rcfile", initFile, "-i")
	default:
		cmd = exec.Command(shell, "-l")
	}
	cmd.Env = env

	f, err := pty.Start(cmd)
	if err != nil {
		cleanup()
		return nil, err
	}

	return &PTY{f: f, cmd: cmd, cleanup: cleanup}, nil
}

// writeInitFile writes the bish shell functions to a temp file.
// The file is sourced by zsh via ZDOTDIR/.zshenv; env vars BISH_CWD_FILE
// and BISH_W_FILE are already set in the shell environment by bish.
func writeInitFile(path string) {
	content := `# auto-injected by bish
precmd() {
  [[ -n "$BISH_CWD_FILE" ]] && printf '%s' "$PWD" > "$BISH_CWD_FILE"
  printf '\033]0;\007' # clear title -> tab label falls back to default
}
preexec() {
  printf '\033]0;%s\007' "$1" # running command as terminal title (zsh-only)
}
w() {
  [[ $# -eq 0 ]] && { echo "usage: w <command> [args...]"; return 1; }
  [[ -z "$BISH_W_FILE" ]] && { echo "w: not inside a bish session"; return 1; }
  printf '%s\t%s\n' "$PWD" "$*" >> "$BISH_W_FILE"
  echo "[bish] launched: $*"
}
gallery() {
  local target="${1:-.}"
  [[ -z "$BISH_GALLERY_FILE" ]] && { echo "gallery: not in bish"; return 1; }
  if [[ "$target" != /* ]]; then target="$PWD/$target"; fi
  printf '%s\n' "$target" > "$BISH_GALLERY_FILE"
  echo "[bish] opening gallery: $target"
}
`
	os.WriteFile(path, []byte(content), 0o644) //nolint
}

// writeBashInit writes the bish shell integration for bash: cwd tracking via
// PROMPT_COMMAND and running-command title via a DEBUG trap (bash's preexec
// analogue), plus the w/gallery helpers. Sources the user's ~/.bashrc first and
// preserves any existing PROMPT_COMMAND.
func writeBashInit(path string) {
	content := `# auto-injected by bish (bash)
[[ -f ~/.bashrc ]] && source ~/.bashrc
__bish_precmd() {
  [[ -n "$BISH_CWD_FILE" ]] && printf '%s' "$PWD" > "$BISH_CWD_FILE"
  printf '\033]0;\007' # clear title -> tab label falls back to default
}
__bish_preexec() {
  [[ -n "$COMP_LINE" ]] && return               # skip completion
  case "$BASH_COMMAND" in __bish_precmd) return;; esac
  printf '\033]0;%s\007' "$BASH_COMMAND"         # running command as title
}
PROMPT_COMMAND="__bish_precmd${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
trap '__bish_preexec' DEBUG
w() {
  [[ $# -eq 0 ]] && { echo "usage: w <command> [args...]"; return 1; }
  [[ -z "$BISH_W_FILE" ]] && { echo "w: not inside a bish session"; return 1; }
  printf '%s\t%s\n' "$PWD" "$*" >> "$BISH_W_FILE"
  echo "[bish] launched: $*"
}
gallery() {
  local target="${1:-.}"
  [[ -z "$BISH_GALLERY_FILE" ]] && { echo "gallery: not in bish"; return 1; }
  if [[ "$target" != /* ]]; then target="$PWD/$target"; fi
  printf '%s\n' "$target" > "$BISH_GALLERY_FILE"
  echo "[bish] opening gallery: $target"
}
`
	os.WriteFile(path, []byte(content), 0o644) //nolint
}

// setupZshZDOTDIR creates a temp ZDOTDIR whose .zshenv sources the bish init
// file then immediately resets ZDOTDIR to $HOME so the rest of the user's zsh
// startup (profile, .zshrc, .zlogin) runs from the normal location.
func setupZshZDOTDIR(pid int, initFile string) (zdotdir string, cleanup func()) {
	zdotdir = fmt.Sprintf("/tmp/bish_zdotdir_%d", pid)
	if err := os.MkdirAll(zdotdir, 0o755); err != nil {
		return "", func() {}
	}

	home, _ := os.UserHomeDir()
	// Source real .zshenv from HOME first (if present), then our init, then
	// hand ZDOTDIR back to HOME so normal startup continues unaffected.
	zshenv := fmt.Sprintf(
		"[[ -f %q ]] && source %q\nsource %q\nexport ZDOTDIR=%q\n",
		filepath.Join(home, ".zshenv"),
		filepath.Join(home, ".zshenv"),
		initFile,
		home,
	)
	if err := os.WriteFile(filepath.Join(zdotdir, ".zshenv"), []byte(zshenv), 0o644); err != nil {
		os.RemoveAll(zdotdir)
		return "", func() {}
	}

	return zdotdir, func() { os.RemoveAll(zdotdir) }
}

func (p *PTY) Write(b []byte) (int, error) {
	return p.f.Write(b)
}

func (p *PTY) Read(b []byte) (int, error) {
	return p.f.Read(b)
}

func (p *PTY) Resize(rows, cols int) {
	if rows <= 0 || cols <= 0 {
		return
	}
	pty.Setsize(p.f, &pty.Winsize{ //nolint
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func (p *PTY) Close() {
	p.f.Close()
	if p.cmd.Process != nil {
		p.cmd.Process.Kill() //nolint
	}
	if p.cleanup != nil {
		p.cleanup()
	}
}
