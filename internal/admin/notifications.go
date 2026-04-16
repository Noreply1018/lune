package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"lune/internal/notify"
	"lune/internal/store"
	"lune/internal/webutil"
)

type notificationChannelResponse struct {
	ID            int64                            `json:"id"`
	Name          string                           `json:"name"`
	Type          string                           `json:"type"`
	Enabled       bool                             `json:"enabled"`
	Config        map[string]any                   `json:"config"`
	Subscriptions []store.NotificationSubscription `json:"subscriptions"`
	TitleTemplate string                           `json:"title_template"`
	BodyTemplate  string                           `json:"body_template"`
	CreatedAt     string                           `json:"created_at"`
	UpdatedAt     string                           `json:"updated_at"`
	LastDelivery  *store.NotificationDeliveryMeta  `json:"last_delivery,omitempty"`
}

type notificationChannelRequest struct {
	Name          string                           `json:"name"`
	Type          string                           `json:"type"`
	Enabled       bool                             `json:"enabled"`
	Config        map[string]any                   `json:"config"`
	Subscriptions []store.NotificationSubscription `json:"subscriptions"`
	TitleTemplate string                           `json:"title_template"`
	BodyTemplate  string                           `json:"body_template"`
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
	if len(req.Subscriptions) == 0 {
		req.Subscriptions = []store.NotificationSubscription{{Event: "*"}}
	}
	encodedConfig, err := h.notifier.Registry().MergeConfig(req.Type, nil, req.Config)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	channel := &store.NotificationChannel{
		Name:          strings.TrimSpace(req.Name),
		Type:          req.Type,
		Enabled:       req.Enabled,
		Config:        encodedConfig,
		Subscriptions: req.Subscriptions,
		TitleTemplate: req.TitleTemplate,
		BodyTemplate:  req.BodyTemplate,
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
	if len(req.Subscriptions) == 0 {
		req.Subscriptions = []store.NotificationSubscription{{Event: "*"}}
	}
	encodedConfig, err := h.notifier.Registry().MergeConfig(req.Type, existing.Config, req.Config)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", err.Error())
		return
	}
	existing.Name = strings.TrimSpace(req.Name)
	existing.Type = req.Type
	existing.Enabled = req.Enabled
	existing.Config = encodedConfig
	existing.Subscriptions = req.Subscriptions
	existing.TitleTemplate = req.TitleTemplate
	existing.BodyTemplate = req.BodyTemplate
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
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
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
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	result, err := h.notifier.SendChannelTest(r.Context(), id, h.notifier.BuildTestNotification(req.Event, req.Severity))
	if err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if limit, err := strconv.Atoi(raw); err == nil {
			filter.Limit = limit
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("before_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			filter.BeforeID = id
		}
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
		ID:            ch.ID,
		Name:          ch.Name,
		Type:          ch.Type,
		Enabled:       ch.Enabled,
		Config:        masked,
		Subscriptions: ch.Subscriptions,
		TitleTemplate: ch.TitleTemplate,
		BodyTemplate:  ch.BodyTemplate,
		CreatedAt:     ch.CreatedAt,
		UpdatedAt:     ch.UpdatedAt,
		LastDelivery:  ch.LastDelivery,
	}, nil
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
