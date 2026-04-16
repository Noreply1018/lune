package notify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"lune/internal/store"
)

type stubChannelDriver struct {
	sendCount int
	lastBody  string
}

func (d *stubChannelDriver) Type() string { return "stub" }
func (d *stubChannelDriver) ValidateConfig(raw json.RawMessage) error { return nil }
func (d *stubChannelDriver) SecretFields() []string { return nil }
func (d *stubChannelDriver) DocsURL() string { return "" }
func (d *stubChannelDriver) Send(ctx context.Context, n Notification, cfg ChannelRuntime) (Result, error) {
	d.sendCount++
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

func TestAttemptOneDoesNotRetryAfterSuccessfulSendWhenRenderFails(t *testing.T) {
	st := newNotifyTestStore(t)
	driver := &stubChannelDriver{}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	channelID, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "stub",
		Type:          "stub",
		Enabled:       true,
		Config:        json.RawMessage(`{}`),
		Subscriptions: []store.NotificationSubscription{{Event: "*"}},
		TitleTemplate: "{{ .Missing",
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	payload, err := json.Marshal(Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	itemID, err := st.InsertNotificationOutbox(&store.NotificationOutbox{
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   string(payload),
		DedupKey:  "dedup",
		Status:    "pending",
	})
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	item := store.NotificationOutbox{
		ID:        itemID,
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   string(payload),
		DedupKey:  "dedup",
		Status:    "pending",
	}
	channel, err := st.GetNotificationChannel(channelID)
	if err != nil || channel == nil {
		t.Fatalf("get channel: %v", err)
	}

	if err := outbox.AttemptOne(context.Background(), item, *channel, false); err != nil {
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
}

func TestAttemptOnePassesFullRenderedBodyToDriver(t *testing.T) {
	st := newNotifyTestStore(t)
	driver := &stubChannelDriver{}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	channelID, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "stub",
		Type:          "stub",
		Enabled:       true,
		Config:        json.RawMessage(`{}`),
		Subscriptions: []store.NotificationSubscription{{Event: "*"}},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	longMessage := ""
	for i := 0; i < 1100; i++ {
		longMessage += "x"
	}
	payload, err := json.Marshal(Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  longMessage,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	itemID, err := st.InsertNotificationOutbox(&store.NotificationOutbox{
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   string(payload),
		DedupKey:  "dedup-body",
		Status:    "pending",
	})
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	item := store.NotificationOutbox{
		ID:        itemID,
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   string(payload),
		DedupKey:  "dedup-body",
		Status:    "pending",
	}
	channel, err := st.GetNotificationChannel(channelID)
	if err != nil || channel == nil {
		t.Fatalf("get channel: %v", err)
	}

	if err := outbox.AttemptOne(context.Background(), item, *channel, false); err != nil {
		t.Fatalf("attempt one: %v", err)
	}
	if len(driver.lastBody) <= 1024 {
		t.Fatalf("expected driver to receive full rendered body, got len=%d", len(driver.lastBody))
	}
}

func TestAttemptOneSkipsSendWhenOutboxRowAlreadyGone(t *testing.T) {
	st := newNotifyTestStore(t)
	driver := &stubChannelDriver{}
	registry := NewRegistry(driver)
	outbox := NewOutbox(st, registry)

	channelID, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "stub",
		Type:          "stub",
		Enabled:       true,
		Config:        json.RawMessage(`{}`),
		Subscriptions: []store.NotificationSubscription{{Event: "*"}},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	payload, err := json.Marshal(Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	itemID, err := st.InsertNotificationOutbox(&store.NotificationOutbox{
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   string(payload),
		DedupKey:  "gone",
		Status:    "pending",
	})
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	item := store.NotificationOutbox{
		ID:        itemID,
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   string(payload),
		DedupKey:  "gone",
		Status:    "pending",
	}
	channel, err := st.GetNotificationChannel(channelID)
	if err != nil || channel == nil {
		t.Fatalf("get channel: %v", err)
	}
	if err := st.DeleteNotificationOutbox(itemID); err != nil {
		t.Fatalf("delete outbox: %v", err)
	}

	if err := outbox.AttemptOne(context.Background(), item, *channel, false); err != nil {
		t.Fatalf("attempt one: %v", err)
	}
	if driver.sendCount != 0 {
		t.Fatalf("expected no send after outbox row was removed, got %d sends", driver.sendCount)
	}
}
