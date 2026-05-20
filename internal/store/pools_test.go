package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestListPoolMembersScansFullAccountColumns(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-members.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, _, err := st.CreatePoolWithDefaultToken("Primary", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{
		Label:         "CPA",
		BaseURL:       "http://127.0.0.1:8317",
		APIKey:        "sk-cpa",
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                    "Codex",
		SourceKind:               "cpa",
		Provider:                 "codex",
		CpaServiceID:             &serviceID,
		CpaProvider:              "codex",
		CpaAccountKey:            "acct-key",
		CpaEmail:                 "codex@example.com",
		CpaPlanType:              "plus",
		CpaOpenaiID:              "acct-openai",
		CpaCredentialStatus:      "needs_login",
		CpaCredentialReason:      "auth_failed",
		CpaCredentialLastError:   "refresh failed",
		CpaCredentialCheckedAt:   "2026-04-29T00:00:00Z",
		CpaSubscriptionExpiresAt: "2026-05-29T00:00:00Z",
		CpaSubscriptionFetchedAt: "2026-04-29T00:00:00Z",
		CpaSubscriptionLastError: "",
		Enabled:                  true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}

	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 1 || members[0].Account == nil {
		t.Fatalf("expected one member with account, got %+v", members)
	}
	account := members[0].Account
	if account.CpaCredentialStatus != "needs_login" ||
		account.CpaCredentialReason != "auth_failed" ||
		account.CpaSubscriptionExpiresAt != "2026-05-29T00:00:00Z" {
		t.Fatalf("account scan lost CPA fields: %+v", account)
	}
}

func TestAddPoolMemberIdempotentReturnsExistingMember(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-idempotent.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:      "Account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	firstID, created, err := st.AddPoolMemberIdempotent(poolID, accountID)
	if err != nil || !created {
		t.Fatalf("first attach: id=%d created=%v err=%v", firstID, created, err)
	}
	secondID, created, err := st.AddPoolMemberIdempotent(poolID, accountID)
	if err != nil || created {
		t.Fatalf("second attach: id=%d created=%v err=%v", secondID, created, err)
	}
	if secondID != firstID {
		t.Fatalf("expected existing member id %d, got %d", firstID, secondID)
	}
}

func TestDeletePoolWithAccountsDeletesMemberAccountsEvenWhenShared(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-delete-accounts.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	firstPoolID, err := st.CreatePool("First", 0, true)
	if err != nil {
		t.Fatalf("create first pool: %v", err)
	}
	secondPoolID, err := st.CreatePool("Second", 1, true)
	if err != nil {
		t.Fatalf("create second pool: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:      "Shared",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create shared account: %v", err)
	}
	if _, err := st.AddPoolMember(firstPoolID, accountID); err != nil {
		t.Fatalf("add first member: %v", err)
	}
	if _, err := st.AddPoolMember(secondPoolID, accountID); err != nil {
		t.Fatalf("add second member: %v", err)
	}

	if err := st.DeletePoolWithAccounts(firstPoolID); err != nil {
		t.Fatalf("delete pool with accounts: %v", err)
	}

	deletedPool, err := st.GetPool(firstPoolID)
	if err != nil {
		t.Fatalf("get deleted pool: %v", err)
	}
	if deletedPool != nil {
		t.Fatalf("expected first pool to be deleted, got %+v", deletedPool)
	}
	account, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get deleted account: %v", err)
	}
	if account != nil {
		t.Fatalf("expected shared account to be deleted, got %+v", account)
	}
	secondMembers, err := st.ListPoolMembers(secondPoolID)
	if err != nil {
		t.Fatalf("list second pool members: %v", err)
	}
	if len(secondMembers) != 0 {
		t.Fatalf("expected shared account membership to be removed from second pool, got %+v", secondMembers)
	}
}

