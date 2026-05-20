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

func TestRoutingPrefersQuotaBlockedOverNeedsLoginReason(t *testing.T) {
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
	accountID := createRouterCpaAccount(t, st, poolID, serviceID, "quota-vs-login", "needs_login")
	if err := st.UpdateAccountCodexQuotaStatus(accountID, "blocked", "quota blocked by upstream", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCodexQuotaStatus: %v", err)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	decision := rt.accountDecision(&store.Account{
		Enabled:             true,
		Status:              "healthy",
		SourceKind:          "cpa",
		CpaProvider:         "codex",
		CpaCredentialStatus: "needs_login",
		CpaQuotaStatus:      "blocked",
	}, ResolveOptions{})
	if decision.Reason != "quota_blocked" {
		t.Fatalf("expected quota to win over login in routing reason, got %+v", decision)
	}
	if decision.Routable {
		t.Fatalf("expected blocked account to remain unroutable, got %+v", decision)
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

func TestRoutingOrderedPolicyChoosesFirstLightlyDegradedAccount(t *testing.T) {
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
	suspectID := createRouterCpaAccount(t, st, poolID, serviceID, "suspect-first", "auth_suspect")
	okID := createRouterCpaAccount(t, st, poolID, serviceID, "ok-second", "ok")

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected health_first route, got %v", err)
	}
	if resolved.AccountID != okID {
		t.Fatalf("health_first should choose ok account %d over suspect account %d, got %d", okID, suspectID, resolved.AccountID)
	}

	if err := st.UpdatePoolWithRoutingPolicy(poolID, "Pool", 0, true, "ordered"); err != nil {
		t.Fatalf("UpdatePoolWithRoutingPolicy: %v", err)
	}
	rt = NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err = rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected ordered route, got %v", err)
	}
	if resolved.AccountID != suspectID {
		t.Fatalf("ordered should choose first routable account %d, got %d", suspectID, resolved.AccountID)
	}
}

