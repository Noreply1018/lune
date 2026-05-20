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
		`INSERT INTO accounts (label, source_kind, cpa_provider, cpa_expired_at, cpa_subscription_expires_at) VALUES (?, 'cpa', '', ?, ''), (?, 'cpa', '', ?, ''), (?, 'cpa', '', ?, ''), (?, 'cpa', 'codex', ?, ''), (?, 'cpa', 'codex', '', ?)`,
		"expired-account", expiredAt,
		"expiring-account", expiringSoon,
		"future-account", outsideWindow,
		"codex-credential", codexExpiringSoon,
		"codex-subscription", codexExpiringSoon,
	); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	notifications, err := st.ListSystemNotifications()
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifications) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(notifications))
	}

	byTitle := make(map[string]SystemNotification, len(notifications))
	for _, item := range notifications {
		byTitle[item.Title] = item
		if item.Label == "codex-credential" {
			t.Fatalf("codex credential expiry must not generate a system notification: %+v", notifications)
		}
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
		`INSERT INTO accounts (label, source_kind, cpa_provider, cpa_expired_at, cpa_subscription_expires_at, enabled) VALUES (?, 'cpa', '', ?, '', 1), (?, 'cpa', 'Codex', ?, '', 1), (?, 'cpa', 'Codex', '', ?, 1)`,
		"claude-credential", expiringSoon,
		"codex-credential", expiringSoon,
		"codex-subscription", expiringSoon,
	); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if len(overview.Alerts) != 2 {
		t.Fatalf("expected non-codex and codex subscription expiry alerts, got %+v", overview.Alerts)
	}
	messages := map[string]bool{}
	for _, alert := range overview.Alerts {
		messages[alert.Message] = true
		if alert.Message == fmt.Sprintf("Account %q expires at %s", "codex-credential", expiringSoon) {
			t.Fatalf("codex credential expiry must not generate an overview alert: %+v", overview.Alerts)
		}
	}
	if !messages[fmt.Sprintf("Account %q expires at %s", "claude-credential", expiringSoon)] {
		t.Fatalf("missing non-codex alert: %+v", overview.Alerts)
	}
	if !messages[fmt.Sprintf("Account %q expires at %s", "codex-subscription", expiringSoon)] {
		t.Fatalf("missing codex subscription alert: %+v", overview.Alerts)
	}
}

func TestCpaCredentialNeedsLoginCreatesDedicatedAlerts(t *testing.T) {
	st := newTestStore(t)

	if _, err := st.db.Exec(
		`INSERT INTO accounts (label, source_kind, cpa_provider, cpa_credential_status, cpa_credential_reason, cpa_credential_last_error, enabled) VALUES (?, 'cpa', 'codex', 'needs_login', 'refresh_failed', 'refresh token invalid', 1)`,
		"codex-login",
	); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	foundOverview := false
	for _, alert := range overview.Alerts {
		if alert.Type == "cpa_credential_error" && alert.Message == `Account "codex-login" CPA credential requires login: refresh token invalid` {
			foundOverview = true
		}
	}
	if !foundOverview {
		t.Fatalf("missing CPA credential overview alert: %+v", overview.Alerts)
	}

	notifications, err := st.ListSystemNotifications()
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	foundNotification := false
	for _, item := range notifications {
		if item.Type == "cpa_credential_error" && item.Label == "codex-login" && item.LastError == "refresh token invalid" {
			foundNotification = true
		}
	}
	if !foundNotification {
		t.Fatalf("missing CPA credential notification: %+v", notifications)
	}
}

