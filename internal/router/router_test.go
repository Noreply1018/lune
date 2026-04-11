package router

import (
	"testing"

	"lune/internal/config"
	"lune/internal/execution"
)

func TestResolveIncludesFallbackPoolsInOrder(t *testing.T) {
	r := New(config.Config{
		Platforms: []config.Platform{
			{ID: "chatgpt-web", Adapter: "chatgpt-web", Enabled: true},
			{ID: "claude-web", Adapter: "claude-web", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "chatgpt-web", Enabled: true, Status: "healthy"},
			{ID: "pro-b", Platform: "claude-web", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "chatgpt-plus", Platform: "chatgpt-web", Enabled: true, Members: []string{"plus-a"}},
			{ID: "claude-pro", Platform: "claude-web", Enabled: true, Members: []string{"pro-b"}},
		},
		Models: []config.ModelRoute{
			{
				Alias:       "cheap-chat",
				AccountPool: "chatgpt-plus",
				TargetModel: "gpt-4o",
				Fallbacks:   []string{"claude-pro:claude-3-7-sonnet"},
			},
		},
	})

	plans, err := r.Resolve(execution.Request{ModelAlias: "cheap-chat"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}
	if plans[0].AccountID != "plus-a" || plans[0].TargetModel != "gpt-4o" || plans[0].AttemptIndex != 0 {
		t.Fatalf("unexpected primary plan: %+v", plans[0])
	}
	if plans[1].AccountID != "pro-b" || plans[1].TargetModel != "claude-3-7-sonnet" || plans[1].AttemptIndex != 1 {
		t.Fatalf("unexpected fallback plan: %+v", plans[1])
	}
}

func TestResolveRejectsUnknownAlias(t *testing.T) {
	r := New(config.Config{})
	if _, err := r.Resolve(execution.Request{ModelAlias: "missing"}); err == nil {
		t.Fatalf("expected missing alias error")
	}
}
