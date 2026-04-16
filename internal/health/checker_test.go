package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"lune/internal/notify"
	"lune/internal/notify/drivers"
	"lune/internal/store"
	"lune/internal/syscfg"
)

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

func newTestNotifier(st *store.Store) *notify.Service {
	return notify.NewServiceWithRegistry(
		st,
		notify.NewRegistry(
			drivers.NewGenericWebhookDriver(),
			drivers.NewWeChatWorkBotDriver(),
			drivers.NewFeishuBotDriver(),
			drivers.NewEmailSMTPDriver(),
		),
	)
}

func createTestChannel(t *testing.T, st *store.Store, url string) {
	t.Helper()
	cfg := json.RawMessage(`{"schema":1,"url":"` + url + `"}`)
	if _, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "Test Webhook",
		Type:          "generic_webhook",
		Enabled:       true,
		Config:        cfg,
		Subscriptions: []store.NotificationSubscription{{Event: "*"}},
	}); err != nil {
		t.Fatalf("create notification channel: %v", err)
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
	}))
	defer server.Close()
	createTestChannel(t, st, server.URL)

	if err := st.UpdateSettings(map[string]string{
		"notification_error_enabled":    syscfg.BoolString(true),
		"notification_expiring_enabled": syscfg.BoolString(false),
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, "", newTestNotifier(st))

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
	channels, err := st.ListEnabledNotificationChannels()
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) == 0 {
		t.Fatalf("expected notification channels to be enabled")
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
	}))
	defer server.Close()
	createTestChannel(t, st, server.URL)

	if err := st.UpdateSettings(map[string]string{
		"notification_error_enabled":    syscfg.BoolString(true),
		"notification_expiring_enabled": syscfg.BoolString(false),
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, "", newTestNotifier(st))
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
	}))
	defer server.Close()
	createTestChannel(t, st, server.URL)

	if err := st.UpdateSettings(map[string]string{
		"webhook_enabled":               syscfg.BoolString(true),
		"webhook_url":                   server.URL,
		"notification_error_enabled":    syscfg.BoolString(false),
		"notification_expiring_enabled": syscfg.BoolString(true),
		"notification_expiring_days":    "7",
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	expiringSoon := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	if err := setAccountExpiry(st, accountID, expiringSoon); err != nil {
		t.Fatalf("set expiring-soon expiry: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, "", newTestNotifier(st))
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
