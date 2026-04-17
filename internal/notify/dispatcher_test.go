package notify

import (
	"context"
	"path/filepath"
	"testing"

	"lune/internal/store"
)

func newDispatcherTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "dispatcher-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestDispatchSkipsOutboxWhenSettingsDisabled(t *testing.T) {
	st := newDispatcherTestStore(t)
	service := NewServiceWithRegistry(st, NewRegistry(&stubChannelDriver{}))

	// Settings are disabled by default after migration — no channel enable.
	if err := service.Dispatch(context.Background(), Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	outbox, err := st.ListDueNotificationOutbox(10)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 0 {
		t.Fatalf("expected no outbox items when disabled, got %+v", outbox)
	}
}

func TestDispatchSkipsOutboxWhenEventUnsubscribed(t *testing.T) {
	st := newDispatcherTestStore(t)
	service := NewServiceWithRegistry(st, NewRegistry(&stubChannelDriver{}))

	if err := st.UpdateNotificationSettings(store.NotificationSettings{
		Enabled:           true,
		WebhookURL:        "https://example.com/hook",
		Format:            "markdown",
		MentionMobileList: []string{},
	}); err != nil {
		t.Fatalf("enable settings: %v", err)
	}
	if err := st.UpdateNotificationSubscription("account_error", false, "t", "b"); err != nil {
		t.Fatalf("disable sub: %v", err)
	}

	if err := service.Dispatch(context.Background(), Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	outbox, err := st.ListDueNotificationOutbox(10)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 0 {
		t.Fatalf("expected no outbox items when unsubscribed, got %+v", outbox)
	}
}

func TestDispatchCreatesDeliveryWhenSubscribed(t *testing.T) {
	st := newDispatcherTestStore(t)
	driver := &stubChannelDriver{}
	service := NewServiceWithRegistry(st, NewRegistry(driver))

	if err := st.UpdateNotificationSettings(store.NotificationSettings{
		Enabled:           true,
		WebhookURL:        "https://example.com/hook",
		Format:            "markdown",
		MentionMobileList: []string{},
	}); err != nil {
		t.Fatalf("enable settings: %v", err)
	}

	if err := service.Dispatch(context.Background(), Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if driver.sendCount != 1 {
		t.Fatalf("expected immediate send, got %d", driver.sendCount)
	}
	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Event != "account_error" {
		t.Fatalf("expected one account_error delivery, got %+v", deliveries)
	}
}
