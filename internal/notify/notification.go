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
	Event                string         `json:"event"`
	Label                string         `json:"label"`
	DefaultSeverity      string         `json:"default_severity"`
	DefaultTitleTemplate string         `json:"default_title_template"`
	DefaultBodyTemplate  string         `json:"default_body_template"`
	SampleVars           map[string]any `json:"sample_vars"`
}

var builtInEventTypes = []EventType{
	{
		Event:                "account_expiring",
		Label:                "Account Expiring",
		DefaultSeverity:      "warning",
		DefaultTitleTemplate: `Lune · {{ .Title }}`,
		DefaultBodyTemplate:  `{{ .Message }}`,
		SampleVars: map[string]any{
			"account_label": "account-1",
			"expires_at":    time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
		},
	},
	{
		Event:                "account_error",
		Label:                "Account Error",
		DefaultSeverity:      "critical",
		DefaultTitleTemplate: `Lune · {{ .Title }}`,
		DefaultBodyTemplate:  `{{ .Message }}`,
		SampleVars: map[string]any{
			"account_label": "account-1",
			"last_error":    "upstream timeout",
		},
	},
	{
		Event:                "cpa_service_error",
		Label:                "CPA Service Error",
		DefaultSeverity:      "critical",
		DefaultTitleTemplate: `Lune · {{ .Title }}`,
		DefaultBodyTemplate:  `{{ .Message }}`,
		SampleVars: map[string]any{
			"service_label": "default-cpa",
			"last_error":    "healthz returned 500",
		},
	},
	{
		Event:                "test",
		Label:                "Test Message",
		DefaultSeverity:      "info",
		DefaultTitleTemplate: `Lune 测试消息`,
		DefaultBodyTemplate:  `这是一条用于验证渠道可达性的真实消息，可忽略。`,
		SampleVars: map[string]any{
			"instance_id": "lune",
			"admin_url":   "http://127.0.0.1:7788/admin",
		},
	},
}

func EventTypes() []EventType {
	out := make([]EventType, len(builtInEventTypes))
	copy(out, builtInEventTypes)
	return out
}