func TestCpaQuotaBlockedCreatesDedicatedAlerts(t *testing.T) {
	st := newTestStore(t)

	if _, err := st.db.Exec(
		`INSERT INTO accounts (label, source_kind, cpa_provider, cpa_quota_status, cpa_quota_last_error, enabled) VALUES (?, 'cpa', 'codex', 'blocked', 'quota blocked by upstream', 1)`,
		"codex-quota",
	); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	foundOverview := false
	for _, alert := range overview.Alerts {
		if alert.Type == "cpa_quota_blocked" && alert.Message == `Account "codex-quota" CPA quota is blocked: quota blocked by upstream` {
			foundOverview = true
		}
	}
	if !foundOverview {
		t.Fatalf("missing CPA quota overview alert: %+v", overview.Alerts)
	}

	notifications, err := st.ListSystemNotifications()
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	foundNotification := false
	for _, item := range notifications {
		if item.Type == "cpa_quota_blocked" && item.Label == "codex-quota" && item.LastError == "quota blocked by upstream" {
			foundNotification = true
		}
	}
	if !foundNotification {
		t.Fatalf("missing CPA quota notification: %+v", notifications)
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
	if preview.OperationsToDelete != 0 || preview.OperationItemsToDelete != 0 {
		t.Fatalf("expected recent operations to be retained, got %+v", preview)
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

func TestPruneDataRetentionDeletesOldOperationsAndAuditsRun(t *testing.T) {
	st := newTestStore(t)

	oldAt := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02 15:04:05")
	newAt := time.Now().UTC().AddDate(0, 0, -5).Format("2006-01-02 15:04:05")
	for i := 0; i < 201; i++ {
		id := fmt.Sprintf("op-prune-%03d", i)
		at := newAt
		if i == 0 {
			at = oldAt
		}
		if err := st.RecordOperation(&Operation{
			OperationID:   id,
			OperationType: "cpa_import_batch",
			Status:        "succeeded",
			StartedAt:     at,
			FinishedAt:    at,
			Items: []OperationItem{
				{Status: "succeeded", Stage: "write_file"},
				{Status: "succeeded", Stage: "insert_pool_member"},
			},
		}); err != nil {
			t.Fatalf("seed operation %s: %v", id, err)
		}
		if _, err := st.db.Exec(`UPDATE operations SET created_at = ? WHERE operation_id = ?`, at, id); err != nil {
			t.Fatalf("age operation %s: %v", id, err)
		}
	}

	preview, err := st.GetDataRetentionPreview(30)
	if err != nil {
		t.Fatalf("get preview: %v", err)
	}
	if preview.OperationsToDelete != 1 || preview.OperationItemsToDelete != 2 {
		t.Fatalf("preview should match operation prune result, got %+v", preview)
	}

	result, err := st.PruneDataRetention(30, "unit_test")
	if err != nil {
		t.Fatalf("prune data retention: %v", err)
	}
	if result.DeletedOperations != 1 || result.DeletedOperationItems != 2 {
		t.Fatalf("expected one old operation outside recent 200 to be deleted, got %+v", result)
	}
	if op, err := st.GetOperation("op-prune-000"); err != nil || op != nil {
		t.Fatalf("old operation should be pruned, op=%+v err=%v", op, err)
	}
	if op, err := st.GetOperation("op-prune-200"); err != nil || op == nil || len(op.Items) != 2 {
		t.Fatalf("fresh operation should remain with items, op=%+v err=%v", op, err)
	}

	ops, err := st.ListRecentOperations(10)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	foundPrune := false
	for _, op := range ops {
		if op.OperationType == "data_retention_prune" {
			foundPrune = true
			if op.Source != "unit_test" || op.Status != "succeeded" {
				t.Fatalf("unexpected prune operation: %+v", op)
			}
			detail, err := st.GetOperation(op.OperationID)
			if err != nil {
				t.Fatalf("get prune operation: %v", err)
			}
			if detail == nil || len(detail.Items) != 5 {
				t.Fatalf("expected prune operation items, got %+v", detail)
			}
		}
	}
	if !foundPrune {
		t.Fatalf("missing data_retention_prune operation: %+v", ops)
	}

	summary, err := st.GetDataRetentionSummary(30)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.LastPruneAt == nil || summary.LastPruneDeletedOperations != 1 || summary.LastPruneDeletedOperationItems != 2 {
		t.Fatalf("summary missing prune result: %+v", summary)
	}
}

func TestPruneDataRetentionRetainsRecentTwoHundredExpiredOperations(t *testing.T) {
	st := newTestStore(t)

	oldAt := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02 15:04:05")
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("op-retained-%03d", i)
		if err := st.RecordOperation(&Operation{
			OperationID:   id,
			OperationType: "cpa_import_batch",
			Status:        "succeeded",
			StartedAt:     oldAt,
			FinishedAt:    oldAt,
			Items:         []OperationItem{{Status: "succeeded", Stage: "write_file"}},
		}); err != nil {
			t.Fatalf("seed operation %s: %v", id, err)
		}
		if _, err := st.db.Exec(`UPDATE operations SET created_at = ? WHERE operation_id = ?`, oldAt, id); err != nil {
			t.Fatalf("age operation %s: %v", id, err)
		}
	}

	preview, err := st.GetDataRetentionPreview(30)
	if err != nil {
		t.Fatalf("get preview: %v", err)
	}
	if preview.OperationsToDelete != 0 || preview.OperationItemsToDelete != 0 {
		t.Fatalf("recent 200 expired operations must be retained, got %+v", preview)
	}
	result, err := st.PruneDataRetention(30, "unit_test")
	if err != nil {
		t.Fatalf("prune data retention: %v", err)
	}
	if result.DeletedOperations != 0 || result.DeletedOperationItems != 0 {
		t.Fatalf("recent 200 expired operations must not be pruned, got %+v", result)
	}
}

func TestPruneDataRetentionHealthNoopDoesNotWriteOperation(t *testing.T) {
	st := newTestStore(t)

	result, err := st.PruneDataRetention(30, "health_checker")
	if err != nil {
		t.Fatalf("prune data retention: %v", err)
	}
	if result.DeletedLogs != 0 || result.DeletedOperations != 0 || result.DeletedOperationItems != 0 {
		t.Fatalf("expected no deletions, got %+v", result)
	}
	ops, err := st.ListRecentOperations(10)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	for _, op := range ops {
		if op.OperationType == "data_retention_prune" {
			t.Fatalf("health no-op prune must not write operation, got %+v", op)
		}
	}
	summary, err := st.GetDataRetentionSummary(30)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.LastPruneAt == nil {
		t.Fatalf("health no-op prune should still record last_prune_at")
	}
}

func TestPruneDataRetentionDisabledDoesNotWriteOperationOrSummary(t *testing.T) {
	st := newTestStore(t)

	result, err := st.PruneDataRetention(0, "admin_api")
	if err != nil {
		t.Fatalf("prune data retention: %v", err)
	}
	if result.DeletedLogs != 0 || result.DeletedOperations != 0 || result.DeletedOperationItems != 0 {
		t.Fatalf("expected no deletions when disabled, got %+v", result)
	}
	ops, err := st.ListRecentOperations(10)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("disabled prune should not write operations, got %+v", ops)
	}
	summary, err := st.GetDataRetentionSummary(0)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.LastPruneAt != nil {
		t.Fatalf("disabled prune should not record last_prune_at, got %+v", summary)
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