func TestPoolRoutableCountHonorsQuotaAndServingState(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-routable.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	blockedID, err := st.CreateAccount(&Account{
		Label:                 "Blocked",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-blocked@example.com-plus",
		CpaCredentialStatus:   "ok",
		CpaQuotaStatus:        "blocked",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create blocked account: %v", err)
	}
	healthyID, err := st.CreateAccount(&Account{
		Label:      "Healthy",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create healthy account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, blockedID); err != nil {
		t.Fatalf("add blocked member: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, healthyID); err != nil {
		t.Fatalf("add healthy member: %v", err)
	}
	if err := st.MarkAccountServingFailure(healthyID, "EOF", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("mark serving failure: %v", err)
	}
	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected no routable accounts, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountMatchesCpaRouteState(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-cpa-routable.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	authSuspectID, err := st.CreateAccount(&Account{
		Label:                 "Auth suspect",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-suspect@example.com-plus",
		CpaCredentialStatus:   "auth_suspect",
		CpaSubscriptionStatus: "active",
		CpaQuotaStatus:        "ok",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create auth suspect account: %v", err)
	}
	expiredSubID, err := st.CreateAccount(&Account{
		Label:                 "Expired subscription",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-expired@example.com-plus",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "expired",
		CpaQuotaStatus:        "ok",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create expired subscription account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, authSuspectID); err != nil {
		t.Fatalf("add auth suspect member: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, expiredSubID); err != nil {
		t.Fatalf("add expired subscription member: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected CPA accounts to fail closed before runtime binding support, got %d", pool.RoutableAccountCount)
	}

	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}
	pool, err = st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool after enabling pinning: %v", err)
	}
	if pool.RoutableAccountCount != 1 {
		t.Fatalf("expected only auth_suspect CPA account to be routable with provider pinning, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountBlocksCodexModelRequest429Evidence(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                 "Model limited",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-limited@example.com-plus",
		CpaCredentialStatus:   "ok",
		CpaQuotaStatus:        "error",
		CpaQuotaLastError:     "HTTP 429 from model request",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected Codex model-request 429 evidence to block routing, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountAllowsNonCodexCpaWithoutSubscriptionStatus(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-non-codex.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:               "Claude CPA",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "claude",
		CpaAccountKey:       "claude-key",
		CpaCredentialStatus: "ok",
		CpaQuotaStatus:      "ok",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 1 {
		t.Fatalf("expected non-Codex CPA account to be routable without subscription status, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountBlocksMixedCaseCodexWithoutActiveSubscription(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                 "Mixed case Codex",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "Codex",
		CpaAccountKey:         "codex-key",
		CpaCredentialStatus:   "ok",
		CpaQuotaStatus:        "ok",
		CpaSubscriptionStatus: "unknown",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected mixed-case Codex account with unknown subscription to be blocked, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountAllowsCodexAccessEligibleWithoutSubscription(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                 "Free eligible",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "Codex",
		CpaAccountKey:         "codex-free@example.com-free",
		CpaCredentialStatus:   "ok",
		CpaQuotaStatus:        "unknown",
		CpaSubscriptionStatus: "unknown",
		CpaAccessStatus:       "eligible",
		CpaAccessReason:       "wham_usage_allowed",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 1 {
		t.Fatalf("expected access eligible Codex account to be routable with quota warn, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountBlocksCodexAccessPendingEvenWithActiveSubscription(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                 "Pending access",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-pending@example.com-plus",
		CpaCredentialStatus:   "ok",
		CpaQuotaStatus:        "ok",
		CpaSubscriptionStatus: "active",
		CpaAccessStatus:       "pending",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected explicit access pending to block routing, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountHonorsDiagnosticSchedulerStatus(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stable   string
		expected int
	}{
		{name: "usable", stable: "usable", expected: 1},
		{name: "quota-auth-failed-but-usable", stable: "quota_probe_auth_failed_but_usable", expected: 1},
		{name: "unknown", stable: "unknown", expected: 1},
		{name: "banned", stable: "banned", expected: 0},
		{name: "quota-exhausted", stable: "quota_exhausted", expected: 0},
		{name: "auth-invalid", stable: "auth_invalid", expected: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			poolID, err := st.CreatePool("Pool", 0, true)
			if err != nil {
				t.Fatalf("create pool: %v", err)
			}
			accountID, err := st.CreateAccount(&Account{
				Label:      tc.name,
				SourceKind: "openai_compat",
				BaseURL:    "https://" + tc.name + ".example.com/v1",
				APIKey:     "sk-test",
				Enabled:    true,
			})
			if err != nil {
				t.Fatalf("create account: %v", err)
			}
			if _, err := st.AddPoolMember(poolID, accountID); err != nil {
				t.Fatalf("add member: %v", err)
			}
			if err := st.UpdateAccountDiagnostic(accountID, AccountDiagnosticUpdate{
				StableDiagnosticStatus: tc.stable,
				LastProbeStatus:        "succeeded",
				SafeSummary:            tc.name,
			}); err != nil {
				t.Fatalf("update diagnostic: %v", err)
			}

			pool, err := st.GetPool(poolID)
			if err != nil || pool == nil {
				t.Fatalf("get pool: %v", err)
			}
			if pool.RoutableAccountCount != tc.expected {
				t.Fatalf("expected routable count %d, got %d", tc.expected, pool.RoutableAccountCount)
			}
		})
	}
}

func TestPoolRoutableCountRejectsDirtyFatalDiagnosticScheduler(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:      "Dirty banned",
		SourceKind: "openai_compat",
		BaseURL:    "https://dirty.example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := st.DB().Exec(
		`UPDATE account_diagnostics
		 SET stable_diagnostic_status='banned', scheduler_status='eligible', scheduler_override=''
		 WHERE account_id=?`,
		accountID,
	); err != nil {
		t.Fatalf("dirty diagnostic update: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected dirty fatal diagnostic to be non-routable, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountTrimsDiagnosticSchedulerAndOverride(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:      "Blank override",
		SourceKind: "openai_compat",
		BaseURL:    "https://blank.example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := st.DB().Exec(
		`UPDATE account_diagnostics
		 SET stable_diagnostic_status=' banned ', scheduler_status=' eligible ', scheduler_override='   '
		 WHERE account_id=?`,
		accountID,
	); err != nil {
		t.Fatalf("dirty diagnostic update: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected blank override with fatal stable status to be non-routable, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountAcceptsTrimmedEligibleScheduler(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:      "Trimmed eligible",
		SourceKind: "openai_compat",
		BaseURL:    "https://trimmed.example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := st.DB().Exec(
		`UPDATE account_diagnostics
		 SET stable_diagnostic_status=' usable ', scheduler_status=' eligible ', scheduler_override=''
		 WHERE account_id=?`,
		accountID,
	); err != nil {
		t.Fatalf("dirty diagnostic update: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 1 {
		t.Fatalf("expected trimmed eligible scheduler to stay routable, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountRejectsUnknownDiagnosticSchedulerStatus(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:      "Unknown scheduler",
		SourceKind: "openai_compat",
		BaseURL:    "https://unknown.example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := st.DB().Exec(
		`UPDATE account_diagnostics
		 SET stable_diagnostic_status='usable', scheduler_status='future_value'
		 WHERE account_id=?`,
		accountID,
	); err != nil {
		t.Fatalf("dirty diagnostic update: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected unknown scheduler status to be non-routable, got %d", pool.RoutableAccountCount)
	}
}

func TestPoolRoutableCountBlocksMixedCaseCredentialStatus(t *testing.T) {
	st := newTestStore(t)
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{Label: "CPA", BaseURL: "https://cpa.example.com", Enabled: true})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                 "Mixed case credential",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "claude",
		CpaAccountKey:         "claude-key",
		CpaCredentialStatus:   "NEEDS_LOGIN",
		CpaSubscriptionStatus: "unknown",
		CpaQuotaStatus:        "ok",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("enable provider pinning setting: %v", err)
	}

	pool, err := st.GetPool(poolID)
	if err != nil || pool == nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.RoutableAccountCount != 0 {
		t.Fatalf("expected mixed-case credential status to be blocked, got %d", pool.RoutableAccountCount)
	}
}
