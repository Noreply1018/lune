package notify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"lune/internal/store"
)

type stubChannelDriver struct {
	sendCount int
	lastBody  string
	failCount int
}

func (d *stubChannelDriver) Type() string                             { return store.SingletonChannelType }
func (d *stubChannelDriver) ValidateConfig(raw json.RawMessage) error { return nil }
func (d *stubChannelDriver) SecretFields() []string                   { return nil }
func (d *stubChannelDriver) DocsURL() string                          { return "" }
func (d *stubChannelDriver) Send(ctx context.Context, n Notification, cfg ChannelRuntime) (Result, error) {
	d.sendCount++
	if d.failCount > 0 {
		d.failCount--
		return Result{OK: false, UpstreamCode: "http 500", UpstreamMessage: "boom"}, context.DeadlineExceeded
	}
	if cfg.Rendered != nil {
		d.lastBody = cfg.Rendered.Body
	}
	return Result{OK: true, UpstreamCode: "ok", UpstreamMessage: "ok"}, nil
}

func newNotifyTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "notify-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func enableTestSettings(t *testing.T, st *store.Store) store.NotificationSettings {
	t.Helper()
	settings := store.NotificationSettings{
		Enabled:           true,
		WebhookURL:        "https://example.com/hook",
		Format:            "markdown",
		MentionMobileList: []string{},
	}
	if err := st.UpdateNotificationSettings(settings); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	return settings
}

func getSub(t *testing.T, st *store.Store, event string) store.NotificationSubscription {
	t.Helper()
	sub, err := st.GetNotificationSubscription(event)
	if err != nil {
		t.Fatalf("get sub %q: %v", event, err)
	}
	if sub == nil {
		t.Fatalf("expected subscription for %q", event)
	}
	return *sub
}

func insertOutboxItem(t *testing.T, st *store.Store, n Notification, dedup, status string, attempt int) store.NotificationOutbox {
	t.Helper()
	payload, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	id, err := st.InsertNotificationOutbox(&store.NotificationOutbox{
		ChannelID: store.SingletonChannelID,
		Event:     n.Event,
		Severity:  n.Severity,
		Payload:   string(payload),
		DedupKey:  dedup,
		Status:    status,
		Attempt:   attempt,
	})
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	return store.NotificationOutbox{
		ID:        id,
		ChannelID: store.SingletonChannelID,
		Event:     n.Event,
		Severity:  n.Severity,
		Payload:   string(payload),
		DedupKey:  dedup,
		Status:    status,
		Attempt:   attempt,
	}
}

func TestAttemptOneDoesNotRetryAfterSuccessfulSendWhenRenderFails(t *testing.T) {
	st := newNotifyTestStore(t)
	settings := enableTestSettings(t, st)
	if err := st.UpdateNotificationSubscription("account_error", true, "{{ .Missing", "{{ .Message }}"); err != nil {
		t.Fatalf("update sub: %v", err)
	}
	sub := getSub(t, st, "account_error")

	driver := &stubChannelDriver{}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	item := insertOutboxItem(t, st, Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}, "dedup", "pending", 0)

	if err := outbox.AttemptOne(context.Background(), item, settings, sub, false); err != nil {
		t.Fatalf("attempt one: %v", err)
	}
	if driver.sendCount != 1 {
		t.Fatalf("expected one send, got %d", driver.sendCount)
	}

	outboxItems, err := st.ListDueNotificationOutbox(10)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outboxItems) != 0 {
		t.Fatalf("expected outbox row to be deleted after success, got %d rows", len(outboxItems))
	}

	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != "success" {
		t.Fatalf("expected one successful delivery, got %+v", deliveries)
	}
	outbox.locksMu.Lock()
	_, stillLocked := outbox.locks[item.ID]
	outbox.locksMu.Unlock()
	if stillLocked {
		t.Fatalf("expected item lock to be released after successful terminal state")
	}
}

