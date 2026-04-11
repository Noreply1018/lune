package site

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
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
		cleanPath := path.Clean(r.URL.Path)
		switch {
		case cleanPath == "/":
			serveIndex(subtree, w)
			return
		case cleanPath == "/admin":
			serveIndex(subtree, w)
			return
		case strings.HasPrefix(cleanPath, "/assets/"):
			fileServer.ServeHTTP(w, r)
			return
		default:
			fileServer.ServeHTTP(w, r)
		}
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
