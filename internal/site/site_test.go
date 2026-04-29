package site

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesAdminDeepLinkAsSPA(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/pools/1", http.NoBody)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected html content type, got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), `<div id="root">`) {
		t.Fatalf("expected SPA index body, got %q", rr.Body.String())
	}
}

func TestHandlerServesAdminAssetWithoutMutatingOriginalPath(t *testing.T) {
	matches, err := fs.Glob(dist, "dist/assets/index-*.js")
	if err != nil {
		t.Fatalf("glob asset: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected built index asset")
	}
	path := "/admin/" + strings.TrimPrefix(matches[0], "dist/")
	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if req.URL.Path != path {
		t.Fatalf("request path was mutated: %q", req.URL.Path)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("expected javascript content type, got %q", ct)
	}
}

func TestHandlerServesAdminFaviconAsStaticFile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/favicon.svg", http.NoBody)
	rr := httptest.NewRecorder()

	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if req.URL.Path != "/admin/favicon.svg" {
		t.Fatalf("request path was mutated: %q", req.URL.Path)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("expected svg content type, got %q", ct)
	}
	if strings.Contains(rr.Body.String(), `<div id="root">`) {
		t.Fatalf("expected favicon body, got SPA index")
	}
}
