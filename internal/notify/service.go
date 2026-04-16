package notify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"lune/internal/store"
	"lune/internal/syscfg"
)

type Service struct {
	store      *store.Store
	registry   *Registry
	outbox     *Outbox
	dispatcher *Dispatcher
}

func NewService(st *store.Store) *Service {
	return NewServiceWithRegistry(st, NewRegistry())
}

func NewServiceWithRegistry(st *store.Store, registry *Registry) *Service {
	outbox := NewOutbox(st, registry)
	dispatcher := NewDispatcher(st, registry, outbox)
	return &Service{
		store:      st,
		registry:   registry,
		outbox:     outbox,
		dispatcher: dispatcher,
	}
}

func (s *Service) Registry() *Registry {
	return s.registry
}

func (s *Service) Dispatch(ctx context.Context, n Notification) error {
	return s.dispatcher.Dispatch(ctx, n)
}

func (s *Service) Run(ctx context.Context) {
	s.outbox.Run(ctx)
}

func (s *Service) BuildTestNotification(event, severity string) Notification {
	event = firstNonEmpty(strings.TrimSpace(event), "test")
	severity = firstNonEmpty(strings.TrimSpace(severity), "info")
	return Notification{
		Event:     event,
		Severity:  severity,
		Title:     "Lune 测试消息",
		Message:   "这是一条用于验证渠道可达性的真实消息，可忽略。",
		Timestamp: time.Now().UTC(),
		Vars: map[string]any{
			"instance_id": "lune",
			"admin_url":   "/admin",
		},
	}
}

func (s *Service) BuildPreviewNotification(event, severity string) Notification {
	event = strings.TrimSpace(event)
	severity = strings.TrimSpace(severity)
	if event == "" {
		event = "account_error"
	}
	var sample *EventType
	for _, item := range EventTypes() {
		if item.Event == event {
			copied := item
			sample = &copied
			break
		}
	}
	if severity == "" {
		if sample != nil && sample.DefaultSeverity != "" {
			severity = sample.DefaultSeverity
		} else {
			severity = "info"
		}
	}
	title := "Lune Notification Preview"
	message := "This is a preview of the rendered notification body."
	vars := map[string]any{
		"instance_id": "lune",
		"admin_url":   "/admin",
	}
	if sample != nil {
		title = sample.Label
		message = fmt.Sprintf("Preview payload for %s.", sample.Label)
		for key, value := range sample.SampleVars {
			vars[key] = value
		}
	}
	return Notification{
		Event:     event,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Vars:      vars,
	}
}

func (s *Service) Preview(n Notification) ([]PreviewItem, error) {
	channels, err := s.store.ListNotificationChannels()
	if err != nil {
		return nil, err
	}
	items := make([]PreviewItem, 0, len(channels))
	for _, ch := range channels {
		rendered, matched, reason, err := s.previewOne(ch, n)
		if err != nil {
			return nil, err
		}
		item := PreviewItem{
			ChannelID:     ch.ID,
			ChannelName:   ch.Name,
			ChannelType:   ch.Type,
			Matched:       matched,
			RenderedTitle: rendered.Title,
			RenderedBody:  rendered.Body,
			DryRunOK:      matched,
			SkippedReason: reason,
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) previewOne(ch store.NotificationChannel, n Notification) (RenderedMessage, bool, string, error) {
	if !ch.Enabled {
		return RenderedMessage{}, false, "channel_disabled", nil
	}
	if !matchSubscriptions(ch.Subscriptions, n.Event, n.Severity) {
		return RenderedMessage{}, false, "subscription_mismatch", nil
	}
	rendered, err := RenderNotification(n, ch.TitleTemplate, ch.BodyTemplate)
	if err != nil {
		return RenderedMessage{}, false, "", err
	}
	return rendered, true, "", nil
}

func (s *Service) SendChannelTest(ctx context.Context, channelID int64, n Notification) (Result, error) {
	channel, err := s.store.GetNotificationChannel(channelID)
	if err != nil {
		return Result{}, err
	}
	if channel == nil {
		return Result{}, fmt.Errorf("notification channel not found")
	}
	driver, ok := s.registry.Get(channel.Type)
	if !ok {
		return Result{}, fmt.Errorf("unsupported channel type: %s", channel.Type)
	}
	result, err := driver.Send(ctx, n, ChannelRuntime{
		ID:        channel.ID,
		Name:      channel.Name,
		Type:      channel.Type,
		Config:    channel.Config,
		TitleTpl:  channel.TitleTemplate,
		BodyTpl:   channel.BodyTemplate,
		Triggered: "test",
	})
	deliveryStatus := "success"
	if err != nil || !result.OK {
		deliveryStatus = "failed"
	}
	rendered, renderErr := RenderNotification(n, channel.TitleTemplate, channel.BodyTemplate)
	if renderErr == nil {
		_, _ = s.store.CreateNotificationDelivery(&store.NotificationDelivery{
			ChannelID:       channel.ID,
			ChannelName:     channel.Name,
			ChannelType:     channel.Type,
			Event:           n.Event,
			Severity:        n.Severity,
			Title:           rendered.Title,
			PayloadSummary:  truncateString(rendered.Body, 1024),
			Status:          deliveryStatus,
			UpstreamCode:    result.UpstreamCode,
			UpstreamMessage: firstNonEmpty(result.UpstreamMessage, errorString(err)),
			LatencyMS:       result.LatencyMS,
			Attempt:         1,
			DedupKey:        "",
			TriggeredBy:     "test",
		})
	}
	return result, err
}

func (s *Service) SendLegacyWebhookTest(ctx context.Context, rawURL string, n Notification) (Result, error) {
	driver := s.registry.MustGet("generic_webhook")
	cfg, err := json.Marshal(map[string]any{
		"schema": 1,
		"url":    strings.TrimSpace(rawURL),
	})
	if err != nil {
		return Result{}, err
	}
	return driver.Send(ctx, n, ChannelRuntime{
		ID:        0,
		Name:      "Legacy Webhook",
		Type:      "generic_webhook",
		Config:    cfg,
		TitleTpl:  "",
		BodyTpl:   "",
		Triggered: "test",
	})
}

func BuildDedupKey(n Notification) string {
	var source string
	switch {
	case n.Source.AccountID != nil:
		source = fmt.Sprintf("account:%d", *n.Source.AccountID)
	case n.Source.ServiceID != nil:
		source = fmt.Sprintf("service:%d", *n.Source.ServiceID)
	case n.Source.PoolID != nil:
		source = fmt.Sprintf("pool:%d", *n.Source.PoolID)
	default:
		source = "global"
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%s", n.Event, n.Severity, source)))
	return hex.EncodeToString(sum[:])
}

func DedupBackoff(settings map[string]string) time.Duration {
	seconds := syscfg.ParsePositiveInt(settings["notification_dedup_backoff_seconds"], 3600)
	return time.Duration(seconds) * time.Second
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

type PreviewItem struct {
	ChannelID     int64  `json:"channel_id"`
	ChannelName   string `json:"channel_name"`
	ChannelType   string `json:"channel_type"`
	Matched       bool   `json:"matched"`
	RenderedTitle string `json:"rendered_title"`
	RenderedBody  string `json:"rendered_body"`
	DryRunOK      bool   `json:"dry_run_ok"`
	SkippedReason string `json:"skipped_reason,omitempty"`
}
