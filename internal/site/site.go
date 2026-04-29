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
		if staticReq, ok := staticFileRequest(subtree, r); ok {
			fileServer.ServeHTTP(w, staticReq)
			return
		}

		// all other /admin paths → SPA index.html
		serveIndex(subtree, w)
	})
}

func staticFileRequest(files fs.FS, r *http.Request) (*http.Request, bool) {
	filePath := r.URL.Path
	if strings.HasPrefix(filePath, "/admin/") {
		filePath = strings.TrimPrefix(filePath, "/admin")
	}

	info, err := fs.Stat(files, strings.TrimPrefix(filePath, "/"))
	if err != nil || info.IsDir() {
		return nil, false
	}
	if filePath == r.URL.Path {
		return r, true
	}

	staticReq := r.Clone(r.Context())
	staticReq.URL.Path = filePath
	return staticReq, true
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
