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
	channelIDs := make([]int64, 0, len(items))
	for _, item := range items {
		channelIDs = append(channelIDs, item.ChannelID)
	}
	channelsByID, err := o.store.ListNotificationChannelsByIDs(channelIDs)
	if err != nil {
		slog.Error("load due notification channels", "err", err)
		return
	}
	for _, item := range items {
		ch, ok := channelsByID[item.ChannelID]
		if !ok {
			_ = o.store.DeleteNotificationOutbox(item.ID)
			continue
		}
		if err := o.AttemptOne(ctx, item, ch, true); err != nil {
			slog.Error("retry notification outbox attempt failed", "outbox_id", item.ID, "channel_id", item.ChannelID, "err", err)
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
	err = o.store.RecordNotificationAttemptSuccess(item.ID, &store.NotificationDelivery{
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
	return nil
}

func (o *Outbox) fail(item store.NotificationOutbox, channel store.NotificationChannel, result Result, message string, retrying bool) error {
	title := item.Event
	payloadSummary := truncateString(item.Payload, 1024)
	if n, err := decodeNotificationPayload(item.Payload); err == nil {
		if rendered, renderErr := RenderNotification(n, channel.TitleTemplate, channel.BodyTemplate); renderErr == nil {
			title = rendered.Title
			payloadSummary = truncateString(rendered.Body, 1024)
		}
	}
	attempt := item.Attempt + 1
	status := "failed"
	if attempt > len(retrySchedule) {
		status = "dropped"
	}
	delivery := &store.NotificationDelivery{
		ChannelID:       channel.ID,
		ChannelName:     channel.Name,
		ChannelType:     channel.Type,
		Event:           item.Event,
		Severity:        item.Severity,
		Title:           title,
		PayloadSummary:  payloadSummary,
		Status:          status,
		UpstreamCode:    result.UpstreamCode,
		UpstreamMessage: firstNonEmpty(result.UpstreamMessage, message),
		LatencyMS:       result.LatencyMS,
		Attempt:         attempt,
		DedupKey:        item.DedupKey,
		TriggeredBy:     "system",
	}

	if attempt > len(retrySchedule) {
		return o.store.RecordNotificationAttemptDropped(item.ID, delivery, attempt, firstNonEmpty(result.UpstreamMessage, message))
	}
	nextAttemptAt := time.Now().UTC().Add(retrySchedule[attempt-1]).Format("2006-01-02 15:04:05")
	return o.store.RecordNotificationAttemptRetry(item.ID, delivery, attempt, nextAttemptAt, firstNonEmpty(result.UpstreamMessage, message))
}

func (o *Outbox) channelMutex(channelID int64) *sync.Mutex {
	actual, _ := o.channelMu.LoadOrStore(channelID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}
