package config

import "testing"

func TestValidateAcceptsMinimalValidConfig(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port: 7788,
		},
		Auth: AuthConfig{
			AdminToken: "admin",
			AccessTokens: []AccessToken{
				{Name: "default", Token: "sk-test", Enabled: true},
			},
		},
		Platforms: []Platform{
			{ID: "upstream", Enabled: true},
		},
		Accounts: []Account{
			{ID: "plus-a", Platform: "upstream", Enabled: true},
		},
		AccountPools: []AccountPool{
			{ID: "default-pool", Platform: "upstream", Enabled: true, Members: []string{"plus-a"}},
		},
		Models: []ModelRoute{
			{Alias: "gpt-4o", TargetKind: "account_pool", TargetID: "default-pool", TargetModel: "gpt-4o"},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected config to validate, got error: %v", err)
	}
}

func TestValidateRejectsUnknownModelTarget(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port: 7788,
		},
		Auth: AuthConfig{
			AdminToken: "admin",
			AccessTokens: []AccessToken{
				{Name: "default", Token: "sk-test", Enabled: true},
			},
		},
		Platforms: []Platform{
			{ID: "upstream", Enabled: true},
		},
		Models: []ModelRoute{
			{Alias: "plus-chat", TargetKind: "account_pool", TargetID: "missing-pool", TargetModel: "gpt-4o"},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected unknown target validation error")
	}
}

func TestNormalizeAccountPoolTarget(t *testing.T) {
	cfg := Config{
		Models: []ModelRoute{
			{Alias: "plus-chat", AccountPool: "chatgpt-plus", TargetModel: "gpt-4o"},
		},
	}

	cfg.normalize()

	if cfg.Models[0].TargetKind != "account_pool" || cfg.Models[0].TargetID != "chatgpt-plus" {
		t.Fatalf("expected model target to be normalized: %+v", cfg.Models[0])
	}
}
