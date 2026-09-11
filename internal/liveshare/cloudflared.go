package liveshare

// Bundles a Cloudflare Quick Tunnel (cloudflared binary, fetched at build
// time by scripts/fetch-cloudflared.sh into assets/cloudflared/) so share
// links work off the host's LAN, not just on it. Quick Tunnels need no
// Cloudflare account: cloudflared requests a random *.trycloudflare.com
// hostname and prints it to stderr once the tunnel is up. There's no
// official Go SDK for this (cloudflare/cloudflared#986) — cloudflared is a
// full daemon, so this shells out to the embedded binary rather than
// linking any tunnel logic in-process.
//
// assets/cloudflared/ only ever contains the platform(s) actually fetched
// for the current build; a platform with nothing embedded (e.g. a `go
// build` run without the Makefile's fetch step first) just gets
// ErrCloudflaredUnavailable back, and callers fall back to the LAN-only
// link — see Manager.startServerLocked.

import (
	"bufio"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"
)

//go:embed assets/cloudflared
var cloudflaredFS embed.FS

var quickTunnelURLRe = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// ErrCloudflaredUnavailable means this platform has no bundled cloudflared
// binary (not fetched at build time) — ask the caller to fall back.
var ErrCloudflaredUnavailable = errors.New("cloudflared not bundled for this platform")

func cloudflaredAssetName() string {
	name := fmt.Sprintf("cloudflared-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

var (
	extractOnce   sync.Once
	extractedPath string
	extractErr    error
)

// extractCloudflared writes the embedded binary for this platform out to a
// stable cache path (once per process) and returns it.
func extractCloudflared() (string, error) {
	extractOnce.Do(func() {
		data, err := cloudflaredFS.ReadFile("assets/cloudflared/" + cloudflaredAssetName())
		if err != nil {
			extractErr = ErrCloudflaredUnavailable
			return
		}
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		dir = filepath.Join(dir, "bish")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			extractErr = err
			return
		}
		path := filepath.Join(dir, cloudflaredAssetName())
		// only rewrite if missing/stale — skips a disk write on every launch
		// once the cache is warm.
		if fi, statErr := os.Stat(path); statErr != nil || fi.Size() != int64(len(data)) {
			if err := os.WriteFile(path, data, 0o755); err != nil {
				extractErr = err
				return
			}
		}
		extractedPath = path
	})
	return extractedPath, extractErr
}

// quickTunnel is one running `cloudflared tunnel --url` subprocess.
type quickTunnel struct {
	cmd *exec.Cmd
	URL string
}

// startQuickTunnel launches cloudflared pointed at the given local port and
// blocks until it reports its public URL (or errors/times out). Callers
// should treat any error as "no tunnel available" and fall back to a
// LAN-only link — this is best-effort, not a hard requirement to share.
func startQuickTunnel(port int) (*quickTunnel, error) {
	bin, err := extractCloudflared()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, "tunnel", "--url", fmt.Sprintf("http://127.0.0.1:%d", port), "--no-autoupdate")
	cmd.Stdout = io.Discard
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	urlCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if m := quickTunnelURLRe.FindString(scanner.Text()); m != "" {
				select {
				case urlCh <- m:
				default: // already sent — just keep draining stderr so cloudflared never blocks on a full pipe
				}
			}
		}
	}()

	select {
	case u := <-urlCh:
		return &quickTunnel{cmd: cmd, URL: u}, nil
	case <-time.After(15 * time.Second):
		cmd.Process.Kill() //nolint
		return nil, errors.New("timed out waiting for cloudflare tunnel URL")
	}
}

func (q *quickTunnel) Stop() {
	if q == nil || q.cmd == nil || q.cmd.Process == nil {
		return
	}
	q.cmd.Process.Kill() //nolint
	q.cmd.Wait()         //nolint
}
