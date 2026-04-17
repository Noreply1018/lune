package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"lune/internal/notify"
	"lune/internal/store"
	"lune/internal/webutil"
)

type notificationChannelResponse struct {
	ID               int64                            `json:"id"`
	Name             string                           `json:"name"`
	Type             string                           `json:"type"`
	Enabled          bool                             `json:"enabled"`
	Config           map[string]any                   `json:"config"`
	Subscriptions    []store.NotificationSubscription `json:"subscriptions"`
	TitleTemplate    string                           `json:"title_template"`
	BodyTemplate     string                           `json:"body_template"`
	RetryMaxAttempts int                              `json:"retry_max_attempts"`
	RetrySchedule    []int                            `json:"retry_schedule_seconds"`
	CreatedAt        string                           `json:"created_at"`
	UpdatedAt        string                           `json:"updated_at"`
	LastDelivery     *store.NotificationDeliveryMeta  `json:"last_delivery,omitempty"`
	RecentDeliveries []store.NotificationDeliveryMeta `json:"recent_deliveries,omitempty"`
}

type notificationChannelRequest struct {
	Name             string                           `json:"name"`
	Type             string                           `json:"type"`
	Enabled          bool                             `json:"enabled"`
	Config           map[string]any                   `json:"config"`
	Subscriptions    []store.NotificationSubscription `json:"subscriptions"`
	TitleTemplate    string                           `json:"title_template"`
	BodyTemplate     string                           `json:"body_template"`
	RetryMaxAttempts int                              `json:"retry_max_attempts"`
	RetrySchedule    []int                            `json:"retry_schedule_seconds"`
}

type notificationChannelEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Handler) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.store.ListNotificationChannels()
	if err != nil {
		h.internalError(w, err)
		return
	}
	resp, err := h.notificationChannelsResponse(channels)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteList(w, resp, len(resp))
}

func (h *Handler) getNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	channel, err := h.store.GetNotificationChannel(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if channel == nil {
		webutil.WriteAdminError(w, 404, "not_found", "notification channel not found")
		return
	}
	resp, err := h.notificationChannelResponse(*channel)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, resp)
}

func (h *Handler) createNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var req notificationChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Type) == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "name and type are required")
		return
	}
	var err error
	req.Subscriptions, err = normalizeSubscriptions(req.Subscriptions)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	retryMaxAttempts, retrySchedule, err := normalizeRetryConfig(req.RetryMaxAttempts, req.RetrySchedule)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	encodedConfig, err := h.notifier.Registry().MergeConfig(req.Type, nil, req.Config)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	channel := &store.NotificationChannel{
		Name:             strings.TrimSpace(req.Name),
		Type:             req.Type,
		Enabled:          req.Enabled,
		Config:           encodedConfig,
		Subscriptions:    req.Subscriptions,
		TitleTemplate:    strings.TrimSpace(req.TitleTemplate),
		BodyTemplate:     strings.TrimSpace(req.BodyTemplate),
		RetryMaxAttempts: retryMaxAttempts,
		RetrySchedule:    retrySchedule,
	}
	id, err := h.store.CreateNotificationChannel(channel)
	if err != nil {
		h.internalError(w, err)
		return
	}
	created, err := h.store.GetNotificationChannel(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	resp, err := h.notificationChannelResponse(*created)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 201, resp)
}

func (h *Handler) updateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	existing, err := h.store.GetNotificationChannel(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if existing == nil {
		webutil.WriteAdminError(w, 404, "not_found", "notification channel not found")
		return
	}
	var req notificationChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Type) == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "name and type are required")
		return
	}
	req.Subscriptions, err = normalizeSubscriptions(req.Subscriptions)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	retryMaxAttempts, retrySchedule, err := normalizeRetryConfig(req.RetryMaxAttempts, req.RetrySchedule)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	baseConfig := existing.Config
	if req.Type != existing.Type {
		baseConfig = nil
	}
	encodedConfig, err := h.notifier.Registry().MergeConfig(req.Type, baseConfig, req.Config)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	existing.Name = strings.TrimSpace(req.Name)
	existing.Type = req.Type
	existing.Enabled = req.Enabled
	existing.Config = encodedConfig
	existing.Subscriptions = req.Subscriptions
	existing.TitleTemplate = strings.TrimSpace(req.TitleTemplate)
	existing.BodyTemplate = strings.TrimSpace(req.BodyTemplate)
	existing.RetryMaxAttempts = retryMaxAttempts
	existing.RetrySchedule = retrySchedule
	if err := h.store.UpdateNotificationChannel(id, existing); err != nil {
		h.internalError(w, err)
		return
	}
	updated, err := h.store.GetNotificationChannel(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	resp, err := h.notificationChannelResponse(*updated)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, resp)
}

