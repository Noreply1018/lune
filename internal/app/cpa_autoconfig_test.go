package app

import (
	"path/filepath"
	"testing"

	"lune/internal/store"
)

func newTestApp(t *testing.T, cfg Config) *App {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	if cfg.GatewayTmpDir == "" {
		cfg.GatewayTmpDir = filepath.Join(cfg.DataDir, "tmp")
	}
	st, err := store.New(filepath.Join(cfg.DataDir, "lune.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &App{cfg: cfg, store: st, cache: store.NewRoutingCache(st)}
}

func TestEnsureDefaultCpaCreatesEmbeddedService(t *testing.T) {
	app := newTestApp(t, Config{
		CpaBaseURL:       "http://127.0.0.1:8317",
		CpaAPIKey:        "sk-test",
		CpaManagementKey: "mgmt-test",
	})

	app.ensureDefaultCpa()

	svc, err := app.store.GetCpaService()
	if err != nil {
		t.Fatalf("GetCpaService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected CPA service")
	}
	if svc.Label != "Default CPA" || svc.BaseURL != "http://127.0.0.1:8317" || svc.APIKey != "sk-test" || svc.ManagementKey != "mgmt-test" {
		t.Fatalf("unexpected service: %+v", svc)
	}
}

func TestEnsureDefaultCpaMigratesOldComposeURL(t *testing.T) {
	app := newTestApp(t, Config{
		CpaBaseURL:       "http://127.0.0.1:8317",
		CpaAPIKey:        "new-key",
		CpaManagementKey: "new-mgmt",
	})
	_, err := app.store.CreateCpaService(&store.CpaService{
		Label:         "Renamed CPA",
		BaseURL:       "http://cpa:8317",
		APIKey:        "old-key",
		ManagementKey: "old-mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}

	app.ensureDefaultCpa()

	svc, err := app.store.GetCpaService()
	if err != nil {
		t.Fatalf("GetCpaService: %v", err)
	}
	if svc.BaseURL != "http://127.0.0.1:8317" || svc.APIKey != "new-key" || svc.ManagementKey != "new-mgmt" {
		t.Fatalf("old compose service not migrated: %+v", svc)
	}
}

func TestEnsureDefaultCpaPreservesCustomService(t *testing.T) {
	app := newTestApp(t, Config{
		CpaBaseURL:       "http://127.0.0.1:8317",
		CpaAPIKey:        "new-key",
		CpaManagementKey: "new-mgmt",
	})
	_, err := app.store.CreateCpaService(&store.CpaService{
		Label:         "Custom CPA",
		BaseURL:       "http://custom-cpa:8317",
		APIKey:        "old-key",
		ManagementKey: "old-mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}

	app.ensureDefaultCpa()

	svc, err := app.store.GetCpaService()
	if err != nil {
		t.Fatalf("GetCpaService: %v", err)
	}
	if svc.BaseURL != "http://custom-cpa:8317" || svc.APIKey != "old-key" || svc.ManagementKey != "old-mgmt" {
		t.Fatalf("custom service was overwritten: %+v", svc)
	}
}
