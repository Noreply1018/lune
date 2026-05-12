package store

import (
	"testing"
	"time"
)

func TestGetOverviewCountsDegradedPoolsAsHealthy(t *testing.T) {
	st := newTestStore(t)

	poolID, err := st.CreatePool("OpenAI", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	accountID, err := st.CreateAccount(&Account{
		Label:      "degraded-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
		Status:     "degraded",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "degraded", ""); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}

	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.PoolsTotal != 1 {
		t.Fatalf("expected 1 pool total, got %d", overview.PoolsTotal)
	}
	if overview.PoolsHealthy != 1 {
		t.Fatalf("expected degraded pool to count as healthy, got %d", overview.PoolsHealthy)
	}
}

func TestGetOverviewCountsPoolHealthyWhenAnyRoutableAccountExists(t *testing.T) {
	st := newTestStore(t)

	poolID, err := st.CreatePool("Mixed", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	routableID, err := st.CreateAccount(&Account{
		Label:      "healthy-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-healthy",
		Enabled:    true,
		Status:     "healthy",
	})
	if err != nil {
		t.Fatalf("create healthy account: %v", err)
	}

	errorID, err := st.CreateAccount(&Account{
		Label:      "error-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-error",
		Enabled:    true,
		Status:     "error",
	})
	if err != nil {
		t.Fatalf("create error account: %v", err)
	}
	if err := st.UpdateAccountHealth(errorID, "error", "upstream unavailable"); err != nil {
		t.Fatalf("mark error: %v", err)
	}

	if _, err := st.AddPoolMember(poolID, routableID); err != nil {
		t.Fatalf("add healthy member: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, errorID); err != nil {
		t.Fatalf("add error member: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.PoolsHealthy != 1 {
		t.Fatalf("expected pool with at least one routable account to count as healthy, got %d", overview.PoolsHealthy)
	}
}

func TestGetOverviewUsesRoutableAccountState(t *testing.T) {
	st := newTestStore(t)

	quotaPoolID, err := st.CreatePool("Quota blocked", 0, true)
	if err != nil {
		t.Fatalf("create quota pool: %v", err)
	}
	quotaID, err := st.CreateAccount(&Account{
		Label:          "quota-blocked",
		SourceKind:     "cpa",
		CpaProvider:    "codex",
		CpaAccountKey:  "codex-quota@example.com-plus",
		CpaQuotaStatus: "blocked",
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("create quota account: %v", err)
	}
	if _, err := st.AddPoolMember(quotaPoolID, quotaID); err != nil {
		t.Fatalf("add quota member: %v", err)
	}

	cooldownPoolID, err := st.CreatePool("Cooldown", 1, true)
	if err != nil {
		t.Fatalf("create cooldown pool: %v", err)
	}
	cooldownID, err := st.CreateAccount(&Account{
		Label:      "cooldown-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-cooldown",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create cooldown account: %v", err)
	}
	if err := st.MarkAccountServingFailure(cooldownID, "connection refused", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("mark cooldown: %v", err)
	}
	if _, err := st.AddPoolMember(cooldownPoolID, cooldownID); err != nil {
		t.Fatalf("add cooldown member: %v", err)
	}

	emptyCooldownPoolID, err := st.CreatePool("Empty cooldown", 2, true)
	if err != nil {
		t.Fatalf("create empty cooldown pool: %v", err)
	}
	emptyCooldownID, err := st.CreateAccount(&Account{
		Label:         "empty-cooldown-account",
		SourceKind:    "openai_compat",
		BaseURL:       "https://example.com/v1",
		APIKey:        "sk-empty-cooldown",
		ServingStatus: "cooldown",
		CooldownUntil: "",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create empty cooldown account: %v", err)
	}
	if _, err := st.AddPoolMember(emptyCooldownPoolID, emptyCooldownID); err != nil {
		t.Fatalf("add empty cooldown member: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.PoolsTotal != 3 {
		t.Fatalf("expected 3 pools total, got %d", overview.PoolsTotal)
	}
	if overview.PoolsHealthy != 0 {
		t.Fatalf("expected no healthy pools when all accounts are non-routable, got %d", overview.PoolsHealthy)
	}
	if overview.AccountsTotal != 3 {
		t.Fatalf("expected 3 enabled accounts, got %d", overview.AccountsTotal)
	}
	if overview.AccountsHealthy != 0 {
		t.Fatalf("expected no healthy accounts when all accounts are non-routable, got %d", overview.AccountsHealthy)
	}

	var unhealthyPools int
	for _, alert := range overview.Alerts {
		if alert.Type == "pool_unhealthy" {
			unhealthyPools++
		}
	}
	if unhealthyPools != 3 {
		t.Fatalf("expected 3 pool_unhealthy alerts, got %d: %+v", unhealthyPools, overview.Alerts)
	}
}

func TestGetOverviewCountsOnlyEnabledAccounts(t *testing.T) {
	st := newTestStore(t)

	enabledID, err := st.CreateAccount(&Account{
		Label:      "enabled-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-enabled",
		Enabled:    true,
		Status:     "healthy",
	})
	if err != nil {
		t.Fatalf("create enabled account: %v", err)
	}
	if err := st.DisableAccount(enabledID); err != nil {
		t.Fatalf("disable account: %v", err)
	}

	_, err = st.CreateAccount(&Account{
		Label:      "active-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-active",
		Enabled:    true,
		Status:     "healthy",
	})
	if err != nil {
		t.Fatalf("create active account: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.AccountsTotal != 1 {
		t.Fatalf("expected only enabled accounts to count, got %d", overview.AccountsTotal)
	}
	if overview.AccountsHealthy != 1 {
		t.Fatalf("expected only enabled healthy accounts to count, got %d", overview.AccountsHealthy)
	}
}

func TestUsageSummaryCountsOnlyConfirmedCpaBindingsAsTrustedUsage(t *testing.T) {
	st := newTestStore(t)

	if err := st.InsertLog(&RequestLog{
		RequestID:            "confirmed-cpa",
		AccessTokenName:      "pool-token",
		ModelRequested:       "gpt-test",
		ModelActual:          "gpt-test",
		AccountID:            10,
		StatusCode:           200,
		Success:              true,
		SourceKind:           "cpa",
		InputTokens:          3,
		OutputTokens:         4,
		RuntimeBindingStatus: "confirmed",
		RuntimeAuthIndex:     "idx-10",
		RuntimeAuthID:        "auth-10",
		RuntimeAccountKey:    "codex-a",
	}); err != nil {
		t.Fatalf("insert confirmed cpa log: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:            "unconfirmed-cpa",
		AccessTokenName:      "pool-token",
		ModelRequested:       "gpt-test",
		ModelActual:          "gpt-test",
		AccountID:            11,
		StatusCode:           503,
		Success:              false,
		SourceKind:           "cpa",
		InputTokens:          30,
		OutputTokens:         40,
		RuntimeBindingStatus: "runtime_error",
		RuntimeBindingReason: "runtime_auth_binding_unavailable",
	}); err != nil {
		t.Fatalf("insert unconfirmed cpa log: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:       "direct",
		AccessTokenName: "direct-token",
		ModelRequested:  "gpt-test",
		ModelActual:     "gpt-test",
		AccountID:       12,
		StatusCode:      200,
		Success:         true,
		SourceKind:      "openai_compat",
		InputTokens:     5,
		OutputTokens:    6,
	}); err != nil {
		t.Fatalf("insert direct log: %v", err)
	}

	stats, err := st.GetUsageSummary(UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if stats.TotalRequests != 2 {
		t.Fatalf("expected trusted total requests to exclude unconfirmed CPA logs, got %d", stats.TotalRequests)
	}
	if stats.TotalInputTokens != 8 || stats.TotalOutputTokens != 10 {
		t.Fatalf("expected trusted token totals to exclude unconfirmed CPA usage, got input=%d output=%d", stats.TotalInputTokens, stats.TotalOutputTokens)
	}
	if len(stats.ByAccount) != 2 {
		t.Fatalf("expected only confirmed CPA plus direct account stats, got %+v", stats.ByAccount)
	}
	seen := map[int64]bool{}
	for _, row := range stats.ByAccount {
		seen[row.AccountID] = true
	}
	if !seen[10] || !seen[12] || seen[11] {
		t.Fatalf("unexpected by-account rows: %+v", stats.ByAccount)
	}
	if len(stats.ByToken) != 2 {
		t.Fatalf("expected unconfirmed CPA token usage to be excluded, got %+v", stats.ByToken)
	}
	for _, row := range stats.ByToken {
		if row.TokenName == "pool-token" && (row.InputTokens != 3 || row.OutputTokens != 4 || row.Requests != 1) {
			t.Fatalf("pool-token should include only confirmed CPA usage, got %+v", row)
		}
	}
}

func TestGetPoolStatsCountsOnlyConfirmedCpaBindingsAsTrustedUsage(t *testing.T) {
	st := newTestStore(t)

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:            "confirmed-pool",
		AccessTokenName:      "pool-token",
		ModelRequested:       "gpt-test",
		ModelActual:          "gpt-test",
		PoolID:               poolID,
		AccountID:            20,
		StatusCode:           200,
		Success:              true,
		SourceKind:           "cpa",
		InputTokens:          7,
		OutputTokens:         8,
		RuntimeBindingStatus: "confirmed",
		RuntimeAuthIndex:     "idx-20",
		RuntimeAuthID:        "auth-20",
		RuntimeAccountKey:    "codex-b",
	}); err != nil {
		t.Fatalf("insert confirmed pool cpa log: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:            "unconfirmed-pool",
		AccessTokenName:      "pool-token",
		ModelRequested:       "gpt-test",
		ModelActual:          "gpt-test",
		PoolID:               poolID,
		AccountID:            21,
		StatusCode:           503,
		Success:              false,
		SourceKind:           "cpa",
		InputTokens:          70,
		OutputTokens:         80,
		RuntimeBindingStatus: "runtime_error",
		RuntimeBindingReason: "runtime_auth_binding_unavailable",
	}); err != nil {
		t.Fatalf("insert unconfirmed pool cpa log: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:       "direct-pool",
		AccessTokenName: "direct-token",
		ModelRequested:  "gpt-test",
		ModelActual:     "gpt-test",
		PoolID:          poolID,
		AccountID:       22,
		StatusCode:      200,
		Success:         true,
		SourceKind:      "openai_compat",
		InputTokens:     5,
		OutputTokens:    6,
	}); err != nil {
		t.Fatalf("insert direct pool log: %v", err)
	}

	stats, err := st.GetPoolStats(poolID, "24h")
	if err != nil {
		t.Fatalf("GetPoolStats: %v", err)
	}
	if stats.TotalRequests != 2 {
		t.Fatalf("expected trusted pool total requests to exclude unconfirmed CPA logs, got %d", stats.TotalRequests)
	}
	if stats.TotalInputTokens != 12 || stats.TotalOutputTokens != 14 {
		t.Fatalf("expected trusted pool token totals to exclude unconfirmed CPA usage, got input=%d output=%d", stats.TotalInputTokens, stats.TotalOutputTokens)
	}
	if len(stats.ByAccount) != 2 {
		t.Fatalf("expected only confirmed CPA plus direct pool stats, got %+v", stats.ByAccount)
	}
	seen := map[int64]bool{}
	for _, row := range stats.ByAccount {
		seen[row.AccountID] = true
	}
	if !seen[20] || !seen[22] || seen[21] {
		t.Fatalf("unexpected by-account rows: %+v", stats.ByAccount)
	}
	if len(stats.ByToken) != 2 {
		t.Fatalf("expected unconfirmed CPA pool token usage to be excluded, got %+v", stats.ByToken)
	}
	for _, row := range stats.ByToken {
		if row.TokenName == "pool-token" && (row.InputTokens != 7 || row.OutputTokens != 8 || row.Requests != 1) {
			t.Fatalf("pool-token should include only confirmed CPA usage, got %+v", row)
		}
	}
}
