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

// Dispatch enqueues and (if due) immediately attempts a notification using the
// singleton settings. It silently skips when the top-level switch is off, the
// event isn't subscribed, or a recent equivalent delivery already succeeded.
func (d *Dispatcher) Dispatch(ctx context.Context, n Notification) error {
	settings, err := d.store.GetNotificationSettings()
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	sub, err := d.store.GetNotificationSubscription(n.Event)
	if err != nil {
		return err
	}
	if sub == nil || !sub.Subscribed {
		return nil
	}

	sysSettings, err := d.store.GetSettings()
	if err != nil {
		return err
	}
	backoff := DedupBackoff(sysSettings)
	dedupKey := BuildDedupKey(n)
	payload, err := json.Marshal(n)
	if err != nil {
		return err
	}

	exists, err := d.store.HasRecentNotificationDelivery(store.SingletonChannelID, dedupKey, time.Now().UTC().Add(-backoff))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	pending, err := d.store.HasPendingNotificationOutbox(store.SingletonChannelID, dedupKey)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}

	item := store.NotificationOutbox{
		ChannelID:     store.SingletonChannelID,
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
			return nil
		}
		return err
	}
	item.ID = id
	if err := d.outbox.AttemptOne(ctx, item, settings, *sub, false); err != nil {
		slog.Error("notification outbox immediate attempt failed", "outbox_id", item.ID, "event", n.Event, "err", err)
	}
	return nil
}

func decodeNotificationPayload(raw string) (Notification, error) {
	var n Notification
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return Notification{}, fmt.Errorf("decode outbox payload: %w", err)
	}
	return n, nil
}
