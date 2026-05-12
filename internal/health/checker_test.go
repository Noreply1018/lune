package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"lune/internal/cpa"
	"lune/internal/notify"
	"lune/internal/notify/drivers"
	"lune/internal/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "lune-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestProviderPinningSupportedDefaultsFalse(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	checker := NewChecker(st, cache, "", "", nil)
	if checker.ProviderPinningSupported() {
		t.Fatal("external CPA runtime must not enable provider pinning by default")
	}
	checker.SetProviderPinningSupported(true)
	if !checker.ProviderPinningSupported() {
		t.Fatal("expected explicit provider pinning support override")
	}
}

func TestCheckCpaServiceRetriesStartupConnectionFailures(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://cpa.local",
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	cache.Invalidate()

	attempts := 0
	checker := NewChecker(st, cache, "", "", nil)
	checker.cpaHealthAttempts = 3
	checker.cpaHealthRetryDelay = time.Millisecond
	checker.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			attempts++
			if r.Header.Get("Authorization") != "Bearer service-key" {
				t.Fatalf("missing CPA service API key header")
			}
			if attempts < 3 {
				return nil, errors.New("connection refused")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       http.NoBody,
			}, nil
		}),
	}

	svc, err := st.GetCpaServiceByID(serviceID)
	if err != nil {
		t.Fatalf("get cpa service: %v", err)
	}
	checker.checkCpaService(context.Background(), svc)

	updated, err := st.GetCpaServiceByID(serviceID)
	if err != nil {
		t.Fatalf("get updated cpa service: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 CPA health attempts, got %d", attempts)
	}
	if updated.Status != "healthy" || updated.LastError != "" {
		t.Fatalf("expected healthy CPA service, got status=%q error=%q", updated.Status, updated.LastError)
	}
}

func TestCheckCpaServiceDoesNotRetryHttpStatusFailures(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://cpa.local",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}

	attempts := 0
	checker := NewChecker(st, cache, "", "", nil)
	checker.cpaHealthAttempts = 3
	checker.cpaHealthRetryDelay = time.Millisecond
	checker.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       http.NoBody,
			}, nil
		}),
	}

	svc, err := st.GetCpaServiceByID(serviceID)
	if err != nil {
		t.Fatalf("get cpa service: %v", err)
	}
	checker.checkCpaService(context.Background(), svc)

	updated, err := st.GetCpaServiceByID(serviceID)
	if err != nil {
		t.Fatalf("get updated cpa service: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected one CPA health attempt, got %d", attempts)
	}
	if updated.Status != "error" || updated.LastError != "HTTP 401" {
		t.Fatalf("expected HTTP 401 error, got status=%q error=%q", updated.Status, updated.LastError)
	}
}

func TestFetchCodexSubscriptionsUsesAuthFileMetadata(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":         "codex-user@example.com-plus.json",
						"auth_index": "idx_1",
						"id_token": map[string]any{
							"chatgpt_subscription_active_until": "2026-05-08T05:02:45+00:00",
						},
					},
				},
			})
		case "/v0/management/api-call":
			t.Fatalf("subscription refresh must not use api-call")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "Codex",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, authDir, "", nil)
	checker.fetchCodexSubscriptions(context.Background())

	acc, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.CpaSubscriptionExpiresAt != "2026-05-08T05:02:45Z" {
		t.Fatalf("subscription expiry = %q", acc.CpaSubscriptionExpiresAt)
	}
	if acc.CpaSubscriptionLastError != "" {
		t.Fatalf("subscription error = %q", acc.CpaSubscriptionLastError)
	}
}

func TestFetchCodexSubscriptionMissingMetadataDoesNotMarkNeedsLogin(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":         "codex-user@example.com-plus.json",
						"auth_index": "idx_1",
						"id_token":   map[string]any{},
					},
				},
			})
		case "/v0/management/api-call":
			t.Fatalf("subscription refresh must not use api-call")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:                    "Codex",
		SourceKind:               "cpa",
		CpaServiceID:             &serviceID,
		CpaProvider:              "codex",
		CpaAccountKey:            "codex-user@example.com-plus",
		CpaCredentialStatus:      "unknown",
		CpaSubscriptionExpiresAt: "2026-05-01T00:00:00Z",
		Enabled:                  true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, authDir, "", nil)
	checker.fetchCodexSubscriptions(context.Background())

	acc, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.CpaCredentialStatus == "needs_login" {
		t.Fatalf("credential status should not become needs_login after missing subscription metadata")
	}
	if acc.CpaSubscriptionExpiresAt != "2026-05-01T00:00:00Z" {
		t.Fatalf("subscription expiry should be preserved, got %q", acc.CpaSubscriptionExpiresAt)
	}
	if acc.CpaSubscriptionLastError != "subscription metadata pending" {
		t.Fatalf("subscription error = %q", acc.CpaSubscriptionLastError)
	}
}

