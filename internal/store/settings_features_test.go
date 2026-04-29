package store

import (
	"fmt"
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
		"notification_expiring_days": "7",
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	expiredAt := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	expiringSoon := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	outsideWindow := time.Now().UTC().Add(10 * 24 * time.Hour).Format(time.RFC3339)
	codexExpiringSoon := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	if _, err := st.db.Exec(
		`INSERT INTO accounts (label, source_kind, cpa_provider, cpa_expired_at) VALUES (?, 'cpa', '', ?), (?, 'cpa', '', ?), (?, 'cpa', '', ?), (?, 'cpa', 'codex', ?)`,
		"expired-account", expiredAt,
		"expiring-account", expiringSoon,
		"future-account", outsideWindow,
		"codex-credential", codexExpiringSoon,
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

func TestOverviewExpiryAlertsIgnoreCodexCredentials(t *testing.T) {
	st := newTestStore(t)

	expiringSoon := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	if _, err := st.db.Exec(
		`INSERT INTO accounts (label, source_kind, cpa_provider, cpa_expired_at, enabled) VALUES (?, 'cpa', '', ?, 1), (?, 'cpa', 'Codex', ?, 1)`,
		"claude-credential", expiringSoon,
		"codex-credential", expiringSoon,
	); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if len(overview.Alerts) != 1 {
		t.Fatalf("expected one non-codex expiry alert, got %+v", overview.Alerts)
	}
	if overview.Alerts[0].Message != fmt.Sprintf("Account %q expires at %s", "claude-credential", expiringSoon) {
		t.Fatalf("unexpected alert: %+v", overview.Alerts[0])
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

	if _, err := st.CreateNotificationDelivery(&NotificationDelivery{
		ChannelID:      SingletonChannelID,
		ChannelName:    SingletonChannelName,
		ChannelType:    SingletonChannelType,
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
		ChannelID: SingletonChannelID,
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

func TestGetDataRetentionPreviewReturnsCountsAndBytes(t *testing.T) {
	st := newTestStore(t)

	// Two logs: one inside the retention window, one outside.
	expired := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02 15:04:05")
	fresh := time.Now().UTC().AddDate(0, 0, -5).Format("2006-01-02 15:04:05")
	if _, err := st.db.Exec(
		`INSERT INTO request_logs (request_id, access_token_name, model_requested, model_actual, request_ip, error_message, source_kind, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?)`,
		"req-old", "token-old", "gpt-4", "gpt-4-actual", "127.0.0.1", "", "openai_compat", expired,
		"req-new", "token-new", "gpt-4", "gpt-4-actual", "127.0.0.1", "", "openai_compat", fresh,
	); err != nil {
		t.Fatalf("seed request logs: %v", err)
	}
	if _, err := st.db.Exec(
		`INSERT INTO notification_deliveries (channel_id, channel_name, channel_type, event, severity, title, payload_summary, status, attempt, triggered_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		SingletonChannelID, SingletonChannelName, SingletonChannelType,
		"test", "info", "t", "b", "success", 1, "test", expired,
	); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	if _, err := st.db.Exec(
		`INSERT INTO notification_outbox (channel_id, event, severity, payload, status, created_at)
		 VALUES (?, ?, ?, ?, 'dropped', ?)`,
		SingletonChannelID, "account_error", "critical", `{"event":"account_error"}`,
		time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02 15:04:05"),
	); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	preview, err := st.GetDataRetentionPreview(30)
	if err != nil {
		t.Fatalf("get preview: %v", err)
	}
	if preview.LogsToDelete != 1 {
		t.Fatalf("expected 1 log to delete, got %d", preview.LogsToDelete)
	}
	if preview.LogsToDeleteSizeBytes <= 0 {
		t.Fatalf("expected positive estimated size, got %d", preview.LogsToDeleteSizeBytes)
	}
	if preview.DeliveriesToDelete != 1 {
		t.Fatalf("expected 1 delivery to delete, got %d", preview.DeliveriesToDelete)
	}
	if preview.OutboxToDelete != 1 {
		t.Fatalf("expected 1 outbox to delete, got %d", preview.OutboxToDelete)
	}
	if preview.OutboxSafetyDays < 7 {
		t.Fatalf("expected outbox safety window >= 7 days, got %d", preview.OutboxSafetyDays)
	}

	// Preview must be non-mutating.
	summary, err := st.GetDataRetentionSummary(30)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.TotalLogs != 2 {
		t.Fatalf("preview mutated logs: expected 2 remaining, got %d", summary.TotalLogs)
	}

	// Disabled auto-prune short-circuits to zeros without running any SQL.
	disabled, err := st.GetDataRetentionPreview(0)
	if err != nil {
		t.Fatalf("get preview (disabled): %v", err)
	}
	if disabled.LogsToDelete != 0 || disabled.LogsToDeleteSizeBytes != 0 {
		t.Fatalf("expected zeros when disabled, got %+v", disabled)
	}
}

func TestPruneNotificationHistoryRetentionZeroKeepsOutbox(t *testing.T) {
	st := newTestStore(t)

	if _, err := st.InsertNotificationOutbox(&NotificationOutbox{
		ChannelID:     SingletonChannelID,
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
	if _, err := st.db.Exec(`UPDATE notification_outbox SET created_at = ?`, time.Now().UTC().Add(-10*24*time.Hour).Format("2006-01-02 15:04:05")); err != nil {
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

func TestValidateNotificationDeliveryCursorRejectsRFC3339(t *testing.T) {
	if err := ValidateNotificationDeliveryCursor("2026-04-16T00:00:00Z"); err == nil {
		t.Fatalf("expected RFC3339 cursor to be rejected")
	}
	if err := ValidateNotificationDeliveryCursor("2026-04-16 00:00:00"); err != nil {
		t.Fatalf("expected sqlite timestamp cursor to pass, got %v", err)
	}
}
