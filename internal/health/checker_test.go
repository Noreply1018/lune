package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"lune/internal/store"
	"lune/internal/syscfg"
	"lune/internal/webhook"
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

func TestSendWebhookNotificationsDedupesAndResendsAfterRecovery(t *testing.T) {
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

	var received []webhook.Payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload webhook.Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		received = append(received, payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := st.UpdateSettings(map[string]string{
		"webhook_enabled":               syscfg.BoolString(true),
		"webhook_url":                   server.URL,
		"notification_error_enabled":    syscfg.BoolString(true),
		"notification_expiring_enabled": syscfg.BoolString(false),
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	cache.Invalidate()

	sender := webhook.NewSender()
	checker := NewChecker(st, cache, "", sender)

	if err := st.UpdateAccountHealth(accountID, "error", "boom"); err != nil {
		t.Fatalf("set account error: %v", err)
	}
	cache.Invalidate()

	checker.sendWebhookNotifications(context.Background())
	waitFor(t, func() bool { return len(received) == 1 })
	if len(received) != 1 {
		t.Fatalf("expected first webhook, got %d", len(received))
	}
	if received[0].Event != "account_error" {
		t.Fatalf("expected account_error event, got %q", received[0].Event)
	}

	checker.sendWebhookNotifications(context.Background())
	if len(received) != 1 {
		t.Fatalf("expected dedupe to suppress resend, got %d", len(received))
	}

	if err := st.UpdateAccountHealth(accountID, "healthy", ""); err != nil {
		t.Fatalf("recover account: %v", err)
	}
	cache.Invalidate()
	checker.sendWebhookNotifications(context.Background())
	if len(received) != 1 {
		t.Fatalf("expected no webhook after recovery, got %d", len(received))
	}

	if err := st.UpdateAccountHealth(accountID, "error", "boom again"); err != nil {
		t.Fatalf("set account error again: %v", err)
	}
	cache.Invalidate()
	checker.sendWebhookNotifications(context.Background())
	waitFor(t, func() bool { return len(received) == 2 })
	if len(received) != 2 {
		t.Fatalf("expected webhook after recovery, got %d", len(received))
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

	if err := st.UpdateSettings(map[string]string{
		"webhook_enabled":               syscfg.BoolString(true),
		"webhook_url":                   server.URL,
		"notification_error_enabled":    syscfg.BoolString(true),
		"notification_expiring_enabled": syscfg.BoolString(false),
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	cache.Invalidate()

	checker := NewChecker(st, cache, "", webhook.NewSender())
	checker.sendWebhookNotifications(context.Background())
	waitFor(t, func() bool { return attempts >= 1 })
	checker.sendWebhookNotifications(context.Background())
	waitFor(t, func() bool { return attempts == 2 })

	if attempts != 2 {
		t.Fatalf("expected retry after first failure, got %d attempts", attempts)
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

	var received []webhook.Payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload webhook.Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		received = append(received, payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

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

	checker := NewChecker(st, cache, "", webhook.NewSender())
	checker.sendWebhookNotifications(context.Background())
	waitFor(t, func() bool { return len(received) == 1 })

	expiredAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := setAccountExpiry(st, accountID, expiredAt); err != nil {
		t.Fatalf("set expired expiry: %v", err)
	}
	cache.Invalidate()
	checker.sendWebhookNotifications(context.Background())
	waitFor(t, func() bool { return len(received) == 2 })

	if len(received) != 2 {
		t.Fatalf("expected warning and critical notifications, got %d", len(received))
	}
	if received[0].Severity != "warning" {
		t.Fatalf("expected first notification to be warning, got %q", received[0].Severity)
	}
	if received[1].Severity != "critical" {
		t.Fatalf("expected second notification to be critical, got %q", received[1].Severity)
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
