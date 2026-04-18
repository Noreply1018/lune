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

// fixedRetrySchedule is the hard-coded backoff (seconds) between delivery
// attempts. Indexed by attempt-1: the first retry waits 30s, the second 5m,
// the third 30m. Total span across the 3 retries ≈ 30 min.
var fixedRetrySchedule = []int{30, 300, 1800}

// fixedMaxAttempts caps total attempts (1 initial + 3 retries = 4). On the
// 4th failed attempt the outbox entry is marked dropped.
const fixedMaxAttempts = 4

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
			// Event was unsubscribed after it entered the queue. Drop the pending
			// row so re-enabling the subscription later doesn't resurface a stale
			// alert, and so processDue doesn't keep re-reading the row forever.
			_ = o.store.DeleteNotificationOutbox(item.ID)
			continue
		}
		if err := o.AttemptOne(ctx, item, settings, sub); err != nil {
			slog.Error("retry notification outbox attempt failed", "outbox_id", item.ID, "event", item.Event, "err", err)
			continue
		}
	}
}

// AttemptOne delivers a single outbox entry using the singleton WeChat-Work
// driver. On success the entry is cleared; on failure the entry is either
// re-scheduled (up to fixedMaxAttempts) or dropped.
func (o *Outbox) AttemptOne(ctx context.Context, item store.NotificationOutbox, settings store.NotificationSettings, sub store.NotificationSubscription) error {
	driver, ok := o.registry.Get(store.SingletonChannelType)
	if !ok {
		return o.fail(item, Result{}, AutoTitle(item.Event), truncateString(item.Payload, 1024), "wechat_work_bot driver is not registered")
	}
	n, err := decodeNotificationPayload(item.Payload)
	if err != nil {
		return o.fail(item, Result{}, AutoTitle(item.Event), truncateString(item.Payload, 1024), err.Error())
	}
	// Render before sending. If the template fails to compile/execute, surface
	// the error via fail() instead of falling back to the raw JSON payload —
	// dumping internal struct shape to an end-user's WeChat is worse than a
	// retry with a clear error.
	rendered, renderErr := RenderNotification(n, AutoTitle(item.Event), sub.BodyTemplate)
	if renderErr != nil {
		return o.fail(item, Result{}, AutoTitle(item.Event), truncateString(item.Payload, 1024), "render body: "+renderErr.Error())
	}
	title := rendered.Title
	renderedBody := rendered.Body
	payloadSummary := truncateString(rendered.Body, 1024)
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
		return o.fail(item, Result{}, title, payloadSummary, err.Error())
	}
	result, sendErr := driver.Send(ctx, n, runtime)
	if sendErr != nil || !result.OK {
		return o.fail(item, result, title, payloadSummary, firstNonEmpty(result.UpstreamMessage, errorString(sendErr)))
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

// fail records a failed delivery attempt. The caller passes title and
// payloadSummary so fail() does not need to re-render (which would re-fail
// on the very templates that caused AttemptOne to call it in the first place).
func (o *Outbox) fail(item store.NotificationOutbox, result Result, title string, payloadSummary string, message string) error {
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
// JSON config with webhook_url / mention_mobile_list; format is hardcoded to
// text so WeCom's native @ mention (via mentioned_mobile_list) works.
func buildChannelRuntime(settings store.NotificationSettings, sub store.NotificationSubscription, triggered string, rendered *RenderedMessage) (ChannelRuntime, error) {
	url := strings.TrimSpace(settings.WebhookURL)
	if url == "" {
		return ChannelRuntime{}, errors.New("webhook url is empty")
	}
	mobiles := settings.MentionMobileList
	if mobiles == nil {
		mobiles = []string{}
	}
	cfg, err := json.Marshal(map[string]any{
		"webhook_url":         url,
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
		Triggered: triggered,
		Rendered:  rendered,
	}, nil
}
