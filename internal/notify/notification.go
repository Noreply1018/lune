package notify

import "time"

type NotificationSource struct {
	AccountID *int64 `json:"account_id,omitempty"`
	ServiceID *int64 `json:"service_id,omitempty"`
	PoolID    *int64 `json:"pool_id,omitempty"`
}

type Notification struct {
	Event     string             `json:"event"`
	Severity  string             `json:"severity"`
	Title     string             `json:"title"`
	Message   string             `json:"message"`
	Vars      map[string]any     `json:"vars,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
	Source    NotificationSource `json:"source"`
}

type EventType struct {
	Event               string         `json:"event"`
	Label               string         `json:"label"`
	DefaultSeverity     string         `json:"default_severity"`
	DefaultBodyTemplate string         `json:"default_body_template"`
	SampleVars          map[string]any `json:"sample_vars"`
}

func EventTypes() []EventType {
	now := time.Now().UTC()
	builtInEventTypes := []EventType{
		{
			Event:               "account_expiring",
			Label:               "账号即将过期",
			DefaultSeverity:     "warning",
			DefaultBodyTemplate: `账号 {{ .Vars.account_label }} 将在 {{ .Vars.expires_at }} 过期。`,
			SampleVars: map[string]any{
				"account_label": "account-1",
				"expires_at":    now.Add(48 * time.Hour).Format(time.RFC3339),
			},
		},
		{
			Event:               "cpa_credential_error",
			Label:               "CPA 登录态失效",
			DefaultSeverity:     "critical",
			DefaultBodyTemplate: `账号 {{ .Vars.account_label }} 的 CPA 登录态失效：{{ .Vars.last_error }}。请重新登录。`,
			SampleVars: map[string]any{
				"account_label": "account-1",
				"last_error":    "refresh token invalid",
			},
		},
		{
			Event:               "account_error",
			Label:               "账号不可用",
			DefaultSeverity:     "critical",
			DefaultBodyTemplate: `账号 {{ .Vars.account_label }} 最近错误：{{ .Vars.last_error }}`,
			SampleVars: map[string]any{
				"account_label": "account-1",
				"last_error":    "upstream timeout",
			},
		},
		{
			Event:               "cpa_service_error",
			Label:               "CPA Runtime 异常",
			DefaultSeverity:     "critical",
			DefaultBodyTemplate: `CPA runtime {{ .Vars.service_label }} 最近错误：{{ .Vars.last_error }}`,
			SampleVars: map[string]any{
				"service_label": "default-cpa",
				"last_error":    "healthz returned 500",
			},
		},
		{
			Event:               "test",
			Label:               "测试消息",
			DefaultSeverity:     "info",
			DefaultBodyTemplate: `这是一条用于验证渠道可达性的真实消息，可忽略。`,
			SampleVars: map[string]any{
				"instance_id": "lune",
				"admin_url":   "/admin",
			},
		},
	}
	out := make([]EventType, 0, len(builtInEventTypes))
	for _, item := range builtInEventTypes {
		sampleVars := make(map[string]any, len(item.SampleVars))
		for key, value := range item.SampleVars {
			sampleVars[key] = value
		}
		item.SampleVars = sampleVars
		out = append(out, item)
	}
	return out
}

// EventLabel returns the Chinese label for the given event key. When the event
// is not in the built-in catalog (defensive path for stray / legacy events)
// the event key itself is returned so the caller still gets a non-empty label
// to compose the auto-generated title.
func EventLabel(event string) string {
	for _, item := range EventTypes() {
		if item.Event == event {
			return item.Label
		}
	}
	return event
}

// AutoTitle composes the auto-generated title sent over the wire for an event.
// Titles are no longer user-editable; every delivery uses this exact prefix.
func AutoTitle(event string) string {
	return "Lune 通知：" + EventLabel(event)
}
