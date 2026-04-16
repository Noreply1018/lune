package store

import (
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
