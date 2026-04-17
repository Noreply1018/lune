package notify

import (
	"context"
	"encoding/json"
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

func TestDispatchSkipsOutboxWhenNoChannelMatches(t *testing.T) {
	st := newDispatcherTestStore(t)
	service := NewServiceWithRegistry(st, NewRegistry(&stubChannelDriver{}))

	if _, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "ops",
		Type:          "stub",
		Enabled:       true,
		Config:        json.RawMessage(`{}`),
		Subscriptions: []store.NotificationSubscription{{Event: "account_expiring"}},
	}); err != nil {
		t.Fatalf("create channel: %v", err)
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
		t.Fatalf("expected no outbox items, got %+v", outbox)
	}
}

func TestDispatchSkipsOutboxWhenNoChannelsExist(t *testing.T) {
	st := newDispatcherTestStore(t)
	service := NewServiceWithRegistry(st, NewRegistry(&stubChannelDriver{}))

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
		t.Fatalf("expected no outbox items, got %+v", outbox)
	}
	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("expected no deliveries, got %+v", deliveries)
	}
}

func TestDispatchCreatesDeliveryWhenChannelMatches(t *testing.T) {
	st := newDispatcherTestStore(t)
	driver := &stubChannelDriver{}
	service := NewServiceWithRegistry(st, NewRegistry(driver))

	if _, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "ops",
		Type:          "stub",
		Enabled:       true,
		Config:        json.RawMessage(`{}`),
		Subscriptions: []store.NotificationSubscription{{Event: "account_error"}},
	}); err != nil {
		t.Fatalf("create channel: %v", err)
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