func TestRefreshAccountWaitsForCpaAuthIndex(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
		Disabled:  false,
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	authFilesAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			authFilesAttempts++
			files := []map[string]any{}
			if authFilesAttempts >= 3 {
				files = append(files, map[string]any{
					"id":         "codex-user@example.com-plus.json",
					"auth_index": "idx_1",
					"provider":   "codex",
					"email":      "user@example.com",
					"id_token": map[string]any{
						"plan_type": "plus",
					},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": files})
		case "/api/provider/codex/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "gpt-5-codex"}},
			})
		case "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 200,
				"body":        `{"period":"day","used":1}`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		APIKey:        "service-key",
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "Codex",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		CpaOpenaiID:   "acct_123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	result, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{
		Models:        true,
		Quota:         true,
		Subscription:  true,
		WaitAuthIndex: true,
	})
	if err != nil && result.QuotaError != "" && result.ModelsError != "" {
		t.Fatalf("refresh account: result=%+v err=%v", result, err)
	}
	if authFilesAttempts < 3 {
		t.Fatalf("expected auth-files retry, got %d attempts", authFilesAttempts)
	}
	if !result.ModelsRefreshed || !result.QuotaRefreshed {
		t.Fatalf("expected models and quota refreshed, got %+v", result)
	}
	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after refresh: %v", err)
	}
	if acc.CpaCredentialStatus != "ok" {
		t.Fatalf("expected credential ok, got status=%q reason=%q", acc.CpaCredentialStatus, acc.CpaCredentialReason)
	}
	if acc.CodexQuotaJSON == "" {
		t.Fatalf("expected quota JSON to be persisted")
	}
}

func TestCodexQuotaUnauthorizedDoesNotMarkCredentialNeedsLogin(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":         "codex-user@example.com-plus.json",
						"auth_index": "idx_1",
						"provider":   "codex",
						"email":      "user@example.com",
						"id_token": map[string]any{
							"chatgpt_account_id": "acct_123",
							"plan_type":          "plus",
						},
					},
				},
			})
		case "/api/provider/codex/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "gpt-5-codex"}},
			})
		case "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 401,
				"body":        `{"error":"unauthorized"}`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "Codex",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "codex",
		CpaAccountKey:       "codex-user@example.com-plus",
		CpaOpenaiID:         "acct_123",
		CpaCredentialStatus: "ok",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	result, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{
		Models:        true,
		Quota:         true,
		WaitAuthIndex: true,
	})
	if err == nil || result == nil || result.QuotaError != "HTTP 401" {
		t.Fatalf("expected quota HTTP 401 error, result=%+v err=%v", result, err)
	}
	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after refresh: %v", err)
	}
	if acc.CpaCredentialStatus == "needs_login" {
		t.Fatalf("quota 401 must not mark credential needs_login")
	}
	if acc.CpaQuotaStatus != "error" || acc.CpaQuotaLastError != "HTTP 401" {
		t.Fatalf("expected quota error state, got status=%q err=%q", acc.CpaQuotaStatus, acc.CpaQuotaLastError)
	}
}

