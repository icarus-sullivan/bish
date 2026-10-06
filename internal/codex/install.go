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
//
// We fetch the full "codex-package" archive, not the bare binary: codex
// expects its helpers (bin/codex-code-mode-host, codex-path/rg,
// codex-resources/zsh, …) laid out around bin/codex, and fails closed
// without them.
const pinnedVersion = "0.160.1"

const releaseBase = "https://github.com/openai/codex/releases/download/rust-v" + pinnedVersion + "/"

type asset struct{ name, sha256 string }

var pinnedAssets = map[string]asset{
	"darwin/arm64": {"codex-package-aarch64-apple-darwin.tar.gz", "f73527ee09c6db869acbb37b709866b339ea74ef91d2de255e9c74ec960c6314"},
	"darwin/amd64": {"codex-package-x86_64-apple-darwin.tar.gz", "a98f330c9b1652cef2edc7bc2ee4c47a0fe19fa098b686381be3c8842abf0ac0"},
	"linux/arm64":  {"codex-package-aarch64-unknown-linux-musl.tar.gz", "dff0954438fa455c2197ddb1f421d8d68625d98de610f76bedb6e5bc837ea35b"},
	"linux/amd64":  {"codex-package-x86_64-unknown-linux-musl.tar.gz", "340801565906a7028f6baaa9ab6853addaef221f0016a1417a7c1ffdd96c21f0"},
}

const (
	maxArchive   = 400 << 20 // compressed download
	maxExtracted = 2 << 30   // total unpacked bytes
)

// managedDir is the unpacked package root for the pinned version.
func managedDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "bish", "tools", "codex", pinnedVersion), nil
}

func managedPath() (string, error) {
	dir, err := managedDir()
	if err != nil {
		return "", err
	}
	return entrypoint(dir), nil
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
	dir, err := managedDir()
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(parent, "download-*")
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
	staging, err := os.MkdirTemp(parent, pinnedVersion+".partial-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging) // no-op once renamed into place
	if err := extractPackage(tmp, staging); err != nil {
		return "", err
	}
	if _, err := os.Stat(entrypoint(staging)); err != nil {
		return "", fmt.Errorf("codex: package has no bin/codex")
	}
	// Replaces any older layout (e.g. a lone binary from earlier bish builds).
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.Rename(staging, dir); err != nil {
		return "", err
	}
	return entrypoint(dir), nil
}

// entrypoint is the codex binary inside an unpacked package root.
func entrypoint(dir string) string {
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "bin", name)
}

// extractPackage unpacks the codex package archive under root. Only
// directories and regular files are written; links, devices, absolute
// paths and anything escaping root are skipped or rejected.
func extractPackage(r io.Reader, root string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("codex: bad archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("codex: bad archive: %w", err)
		}
		name := filepath.FromSlash(strings.TrimPrefix(hdr.Name, "./"))
		if name == "" || name == "." {
			continue
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("codex: unsafe path in archive: %q", hdr.Name)
		}
		out := filepath.Join(root, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += hdr.Size
			if total > maxExtracted {
				return fmt.Errorf("codex: archive too large")
			}
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if hdr.FileInfo().Mode()&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, hdr.Size))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
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
