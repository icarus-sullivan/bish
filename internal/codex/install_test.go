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

func TestExtractPackage(t *testing.T) {
	root := t.TempDir()
	err := extractPackage(tgz(t, map[string]string{
		"bin/codex":                   "BIN",
		"./bin/codex-code-mode-host":  "HOST",
		"codex-resources/zsh/bin/zsh": "ZSH",
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"bin/codex": "BIN", "bin/codex-code-mode-host": "HOST", "codex-resources/zsh/bin/zsh": "ZSH"} {
		if b, _ := os.ReadFile(filepath.Join(root, name)); string(b) != want {
			t.Fatalf("%s: got %q", name, b)
		}
	}
	// archive paths never escape root
	for _, bad := range []string{"../evil", "/abs/evil", "bin/../../evil"} {
		if err := extractPackage(tgz(t, map[string]string{bad: "x"}), t.TempDir()); err == nil {
			t.Fatalf("%q: expected error", bad)
		}
	}
}

func TestPinnedAssets(t *testing.T) {
	for k, a := range pinnedAssets {
		if len(a.sha256) != 64 || a.name == "" {
			t.Errorf("%s: bad pin %+v", k, a)
		}
	}
}