func TestRoutingPoliciesDoNotFallbackToExplicitModelMismatch(t *testing.T) {
	for _, policy := range []string{"health_first", "ordered"} {
		t.Run(policy, func(t *testing.T) {
			st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
			if err != nil {
				t.Fatalf("store.New: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })

			poolID, err := st.CreatePool("Pool", 0, true)
			if err != nil {
				t.Fatalf("CreatePool: %v", err)
			}
			if err := st.UpdatePoolWithRoutingPolicy(poolID, "Pool", 0, true, policy); err != nil {
				t.Fatalf("UpdatePoolWithRoutingPolicy: %v", err)
			}
			firstID, err := st.CreateAccount(&store.Account{
				Label:      "explicit-mismatch",
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
				Label:      "unknown-models",
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
			if err := st.RefreshAccountModels(firstID, []string{"gpt-a"}); err != nil {
				t.Fatalf("RefreshAccountModels first: %v", err)
			}

			rt := New(store.NewRoutingCache(st))
			resolved, err := rt.Resolve("gpt-b", &poolID, nil)
			if err != nil {
				t.Fatalf("expected fallback to unknown model account, got %v", err)
			}
			if resolved.AccountID != secondID {
				t.Fatalf("%s should skip explicit mismatch %d and choose unknown %d, got %d", policy, firstID, secondID, resolved.AccountID)
			}
		})
	}
}

func TestRoutingCodexAccessStatusControlsNormalAndForcedRoutes(t *testing.T) {
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
	eligibleID := createRouterCpaAccount(t, st, poolID, serviceID, "free-eligible", "ok")
	if err := st.UpdateAccountCpaSubscriptionStatus(eligibleID, "unknown", "free account", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaSubscriptionStatus eligible: %v", err)
	}
	if err := st.UpdateAccountCpaAccessStatus(eligibleID, "eligible", "wham_usage_allowed", "", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaAccessStatus eligible: %v", err)
	}
	unknownID := createRouterCpaAccount(t, st, poolID, serviceID, "free-unknown", "ok")
	if err := st.UpdateAccountCpaSubscriptionStatus(unknownID, "unknown", "free account", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaSubscriptionStatus unknown: %v", err)
	}
	if err := st.UpdateAccountCpaAccessStatus(unknownID, "unknown", "pending_probe", "", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaAccessStatus unknown: %v", err)
	}
	ineligibleID := createRouterCpaAccount(t, st, poolID, serviceID, "free-ineligible", "ok")
	if err := st.UpdateAccountCpaAccessStatus(ineligibleID, "ineligible", "access_denied", "access denied", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaAccessStatus ineligible: %v", err)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	for _, accountID := range []int64{unknownID, ineligibleID} {
		if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
			t.Fatalf("access blocked account %d should block forced route, got %v", accountID, err)
		}
	}
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected eligible Free account to route, got %v", err)
	}
	if resolved.AccountID != eligibleID {
		t.Fatalf("expected eligible access account %d, got %d", eligibleID, resolved.AccountID)
	}
}

func TestRoutingCodexSubscriptionStillProvidesPaidAccessEvidence(t *testing.T) {
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
	activeID := createRouterCpaAccount(t, st, poolID, serviceID, "paid-active", "ok")
	expiredID := createRouterCpaAccount(t, st, poolID, serviceID, "paid-expired", "ok")
	if err := st.UpdateAccountCpaSubscriptionStatus(expiredID, "expired", "expired", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaSubscriptionStatus: %v", err)
	}
	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected active paid subscription to route, got %v", err)
	}
	if resolved.AccountID != activeID {
		t.Fatalf("expected active account %d, got %d", activeID, resolved.AccountID)
	}
	if _, err := rt.Resolve("gpt-test", &poolID, &expiredID); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expired subscription should block forced route, got %v", err)
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

func TestRoutingUsesPersistedDiagnosticSchedulerStatus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stable      string
		scheduler   string
		wantBlocked bool
	}{
		{name: "usable", stable: "usable", scheduler: "eligible"},
		{name: "quota-auth-failed-but-usable", stable: "quota_probe_auth_failed_but_usable", scheduler: "eligible_with_warning"},
		{name: "unknown", stable: "unknown", scheduler: "eligible_with_warning"},
		{name: "banned", stable: "banned", scheduler: "ineligible", wantBlocked: true},
		{name: "quota-exhausted", stable: "quota_exhausted", scheduler: "ineligible", wantBlocked: true},
		{name: "auth-invalid", stable: "auth_invalid", scheduler: "ineligible", wantBlocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			accountID := createRouterCpaAccount(t, st, poolID, serviceID, tc.name, "ok")
			if err := st.UpdateAccountDiagnostic(accountID, store.AccountDiagnosticUpdate{
				StableDiagnosticStatus: tc.stable,
				LastProbeStatus:        "succeeded",
				SchedulerStatus:        tc.scheduler,
				SafeSummary:            tc.name,
			}); err != nil {
				t.Fatalf("UpdateAccountDiagnostic: %v", err)
			}

			rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
			resolved, err := rt.Resolve("gpt-test", &poolID, &accountID)
			if tc.wantBlocked {
				if !errors.Is(err, ErrNoHealthyAccount) {
					t.Fatalf("expected diagnostic status %s to block forced route, got %v", tc.stable, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected diagnostic status %s to route, got %v", tc.stable, err)
			}
			if resolved.AccountID != accountID {
				t.Fatalf("expected account %d, got %d", accountID, resolved.AccountID)
			}
		})
	}
}

func TestDiagnosticForceRouteBypassesCpaCredentialAndQuotaBlocks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		credential string
		quota      string
	}{
		{name: "needs-login", credential: "needs_login", quota: "unknown"},
		{name: "quota-blocked", credential: "ok", quota: "blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			accountID := createRouterCpaAccount(t, st, poolID, serviceID, tc.name, tc.credential)
			if tc.quota != "" {
				if err := st.UpdateAccountCodexQuotaStatus(accountID, tc.quota, "test quota state", time.Now().UTC().Format(time.RFC3339)); err != nil {
					t.Fatalf("UpdateAccountCodexQuotaStatus: %v", err)
				}
			}

			rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
			if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
				t.Fatalf("expected normal forced route to reject %s, got %v", tc.name, err)
			}
			resolved, err := rt.ResolveWithOptions("gpt-test", &poolID, &accountID, ResolveOptions{Diagnostic: true})
			if err != nil {
				t.Fatalf("expected diagnostic forced route to bypass %s, got %v", tc.name, err)
			}
			if resolved.AccountID != accountID {
				t.Fatalf("expected account %d, got %d", accountID, resolved.AccountID)
			}
		})
	}
}

func TestRoutingDoesNotAllowFatalDiagnosticWithEligibleSchedulerWithoutOverride(t *testing.T) {
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
	accountID := createRouterCpaAccount(t, st, poolID, serviceID, "dirty-banned", "ok")
	if _, err := st.DB().Exec(
		`UPDATE account_diagnostics
		 SET stable_diagnostic_status='banned', last_probe_status='upstream_banned_signal', scheduler_status='eligible', scheduler_override=''
		 WHERE account_id=?`,
		accountID,
	); err != nil {
		t.Fatalf("dirty diagnostic update: %v", err)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("fatal stable diagnostic must block without override, got %v", err)
	}
}

