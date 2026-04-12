package backendproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lune/internal/config"
	"lune/internal/runtimeconfig"
)

func TestHandlerLogsInAndInjectsBackendToken(t *testing.T) {
	t.Setenv("LUNE_BACKEND_ADMIN_USERNAME", "root")
	t.Setenv("LUNE_BACKEND_ADMIN_PASSWORD", "secret")

	loginCalls := 0
	apiCalls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    "backend-token",
			})
		case "/api/channel/":
			apiCalls++
			if got := r.Header.Get("Authorization"); got != "Bearer backend-token" {
				t.Fatalf("expected backend token, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	handler := Handler(runtimeconfig.New("configs/config.json", config.Config{
		Server: config.ServerConfig{UpstreamURL: backend.URL},
	}, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/backend/api/channel/?p=0&page_size=100", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if loginCalls != 1 {
		t.Fatalf("expected 1 login call, got %d", loginCalls)
	}
	if apiCalls != 1 {
		t.Fatalf("expected 1 api call, got %d", apiCalls)
	}
}

func TestHandlerRelogsAfterBackendUnauthorized(t *testing.T) {
	t.Setenv("LUNE_BACKEND_ADMIN_USERNAME", "root")
	t.Setenv("LUNE_BACKEND_ADMIN_PASSWORD", "secret")

	loginCalls := 0
	apiCalls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginCalls++
			token := "token-1"
			if loginCalls > 1 {
				token = "token-2"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    token,
			})
		case "/api/channel/":
			apiCalls++
			switch r.Header.Get("Authorization") {
			case "Bearer token-1":
				http.Error(w, "expired", http.StatusUnauthorized)
			case "Bearer token-2":
				_, _ = io.WriteString(w, `{"ok":true}`)
			default:
				t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	handler := Handler(runtimeconfig.New("configs/config.json", config.Config{
		Server: config.ServerConfig{UpstreamURL: backend.URL},
	}, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/backend/api/channel/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if loginCalls != 2 {
		t.Fatalf("expected 2 login calls, got %d", loginCalls)
	}
	if apiCalls != 2 {
		t.Fatalf("expected 2 api calls, got %d", apiCalls)
	}
}

func TestHandlerReturnsClearErrorWhenBackendCredsMissing(t *testing.T) {
	t.Setenv("LUNE_BACKEND_ADMIN_USERNAME", "")
	t.Setenv("LUNE_BACKEND_ADMIN_PASSWORD", "")

	handler := Handler(runtimeconfig.New("configs/config.json", config.Config{
		Server: config.ServerConfig{UpstreamURL: "http://127.0.0.1:1"},
	}, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/backend/api/channel/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, errBackendAdminCredsMissing.Error()) {
		t.Fatalf("expected missing creds message, got %q", body)
	}
}
