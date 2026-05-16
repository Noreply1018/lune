package router

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"lune/internal/store"
)

func TestForceAccountCannotBypassUnroutableAccount(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Probe Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:      "recovering-account",
		SourceKind: "openai_compat",
		BaseURL:    "http://example.invalid/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "previous failure"); err != nil {
		t.Fatalf("UpdateAccountHealth: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-probe"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	if _, err := rt.Resolve("gpt-probe", &poolID, nil); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected normal routing to reject unhealthy account, got %v", err)
	}

	if _, err := rt.Resolve("gpt-probe", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected forced routing to reject unhealthy account, got %v", err)
	}
}

func TestRoutingSkipsAccountInServingCooldown(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	firstID, err := st.CreateAccount(&store.Account{
		Label:      "cooldown",
		SourceKind: "openai_compat",
		BaseURL:    "http://first.example/v1",
		APIKey:     "sk-first",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount first: %v", err)
	}
	secondID, err := st.CreateAccount(&store.Account{
		Label:      "healthy",
		SourceKind: "openai_compat",
		BaseURL:    "http://second.example/v1",
		APIKey:     "sk-second",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount second: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, firstID); err != nil {
		t.Fatalf("AddPoolMember first: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, secondID); err != nil {
		t.Fatalf("AddPoolMember second: %v", err)
	}
	if err := st.RefreshAccountModels(firstID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels first: %v", err)
	}
	if err := st.RefreshAccountModels(secondID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels second: %v", err)
	}
	if err := st.MarkAccountServingFailure(firstID, "EOF", time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("MarkAccountServingFailure: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected route to second account, got %v", err)
	}
	if resolved.AccountID != secondID {
		t.Fatalf("expected second account, got %d", resolved.AccountID)
	}
}

func TestRoutingAllowsAccountAfterServingCooldownExpires(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:      "expired-cooldown",
		SourceKind: "openai_compat",
		BaseURL:    "http://example.invalid/v1",
		APIKey:     "sk-test",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	if err := st.MarkAccountServingFailure(accountID, "EOF", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("MarkAccountServingFailure: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected expired cooldown to route, got %v", err)
	}
	if resolved.AccountID != accountID {
		t.Fatalf("expected account %d, got %d", accountID, resolved.AccountID)
	}
}

func TestRoutingSkipsAccountInServingError(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "serving-error",
		SourceKind:    "openai_compat",
		BaseURL:       "http://example.invalid/v1",
		APIKey:        "sk-test",
		Provider:      "openai",
		ServingStatus: "error",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	if _, err := rt.Resolve("gpt-test", &poolID, nil); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected serving error account to be skipped, got %v", err)
	}
}

func TestRoutingCpaAccountsFailClosedUntilRuntimeBindingSupported(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{Label: "CPA", BaseURL: "http://cpa.example", APIKey: "sk", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	suspectID := createRouterCpaAccount(t, st, poolID, serviceID, "suspect", "auth_suspect")
	okID := createRouterCpaAccount(t, st, poolID, serviceID, "ok", "ok")

	rt := New(store.NewRoutingCache(st))
	if _, err := rt.Resolve("gpt-test", &poolID, nil); !errors.Is(err, ErrRuntimeBinding) {
		t.Fatalf("expected CPA accounts to fail closed before runtime binding support, got %v", err)
	}
	if _, err := rt.Resolve("gpt-test", &poolID, &okID); !errors.Is(err, ErrRuntimeBinding) {
		t.Fatalf("expected forced ok CPA account to fail closed, got %v", err)
	}
	if _, err := rt.Resolve("gpt-test", &poolID, &suspectID); !errors.Is(err, ErrRuntimeBinding) {
		t.Fatalf("expected forced auth_suspect CPA account to fail closed, got %v", err)
	}

	if err := st.UpdateAccountCpaCredentialStatus(okID, "needs_login", "auth_failed", "bad", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaCredentialStatus: %v", err)
	}
	rt = New(store.NewRoutingCache(st))
	if _, err := rt.Resolve("gpt-test", &poolID, nil); !errors.Is(err, ErrRuntimeBinding) {
		t.Fatalf("expected CPA fallback to remain closed, got %v", err)
	}
}

func TestRoutingCpaRuntimeBindingDoesNotMaskSpecificBlockingState(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{Label: "CPA", BaseURL: "http://cpa.example", APIKey: "sk", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	needsLoginID := createRouterCpaAccount(t, st, poolID, serviceID, "needs-login", "needs_login")
	expiredID := createRouterCpaAccount(t, st, poolID, serviceID, "expired", "ok")
	if err := st.UpdateAccountCpaSubscriptionStatus(expiredID, "expired", "expired", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaSubscriptionStatus: %v", err)
	}
	blockedID := createRouterCpaAccount(t, st, poolID, serviceID, "blocked", "ok")
	if err := st.UpdateAccountCodexQuotaStatus(blockedID, "blocked", "limit reached", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCodexQuotaStatus: %v", err)
	}
	model429ID := createRouterCpaAccount(t, st, poolID, serviceID, "model-429", "ok")
	if err := st.UpdateAccountCodexQuotaStatus(model429ID, "error", "HTTP 429 from model request", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCodexQuotaStatus: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	for _, accountID := range []int64{needsLoginID, expiredID, blockedID, model429ID} {
		if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
			t.Fatalf("expected specific CPA blocker to remain no_healthy_account for forced account %d, got %v", accountID, err)
		}
	}
	if _, err := rt.Resolve("gpt-test", &poolID, nil); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected all specifically blocked CPA accounts to remain no_healthy_account, got %v", err)
	}
}

func TestRoutingCpaAuthSuspectIsRoutableButDeprioritizedWhenBindingSupported(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{Label: "CPA", BaseURL: "http://cpa.example", APIKey: "sk", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	suspectID := createRouterCpaAccount(t, st, poolID, serviceID, "suspect", "auth_suspect")
	okID := createRouterCpaAccount(t, st, poolID, serviceID, "ok", "ok")

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected route, got %v", err)
	}
	if resolved.AccountID != okID {
		t.Fatalf("expected ok account %d to outrank auth_suspect account %d, got %d", okID, suspectID, resolved.AccountID)
	}

	if err := st.UpdateAccountCpaCredentialStatus(okID, "needs_login", "auth_failed", "bad", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaCredentialStatus: %v", err)
	}
	rt = NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err = rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected auth_suspect fallback route, got %v", err)
	}
	if resolved.AccountID != suspectID {
		t.Fatalf("expected auth_suspect account %d when no ok account remains, got %d", suspectID, resolved.AccountID)
	}
}

func TestRoutingCpaSubscriptionStatusBlocksNormalAndForcedRoutes(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{Label: "CPA", BaseURL: "http://cpa.example", APIKey: "sk", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	for _, status := range []string{"expired", "free", "pending", "error", "unknown"} {
		accountID := createRouterCpaAccount(t, st, poolID, serviceID, "sub-"+status, "ok")
		if err := st.UpdateAccountCpaSubscriptionStatus(accountID, status, status, time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatalf("UpdateAccountCpaSubscriptionStatus: %v", err)
		}
		rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
		if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
			t.Fatalf("subscription status %s should block forced route, got %v", status, err)
		}
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	if _, err := rt.Resolve("gpt-test", &poolID, nil); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected normal route to skip all blocked subscription states, got %v", err)
	}
}

func TestRoutingAllowsNonCodexCpaWithoutSubscriptionStatus(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{Label: "CPA", BaseURL: "http://cpa.example", APIKey: "sk", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "claude-cpa",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "claude",
		CpaAccountKey:       "claude-key",
		CpaCredentialStatus: "ok",
		CpaQuotaStatus:      "ok",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"claude-test"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err := rt.Resolve("claude-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected non-Codex CPA account to route without subscription status, got %v", err)
	}
	if resolved.AccountID != accountID {
		t.Fatalf("expected account %d, got %d", accountID, resolved.AccountID)
	}
}

func createRouterCpaAccount(t *testing.T, st *store.Store, poolID, serviceID int64, label, credentialStatus string) int64 {
	t.Helper()
	accountID, err := st.CreateAccount(&store.Account{
		Label:                 label,
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         label + "-key",
		CpaCredentialStatus:   credentialStatus,
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	return accountID
}