func TestRoutingTrimsBlankDiagnosticOverride(t *testing.T) {
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
	accountID := createRouterCpaAccount(t, st, poolID, serviceID, "blank-override", "ok")
	if err := st.UpdateAccountDiagnostic(accountID, store.AccountDiagnosticUpdate{
		StableDiagnosticStatus: " banned ",
		LastProbeStatus:        " upstream_banned_signal ",
		SchedulerStatus:        " eligible ",
		SchedulerOverride:      "   ",
		SafeSummary:            " blank override ",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnostic: %v", err)
	}

	diag, err := st.GetAccountDiagnostic(accountID)
	if err != nil {
		t.Fatalf("GetAccountDiagnostic: %v", err)
	}
	if diag.SchedulerStatus != "ineligible" || diag.SchedulerOverride != "" || diag.StableDiagnosticStatus != "banned" {
		t.Fatalf("expected trimmed default diagnostic mapping, got %+v", diag)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("blank override must not allow fatal stable diagnostic, got %v", err)
	}
}

func TestRoutingAllowsFatalDiagnosticOnlyWithAuditableOverride(t *testing.T) {
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
	accountID := createRouterCpaAccount(t, st, poolID, serviceID, "override-banned", "ok")
	if err := st.UpdateAccountDiagnostic(accountID, store.AccountDiagnosticUpdate{
		StableDiagnosticStatus: "banned",
		LastProbeStatus:        "upstream_banned_signal",
		SchedulerStatus:        "eligible",
		SchedulerOverride:      "manual smoke override",
		SafeSummary:            "operator override",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnostic: %v", err)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err := rt.Resolve("gpt-test", &poolID, &accountID)
	if err != nil {
		t.Fatalf("expected auditable override to route, got %v", err)
	}
	if resolved.AccountID != accountID {
		t.Fatalf("expected account %d, got %d", accountID, resolved.AccountID)
	}
}

func TestRoutingCacheReflectsDiagnosticAfterInvalidate(t *testing.T) {
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
	accountID := createRouterCpaAccount(t, st, poolID, serviceID, "cache-invalidate", "ok")
	cache := store.NewRoutingCache(st)
	rt := NewWithOptions(cache, Options{CpaRuntimeBindingSupported: true})
	if _, err := rt.Resolve("gpt-test", &poolID, &accountID); err != nil {
		t.Fatalf("expected initial route, got %v", err)
	}

	if err := st.UpdateAccountDiagnostic(accountID, store.AccountDiagnosticUpdate{
		StableDiagnosticStatus: "banned",
		LastProbeStatus:        "upstream_banned_signal",
		SafeSummary:            "banned after cache load",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnostic: %v", err)
	}
	cache.Invalidate()
	if _, err := rt.Resolve("gpt-test", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected invalidated cache to block banned account, got %v", err)
	}
}

func TestRoutingDeprioritizesDiagnosticWarningStatus(t *testing.T) {
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
	warnID := createRouterCpaAccount(t, st, poolID, serviceID, "quota-warning", "ok")
	okID := createRouterCpaAccount(t, st, poolID, serviceID, "usable", "ok")
	if err := st.UpdateAccountDiagnostic(warnID, store.AccountDiagnosticUpdate{
		StableDiagnosticStatus: "quota_probe_auth_failed_but_usable",
		LastProbeStatus:        "quota_probe_auth_failed",
		SchedulerStatus:        "eligible_with_warning",
		SafeSummary:            "quota probe auth failed but account remains usable",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnostic warn: %v", err)
	}
	if err := st.UpdateAccountDiagnostic(okID, store.AccountDiagnosticUpdate{
		StableDiagnosticStatus: "usable",
		LastProbeStatus:        "succeeded",
		SchedulerStatus:        "eligible",
		SafeSummary:            "usable",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnostic ok: %v", err)
	}

	rt := NewWithOptions(store.NewRoutingCache(st), Options{CpaRuntimeBindingSupported: true})
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected route, got %v", err)
	}
	if resolved.AccountID != okID {
		t.Fatalf("expected eligible account %d to outrank warning account %d, got %d", okID, warnID, resolved.AccountID)
	}

	resolved, err = rt.Resolve("gpt-test", &poolID, &warnID)
	if err != nil {
		t.Fatalf("expected warning account to remain force-routable, got %v", err)
	}
	if resolved.AccountID != warnID {
		t.Fatalf("expected forced warning account %d, got %d", warnID, resolved.AccountID)
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
