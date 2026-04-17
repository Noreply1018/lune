package notify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
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

// ErrNotificationDisabled is returned by SendSingletonTest when the admin
// disabled the top-level switch or left the webhook URL blank.
var ErrNotificationDisabled = errors.New("notifications are disabled or webhook url is empty")

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

// BuildTestNotification produces the canonical "test" notification used for
// the Send Test button. Vars match the built-in test event's sample vars so
// template placeholders resolve sensibly.
func (s *Service) BuildTestNotification() Notification {
	return Notification{
		Event:     "test",
		Severity:  "info",
		Title:     "Lune 测试消息",
		Message:   "这是一条用于验证渠道可达性的真实消息，可忽略。",
		Timestamp: time.Now().UTC(),
		Vars: map[string]any{
			"instance_id": "lune",
			"admin_url":   "/admin",
		},
	}
}

// SendSingletonTest sends the given notification through the WeChat-Work
// driver using the current singleton settings, recording the attempt in
// notification_deliveries. It does not go through the outbox and does not
// honour the subscription's subscribed flag.
func (s *Service) SendSingletonTest(ctx context.Context, n Notification) (Result, error) {
	settings, err := s.store.GetNotificationSettings()
	if err != nil {
		return Result{}, err
	}
	if !settings.Enabled || strings.TrimSpace(settings.WebhookURL) == "" {
		return Result{}, ErrNotificationDisabled
	}
	sub, err := s.store.GetNotificationSubscription(n.Event)
	if err != nil {
		return Result{}, err
	}
	// For events that aren't in the built-in list we synthesize a passthrough
	// subscription so the driver still receives a title/body pair.
	var effectiveSub store.NotificationSubscription
	if sub != nil {
		effectiveSub = *sub
	} else {
		effectiveSub = store.NotificationSubscription{
			Event:         n.Event,
			Subscribed:    true,
			TitleTemplate: "{{ .Title }}",
			BodyTemplate:  "{{ .Message }}",
		}
	}
	rendered, err := RenderNotification(n, effectiveSub.TitleTemplate, effectiveSub.BodyTemplate)
	if err != nil {
		return Result{}, err
	}
	driver, ok := s.registry.Get(store.SingletonChannelType)
	if !ok {
		return Result{}, fmt.Errorf("wechat_work_bot driver is not registered")
	}
	runtime, err := buildChannelRuntime(settings, effectiveSub, "test", &rendered)
	if err != nil {
		return Result{}, err
	}
	result, sendErr := driver.Send(ctx, n, runtime)
	deliveryStatus := "success"
	if sendErr != nil || !result.OK {
		deliveryStatus = "failed"
	}
	_, _ = s.store.CreateNotificationDelivery(&store.NotificationDelivery{
		ChannelID:       store.SingletonChannelID,
		ChannelName:     store.SingletonChannelName,
		ChannelType:     store.SingletonChannelType,
		Event:           n.Event,
		Severity:        n.Severity,
		Title:           rendered.Title,
		PayloadSummary:  truncateString(rendered.Body, 1024),
		Status:          deliveryStatus,
		UpstreamCode:    result.UpstreamCode,
		UpstreamMessage: firstNonEmpty(result.UpstreamMessage, errorString(sendErr)),
		LatencyMS:       result.LatencyMS,
		Attempt:         1,
		DedupKey:        "",
		TriggeredBy:     "test",
	})
	return result, sendErr
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
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
