package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"lune/internal/config"
	"lune/internal/metrics"
	"lune/internal/platform"
	"lune/internal/runtimeconfig"
	"lune/internal/store"
)

func TestOverviewAPIUsesStructuredFields(t *testing.T) {
	h, st := newTestHandler(t)
	defer st.Close()

	req := httptest.NewRequest(http.MethodGet, "/admin/api/overview", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()

	h.Route(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Overview overviewPayload `json:"overview"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if payload.Overview.AccountsTotal != 1 {
		t.Fatalf("expected accounts_total=1, got %+v", payload.Overview)
	}
	if payload.Overview.DefaultModelAlias == "" {
		t.Fatalf("expected default model alias to be present")
	}
}

func TestAccountsAPIAndToggleUpdateConfig(t *testing.T) {
	h, st := newTestHandler(t)
	defer st.Close()

	createBody := bytes.NewBufferString(`{"id":"plus-b","label":"Backup Plus","credential_type":"api_key","credential_env":"UPSTREAM_API_KEY_B","plan_type":"plus","enabled":true,"status":"healthy"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/admin/api/accounts", createBody)
	createReq.Header.Set("Authorization", "Bearer admin-token")
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()

	h.Route(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected account create 200, got %d: %s", createRec.Code, createRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/admin/api/accounts", nil)
	accountsReq.Header.Set("Authorization", "Bearer admin-token")
	accountsRec := httptest.NewRecorder()
	h.Route(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts 200, got %d: %s", accountsRec.Code, accountsRec.Body.String())
	}

	var accountsPayload struct {
		Accounts []store.AccountRecord `json:"accounts"`
	}
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsPayload); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	if len(accountsPayload.Accounts) != 2 {
		t.Fatalf("expected 2 accounts after create, got %+v", accountsPayload.Accounts)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/plus-b/disable", nil)
	disableReq.Header.Set("Authorization", "Bearer admin-token")
	disableRec := httptest.NewRecorder()
	h.Route(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected disable 200, got %d: %s", disableRec.Code, disableRec.Body.String())
	}

	items, err := st.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts from store: %v", err)
	}
	var disabled *store.AccountRecord
	for i := range items {
		if items[i].ID == "plus-b" {
			disabled = &items[i]
			break
		}
	}
	if disabled == nil || disabled.Enabled {
		t.Fatalf("expected plus-b disabled in store, got %+v", disabled)
	}
}

func newTestHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	cfg := config.Config{
		Server: config.ServerConfig{Port: 7788, DataDir: "data", RequestTimeoutS: 120, ShutdownTimeoutS: 10, PlatformRefreshInterval: 60},
		Auth: config.AuthConfig{
			AdminToken: "admin-token",
			AccessTokens: []config.AccessToken{
				{Name: "default", Token: "sk-lune", Enabled: true, QuotaCalls: 1000, CostPerRequest: 1},
			},
		},
		Platforms:    []config.Platform{{ID: "upstream", Type: "openai", Adapter: "openai-upstream", Enabled: true}},
		Accounts:     []config.Account{{ID: "plus-a", Platform: "upstream", Label: "Primary Plus", CredentialType: "api_key", CredentialEnv: "UPSTREAM_API_KEY", PlanType: "plus", Enabled: true, Status: "healthy"}},
		AccountPools: []config.AccountPool{{ID: "default-pool", Platform: "upstream", Strategy: "sticky-first-healthy", Enabled: true, Members: []string{"plus-a"}}},
		Models:       []config.ModelRoute{{Alias: "gpt-4o", AccountPool: "default-pool", TargetKind: "account_pool", TargetID: "default-pool", TargetModel: "gpt-4o"}},
	}
	return newTestHandlerWithConfig(t, cfg)
}

func newTestHandlerWithConfig(t *testing.T, cfg config.Config) (*Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.SyncAccessTokens(context.Background(), cfg.Auth.AccessTokens); err != nil {
		t.Fatalf("sync tokens: %v", err)
	}
	if err := st.SyncAccounts(context.Background(), cfg.Accounts); err != nil {
		t.Fatalf("sync accounts: %v", err)
	}
	if err := st.SyncAccountPools(context.Background(), cfg.AccountPools); err != nil {
		t.Fatalf("sync pools: %v", err)
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw, err := config.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var cfgManager *runtimeconfig.Manager
	registry := platform.New(func() config.Config {
		if cfgManager == nil {
			return cfg
		}
		return cfgManager.Current()
	})
	cfgManager = runtimeconfig.New(configPath, cfg, st, func(applied config.Config) {
		registry.CheckAll(context.Background())
	})

	return NewHandler(cfgManager, st, metrics.New(), registry), st
}

func TestDashboardRedirectsTrailingSlash(t *testing.T) {
	h, st := newTestHandler(t)
	defer st.Close()

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()

	h.Route(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected redirect for /admin/, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/admin" {
		t.Fatalf("expected redirect to /admin, got %q", location)
	}
}

func TestMetricsLegacyShapeMatchesNewOverviewFields(t *testing.T) {
	h, st := newTestHandler(t)
	defer st.Close()

	h.metrics.Record(120000000, true)
	h.metrics.Record(80000000, false)

	req := httptest.NewRequest(http.MethodGet, "/admin/metrics", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()
	h.Route(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	if !bytes.Contains(body, []byte("success_requests")) || !bytes.Contains(body, []byte("failed_requests")) {
		t.Fatalf("expected remapped metrics keys, got %s", string(body))
	}
}

func TestConfigSchemaReturnsDefaults(t *testing.T) {
	h, st := newTestHandler(t)
	defer st.Close()

	req := httptest.NewRequest(http.MethodGet, "/admin/config/schema", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()

	h.Route(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Defaults map[string]map[string]any `json:"defaults"`
		Help     map[string]string         `json:"help"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode config schema: %v", err)
	}
	if payload.Defaults["account"]["credential_type"] != "api_key" {
		t.Fatalf("expected api_key default credential_type, got %+v", payload.Defaults["account"])
	}
}