func TestCodexQuotaSuccessDoesNotClearCredentialState(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{{
				"id":         "codex-user@example.com-plus.json",
				"auth_index": "idx_1",
				"provider":   "codex",
				"email":      "user@example.com",
				"id_token": map[string]any{
					"chatgpt_account_id": "acct_123",
				},
			}}})
		case "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 200,
				"body":        `{"period":"day","used":1}`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:                  "Codex",
		SourceKind:             "cpa",
		CpaServiceID:           &serviceID,
		CpaProvider:            "codex",
		CpaAccountKey:          "codex-user@example.com-plus",
		CpaOpenaiID:            "acct_123",
		CpaCredentialStatus:    "auth_suspect",
		CpaCredentialReason:    "prior_upstream_401",
		CpaCredentialLastError: "previous credential suspicion",
		CpaSubscriptionStatus:  "active",
		CpaQuotaStatus:         "ok",
		Enabled:                true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	result, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{Quota: true, WaitAuthIndex: true})
	if err != nil || result == nil || !result.QuotaRefreshed {
		t.Fatalf("expected quota refresh success, result=%+v err=%v", result, err)
	}
	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after refresh: %v", err)
	}
	if acc.CpaCredentialStatus != "auth_suspect" || acc.CpaCredentialReason != "prior_upstream_401" {
		t.Fatalf("quota success must not clear credential state, got status=%q reason=%q", acc.CpaCredentialStatus, acc.CpaCredentialReason)
	}
	if acc.CodexQuotaJSON == "" {
		t.Fatalf("expected quota JSON to be persisted")
	}
}

func TestCodexQuotaUnauthorizedPreservesBlockedSnapshotStatus(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{{
				"id":         "codex-user@example.com-plus.json",
				"auth_index": "idx_1",
				"provider":   "codex",
				"email":      "user@example.com",
				"id_token": map[string]any{
					"chatgpt_account_id": "acct_123",
				},
			}}})
		case "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 401,
				"body":        `{"error":"unauthorized"}`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "Codex",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "codex",
		CpaAccountKey:       "codex-user@example.com-plus",
		CpaOpenaiID:         "acct_123",
		CpaCredentialStatus: "ok",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.UpdateAccountCodexQuota(accountID, `{"rate_limit":{"allowed":false,"limit_reached":true}}`, "2026-05-10 12:00:00"); err != nil {
		t.Fatalf("seed quota snapshot: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	if _, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{Quota: true, WaitAuthIndex: true}); err == nil {
		t.Fatalf("expected quota refresh error")
	}
	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after refresh: %v", err)
	}
	if acc.CpaQuotaStatus != "blocked" || acc.CpaQuotaLastError != "HTTP 401" {
		t.Fatalf("expected blocked quota state, got status=%q err=%q", acc.CpaQuotaStatus, acc.CpaQuotaLastError)
	}
}

func TestSyncCpaMetadataDoesNotOverwriteNewerLoginWithOlderAuthFile(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID:   "acct_123",
		Email:       "user@example.com",
		Type:        "codex",
		Expired:     "2026-05-10T09:00:00Z",
		LastRefresh: "2026-05-10T08:00:00Z",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:            "Codex",
		SourceKind:       "cpa",
		CpaServiceID:     &serviceID,
		CpaProvider:      "codex",
		CpaAccountKey:    "codex-user@example.com-plus",
		CpaExpiredAt:     "2026-05-10T13:00:00Z",
		CpaLastRefreshAt: "2026-05-10T12:00:00Z",
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, authDir, "", nil)
	checker.syncCpaMetadata()

	acc, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.CpaLastRefreshAt != "2026-05-10T12:00:00Z" || acc.CpaExpiredAt != "2026-05-10T13:00:00Z" {
		t.Fatalf("older auth file metadata overwrote DB state: last_refresh=%q expired=%q", acc.CpaLastRefreshAt, acc.CpaExpiredAt)
	}
}

func TestResolveCpaRuntimeDoesNotOverwriteNewerLoginWithOlderAuthFile(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID:   "acct_123",
		Email:       "user@example.com",
		Type:        "codex",
		Expired:     "2026-05-10T09:00:00Z",
		LastRefresh: "2026-05-10T08:00:00Z",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:            "Codex",
		SourceKind:       "cpa",
		CpaServiceID:     &serviceID,
		CpaProvider:      "codex",
		CpaAccountKey:    "codex-user@example.com-plus",
		CpaExpiredAt:     "2026-05-10T13:00:00Z",
		CpaLastRefreshAt: "2026-05-10T12:00:00Z",
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()
	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}

	checker := NewChecker(st, cache, authDir, "", nil)
	if _, err := checker.resolveCpaRuntime(context.Background(), *acc, resolveOptions{}); err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after resolve: %v", err)
	}
	if acc.CpaLastRefreshAt != "2026-05-10T12:00:00Z" || acc.CpaExpiredAt != "2026-05-10T13:00:00Z" {
		t.Fatalf("older auth file metadata overwrote DB state: last_refresh=%q expired=%q", acc.CpaLastRefreshAt, acc.CpaExpiredAt)
	}
}

