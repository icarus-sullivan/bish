package codex

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func tgz(t *testing.T, files map[string]string) *bytes.Buffer {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return &buf
}

func TestExtractBinary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "codex")
	// archive paths never steer the destination
	if err := extractBinary(tgz(t, map[string]string{"../../evil/codex-aarch64-apple-darwin": "BIN"}), out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "BIN" {
		t.Fatalf("got %q", b)
	}
	if err := extractBinary(tgz(t, map[string]string{"README": "x"}), out+"2"); err == nil {
		t.Fatal("expected error for archive without codex binary")
	}
}

func TestPinnedAssets(t *testing.T) {
	for k, a := range pinnedAssets {
		if len(a.sha256) != 64 || a.name == "" {
			t.Errorf("%s: bad pin %+v", k, a)
		}
	}
}
