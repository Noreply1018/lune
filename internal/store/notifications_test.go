package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func newNotificationStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(filepath.Join(t.TempDir(), "notifications-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestRecordNotificationAttemptSuccessRejectsMissingOutboxRow(t *testing.T) {
	st := newNotificationStore(t)
	channelID, err := st.CreateNotificationChannel(&NotificationChannel{
		Name:          "Ops",
		Type:          "generic_webhook",
		Enabled:       true,
		Config:        json.RawMessage(`{"schema":1,"url":"https://example.com/hook"}`),
		Subscriptions: []NotificationSubscription{{Event: "*"}},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	outboxID, err := st.InsertNotificationOutbox(&NotificationOutbox{
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   `{"event":"account_error"}`,
		DedupKey:  "dedup",
		Status:    "pending",
	})
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	delivery := &NotificationDelivery{
		ChannelID:      channelID,
		ChannelName:    "Ops",
		ChannelType:    "generic_webhook",
		Event:          "account_error",
		Severity:       "critical",
		Title:          "title",
		PayloadSummary: "body",
		Status:         "success",
		Attempt:        1,
		DedupKey:       "dedup",
		TriggeredBy:    "system",
	}

	if err := st.RecordNotificationAttemptSuccess(outboxID, delivery); err != nil {
		t.Fatalf("first success record failed: %v", err)
	}
	if err := st.RecordNotificationAttemptSuccess(outboxID, delivery); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected second success record to fail with sql.ErrNoRows, got %v", err)
	}

	deliveries, err := st.ListNotificationDeliveries(NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected exactly one delivery after duplicate success attempt, got %d", len(deliveries))
	}
}