func TestRefreshAccountPendingAuthIndexIsNotARefreshError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{}})
			return
		case "/api/provider/codex/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "gpt-5-codex"}},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "Codex",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		CpaOpenaiID:   "acct_123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	result, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{
		Models:       true,
		Quota:        true,
		Subscription: true,
	})
	if err != nil {
		t.Fatalf("pending auth index should not fail refresh: result=%+v err=%v", result, err)
	}
	if !result.ModelsRefreshed {
		t.Fatalf("models should refresh while quota is pending: %+v", result)
	}
	if !result.QuotaPending || !result.SubscriptionPending {
		t.Fatalf("expected quota and subscription pending, got %+v", result)
	}
	if result.QuotaError != "" || result.SubscriptionError != "" {
		t.Fatalf("pending auth index should not populate errors: %+v", result)
	}
	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after refresh: %v", err)
	}
	if acc.CpaCredentialStatus == "needs_login" {
		t.Fatalf("pending auth index should not require login")
	}
	if acc.CpaCredentialStatus != "runtime_pending" || acc.CpaCredentialReason != "auth_index_pending" {
		t.Fatalf("unexpected credential state: status=%q reason=%q", acc.CpaCredentialStatus, acc.CpaCredentialReason)
	}
}

func TestRefreshAccountReloadsEmbeddedCpaWhenAuthIndexNeverAppears(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	var mu sync.Mutex
	authFilesAttempts := 0
	healthAttempts := 0
	reloaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/healthz":
			healthAttempts++
			reloaded = true
			w.WriteHeader(http.StatusOK)
		case "/v0/management/auth-files":
			authFilesAttempts++
			files := []map[string]any{}
			if reloaded {
				files = append(files, map[string]any{
					"id":         "codex-user@example.com-plus.json",
					"auth_index": "idx_1",
					"provider":   "codex",
					"email":      "user@example.com",
					"id_token": map[string]any{
						"plan_type": "plus",
					},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": files})
		case "/api/provider/codex/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "gpt-5-codex"}},
			})
		case "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 200,
				"body":        `{"period":"day","used":1}`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "Codex",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		CpaOpenaiID:   "acct_123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	checker.authIndexAttempts = 1
	checker.authIndexRetryDelay = time.Millisecond
	checker.cpaHealthAttempts = 3
	checker.cpaHealthRetryDelay = time.Millisecond
	signalPath := filepath.Join(t.TempDir(), "cpa-reload.signal")
	checker.SetCpaReloadSignalPath(signalPath)

	result, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{
		Models:        true,
		Quota:         true,
		WaitAuthIndex: true,
	})
	if err != nil {
		t.Fatalf("refresh after reload: result=%+v err=%v", result, err)
	}
	if !result.QuotaRefreshed || result.QuotaPending {
		t.Fatalf("expected quota refreshed after reload, got %+v", result)
	}
	if _, err := os.Stat(signalPath); err != nil {
		t.Fatalf("expected reload signal to be written: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if authFilesAttempts < 2 || healthAttempts == 0 {
		t.Fatalf("expected auth retry and health probe, authFiles=%d health=%d", authFilesAttempts, healthAttempts)
	}
}

func TestRefreshAccountReloadsEmbeddedCpaWhenAuthMetadataMismatches(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_new",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	var mu sync.Mutex
	reloaded := false
	authFilesAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/healthz":
			reloaded = true
			w.WriteHeader(http.StatusOK)
		case "/v0/management/auth-files":
			authFilesAttempts++
			accountID := "acct_old"
			if reloaded {
				accountID = "acct_new"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{{
				"id":         "codex-user@example.com-plus.json",
				"auth_index": "idx_1",
				"provider":   "codex",
				"email":      "user@example.com",
				"id_token": map[string]any{
					"chatgpt_account_id": accountID,
				},
			}}})
		case "/v0/management/api-call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status_code": 200,
				"body":        `{"period":"day","used":1}`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       server.URL,
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "Codex",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		CpaOpenaiID:   "acct_new",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()
	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}

	checker := NewChecker(st, cache, authDir, "", nil)
	checker.client = server.Client()
	checker.authIndexAttempts = 1
	checker.authIndexRetryDelay = time.Millisecond
	checker.cpaHealthAttempts = 2
	checker.cpaHealthRetryDelay = time.Millisecond
	signalPath := filepath.Join(t.TempDir(), "cpa-reload.signal")
	checker.SetCpaReloadSignalPath(signalPath)

	result, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{Quota: true, WaitAuthIndex: true})
	if err != nil || !result.QuotaRefreshed {
		t.Fatalf("expected refresh after metadata mismatch reload, result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(signalPath); err != nil {
		t.Fatalf("expected reload signal to be written: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reloaded || authFilesAttempts < 2 {
		t.Fatalf("expected reload and auth metadata retry, reloaded=%v attempts=%d", reloaded, authFilesAttempts)
	}
}

func TestRefreshAccountModelsSuccessDoesNotClearCpaCredentialError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/provider/codex/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-5-codex"}},
		})
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: server.URL,
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "Codex",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "codex",
		CpaAccountKey:       "codex-user@example.com-plus",
		CpaCredentialStatus: "needs_login",
		CpaCredentialReason: "refresh_failed",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	if _, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{Models: true}); err != nil {
		t.Fatalf("discover models: %v", err)
	}

	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after discover: %v", err)
	}
	if acc.CpaCredentialStatus != "needs_login" || acc.CpaCredentialReason != "refresh_failed" {
		t.Fatalf("credential state was cleared: status=%q reason=%q", acc.CpaCredentialStatus, acc.CpaCredentialReason)
	}
}