func (h *Handler) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteNotificationChannel(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			webutil.WriteAdminError(w, 404, "not_found", "notification channel not found")
			return
		}
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) setNotificationChannelEnabled(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	channel, err := h.store.GetNotificationChannel(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if channel == nil {
		webutil.WriteAdminError(w, 404, "not_found", "notification channel not found")
		return
	}
	var req notificationChannelEnabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.SetNotificationChannelEnabled(id, req.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			webutil.WriteAdminError(w, 404, "not_found", "notification channel not found")
			return
		}
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, map[string]any{
		"status":  "ok",
		"enabled": req.Enabled,
	})
}

func (h *Handler) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		Event    string `json:"event"`
		Severity string `json:"severity"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
			return
		}
	}
	result, err := h.notifier.SendChannelTest(r.Context(), id, h.notifier.BuildTestNotification(req.Event, req.Severity))
	if err != nil {
		if errors.Is(err, notify.ErrNotificationChannelNotFound) {
			webutil.WriteAdminError(w, 404, "not_found", "notification channel not found")
			return
		}
		webutil.WriteAdminError(w, 502, "channel_test_failed", firstNonEmpty(result.UpstreamMessage, err.Error()))
		return
	}
	webutil.WriteData(w, 200, result)
}

func (h *Handler) previewNotifications(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Event    string `json:"event"`
		Severity string `json:"severity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	items, err := h.notifier.Preview(h.notifier.BuildPreviewNotification(req.Event, req.Severity))
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteList(w, items, len(items))
}

func (h *Handler) listNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	filter := store.NotificationDeliveryFilter{
		Event:       strings.TrimSpace(r.URL.Query().Get("event")),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
		TriggeredBy: strings.TrimSpace(r.URL.Query().Get("triggered_by")),
		Before:      strings.TrimSpace(r.URL.Query().Get("before")),
		Limit:       50,
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("channel_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			filter.ChannelID = id
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if limit, err := strconv.Atoi(raw); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("before_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			filter.BeforeID = id
		}
	}
	if err := store.ValidateNotificationDeliveryCursor(filter.Before); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	items, err := h.store.ListNotificationDeliveries(filter)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteList(w, items, len(items))
}

func (h *Handler) listNotificationEventTypes(w http.ResponseWriter, r *http.Request) {
	webutil.WriteList(w, notify.EventTypes(), len(notify.EventTypes()))
}

func (h *Handler) notificationChannelsResponse(channels []store.NotificationChannel) ([]notificationChannelResponse, error) {
	out := make([]notificationChannelResponse, 0, len(channels))
	for _, ch := range channels {
		item, err := h.notificationChannelResponse(ch)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (h *Handler) notificationChannelResponse(ch store.NotificationChannel) (notificationChannelResponse, error) {
	masked, err := h.notifier.Registry().MaskConfig(ch.Type, ch.Config)
	if err != nil {
		return notificationChannelResponse{}, err
	}
	return notificationChannelResponse{
		ID:               ch.ID,
		Name:             ch.Name,
		Type:             ch.Type,
		Enabled:          ch.Enabled,
		Config:           masked,
		Subscriptions:    ch.Subscriptions,
		TitleTemplate:    ch.TitleTemplate,
		BodyTemplate:     ch.BodyTemplate,
		RetryMaxAttempts: ch.RetryMaxAttempts,
		RetrySchedule:    ch.RetrySchedule,
		CreatedAt:        ch.CreatedAt,
		UpdatedAt:        ch.UpdatedAt,
		LastDelivery:     ch.LastDelivery,
		RecentDeliveries: ch.RecentDeliveries,
	}, nil
}

func normalizeSubscriptions(input []store.NotificationSubscription) ([]store.NotificationSubscription, error) {
	if len(input) == 0 {
		return []store.NotificationSubscription{}, nil
	}
	out := make([]store.NotificationSubscription, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		item.Event = strings.TrimSpace(item.Event)
		item.MinSeverity = strings.TrimSpace(item.MinSeverity)
		item.TitleTemplate = strings.TrimSpace(item.TitleTemplate)
		item.BodyTemplate = strings.TrimSpace(item.BodyTemplate)
		if item.Event == "" {
			continue
		}
		if item.MinSeverity != "" && item.MinSeverity != "info" && item.MinSeverity != "warning" && item.MinSeverity != "critical" {
			return nil, errors.New("min_severity must be info, warning, or critical")
		}
		if _, ok := seen[item.Event]; ok {
			return nil, errors.New("duplicate subscription events are not allowed")
		}
		seen[item.Event] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return []store.NotificationSubscription{}, nil
	}
	return out, nil
}

func normalizeRetryConfig(maxAttempts int, schedule []int) (int, []int, error) {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	normalized := make([]int, 0, len(schedule))
	for _, value := range schedule {
		if value <= 0 {
			return 0, nil, errors.New("retry_schedule_seconds must contain positive integers")
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		normalized = []int{30, 120, 600, 1800, 7200}
	}
	if len(normalized) < maxAttempts {
		return 0, nil, errors.New("retry_schedule_seconds length must be greater than or equal to retry_max_attempts")
	}
	return maxAttempts, normalized, nil
}

func (h *Handler) deprecatedWebhookURL() (string, error) {
	settings, err := h.store.GetSettings()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(settings["webhook_url"]), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
