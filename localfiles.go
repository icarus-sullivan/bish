package main

import (
	"net/http"
	"path/filepath"
)

type localFileHandler struct{}

func (h *localFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/localfile" {
		http.NotFound(w, r)
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