func TestRefreshAccountModelsFailureDoesNotMarkCpaCredentialError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
	}, "codex-user@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/provider/codex/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"access denied"}`))
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: server.URL,
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "Codex",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "codex",
		CpaAccountKey:       "codex-user@example.com-plus",
		CpaCredentialStatus: "unknown",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	cache.Invalidate()

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, authDir, "", nil)
	if _, err := checker.RefreshAccount(context.Background(), *acc, RefreshOptions{Models: true}); err == nil {
		t.Fatalf("expected discover models failure")
	}

	acc, err = st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account after discover: %v", err)
	}
	if acc.CpaCredentialStatus == "needs_login" {
		t.Fatalf("credential state should not be changed by models failure")
	}
}

func TestCheckOneDoesNotClearRecentGatewayAccountError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-test"}},
		})
	}))
	defer server.Close()

	accountID, err := st.CreateAccount(&store.Account{
		Label:      "recent-serving-error",
		SourceKind: "openai_compat",
		BaseURL:    server.URL + "/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "mock upstream EOF before completion"); err != nil {
		t.Fatalf("set account error: %v", err)
	}

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, "", "", nil)
	checker.client = server.Client()
	checker.checkOne(context.Background(), *acc)

	updated, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get updated account: %v", err)
	}
	if updated.Status != "error" || updated.LastError != "mock upstream EOF before completion" {
		t.Fatalf("model discovery should not clear recent gateway error, got status=%q last_error=%q", updated.Status, updated.LastError)
	}
	models, err := st.ListAccountModels(accountID)
	if err != nil {
		t.Fatalf("list account models: %v", err)
	}
	if len(models) != 1 || models[0] != "gpt-test" {
		t.Fatalf("expected model discovery to still refresh models, got %+v", models)
	}
}

func TestCheckOneDoesNotClearRecentGenericGatewayAccountError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-test"}},
		})
	}))
	defer server.Close()

	accountID, err := st.CreateAccount(&store.Account{
		Label:      "generic-serving-error",
		SourceKind: "openai_compat",
		BaseURL:    server.URL + "/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	gatewayErr := "The server had an error processing your request"
	if err := st.UpdateAccountHealth(accountID, "error", gatewayErr); err != nil {
		t.Fatalf("set account error: %v", err)
	}

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, "", "", nil)
	checker.client = server.Client()
	checker.checkOne(context.Background(), *acc)

	updated, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get updated account: %v", err)
	}
	if updated.Status != "error" || updated.LastError != gatewayErr {
		t.Fatalf("model discovery should not clear generic gateway error, got status=%q last_error=%q", updated.Status, updated.LastError)
	}
}

