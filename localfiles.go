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
// other local processes can't fetch arbitrary files off the port.
type localFileHandler struct{ token string }

func (h *localFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/localfile" {
		http.NotFound(w, r)
		return
	}
	if h.token != "" && r.URL.Query().Get("t") != h.token {
		http.Error(w, "forbidden", http.StatusForbidden)
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
// append an encoded path to, or "" if the listener failed.
func startMediaServer() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return ""
	}
	buf := make([]byte, 16)
	rand.Read(buf) //nolint
	token := hex.EncodeToString(buf)
	go http.Serve(ln, &localFileHandler{token: token}) //nolint
	port := ln.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("http://127.0.0.1:%d/localfile?t=%s&path=", port, token)
}