func TestAttemptOnePassesFullRenderedBodyToDriver(t *testing.T) {
	st := newNotifyTestStore(t)
	settings := enableTestSettings(t, st)
	sub := getSub(t, st, "account_error")

	driver := &stubChannelDriver{}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	longMessage := ""
	for i := 0; i < 1100; i++ {
		longMessage += "x"
	}
	if err := st.UpdateNotificationSubscription("account_error", true, "Broken", longMessage); err != nil {
		t.Fatalf("update sub: %v", err)
	}
	sub = getSub(t, st, "account_error")

	item := insertOutboxItem(t, st, Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  longMessage,
	}, "dedup-body", "pending", 0)

	if err := outbox.AttemptOne(context.Background(), item, settings, sub, false); err != nil {
		t.Fatalf("attempt one: %v", err)
	}
	if len(driver.lastBody) <= 1024 {
		t.Fatalf("expected driver to receive full rendered body, got len=%d", len(driver.lastBody))
	}
}

func TestAttemptOneSkipsSendWhenOutboxRowAlreadyGone(t *testing.T) {
	st := newNotifyTestStore(t)
	settings := enableTestSettings(t, st)
	sub := getSub(t, st, "account_error")

	driver := &stubChannelDriver{}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	item := insertOutboxItem(t, st, Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}, "gone", "pending", 0)

	if err := st.DeleteNotificationOutbox(item.ID); err != nil {
		t.Fatalf("delete outbox: %v", err)
	}

	if err := outbox.AttemptOne(context.Background(), item, settings, sub, false); err != nil {
		t.Fatalf("attempt one: %v", err)
	}
	if driver.sendCount != 0 {
		t.Fatalf("expected no send after outbox row was removed, got %d sends", driver.sendCount)
	}
}

func TestAttemptOneUsesFixedRetrySchedule(t *testing.T) {
	st := newNotifyTestStore(t)
	settings := enableTestSettings(t, st)
	sub := getSub(t, st, "account_error")

	driver := &stubChannelDriver{failCount: 1}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	item := insertOutboxItem(t, st, Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}, "retry-schedule", "pending", 0)

	if err := outbox.AttemptOne(context.Background(), item, settings, sub, false); err != nil {
		t.Fatalf("attempt one: %v", err)
	}

	outboxItem, err := st.GetNotificationOutbox(item.ID)
	if err != nil {
		t.Fatalf("get outbox: %v", err)
	}
	if outboxItem == nil {
		t.Fatalf("expected outbox row to remain for retry")
	}
	nextAttemptAt, err := time.Parse("2006-01-02 15:04:05", outboxItem.NextAttemptAt)
	if err != nil {
		t.Fatalf("parse next attempt: %v", err)
	}
	diff := nextAttemptAt.Sub(time.Now().UTC())
	// fixedRetrySchedule[0] is 30s.
	if diff < 20*time.Second || diff > 40*time.Second {
		t.Fatalf("expected retry schedule near 30s, got %s", diff)
	}
}

func TestAttemptOneDropsAfterFixedMaxAttempts(t *testing.T) {
	st := newNotifyTestStore(t)
	settings := enableTestSettings(t, st)
	sub := getSub(t, st, "account_error")

	driver := &stubChannelDriver{failCount: 1}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	// Seed an item already on attempt 2 (0-indexed for this purpose): next fail => attempt=3 => dropped.
	item := insertOutboxItem(t, st, Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}, "drop-once", "retrying", 2)

	if err := outbox.AttemptOne(context.Background(), item, settings, sub, true); err != nil {
		t.Fatalf("attempt one: %v", err)
	}

	outboxItem, err := st.GetNotificationOutbox(item.ID)
	if err != nil {
		t.Fatalf("get outbox: %v", err)
	}
	if outboxItem == nil || outboxItem.Status != "dropped" {
		t.Fatalf("expected outbox row to be dropped, got %+v", outboxItem)
	}
	deliveries, err := st.ListNotificationDeliveries(store.NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) == 0 || deliveries[0].Status != "dropped" {
		t.Fatalf("expected dropped delivery, got %+v", deliveries)
	}
	outbox.locksMu.Lock()
	_, stillLocked := outbox.locks[item.ID]
	outbox.locksMu.Unlock()
	if stillLocked {
		t.Fatalf("expected item lock to be released after dropped terminal state")
	}
}
