package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(filepath.Join(t.TempDir(), "lune-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestListSystemNotificationsHandlesRFC3339Expiry(t *testing.T) {
	st := newTestStore(t)

	if err := st.UpdateSettings(map[string]string{
		"notification_expiring_enabled": "1",
		"notification_error_enabled":    "0",
		"notification_expiring_days":    "7",
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	expiredAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	expiringSoon := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	outsideWindow := time.Now().UTC().Add(10 * 24 * time.Hour).Format(time.RFC3339)

	if _, err := st.db.Exec(
		`INSERT INTO accounts (label, source_kind, cpa_expired_at) VALUES (?, 'cpa', ?), (?, 'cpa', ?), (?, 'cpa', ?)`,
		"expired-account", expiredAt,
		"expiring-account", expiringSoon,
		"future-account", outsideWindow,
	); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	notifications, err := st.ListSystemNotifications()
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifications))
	}

	byTitle := make(map[string]SystemNotification, len(notifications))
	for _, item := range notifications {
		byTitle[item.Title] = item
	}

	if byTitle["CPA account expired"].Severity != "critical" {
		t.Fatalf("expected expired notification to be critical, got %q", byTitle["CPA account expired"].Severity)
	}
	if byTitle["CPA account expiring soon"].Severity != "warning" {
		t.Fatalf("expected expiring notification to be warning, got %q", byTitle["CPA account expiring soon"].Severity)
	}
}

func TestPruneRequestLogsDeletesOnlyExpiredRows(t *testing.T) {
	st := newTestStore(t)

	if _, err := st.db.Exec(
		`INSERT INTO request_logs (request_id, created_at) VALUES (?, ?), (?, ?)`,
		"old", time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02 15:04:05"),
		"new", time.Now().UTC().AddDate(0, 0, -5).Format("2006-01-02 15:04:05"),
	); err != nil {
		t.Fatalf("seed request logs: %v", err)
	}

	deleted, err := st.PruneRequestLogs(30)
	if err != nil {
		t.Fatalf("prune request logs: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted log, got %d", deleted)
	}

	summary, err := st.GetDataRetentionSummary(30)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.TotalLogs != 1 {
		t.Fatalf("expected 1 remaining log, got %d", summary.TotalLogs)
	}
}

func TestGetDataRetentionSummaryIncludesNotificationCounts(t *testing.T) {
	st := newTestStore(t)

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
	if _, err := st.CreateNotificationDelivery(&NotificationDelivery{
		ChannelID:      channelID,
		ChannelName:    "Ops",
		ChannelType:    "generic_webhook",
		Event:          "test",
		Severity:       "info",
		Title:          "t",
		PayloadSummary: "b",
		Status:         "success",
		Attempt:        1,
		TriggeredBy:    "test",
	}); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	if _, err := st.InsertNotificationOutbox(&NotificationOutbox{
		ChannelID: channelID,
		Event:     "account_error",
		Severity:  "critical",
		Payload:   `{"event":"account_error"}`,
		Status:    "pending",
	}); err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	summary, err := st.GetDataRetentionSummary(30)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.TotalNotificationDeliveries != 1 {
		t.Fatalf("expected 1 notification delivery, got %d", summary.TotalNotificationDeliveries)
	}
	if summary.TotalNotificationOutbox != 1 {
		t.Fatalf("expected 1 outbox row, got %d", summary.TotalNotificationOutbox)
	}
}

func TestPruneNotificationHistoryRetentionZeroKeepsOutbox(t *testing.T) {
	st := newTestStore(t)

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
	if _, err := st.InsertNotificationOutbox(&NotificationOutbox{
		ChannelID:     channelID,
		Event:         "account_error",
		Severity:      "critical",
		Payload:       `{"event":"account_error"}`,
		DedupKey:      "dedup",
		Status:        "retrying",
		Attempt:       3,
		NextAttemptAt: time.Now().UTC().Add(-8 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
	}); err != nil {
		t.Fatalf("create outbox: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE notification_outbox SET created_at = ? WHERE channel_id = ?`, time.Now().UTC().Add(-10*24*time.Hour).Format("2006-01-02 15:04:05"), channelID); err != nil {
		t.Fatalf("age outbox: %v", err)
	}

	deletedDeliveries, deletedOutbox, err := st.PruneNotificationHistory(0)
	if err != nil {
		t.Fatalf("prune notification history: %v", err)
	}
	if deletedDeliveries != 0 || deletedOutbox != 0 {
		t.Fatalf("expected no deletions, got deliveries=%d outbox=%d", deletedDeliveries, deletedOutbox)
	}

	items, err := st.ListDueNotificationOutbox(10)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected outbox row to remain, got %d", len(items))
	}
}

func TestDeleteNotificationChannelKeepsDeliveryHistory(t *testing.T) {
	st := newTestStore(t)

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
	if _, err := st.CreateNotificationDelivery(&NotificationDelivery{
		ChannelID:      channelID,
		ChannelName:    "Ops",
		ChannelType:    "generic_webhook",
		Event:          "test",
		Severity:       "info",
		Title:          "hello",
		PayloadSummary: "world",
		Status:         "success",
		Attempt:        1,
		TriggeredBy:    "test",
	}); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	if _, err := st.InsertNotificationOutbox(&NotificationOutbox{
		ChannelID: channelID,
		Event:     "test",
		Severity:  "info",
		Payload:   `{"event":"test"}`,
		Status:    "pending",
	}); err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	if err := st.DeleteNotificationChannel(channelID); err != nil {
		t.Fatalf("delete channel: %v", err)
	}

	deliveries, err := st.ListNotificationDeliveries(NotificationDeliveryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected delivery history to remain, got %d", len(deliveries))
	}
	outbox, err := st.ListDueNotificationOutbox(10)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 0 {
		t.Fatalf("expected outbox to be cleared, got %d rows", len(outbox))
	}
}

func TestValidateNotificationDeliveryCursorRejectsRFC3339(t *testing.T) {
	if err := ValidateNotificationDeliveryCursor("2026-04-16T00:00:00Z"); err == nil {
		t.Fatalf("expected RFC3339 cursor to be rejected")
	}
	if err := ValidateNotificationDeliveryCursor("2026-04-16 00:00:00"); err != nil {
		t.Fatalf("expected sqlite timestamp cursor to pass, got %v", err)
	}
}
