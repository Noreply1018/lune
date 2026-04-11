package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	accountadapter "lune/internal/adapter/account"
	"lune/internal/auth"
	"lune/internal/config"
	"lune/internal/execution"
	"lune/internal/runtimeconfig"
	"lune/internal/store"
)

type fakeAdapter struct {
	responses map[string]fakeAdapterResponse
	calls     []string
}

type fakeAdapterResponse struct {
	status int
	body   string
	err    error
}

func (a *fakeAdapter) ID() string {
	return "openai-upstream"
}

func (a *fakeAdapter) Prepare(_ context.Context, req execution.Request, plan execution.Plan, platform config.Platform, account config.Account) (*execution.PreparedExecution, error) {
	payload := make(map[string]any, len(req.Payload)+1)
	for k, v := range req.Payload {
		payload[k] = v
	}
	payload["model"] = plan.TargetModel

	return &execution.PreparedExecution{
		Request:     req,
		Plan:        plan,
		Platform:    platform,
		Account:     account,
		TargetModel: plan.TargetModel,
		RawBody:     req.RawBody,
		Payload:     payload,
		Headers:     req.Headers.Clone(),
	}, nil
}

func (a *fakeAdapter) Execute(_ context.Context, prepared *execution.PreparedExecution) (*execution.RawResult, error) {
	a.calls = append(a.calls, prepared.Account.ID)
	resp := a.responses[prepared.Account.ID]
	if resp.err != nil {
		return nil, resp.err
	}
	return &execution.RawResult{
		StatusCode: resp.status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(resp.body)),
	}, nil
}

func (a *fakeAdapter) Normalize(_ context.Context, _ *execution.PreparedExecution, raw *execution.RawResult) (*execution.GatewayResponse, error) {
	return &execution.GatewayResponse{
		StatusCode: raw.StatusCode,
		Header:     raw.Header.Clone(),
		Body:       raw.Body,
	}, nil
}

func (a *fakeAdapter) Classify(raw *execution.RawResult, err error) execution.Outcome {
	if err != nil {
		return execution.OutcomeFinalFailure
	}
	if raw == nil {
		return execution.OutcomeFinalFailure
	}
	switch {
	case raw.StatusCode == http.StatusTooManyRequests || raw.StatusCode >= 500:
		return execution.OutcomeRetryableFailure
	case raw.StatusCode >= 200 && raw.StatusCode < 400:
		return execution.OutcomeSuccess
	default:
		return execution.OutcomeFinalFailure
	}
}

