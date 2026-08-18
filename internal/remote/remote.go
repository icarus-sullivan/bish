// Package remote drives a project living on another machine over plain
// `ssh` — no golang.org/x/crypto/ssh, no bundled SFTP client. `ssh` is
// already the one dependency a user doing remote dev has installed and
// configured (keys, agent, ~/.ssh/config aliases all just work), so every
// operation here is a single non-interactive `ssh dest <command>` — the
// same posture the LSP manager takes toward gopls/pyright: shell out to the
// real tool instead of reimplementing its protocol.
package remote

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/csullivan/bish/internal/tree"
)

// ParseDest accepts either a plain ssh destination ("user@host", or a
// ~/.ssh/config alias) or a full ssh command line pasted from a host
// provider's dashboard (RunPod etc. show "ssh root@1.2.3.4 -p 12345 -i
// ~/.ssh/id_ed25519") and splits it into the bare destination token plus any
// extra flags to pass through — ssh has no way to embed -p/-i into the
// destination argument itself, they must be separate argv entries appearing
// before it.
func ParseDest(raw string) (dest string, extraArgs []string) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) > 0 && fields[0] == "ssh" {
		fields = fields[1:]
	}
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		// single-letter flags that take a value (-p 2222, -i key, -l user,
		// -o Foo=Bar, -J proxyhost, ...) — passed straight through to ssh
		if len(f) == 2 && f[0] == '-' && i+1 < len(fields) {
			extraArgs = append(extraArgs, f, fields[i+1])
			i++
			continue
		}
		if dest == "" && !strings.HasPrefix(f, "-") {
			dest = f
		}
	}
	return dest, extraArgs
}

// ShortDest returns just the destination token, stripped of ssh's "ssh "
// prefix and any -p/-i/etc. flags — for display (window title, ...) so
// pasting a full command line doesn't dump the whole thing into the UI.
func ShortDest(raw string) string {
	dest, _ := ParseDest(raw)
	return dest
}

// muxArgs are appended to every invocation: BatchMode disables interactive
// password prompts (key/agent auth only — a GUI app can't answer a TTY
// prompt), and ControlMaster/ControlPath/ControlPersist make OpenSSH itself
// multiplex all these one-shot commands over a single reused connection, so
// browsing a file tree isn't a fresh TCP+auth handshake per directory.
func muxArgs(rawDest string) []string {
	dest, extra := ParseDest(rawDest)
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=8",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=~/.ssh/bish-ctrl-%r@%h:%p",
		"-o", "ControlPersist=10m",
	}
	args = append(args, extra...)
	return append(args, dest)
}

func run(dest, remoteCmd string) ([]byte, error) {
	cmd := exec.Command("ssh", append(muxArgs(dest), remoteCmd)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", ShortDest(dest), msg)
	}
	return stdout.Bytes(), nil
}

// quote single-quotes s for embedding in the remote shell command line.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Reachable does a fast no-op round trip so opening a remote project fails
// fast with a clear error instead of the tree/terminal silently timing out.
func Reachable(dest string) error {
	_, err := run(dest, "true")
	return err
}

func Stat(dest, path string) (isDir bool, err error) {
	out, err := run(dest, fmt.Sprintf(
		`test -d %s && echo d || { test -e %s && echo f || exit 1; }`, quote(path), quote(path)))
	if err != nil {
		return false, fmt.Errorf("no such file or directory: %s", path)
	}
	return strings.TrimSpace(string(out)) == "d", nil
}

// List lists one directory's immediate entries via `ls -Ap`, whose portable
// POSIX -p behavior (trailing slash on directories) works on both GNU and
// BSD ls — unlike the more capable but GNU-only `find -printf`, which would
// silently misbehave against a macOS or BusyBox remote host.
func List(dest, dir string) ([]tree.Entry, error) {
	out, err := run(dest, "ls -Ap -- "+quote(dir))
	if err != nil {
		return nil, err
	}
	var entries []tree.Entry
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		isDir := strings.HasSuffix(line, "/")
		entries = append(entries, tree.Entry{Name: strings.TrimSuffix(line, "/"), IsDir: isDir})
	}
	return entries, nil
}

func ReadFile(dest, path string) (string, error) {
	out, err := run(dest, "cat -- "+quote(path))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// WriteFile streams content over stdin rather than embedding it in the
// command line (arg-length limits, shell-escaping an arbitrary file body).
func WriteFile(dest, path, content string) error {
	cmd := exec.Command("ssh", append(muxArgs(dest), "cat > "+quote(path))...)
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", ShortDest(dest), msg)
	}
	return nil
}

func CreateFile(dest, path string) error {
	_, err := run(dest, "touch -- "+quote(path))
	return err
}

func Mkdir(dest, path string) error {
	_, err := run(dest, "mkdir -p -- "+quote(path))
	return err
}

func Rename(dest, oldPath, newPath string) error {
	_, err := run(dest, "mv -- "+quote(oldPath)+" "+quote(newPath))
	return err
}

func Remove(dest, path string) error {
	_, err := run(dest, "rm -rf -- "+quote(path))
	return err
}

// TreeFS adapts this package's SSH calls to tree.FS so a remote project's
// file tree walks identically to a local one.
type TreeFS struct{ Dest string }

func (f TreeFS) Stat(path string) (bool, error)            { return Stat(f.Dest, path) }
func (f TreeFS) ReadDir(path string) ([]tree.Entry, error) { return List(f.Dest, path) }
