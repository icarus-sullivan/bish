package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
)

// token, when set, is required as ?t= — used for the TCP media server so
// other local processes can't fetch arbitrary files off the port (or attach
// to a terminal). pty, when set, serves the terminal websocket at /pty.
type localFileHandler struct {
	token string
	pty   http.Handler
}

func (h *localFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isPTY := r.URL.Path == "/pty" && h.pty != nil
	if r.URL.Path != "/localfile" && !isPTY {
		http.NotFound(w, r)
		return
	}
	if h.token != "" && r.URL.Query().Get("t") != h.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if isPTY {
		if h.token == "" { // never expose a terminal without the token gate
			http.NotFound(w, r)
			return
		}
		h.pty.ServeHTTP(w, r)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" || !filepath.IsAbs(path) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	// ServeFile handles Content-Type detection and range requests (needed for video).
	http.ServeFile(w, r, path)
}

// startMediaServer serves local files over real HTTP on a random loopback
// port. WKWebView's media loader can't stream <video> through the wails://
// scheme handler, so videos need an http:// URL. Returns the URL prefix to
// append an encoded path to, or "" if the listener failed. pty (optional)
// is mounted at /pty behind the same token — see app.PTYSocketHandler.
func startMediaServer(pty http.Handler) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return ""
	}
	buf := make([]byte, 16)
	rand.Read(buf) //nolint
	token := hex.EncodeToString(buf)
	go http.Serve(ln, &localFileHandler{token: token, pty: pty}) //nolint
	port := ln.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("http://127.0.0.1:%d/localfile?t=%s&path=", port, token)
}
