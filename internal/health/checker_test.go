package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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

	checker := NewChecker(st, cache, "", "", nil)
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

	checker := NewChecker(st, cache, "", "", nil)
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
	if acc.CpaSubscriptionLastError != "subscription metadata missing" {
		t.Fatalf("subscription error = %q", acc.CpaSubscriptionLastError)
	}
}

func TestDiscoverModelsSuccessDoesNotClearCpaCredentialError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

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
	checker := NewChecker(st, cache, "", "", nil)
	if _, err := checker.DiscoverModels(context.Background(), *acc); err != nil {
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

func TestDiscoverModelsFailureDoesNotMarkCpaCredentialError(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

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
	checker := NewChecker(st, cache, "", "", nil)
	if _, err := checker.DiscoverModels(context.Background(), *acc); err == nil {
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