func TestCheckOneDoesNotOverwriteConcurrentGatewayAccountError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	var accountID int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if accountID > 0 {
			if err := st.UpdateAccountHealth(accountID, "error", "Rate limit reached"); err != nil {
				t.Errorf("set concurrent account error: %v", err)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-test"}},
		})
	}))
	defer server.Close()

	var err error
	accountID, err = st.CreateAccount(&store.Account{
		Label:      "concurrent-serving-error",
		SourceKind: "openai_compat",
		BaseURL:    server.URL + "/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, "", "", nil)
	checker.client = server.Client()
	checker.checkOne(context.Background(), *acc)

	updated, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get updated account: %v", err)
	}
	if updated.Status != "error" || updated.LastError != "Rate limit reached" {
		t.Fatalf("model discovery should not overwrite concurrent gateway error, got status=%q last_error=%q", updated.Status, updated.LastError)
	}
}

func TestCheckOneClearsRecentModelDiscoveryError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-test"}},
		})
	}))
	defer server.Close()

	accountID, err := st.CreateAccount(&store.Account{
		Label:      "recent-discovery-error",
		SourceKind: "openai_compat",
		BaseURL:    server.URL + "/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	discoveryErr := `Get "http://127.0.0.1:28080/fail/v1/models": dial tcp 127.0.0.1:28080: connect: connection refused`
	if err := st.UpdateAccountHealth(accountID, "error", discoveryErr); err != nil {
		t.Fatalf("set account error: %v", err)
	}

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, "", "", nil)
	checker.client = server.Client()
	checker.checkOne(context.Background(), *acc)

	updated, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get updated account: %v", err)
	}
	if updated.Status != "healthy" || updated.LastError != "" {
		t.Fatalf("model discovery success should clear discovery error, got status=%q last_error=%q", updated.Status, updated.LastError)
	}
}

func TestCheckOneClearsCpaServiceUnreachableHealthError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-test"}},
		})
	}))
	defer server.Close()

	accountID, err := st.CreateAccount(&store.Account{
		Label:      "cpa-service-health-error",
		SourceKind: "openai_compat",
		BaseURL:    server.URL + "/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "CPA service unreachable"); err != nil {
		t.Fatalf("set account error: %v", err)
	}

	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	checker := NewChecker(st, cache, "", "", nil)
	checker.client = server.Client()
	checker.checkOne(context.Background(), *acc)

	updated, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get updated account: %v", err)
	}
	if updated.Status != "healthy" || updated.LastError != "" {
		t.Fatalf("model discovery success should clear CPA service health error, got status=%q last_error=%q", updated.Status, updated.LastError)
	}
}

func newTestNotifier(st *store.Store) *notify.Service {
	return notify.NewServiceWithRegistry(
		st,
		notify.NewRegistry(drivers.NewWeChatWorkBotDriver()),
	)
}

func configureWebhook(t *testing.T, st *store.Store, url string) {
	t.Helper()
	if err := st.UpdateNotificationSettings(store.NotificationSettings{
		Enabled:           true,
		WebhookURL:        url,
		MentionMobileList: []string{},
	}); err != nil {
		t.Fatalf("configure webhook: %v", err)
	}
}

