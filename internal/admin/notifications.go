package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"lune/internal/notify"
	"lune/internal/store"
	"lune/internal/webutil"
)

// notificationOverviewResponse is the payload returned by
// GET /admin/api/notifications. It bundles the singleton settings, the fixed
// subscription rows, and the built-in event catalog so the frontend can
// render the whole page in one call.
type notificationOverviewResponse struct {
	Settings      store.NotificationSettings       `json:"settings"`
	Subscriptions []store.NotificationSubscription `json:"subscriptions"`
	EventTypes    []notify.EventType               `json:"event_types"`
	LastDelivery  *store.NotificationDeliveryMeta  `json:"last_delivery,omitempty"`
}

type notificationSettingsRequest struct {
	Enabled           bool     `json:"enabled"`
	WebhookURL        string   `json:"webhook_url"`
	MentionMobileList []string `json:"mention_mobile_list"`
}

type notificationSubscriptionRequest struct {
	Subscribed   bool   `json:"subscribed"`
	BodyTemplate string `json:"body_template"`
}

var mobileNumberRegexp = regexp.MustCompile(`^\d{11}$`)

func (h *Handler) getNotifications(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetNotificationSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}
	subs, err := h.store.ListNotificationSubscriptions()
	if err != nil {
		h.internalError(w, err)
		return
	}
	recent, err := h.store.ListRecentNotificationDeliveryMeta(1)
	if err != nil {
		h.internalError(w, err)
		return
	}
	var last *store.NotificationDeliveryMeta
	if len(recent) > 0 {
		copied := recent[0]
		last = &copied
	}
	resp := notificationOverviewResponse{
		Settings:      settings,
		Subscriptions: subs,
		EventTypes:    notify.EventTypes(),
		LastDelivery:  last,
	}
	webutil.WriteData(w, 200, resp)
}

func (h *Handler) updateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var req notificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	req.WebhookURL = strings.TrimSpace(req.WebhookURL)
	mentions := make([]string, 0, len(req.MentionMobileList))
	seen := make(map[string]struct{}, len(req.MentionMobileList))
	for _, raw := range req.MentionMobileList {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if value != "@all" && !mobileNumberRegexp.MatchString(value) {
			webutil.WriteAdminError(w, 400, "bad_request", "mention_mobile_list entries must be an 11-digit mobile number or @all")
			return
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		mentions = append(mentions, value)
	}
	if req.Enabled {
		if req.WebhookURL == "" {
			webutil.WriteAdminError(w, 400, "bad_request", "webhook_url is required when notifications are enabled")
			return
		}
		if err := validateWebhookURL(req.WebhookURL); err != nil {
			webutil.WriteAdminError(w, 400, "bad_request", err.Error())
			return
		}
	} else if req.WebhookURL != "" {
		// Allow the admin to persist a URL while the switch is off, but still
		// sanity-check it so we don't store something that will obviously fail
		// the moment they re-enable.
		if err := validateWebhookURL(req.WebhookURL); err != nil {
			webutil.WriteAdminError(w, 400, "bad_request", err.Error())
			return
		}
	}
	settings := store.NotificationSettings{
		Enabled:           req.Enabled,
		WebhookURL:        req.WebhookURL,
		MentionMobileList: mentions,
	}
	if err := h.store.UpdateNotificationSettings(settings); err != nil {
		h.internalError(w, err)
		return
	}
	stored, err := h.store.GetNotificationSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, stored)
}

func (h *Handler) updateNotificationSubscription(w http.ResponseWriter, r *http.Request) {
	event := strings.TrimSpace(r.PathValue("event"))
	if event == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "event is required")
		return
	}
	if !isKnownEventType(event) {
		webutil.WriteAdminError(w, 404, "not_found", "unknown event type")
		return
	}
	var req notificationSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	body := strings.TrimSpace(req.BodyTemplate)
	if body == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "body_template must not be empty")
		return
	}
	if err := h.store.UpdateNotificationSubscription(event, req.Subscribed, body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			webutil.WriteAdminError(w, 404, "not_found", "subscription not found")
			return
		}
		h.internalError(w, err)
		return
	}
	updated, err := h.store.GetNotificationSubscription(event)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if updated == nil {
		webutil.WriteAdminError(w, 404, "not_found", "subscription not found")
		return
	}
	webutil.WriteData(w, 200, updated)
}

func (h *Handler) testNotification(w http.ResponseWriter, r *http.Request) {
	// Body is ignored; the test event is hardcoded.
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
	}
	n := h.notifier.BuildTestNotification()
	result, err := h.notifier.SendSingletonTest(r.Context(), n)
	if err != nil {
		if errors.Is(err, notify.ErrNotificationDisabled) {
			webutil.WriteAdminError(w, 409, "notifications_disabled", "notifications are disabled or webhook url is empty")
			return
		}
		webutil.WriteAdminError(w, 502, "channel_test_failed", firstNonEmpty(result.UpstreamMessage, err.Error()))
		return
	}
	webutil.WriteData(w, 200, result)
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

func validateWebhookURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("webhook_url must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("webhook_url must start with http:// or https://")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return errors.New("webhook_url must include a host")
	}
	return nil
}

func isKnownEventType(event string) bool {
	for _, item := range notify.EventTypes() {
		if item.Event == event {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
