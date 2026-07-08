package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaServer(t *testing.T) {
	base := startMediaServer()
	if base == "" {
		t.Fatal("media server failed to start")
	}
	f := filepath.Join(t.TempDir(), "clip.mp4")
	os.WriteFile(f, []byte("0123456789"), 0o644) //nolint

	// range request → 206 partial content
	req, _ := http.NewRequest("GET", base+f, nil)
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Fatalf("range request: status=%d body=%q", resp.StatusCode, body)
	}

	// wrong token → 403
	badURL := strings.Replace(base, "t=", "t=x", 1) + f
	resp2, err := http.Get(badURL)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token: status=%d, want 403", resp2.StatusCode)
	}
}