func TestSendWebhookNotificationsDedupesWithinBackoffWindow(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "Test CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}

	accountID, err := st.CreateAccount(&store.Account{
		Label:        "Broken Account",
		SourceKind:   "cpa",
		CpaServiceID: &serviceID,
		CpaProvider:  "openai",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer server.Close()
	configureWebhook(t, st, server.URL)

	checker := NewChecker(st, cache, "", "", newTestNotifier(st))

	if err := st.UpdateAccountHealth(accountID, "error", "boom"); err != nil {
		t.Fatalf("set account error: %v", err)
	}
	cache.Invalidate()
	notifications, err := st.ListSystemNotifications()
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifications) == 0 {
		t.Fatalf("expected system notifications to be generated")
	}

	checker.dispatchSystemNotifications(context.Background())
	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Event != "account_error" || deliveries[0].Status != "success" {
		t.Fatalf("expected one successful account_error delivery, got %+v", deliveries)
	}
	if attempts != 1 {
		t.Fatalf("expected first send attempt, got %d", attempts)
	}

	checker.dispatchSystemNotifications(context.Background())
	deliveries, err = st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries after dedupe: %v", err)
	}
	if len(deliveries) != 1 || attempts != 1 {
		t.Fatalf("expected dedupe to suppress resend, deliveries=%d attempts=%d", len(deliveries), attempts)
	}

	if err := st.UpdateAccountHealth(accountID, "healthy", ""); err != nil {
		t.Fatalf("recover account: %v", err)
	}
	cache.Invalidate()
	checker.dispatchSystemNotifications(context.Background())
	deliveries, err = st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries after recovery: %v", err)
	}
	if len(deliveries) != 1 || attempts != 1 {
		t.Fatalf("expected no resend after recovery, deliveries=%d attempts=%d", len(deliveries), attempts)
	}

	if err := st.UpdateAccountHealth(accountID, "error", "boom again"); err != nil {
		t.Fatalf("set account error again: %v", err)
	}
	cache.Invalidate()
	checker.dispatchSystemNotifications(context.Background())
	deliveries, err = st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries after repeat error: %v", err)
	}
	if len(deliveries) != 1 || attempts != 1 {
		t.Fatalf("expected backoff window dedupe to persist after recovery, deliveries=%d attempts=%d", len(deliveries), attempts)
	}
}

func TestSendWebhookNotificationsRetriesAfterFailure(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	accountID, err := st.CreateAccount(&store.Account{
		Label:      "Broken Account",
		SourceKind: "openai_compat",
		BaseURL:    "https://api.example.com/v1",
		APIKey:     "secret",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "boom"); err != nil {
		t.Fatalf("set account error: %v", err)
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer server.Close()
	configureWebhook(t, st, server.URL)

	checker := NewChecker(st, cache, "", "", newTestNotifier(st))
	checker.dispatchSystemNotifications(context.Background())
	waitFor(t, func() bool { return attempts >= 1 })
	if attempts != 1 {
		t.Fatalf("expected initial send attempt, got %d attempts", attempts)
	}
	outbox, err := st.ListDueNotificationOutbox(10)
	if err != nil {
		t.Fatalf("list due outbox: %v", err)
	}
	if len(outbox) != 0 {
		t.Fatalf("expected retry to be scheduled in the future")
	}
	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) == 0 || deliveries[0].Status != "failed" {
		t.Fatalf("expected failed delivery to be recorded, got %+v", deliveries)
	}
}

func TestSendWebhookNotificationsSendsSeverityUpgrade(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "Test CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}

	accountID, err := st.CreateAccount(&store.Account{
		Label:        "Expiring Account",
		SourceKind:   "cpa",
		CpaServiceID: &serviceID,
		CpaProvider:  "openai",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer server.Close()
	configureWebhook(t, st, server.URL)

	if err := st.UpdateSettings(map[string]string{
		"notification_expiring_days": "7",
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	expiringSoon := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if err := setAccountExpiry(st, accountID, expiringSoon); err != nil {
		t.Fatalf("set expiring-soon expiry: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, "", "", newTestNotifier(st))
	checker.dispatchSystemNotifications(context.Background())
	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Severity != "warning" {
		t.Fatalf("expected first warning delivery, got %+v", deliveries)
	}

	expiredAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := setAccountExpiry(st, accountID, expiredAt); err != nil {
		t.Fatalf("set expired expiry: %v", err)
	}
	cache.Invalidate()
	checker.dispatchSystemNotifications(context.Background())
	deliveries, err = st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries after severity upgrade: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("expected warning and critical deliveries, got %d", len(deliveries))
	}
	if deliveries[1].Severity != "warning" {
		t.Fatalf("expected first persisted delivery to be warning, got %q", deliveries[1].Severity)
	}
	if deliveries[0].Severity != "critical" {
		t.Fatalf("expected second persisted delivery to be critical, got %q", deliveries[0].Severity)
	}
}

func setAccountExpiry(st *store.Store, accountID int64, expiresAt string) error {
	return st.UpdateAccountCpaMetadata(accountID, expiresAt, time.Now().UTC().Format(time.RFC3339), false)
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !condition() {
		t.Fatalf("condition not met before timeout")
	}
}
