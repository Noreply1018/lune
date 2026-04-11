package runtimeconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lune/internal/config"
	"lune/internal/store"
)

func TestManagerApplyWritesFileAndSyncsStore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	initial := config.Config{
		Server: config.ServerConfig{Port: 7788, DataDir: "data"},
		Auth: config.AuthConfig{
			AdminToken: "admin-1",
			AccessTokens: []config.AccessToken{
				{Name: "default", Token: "sk-1", Enabled: true, QuotaCalls: 10, CostPerRequest: 1},
			},
		},
		Platforms: []config.Platform{
			{ID: "chatgpt-web", Adapter: "chatgpt-web", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "chatgpt-web", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "chatgpt-plus", Platform: "chatgpt-web", Enabled: true, Members: []string{"plus-a"}},
		},
		Models: []config.ModelRoute{
			{Alias: "plus-chat", AccountPool: "chatgpt-plus", TargetModel: "gpt-4o"},
		},
	}

	raw, err := config.Marshal(initial)
	if err != nil {
		t.Fatalf("marshal initial: %v", err)
	}
	if err := os.WriteFile(configPath, raw, 0o644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	manager := New(configPath, initial, st, nil)

	next := initial
	next.Auth.AdminToken = "admin-2"
	next.Auth.AccessTokens = append(next.Auth.AccessTokens, config.AccessToken{
		Name: "extra", Token: "sk-2", Enabled: true, QuotaCalls: 20, CostPerRequest: 2,
	})
	next.Accounts = append(next.Accounts, config.Account{
		ID: "plus-b", Platform: "chatgpt-web", Enabled: true, Status: "healthy",
	})
	next.AccountPools[0].Members = []string{"plus-a", "plus-b"}

	applied, err := manager.Apply(context.Background(), next)
	if err != nil {
		t.Fatalf("apply config: %v", err)
	}

	if applied.Auth.AdminToken != "admin-2" {
		t.Fatalf("unexpected applied config: %+v", applied.Auth)
	}
	if manager.Current().Auth.AdminToken != "admin-2" {
		t.Fatalf("manager current not updated")
	}

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if saved.Auth.AdminToken != "admin-2" {
		t.Fatalf("saved config not updated: %+v", saved.Auth)
	}

	tokens, err := st.ListTokenAccounts(context.Background())
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected synced tokens, got %+v", tokens)
	}

	accounts, err := st.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected synced accounts, got %+v", accounts)
	}

	backups, err := filepath.Glob(configPath + ".bak-*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected one backup file, got %v", backups)
	}
}
