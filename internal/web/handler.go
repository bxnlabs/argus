package web

import (
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
)

// NewSPAHandler returns an http.Handler that serves the embedded SPA.
// If webDir is non-empty, it overrides the embedded filesystem with a
// local directory (useful for frontend development).
func NewSPAHandler(webDir string) http.Handler {
	var root fs.FS
	if webDir != "" {
		root = os.DirFS(webDir)
	} else {
		sub, err := fs.Sub(distFS, "dist")
		if err != nil {
			log.Fatalf("embedded web fs: %v", err)
		}
		root = sub
	}
	return spaHandler{root: root, fileServer: http.FileServerFS(root)}
}

// spaHandler serves static files and falls back to index.html for SPA routing.
type spaHandler struct {
	root       fs.FS
	fileServer http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" || path == "/" {
		path = "index.html"
	} else {
		path = path[1:] // strip leading /
	}

	// Try to serve the file directly (skip directories to avoid listings)
	if fi, err := fs.Stat(h.root, path); err == nil && !fi.IsDir() {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	// Fallback to index.html for SPA routes
	fallback := new(http.Request)
	*fallback = *r
	fallback.URL = new(url.URL)
	*fallback.URL = *r.URL
	fallback.URL.Path = "/"
	h.fileServer.ServeHTTP(w, fallback)
}
