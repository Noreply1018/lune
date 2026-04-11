package httpserver

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lune/internal/config"
	"lune/internal/runtimeconfig"
)

func TestServerServesFrontendAtRoot(t *testing.T) {
	server := New(
		runtimeconfig.New("configs/config.json", testConfig(), nil, nil),
		log.New(io.Discard, "", 0),
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for root, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<div id=\"root\"></div>") {
		t.Fatalf("expected frontend html, got %q", body)
	}
}

func TestServerKeepsHealthzRoute(t *testing.T) {
	server := New(
		runtimeconfig.New("configs/config.json", testConfig(), nil, nil),
		log.New(io.Discard, "", 0),
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthz, got %d", rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json content type, got %q", got)
	}
}

func TestServerServesAdminFrontendAtAdminRoot(t *testing.T) {
	server := New(
		runtimeconfig.New("configs/config.json", testConfig(), nil, nil),
		log.New(io.Discard, "", 0),
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /admin, got %d", rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}
}

func TestServerKeepsAdminAPIJSONRoute(t *testing.T) {
	server := New(
		runtimeconfig.New("configs/config.json", testConfig(), nil, nil),
		log.New(io.Discard, "", 0),
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /admin/config, got %d", rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json content type, got %q", got)
	}
}

func testConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			Port:                    7788,
			DataDir:                 "data",
			RequestTimeoutS:         120,
			ShutdownTimeoutS:        10,
			PlatformRefreshInterval: 60,
		},
		Auth: config.AuthConfig{
			AdminToken: "admin-token",
			AccessTokens: []config.AccessToken{
				{Name: "default", Token: "sk-lune", Enabled: true, QuotaCalls: 1000, CostPerRequest: 1},
			},
		},
		Platforms: []config.Platform{
			{ID: "chatgpt-web", Type: "openai", Adapter: "chatgpt-web", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "chatgpt-web", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "chatgpt-plus", Platform: "chatgpt-web", Strategy: "sticky-first-healthy", Enabled: true, Members: []string{"plus-a"}},
		},
		Models: []config.ModelRoute{
			{Alias: "plus-chat", AccountPool: "chatgpt-plus", TargetModel: "gpt-4o"},
		},
	}
}
