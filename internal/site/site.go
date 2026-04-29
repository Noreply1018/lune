package site

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

func Handler() http.Handler {
	subtree, err := fs.Sub(dist, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}

	fileServer := http.FileServer(http.FS(subtree))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// static assets
		if strings.HasPrefix(r.URL.Path, "/admin/assets/") || strings.HasPrefix(r.URL.Path, "/assets/") {
			assetReq := r
			if strings.HasPrefix(r.URL.Path, "/admin/assets/") {
				// Strip /admin for the embedded filesystem without mutating the
				// caller's request. The access logger and downstream handlers should
				// still see the original URL.
				assetReq = r.Clone(r.Context())
				assetReq.URL.Path = strings.TrimPrefix(r.URL.Path, "/admin")
			}
			fileServer.ServeHTTP(w, assetReq)
			return
		}

		// all other /admin paths → SPA index.html
		serveIndex(subtree, w)
	})
}

func serveIndex(files fs.FS, w http.ResponseWriter) {
	indexFile, err := files.Open("index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	defer indexFile.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, indexFile)
}
