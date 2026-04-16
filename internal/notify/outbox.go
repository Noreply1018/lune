package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"lune/internal/store"
)

var retrySchedule = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

type Outbox struct {
	store     *store.Store
	registry  *Registry
	channelMu sync.Map
}

func NewOutbox(st *store.Store, registry *Registry) *Outbox {
	return &Outbox{store: st, registry: registry}
}

func (o *Outbox) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			o.processDue(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (o *Outbox) processDue(ctx context.Context) {
	items, err := o.store.ListDueNotificationOutbox(100)
	if err != nil {
		slog.Error("list due notification outbox", "err", err)
		return
	}
	for _, item := range items {
		ch, err := o.store.GetNotificationChannel(item.ChannelID)
		if err != nil {
			slog.Error("load notification channel", "channel_id", item.ChannelID, "err", err)
			continue
		}
		if ch == nil {
			_ = o.store.DeleteNotificationOutbox(item.ID)
			continue
		}
		if err := o.AttemptOne(ctx, item, *ch, true); err != nil {
			continue
		}
	}
}

func (o *Outbox) AttemptOne(ctx context.Context, item store.NotificationOutbox, channel store.NotificationChannel, retrying bool) error {
	driver, ok := o.registry.Get(channel.Type)
	if !ok {
		return o.fail(item, channel, Result{}, "unsupported channel type", retrying)
	}
	n, err := decodeNotificationPayload(item.Payload)
	if err != nil {
		return o.fail(item, channel, Result{}, err.Error(), retrying)
	}
	mu := o.channelMutex(channel.ID)
	mu.Lock()
	defer mu.Unlock()

	result, sendErr := driver.Send(ctx, n, ChannelRuntime{
		ID:        channel.ID,
		Name:      channel.Name,
		Type:      channel.Type,
		Config:    channel.Config,
		TitleTpl:  channel.TitleTemplate,
		BodyTpl:   channel.BodyTemplate,
		Triggered: "system",
	})
	if sendErr != nil || !result.OK {
		return o.fail(item, channel, result, firstNonEmpty(result.UpstreamMessage, errorString(sendErr)), retrying)
	}

	rendered, err := RenderNotification(n, channel.TitleTemplate, channel.BodyTemplate)
	if err != nil {
		return o.fail(item, channel, result, err.Error(), retrying)
	}
	_, err = o.store.CreateNotificationDelivery(&store.NotificationDelivery{
		ChannelID:       channel.ID,
		ChannelName:     channel.Name,
		ChannelType:     channel.Type,
		Event:           n.Event,
		Severity:        n.Severity,
		Title:           rendered.Title,
		PayloadSummary:  truncateString(rendered.Body, 1024),
		Status:          "success",
		UpstreamCode:    result.UpstreamCode,
		UpstreamMessage: result.UpstreamMessage,
		LatencyMS:       result.LatencyMS,
		Attempt:         max(item.Attempt, 1),
		DedupKey:        item.DedupKey,
		TriggeredBy:     "system",
	})
	if err != nil {
		return err
	}
	return o.store.DeleteNotificationOutbox(item.ID)
}

func (o *Outbox) fail(item store.NotificationOutbox, channel store.NotificationChannel, result Result, message string, retrying bool) error {
	n, _ := decodeNotificationPayload(item.Payload)
	rendered, _ := RenderNotification(n, channel.TitleTemplate, channel.BodyTemplate)
	attempt := item.Attempt + 1
	status := "failed"
	if attempt > len(retrySchedule) {
		status = "dropped"
	}
	if _, err := o.store.CreateNotificationDelivery(&store.NotificationDelivery{
		ChannelID:       channel.ID,
		ChannelName:     channel.Name,
		ChannelType:     channel.Type,
		Event:           item.Event,
		Severity:        item.Severity,
		Title:           rendered.Title,
		PayloadSummary:  truncateString(rendered.Body, 1024),
		Status:          status,
		UpstreamCode:    result.UpstreamCode,
		UpstreamMessage: firstNonEmpty(result.UpstreamMessage, message),
		LatencyMS:       result.LatencyMS,
		Attempt:         attempt,
		DedupKey:        item.DedupKey,
		TriggeredBy:     "system",
	}); err != nil {
		return err
	}

	if attempt > len(retrySchedule) {
		return o.store.MarkNotificationOutboxDropped(item.ID, attempt, firstNonEmpty(result.UpstreamMessage, message))
	}
	nextAttemptAt := time.Now().UTC().Add(retrySchedule[attempt-1]).Format("2006-01-02 15:04:05")
	return o.store.UpdateNotificationOutboxRetry(item.ID, attempt, nextAttemptAt, firstNonEmpty(result.UpstreamMessage, message))
}

func (o *Outbox) channelMutex(channelID int64) *sync.Mutex {
	actual, _ := o.channelMu.LoadOrStore(channelID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}
