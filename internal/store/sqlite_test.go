package store

import (
	"context"
	"testing"
	"time"

	"lune/internal/config"
	"lune/internal/execution"
)

func TestStoreInsertAndListRequestLogs(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	err = s.InsertRequestLog(context.Background(), RequestLog{
		CreatedAt:    time.Now().UTC(),
		Method:       "POST",
		Path:         "/openai/v1/chat/completions",
		ModelAlias:   "gpt-4o",
		PlatformID:   "chatgpt-web",
		TargetModel:  "gpt-4o",
		AccessToken:  "default",
		StatusCode:   200,
		LatencyMS:    120,
		Success:      true,
		ErrorMessage: "",
	})
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}

	items, err := s.ListRequestLogs(context.Background(), 10)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 log item, got %d", len(items))
	}
	if items[0].ModelAlias != "gpt-4o" {
		t.Fatalf("unexpected model alias: %s", items[0].ModelAlias)
	}
}

func TestTokenAccountQuotaFlow(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	err = s.SyncAccessTokens(context.Background(), []config.AccessToken{
		{
			Name:           "default",
			Token:          "sk-test",
			Enabled:        true,
			QuotaCalls:     3,
			CostPerRequest: 1,
		},
	})
	if err != nil {
		t.Fatalf("sync tokens: %v", err)
	}

	account, allowed, err := s.CanConsume(context.Background(), "default")
	if err != nil {
		t.Fatalf("can consume: %v", err)
	}
	if !allowed || account.RemainingCalls() != 3 {
		t.Fatalf("unexpected initial quota state: %+v allowed=%v", account, allowed)
	}

	if err := s.ConsumeRequest(context.Background(), "default"); err != nil {
		t.Fatalf("consume 1: %v", err)
	}
	if err := s.ConsumeRequest(context.Background(), "default"); err != nil {
		t.Fatalf("consume 2: %v", err)
	}
	if err := s.ConsumeRequest(context.Background(), "default"); err != nil {
		t.Fatalf("consume 3: %v", err)
	}

	account, allowed, err = s.CanConsume(context.Background(), "default")
	if err != nil {
		t.Fatalf("can consume after usage: %v", err)
	}
	if allowed {
		t.Fatalf("expected quota to be exhausted")
	}
	if account.RemainingCalls() != 0 {
		t.Fatalf("expected remaining calls to be 0, got %d", account.RemainingCalls())
	}
}

func TestStoreSyncAndListAccountsAndPools(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.SyncAccounts(context.Background(), []config.Account{
		{
			ID:             "plus-a",
			Platform:       "chatgpt-web",
			Label:          "Primary Plus",
			CredentialType: "session",
			CredentialEnv:  "CHATGPT_SESSION_A",
			EgressProxyEnv: "CHATGPT_PROXY_A",
			PlanType:       "plus",
			Enabled:        true,
			Status:         "healthy",
		},
	}); err != nil {
		t.Fatalf("sync accounts: %v", err)
	}

	if err := s.SyncAccountPools(context.Background(), []config.AccountPool{
		{
			ID:       "chatgpt-plus",
			Platform: "chatgpt-web",
			Strategy: "sticky-first-healthy",
			Enabled:  true,
			Members:  []string{"plus-a"},
		},
	}); err != nil {
		t.Fatalf("sync pools: %v", err)
	}

	accounts, err := s.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != "plus-a" {
		t.Fatalf("unexpected accounts: %+v", accounts)
	}
	if accounts[0].EgressProxyEnv != "CHATGPT_PROXY_A" {
		t.Fatalf("unexpected account proxy env: %+v", accounts[0])
	}

	pools, err := s.ListAccountPools(context.Background())
	if err != nil {
		t.Fatalf("list pools: %v", err)
	}
	if len(pools) != 1 || pools[0].ID != "chatgpt-plus" {
		t.Fatalf("unexpected pools: %+v", pools)
	}
	if len(pools[0].Members) != 1 || pools[0].Members[0] != "plus-a" {
		t.Fatalf("unexpected pool members: %+v", pools[0].Members)
	}
}

func TestLedgerRecordSuccessAndFailure(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.SyncAccounts(context.Background(), []config.Account{
		{
			ID:             "plus-a",
			Platform:       "chatgpt-web",
			Label:          "Primary Plus",
			CredentialType: "session",
			CredentialEnv:  "CHATGPT_SESSION_A",
			EgressProxyEnv: "CHATGPT_PROXY_A",
			PlanType:       "plus",
			Enabled:        true,
			Status:         "healthy",
		},
	}); err != nil {
		t.Fatalf("sync accounts: %v", err)
	}

	successAt := time.Now().UTC()
	if err := s.RecordSuccess(context.Background(), execution.Record{
		RequestID:        "req-success",
		CreatedAt:        successAt,
		AccessTokenName:  "default",
		Method:           "POST",
		Endpoint:         "/openai/v1/chat/completions",
		ModelAlias:       "plus-chat",
		PlatformID:       "chatgpt-web",
		AccountID:        "plus-a",
		TargetModel:      "gpt-4o",
		StatusCode:       200,
		LatencyMS:        100,
		Success:          true,
		APICostUnits:     1,
		AccountCostUnits: 1,
		AccountCostType:  "request",
		LastSuccessAt:    &successAt,
	}); err != nil {
		t.Fatalf("record success: %v", err)
	}

	ledger, err := s.ListUsageLedgerEntries(context.Background(), 10)
	if err != nil {
		t.Fatalf("list ledger: %v", err)
	}
	if len(ledger) != 1 || !ledger[0].Success {
		t.Fatalf("unexpected ledger after success: %+v", ledger)
	}

	accounts, err := s.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if accounts[0].LastSuccessAt == nil || accounts[0].LastError != "" {
		t.Fatalf("unexpected account state after success: %+v", accounts[0])
	}

	if err := s.RecordFailure(context.Background(), execution.Record{
		RequestID:        "req-failure",
		CreatedAt:        time.Now().UTC(),
		AccessTokenName:  "default",
		Method:           "POST",
		Endpoint:         "/openai/v1/chat/completions",
		ModelAlias:       "plus-chat",
		PlatformID:       "chatgpt-web",
		AccountID:        "plus-a",
		TargetModel:      "gpt-4o",
		StatusCode:       501,
		LatencyMS:        120,
		Success:          false,
		APICostUnits:     0,
		AccountCostUnits: 0,
		AccountCostType:  "request",
		ErrorMessage:     "adapter missing",
		LastError:        "adapter missing",
	}); err != nil {
		t.Fatalf("record failure: %v", err)
	}

	ledger, err = s.ListUsageLedgerEntries(context.Background(), 10)
	if err != nil {
		t.Fatalf("list ledger after failure: %v", err)
	}
	if len(ledger) != 2 || ledger[0].Success {
		t.Fatalf("unexpected ledger after failure: %+v", ledger)
	}

	accounts, err = s.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts after failure: %v", err)
	}
	if accounts[0].LastError != "adapter missing" {
		t.Fatalf("expected last error to update, got %+v", accounts[0])
	}
}
