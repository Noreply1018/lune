package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"lune/internal/store"
)

type Dispatcher struct {
	store    *store.Store
	registry *Registry
	outbox   *Outbox
}

func NewDispatcher(st *store.Store, registry *Registry, outbox *Outbox) *Dispatcher {
	return &Dispatcher{store: st, registry: registry, outbox: outbox}
}

func (d *Dispatcher) Dispatch(ctx context.Context, n Notification) error {
	channels, err := d.store.ListEnabledNotificationChannels()
	if err != nil {
		return err
	}
	settings, err := d.store.GetSettings()
	if err != nil {
		return err
	}
	backoff := DedupBackoff(settings)
	dedupKey := BuildDedupKey(n)
	payload, err := json.Marshal(n)
	if err != nil {
		return err
	}

	for _, ch := range channels {
		if !matchSubscriptions(ch.Subscriptions, n.Event, n.Severity) {
			continue
		}
		exists, err := d.store.HasRecentNotificationDelivery(ch.ID, dedupKey, time.Now().UTC().Add(-backoff))
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		pending, err := d.store.HasPendingNotificationOutbox(ch.ID, dedupKey)
		if err != nil {
			return err
		}
		if pending {
			continue
		}

		item := store.NotificationOutbox{
			ChannelID:     ch.ID,
			Event:         n.Event,
			Severity:      n.Severity,
			Payload:       string(payload),
			DedupKey:      dedupKey,
			Status:        "pending",
			Attempt:       0,
			NextAttemptAt: time.Now().UTC().Format("2006-01-02 15:04:05"),
		}
		id, err := d.store.InsertNotificationOutbox(&item)
		if err != nil {
			if strings.Contains(err.Error(), "idx_notification_outbox_active_unique") || strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return err
		}
		item.ID = id
		if err := d.outbox.AttemptOne(ctx, item, ch, false); err != nil {
			slog.Error("notification outbox immediate attempt failed", "channel_id", ch.ID, "channel_name", ch.Name, "event", n.Event, "err", err)
			continue
		}
	}
	return nil
}

func matchSubscriptions(subscriptions []store.NotificationSubscription, event, severity string) bool {
	if len(subscriptions) == 0 {
		return false
	}
	for _, sub := range subscriptions {
		if sub.Event != "*" && sub.Event != event {
			continue
		}
		if severityRank(severity) < severityRank(sub.MinSeverity) {
			continue
		}
		return true
	}
	return false
}

func severityRank(value string) int {
	switch value {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func decodeNotificationPayload(raw string) (Notification, error) {
	var n Notification
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return Notification{}, fmt.Errorf("decode outbox payload: %w", err)
	}
	return n, nil
}
