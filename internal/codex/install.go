package codex

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/langext"
)

// Codex ships with bish: if the user has no `codex` of their own, bish
// downloads a pinned official release into its own tools dir the first time
// the Codex tab opens. The version and every archive's SHA-256 are pinned
// here — nothing about what gets executed is decided at runtime by the
// network. Bump all three together when updating.
const pinnedVersion = "0.160.1"

const releaseBase = "https://github.com/openai/codex/releases/download/rust-v" + pinnedVersion + "/"

type asset struct{ name, sha256 string }

var pinnedAssets = map[string]asset{
	"darwin/arm64": {"codex-aarch64-apple-darwin.tar.gz", "670af2b049d9c95afb74d7da385f30c5033d13a07175001dd8958c51944984d0"},
	"darwin/amd64": {"codex-x86_64-apple-darwin.tar.gz", "8d938ddb93c4424b1d45f1606984ed514c5aa70e463302a6a2227fba7af02db7"},
	"linux/arm64":  {"codex-aarch64-unknown-linux-musl.tar.gz", "f54dc5852042445bf41da3aa31156f3cb02f52c5a1a04074de73dc5598f7e1f7"},
	"linux/amd64":  {"codex-x86_64-unknown-linux-musl.tar.gz", "9226581be592d18f7e7f740a352fdb63aa61e45e39f7eb9b09d3888c84bba33f"},
}

const maxArchive = 400 << 20

func managedPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, ".config", "bish", "tools", "codex", pinnedVersion, name), nil
}

// bundledPath is a codex binary shipped inside the app itself (release
// builds may drop one in Contents/Resources or next to the executable).
func bundledPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, p := range []string{filepath.Join(dir, "..", "Resources", "codex"), filepath.Join(dir, "codex")} {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// Resolve finds the codex binary to run: the user's own install (login-shell
// PATH included, so GUI launches see nvm/homebrew installs), then one
// bundled in the app, then bish's managed copy. "" = none yet.
func Resolve() string {
	langext.FixPath()
	if p, err := exec.LookPath("codex"); err == nil {
		return p
	}
	if p := bundledPath(); p != "" {
		return p
	}
	if p, err := managedPath(); err == nil {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

var installMu sync.Mutex

// EnsureInstalled returns a runnable codex, downloading the pinned release
// if none exists. progress receives (bytesDone, bytesTotal).
func EnsureInstalled(progress func(done, total int64)) (string, error) {
	if p := Resolve(); p != "" {
		return p, nil
	}
	installMu.Lock()
	defer installMu.Unlock()
	if p := Resolve(); p != "" { // another caller finished while we waited
		return p, nil
	}
	a, ok := pinnedAssets[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("codex: no prebuilt Codex for %s/%s — install it yourself (npm i -g @openai/codex)", runtime.GOOS, runtime.GOARCH)
	}
	dst, err := managedPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	client := &http.Client{Timeout: 20 * time.Minute}
	resp, err := client.Get(releaseBase + a.name)
	if err != nil {
		return "", fmt.Errorf("codex: download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("codex: download failed: %s", resp.Status)
	}
	h := sha256.New()
	pr := &progressReader{r: io.LimitReader(resp.Body, maxArchive+1), total: resp.ContentLength, fn: progress}
	n, err := io.Copy(io.MultiWriter(tmp, h), pr)
	if err != nil {
		return "", fmt.Errorf("codex: download failed: %w", err)
	}
	if n > maxArchive {
		return "", fmt.Errorf("codex: download too large")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.sha256 {
		return "", fmt.Errorf("codex: checksum mismatch (got %s) — refusing to install", got)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	bin := dst + ".partial"
	if err := extractBinary(tmp, bin); err != nil {
		os.Remove(bin)
		return "", err
	}
	if err := os.Rename(bin, dst); err != nil {
		os.Remove(bin)
		return "", err
	}
	return dst, nil
}

// extractBinary writes the archive's single codex executable to out.
// Only a regular file whose base name starts with "codex" is taken; paths
// in the archive are never used to build a destination.
func extractBinary(r io.Reader, out string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("codex: bad archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("codex: archive has no codex binary")
		}
		if err != nil {
			return fmt.Errorf("codex: bad archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || !strings.HasPrefix(filepath.Base(hdr.Name), "codex") {
			continue
		}
		f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(tr, maxArchive))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	last  time.Time
	fn    func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.fn != nil && (time.Since(p.last) > 150*time.Millisecond || err == io.EOF) {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return n, err
}
