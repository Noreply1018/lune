package store

import (
	"database/sql"
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
	outboxID, err := st.InsertNotificationOutbox(&NotificationOutbox{
		ChannelID: SingletonChannelID,
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
		ChannelID:      SingletonChannelID,
		ChannelName:    SingletonChannelName,
		ChannelType:    SingletonChannelType,
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

func TestNotificationSingletonSettingsUpsert(t *testing.T) {
	st := newNotificationStore(t)
	settings, err := st.GetNotificationSettings()
	if err != nil {
		t.Fatalf("get default settings: %v", err)
	}
	if settings.Enabled {
		t.Fatalf("expected default settings to be disabled, got %+v", settings)
	}

	want := NotificationSettings{
		Enabled:           true,
		WebhookURL:        "https://example.com/hook",
		MentionMobileList: []string{"13800138000", "@all"},
	}
	if err := st.UpdateNotificationSettings(want); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	got, err := st.GetNotificationSettings()
	if err != nil {
		t.Fatalf("re-read settings: %v", err)
	}
	if !got.Enabled || got.WebhookURL != want.WebhookURL {
		t.Fatalf("settings did not persist: %+v", got)
	}
	if len(got.MentionMobileList) != 2 || got.MentionMobileList[0] != "13800138000" || got.MentionMobileList[1] != "@all" {
		t.Fatalf("mention list not persisted: %+v", got.MentionMobileList)
	}
}

func TestNotificationSubscriptionsSeededAndUpdatable(t *testing.T) {
	st := newNotificationStore(t)
	subs, err := st.ListNotificationSubscriptions()
	if err != nil {
		t.Fatalf("list subs: %v", err)
	}
	if len(subs) != 4 {
		t.Fatalf("expected 4 seeded subscriptions, got %d", len(subs))
	}
	seen := map[string]bool{}
	for _, sub := range subs {
		seen[sub.Event] = true
		if sub.BodyTemplate == "" {
			t.Fatalf("expected seeded body for %q, got empty", sub.Event)
		}
	}
	for _, want := range []string{"account_error", "account_expiring", "cpa_service_error", "test"} {
		if !seen[want] {
			t.Fatalf("missing seeded event %q", want)
		}
	}

	if err := st.UpdateNotificationSubscription("account_error", false, "Custom Body"); err != nil {
		t.Fatalf("update sub: %v", err)
	}
	updated, err := st.GetNotificationSubscription("account_error")
	if err != nil {
		t.Fatalf("get sub: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected subscription to exist")
	}
	if updated.Subscribed || updated.BodyTemplate != "Custom Body" {
		t.Fatalf("subscription did not persist: %+v", updated)
	}

	if err := st.UpdateNotificationSubscription("unknown_event", true, "y"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for unknown event, got %v", err)
	}
}
