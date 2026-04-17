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

func TestNotificationChannelPersistsRetryAndSubscriptionOverrides(t *testing.T) {
	st := newNotificationStore(t)
	channelID, err := st.CreateNotificationChannel(&NotificationChannel{
		Name:            "Ops",
		Type:            "generic_webhook",
		Enabled:         true,
		Config:          json.RawMessage(`{"schema":1,"url":"https://example.com/hook"}`),
		Subscriptions:   []NotificationSubscription{{Event: "account_error", MinSeverity: "warning", TitleTemplate: "custom title", BodyTemplate: "custom body"}},
		RetryMaxAttempts: 3,
		RetrySchedule:    []int{15, 45, 120},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	channel, err := st.GetNotificationChannel(channelID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if channel == nil {
		t.Fatalf("expected channel to exist")
	}
	if channel.RetryMaxAttempts != 3 {
		t.Fatalf("expected retry max attempts 3, got %d", channel.RetryMaxAttempts)
	}
	if len(channel.RetrySchedule) != 3 || channel.RetrySchedule[1] != 45 {
		t.Fatalf("unexpected retry schedule: %+v", channel.RetrySchedule)
	}
	if len(channel.Subscriptions) != 1 || channel.Subscriptions[0].TitleTemplate != "custom title" || channel.Subscriptions[0].BodyTemplate != "custom body" {
		t.Fatalf("unexpected subscriptions: %+v", channel.Subscriptions)
	}

	channel.RetryMaxAttempts = 2
	channel.RetrySchedule = []int{10, 30}
	channel.Subscriptions[0].TitleTemplate = "updated title"
	if err := st.UpdateNotificationChannel(channelID, channel); err != nil {
		t.Fatalf("update channel: %v", err)
	}

	updated, err := st.GetNotificationChannel(channelID)
	if err != nil {
		t.Fatalf("get updated channel: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected updated channel to exist")
	}
	if updated.RetryMaxAttempts != 2 || len(updated.RetrySchedule) != 2 || updated.RetrySchedule[0] != 10 {
		t.Fatalf("unexpected updated retry config: max=%d schedule=%+v", updated.RetryMaxAttempts, updated.RetrySchedule)
	}
	if updated.Subscriptions[0].TitleTemplate != "updated title" {
		t.Fatalf("expected updated subscription title override, got %+v", updated.Subscriptions[0])
	}
}

func TestNotificationChannelDefaultsRetryConfigWhenMissing(t *testing.T) {
	st := newNotificationStore(t)

	if _, err := st.DB().Exec(
		`INSERT INTO notification_channels (name, type, enabled, config, subscriptions, title_template, body_template, retry_max_attempts, retry_schedule_seconds)
		 VALUES ('legacy', 'generic_webhook', 1, '{}', '[{"event":"account_error"}]', '', '', 0, '')`,
	); err != nil {
		t.Fatalf("seed legacy-like channel: %v", err)
	}

	channels, err := st.ListNotificationChannels()
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected one channel, got %d", len(channels))
	}
	if channels[0].RetryMaxAttempts != 5 {
		t.Fatalf("expected default retry max attempts 5, got %d", channels[0].RetryMaxAttempts)
	}
	if len(channels[0].RetrySchedule) != 5 {
		t.Fatalf("expected default retry schedule, got %+v", channels[0].RetrySchedule)
	}
	if channels[0].Subscriptions[0].TitleTemplate != "" || channels[0].Subscriptions[0].BodyTemplate != "" {
		t.Fatalf("expected missing subscription template fields to decode as zero values, got %+v", channels[0].Subscriptions[0])
	}
}

func TestNotificationChannelHandlesNullSubscriptionsJSON(t *testing.T) {
	st := newNotificationStore(t)

	if _, err := st.DB().Exec(
		`INSERT INTO notification_channels (name, type, enabled, config, subscriptions, title_template, body_template, retry_max_attempts, retry_schedule_seconds)
		 VALUES ('null-subs', 'generic_webhook', 1, '{}', 'null', '', '', 5, '[30,120,600,1800,7200]')`,
	); err != nil {
		t.Fatalf("seed null subscriptions channel: %v", err)
	}

	channel, err := st.GetNotificationChannel(1)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if channel == nil {
		t.Fatalf("expected channel to exist")
	}
	if channel.Subscriptions == nil {
		t.Fatalf("expected subscriptions to decode to empty slice, got nil")
	}
	if len(channel.Subscriptions) != 0 {
		t.Fatalf("expected empty subscriptions for null JSON, got %+v", channel.Subscriptions)
	}
}