func TestChatCompletionsSuccessRecordsLedgerAndConsumesQuota(t *testing.T) {
	cfg := config.Config{
		Auth: config.AuthConfig{
			AccessTokens: []config.AccessToken{
				{Name: "default", Token: "sk-test", Enabled: true, QuotaCalls: 3, CostPerRequest: 1},
			},
		},
		Platforms: []config.Platform{
			{ID: "upstream", Adapter: "openai-upstream", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "upstream", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "default-pool", Platform: "upstream", Enabled: true, Members: []string{"plus-a"}},
		},
		Models: []config.ModelRoute{
			{Alias: "gpt-4o", TargetKind: "account_pool", TargetID: "default-pool", TargetModel: "gpt-4o"},
		},
	}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.SyncAccessTokens(context.Background(), cfg.Auth.AccessTokens); err != nil {
		t.Fatalf("sync tokens: %v", err)
	}
	if err := st.SyncAccounts(context.Background(), cfg.Accounts); err != nil {
		t.Fatalf("sync accounts: %v", err)
	}
	if err := st.SyncAccountPools(context.Background(), cfg.AccountPools); err != nil {
		t.Fatalf("sync pools: %v", err)
	}

	cfgManager := runtimeconfig.New("configs/config.json", cfg, st, nil)

	svc := New(cfgManager, nil, accountadapter.NewRegistry(&fakeAdapter{
		responses: map[string]fakeAdapterResponse{
			"plus-a": {status: http.StatusOK, body: `{"ok":true}`},
		},
	}), st, nil)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	req = req.WithContext(auth.WithAccessTokenName(req.Context(), "default"))
	rec := httptest.NewRecorder()

	if err := svc.ChatCompletions(rec, req); err != nil {
		t.Fatalf("chat completions: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	logs, err := st.ListRequestLogs(context.Background(), 10)
	if err != nil {
		t.Fatalf("list request logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].Success {
		t.Fatalf("unexpected request logs: %+v", logs)
	}

	ledger, err := st.ListUsageLedgerEntries(context.Background(), 10)
	if err != nil {
		t.Fatalf("list usage ledger: %v", err)
	}
	if len(ledger) != 1 || !ledger[0].Success || ledger[0].APICostUnits != 1 {
		t.Fatalf("unexpected usage ledger: %+v", ledger)
	}

	token, err := st.GetTokenAccount(context.Background(), "default")
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if token.UsedCalls != 1 {
		t.Fatalf("expected token to consume 1 call, got %+v", token)
	}

	accounts, err := st.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if accounts[0].LastSuccessAt == nil || accounts[0].LastError != "" {
		t.Fatalf("expected account success state update, got %+v", accounts[0])
	}
}

func TestChatCompletionsFallsBackOnRetryableStatus(t *testing.T) {
	cfg := config.Config{
		Platforms: []config.Platform{
			{ID: "upstream", Adapter: "openai-upstream", Enabled: true},
			{ID: "upstream-backup", Adapter: "openai-upstream", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "upstream", Enabled: true, Status: "healthy"},
			{ID: "plus-b", Platform: "upstream-backup", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "primary", Platform: "upstream", Enabled: true, Members: []string{"plus-a"}},
			{ID: "backup", Platform: "upstream-backup", Enabled: true, Members: []string{"plus-b"}},
		},
		Models: []config.ModelRoute{
			{Alias: "gpt-4o", TargetKind: "account_pool", TargetID: "primary", TargetModel: "gpt-4o", Fallbacks: []string{"backup:gpt-4.1-mini"}},
		},
	}

	adapter := &fakeAdapter{
		responses: map[string]fakeAdapterResponse{
			"plus-a": {status: http.StatusTooManyRequests, body: `{"error":"rate_limited"}`},
			"plus-b": {status: http.StatusOK, body: `{"ok":true}`},
		},
	}
	cfgManager := runtimeconfig.New("configs/config.json", cfg, nil, nil)
	svc := New(cfgManager, nil, accountadapter.NewRegistry(adapter), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	if err := svc.ChatCompletions(rec, req); err != nil {
		t.Fatalf("chat completions: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after fallback, got %d", rec.Code)
	}
	if len(adapter.calls) != 2 || adapter.calls[0] != "plus-a" || adapter.calls[1] != "plus-b" {
		t.Fatalf("unexpected adapter call order: %+v", adapter.calls)
	}
}

func TestChatCompletionsRecordsFailureBodySnippet(t *testing.T) {
	cfg := config.Config{
		Auth: config.AuthConfig{
			AccessTokens: []config.AccessToken{
				{Name: "default", Token: "sk-test", Enabled: true, QuotaCalls: 3, CostPerRequest: 1},
			},
		},
		Platforms: []config.Platform{
			{ID: "upstream", Adapter: "openai-upstream", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "upstream", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "default-pool", Platform: "upstream", Enabled: true, Members: []string{"plus-a"}},
		},
		Models: []config.ModelRoute{
			{Alias: "gpt-4o", TargetKind: "account_pool", TargetID: "default-pool", TargetModel: "gpt-4o"},
		},
	}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.SyncAccessTokens(context.Background(), cfg.Auth.AccessTokens); err != nil {
		t.Fatalf("sync tokens: %v", err)
	}
	if err := st.SyncAccounts(context.Background(), cfg.Accounts); err != nil {
		t.Fatalf("sync accounts: %v", err)
	}
	if err := st.SyncAccountPools(context.Background(), cfg.AccountPools); err != nil {
		t.Fatalf("sync pools: %v", err)
	}

	cfgManager := runtimeconfig.New("configs/config.json", cfg, st, nil)
	svc := New(cfgManager, nil, accountadapter.NewRegistry(&fakeAdapter{
		responses: map[string]fakeAdapterResponse{
			"plus-a": {status: http.StatusUnprocessableEntity, body: `{"detail":"missing field"}`},
		},
	}), st, nil)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	req = req.WithContext(auth.WithAccessTokenName(req.Context(), "default"))
	rec := httptest.NewRecorder()

	err = svc.ChatCompletions(rec, req)
	if err == nil {
		t.Fatalf("expected chat completions failure")
	}

	logs, err := st.ListRequestLogs(context.Background(), 10)
	if err != nil {
		t.Fatalf("list request logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 request log, got %+v", logs)
	}
	if !strings.Contains(logs[0].ErrorMessage, "missing field") {
		t.Fatalf("expected upstream body snippet in error message, got %+v", logs[0])
	}
}

func TestChatCompletionsReturnsNotImplementedWithoutAdapter(t *testing.T) {
	cfg := config.Config{
		Platforms: []config.Platform{
			{ID: "upstream", Adapter: "openai-upstream", Enabled: true},
		},
		Accounts: []config.Account{
			{ID: "plus-a", Platform: "upstream", Enabled: true, Status: "healthy"},
		},
		AccountPools: []config.AccountPool{
			{ID: "default-pool", Platform: "upstream", Enabled: true, Members: []string{"plus-a"}},
		},
		Models: []config.ModelRoute{
			{Alias: "gpt-4o", TargetKind: "account_pool", TargetID: "default-pool", TargetModel: "gpt-4o"},
		},
	}
	svc := New(runtimeconfig.New("configs/config.json", cfg, nil, nil), nil, accountadapter.NewRegistry(), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	err := svc.ChatCompletions(rec, req)
	if err == nil {
		t.Fatalf("expected not implemented error")
	}

	proxyErr, ok := err.(*ProxyError)
	if !ok {
		t.Fatalf("expected proxy error, got %T", err)
	}
	if proxyErr.Status != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", proxyErr.Status)
	}
}
