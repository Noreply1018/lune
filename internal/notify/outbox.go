package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"lune/internal/store"
)

// fixedRetrySchedule is the hard-coded backoff between delivery attempts
// (in seconds). Total span: ~30 min across 3 retries.
var fixedRetrySchedule = []int{30, 300, 1800}

// fixedMaxAttempts is the total attempts (initial + retries) allowed before
// an outbox entry is marked dropped.
const fixedMaxAttempts = 3

type itemLock struct {
	mu   sync.Mutex
	refs int
}

type Outbox struct {
	store    *store.Store
	registry *Registry
	locksMu  sync.Mutex
	locks    map[int64]*itemLock
}

func NewOutbox(st *store.Store, registry *Registry) *Outbox {
	return &Outbox{
		store:    st,
		registry: registry,
		locks:    make(map[int64]*itemLock),
	}
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
	if len(items) == 0 {
		return
	}
	settings, err := o.store.GetNotificationSettings()
	if err != nil {
		slog.Error("load notification settings", "err", err)
		return
	}
	if !settings.Enabled {
		return
	}
	subs, err := o.store.ListNotificationSubscriptions()
	if err != nil {
		slog.Error("load notification subscriptions", "err", err)
		return
	}
	subsByEvent := make(map[string]store.NotificationSubscription, len(subs))
	for _, sub := range subs {
		subsByEvent[sub.Event] = sub
	}
	for _, item := range items {
		sub, ok := subsByEvent[item.Event]
		if !ok {
			// Event no longer tracked — drop to avoid an infinite retry.
			_ = o.store.DeleteNotificationOutbox(item.ID)
			continue
		}
		if !sub.Subscribed {
			continue
		}
		if err := o.AttemptOne(ctx, item, settings, sub, true); err != nil {
			slog.Error("retry notification outbox attempt failed", "outbox_id", item.ID, "event", item.Event, "err", err)
			continue
		}
	}
}

// AttemptOne delivers a single outbox entry using the singleton WeChat-Work
// driver. On success the entry is cleared; on failure the entry is either
// re-scheduled (up to fixedMaxAttempts) or dropped.
func (o *Outbox) AttemptOne(ctx context.Context, item store.NotificationOutbox, settings store.NotificationSettings, sub store.NotificationSubscription, retrying bool) error {
	driver, ok := o.registry.Get(store.SingletonChannelType)
	if !ok {
		return o.fail(item, sub, Result{}, "wechat_work_bot driver is not registered", retrying)
	}
	n, err := decodeNotificationPayload(item.Payload)
	if err != nil {
		return o.fail(item, sub, Result{}, err.Error(), retrying)
	}
	title := n.Event
	renderedBody := item.Payload
	payloadSummary := truncateString(item.Payload, 1024)
	if rendered, err := RenderNotification(n, sub.TitleTemplate, sub.BodyTemplate); err == nil {
		title = rendered.Title
		renderedBody = rendered.Body
		payloadSummary = truncateString(rendered.Body, 1024)
	}
	lk := o.acquireItemLock(item.ID)
	defer o.releaseItemLock(item.ID, lk)
	current, err := o.store.GetNotificationOutbox(item.ID)
	if err != nil {
		return err
	}
	if current == nil || (current.Status != "pending" && current.Status != "retrying") {
		return nil
	}
	item = *current
	runtime, err := buildChannelRuntime(settings, sub, "system", &RenderedMessage{Title: title, Body: renderedBody})
	if err != nil {
		return o.fail(item, sub, Result{}, err.Error(), retrying)
	}
	result, sendErr := driver.Send(ctx, n, runtime)
	if sendErr != nil || !result.OK {
		return o.fail(item, sub, result, firstNonEmpty(result.UpstreamMessage, errorString(sendErr)), retrying)
	}

	return o.store.RecordNotificationAttemptSuccess(item.ID, &store.NotificationDelivery{
		ChannelID:       store.SingletonChannelID,
		ChannelName:     store.SingletonChannelName,
		ChannelType:     store.SingletonChannelType,
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
}

func (o *Outbox) fail(item store.NotificationOutbox, sub store.NotificationSubscription, result Result, message string, retrying bool) error {
	title := item.Event
	payloadSummary := truncateString(item.Payload, 1024)
	if n, err := decodeNotificationPayload(item.Payload); err == nil {
		if rendered, renderErr := RenderNotification(n, sub.TitleTemplate, sub.BodyTemplate); renderErr == nil {
			title = rendered.Title
			payloadSummary = truncateString(rendered.Body, 1024)
		}
	}
	attempt := item.Attempt + 1
	status := "failed"
	if attempt >= fixedMaxAttempts {
		status = "dropped"
	}
	delivery := &store.NotificationDelivery{
		ChannelID:       store.SingletonChannelID,
		ChannelName:     store.SingletonChannelName,
		ChannelType:     store.SingletonChannelType,
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

	if attempt >= fixedMaxAttempts {
		return o.store.RecordNotificationAttemptDropped(item.ID, delivery, attempt, firstNonEmpty(result.UpstreamMessage, message))
	}
	// attempt is 1-based; the next wait uses index attempt-1 into the schedule.
	waitIndex := attempt - 1
	if waitIndex < 0 {
		waitIndex = 0
	}
	if waitIndex >= len(fixedRetrySchedule) {
		waitIndex = len(fixedRetrySchedule) - 1
	}
	nextAttemptAt := time.Now().UTC().Add(time.Duration(fixedRetrySchedule[waitIndex]) * time.Second).Format("2006-01-02 15:04:05")
	return o.store.RecordNotificationAttemptRetry(item.ID, delivery, attempt, nextAttemptAt, firstNonEmpty(result.UpstreamMessage, message))
}

func (o *Outbox) acquireItemLock(id int64) *itemLock {
	o.locksMu.Lock()
	lk, ok := o.locks[id]
	if !ok {
		lk = &itemLock{}
		o.locks[id] = lk
	}
	lk.refs++
	o.locksMu.Unlock()
	lk.mu.Lock()
	return lk
}

func (o *Outbox) releaseItemLock(id int64, lk *itemLock) {
	lk.mu.Unlock()
	o.locksMu.Lock()
	lk.refs--
	if lk.refs == 0 {
		delete(o.locks, id)
	}
	o.locksMu.Unlock()
}

// buildChannelRuntime translates the singleton settings into a ChannelRuntime
// understood by the WeChat Work Bot driver. The driver expects an embedded
// JSON config with url / format / mention_mobile_list fields, so we marshal
// those back up here.
func buildChannelRuntime(settings store.NotificationSettings, sub store.NotificationSubscription, triggered string, rendered *RenderedMessage) (ChannelRuntime, error) {
	url := strings.TrimSpace(settings.WebhookURL)
	if url == "" {
		return ChannelRuntime{}, errors.New("webhook url is empty")
	}
	format := settings.Format
	if format == "" {
		format = "markdown"
	}
	mobiles := settings.MentionMobileList
	if mobiles == nil {
		mobiles = []string{}
	}
	cfg, err := json.Marshal(map[string]any{
		"webhook_url":         url,
		"format":              format,
		"mention_mobile_list": mobiles,
	})
	if err != nil {
		return ChannelRuntime{}, err
	}
	return ChannelRuntime{
		ID:        store.SingletonChannelID,
		Name:      store.SingletonChannelName,
		Type:      store.SingletonChannelType,
		Config:    cfg,
		TitleTpl:  sub.TitleTemplate,
		BodyTpl:   sub.BodyTemplate,
		Triggered: triggered,
		Rendered:  rendered,
	}, nil
}
