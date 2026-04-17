package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"lune/internal/store"
)

var retrySchedule = []int{30, 120, 600, 1800, 7200}

type Outbox struct {
	store    *store.Store
	registry *Registry
	itemMu   sync.Map
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
			o.itemMu.Delete(item.ID)
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
	title := n.Event
	renderedBody := item.Payload
	payloadSummary := truncateString(item.Payload, 1024)
	if rendered, err := RenderChannelNotification(n, channel); err == nil {
		title = rendered.Title
		renderedBody = rendered.Body
		payloadSummary = truncateString(rendered.Body, 1024)
	}
	mu := o.itemMutex(item.ID)
	mu.Lock()
	defer o.releaseItemMutex(item.ID)
	defer mu.Unlock()
	current, err := o.store.GetNotificationOutbox(item.ID)
	if err != nil {
		return err
	}
	if current == nil || (current.Status != "pending" && current.Status != "retrying") {
		return nil
	}
	item = *current
	titleTpl, bodyTpl := ResolveChannelTemplates(channel, n.Event)
	result, sendErr := driver.Send(ctx, n, ChannelRuntime{
		ID:        channel.ID,
		Name:      channel.Name,
		Type:      channel.Type,
		Config:    channel.Config,
		TitleTpl:  titleTpl,
		BodyTpl:   bodyTpl,
		Triggered: "system",
		Rendered:  &RenderedMessage{Title: title, Body: renderedBody},
	})
	if sendErr != nil || !result.OK {
		return o.fail(item, channel, result, firstNonEmpty(result.UpstreamMessage, errorString(sendErr)), retrying)
	}

	err = o.store.RecordNotificationAttemptSuccess(item.ID, &store.NotificationDelivery{
		ChannelID:       channel.ID,
		ChannelName:     channel.Name,
		ChannelType:     channel.Type,
		Event:           n.Event,
		Severity:        n.Severity,
		Title:           title,
		PayloadSummary:  payloadSummary,
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
		if rendered, renderErr := RenderChannelNotification(n, channel); renderErr == nil {
			title = rendered.Title
			payloadSummary = truncateString(rendered.Body, 1024)
		}
	}
	attempt := item.Attempt + 1
	status := "failed"
	retrySeconds := channelRetrySchedule(channel)
	if attempt > channelRetryMaxAttempts(channel) {
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

	if attempt > channelRetryMaxAttempts(channel) {
		return o.store.RecordNotificationAttemptDropped(item.ID, delivery, attempt, firstNonEmpty(result.UpstreamMessage, message))
	}
	nextAttemptAt := time.Now().UTC().Add(time.Duration(retrySeconds[attempt-1]) * time.Second).Format("2006-01-02 15:04:05")
	return o.store.RecordNotificationAttemptRetry(item.ID, delivery, attempt, nextAttemptAt, firstNonEmpty(result.UpstreamMessage, message))
}

func (o *Outbox) itemMutex(outboxID int64) *sync.Mutex {
	actual, _ := o.itemMu.LoadOrStore(outboxID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (o *Outbox) releaseItemMutex(outboxID int64) {
	current, err := o.store.GetNotificationOutbox(outboxID)
	if err != nil {
		return
	}
	if current == nil || current.Status == "success" || current.Status == "dropped" {
		o.itemMu.Delete(outboxID)
	}
}

func channelRetryMaxAttempts(channel store.NotificationChannel) int {
	if channel.RetryMaxAttempts > 0 {
		return channel.RetryMaxAttempts
	}
	return len(retrySchedule)
}

func channelRetrySchedule(channel store.NotificationChannel) []int {
	if len(channel.RetrySchedule) >= channelRetryMaxAttempts(channel) {
		return channel.RetrySchedule
	}
	return retrySchedule
}
