package syscfg

import "strconv"

const (
	DefaultHealthCheckInterval         = 60
	DefaultRequestTimeout              = 120
	DefaultMaxRetryAttempts            = 3
	DefaultNotificationExpiringDays    = 7
	DefaultNotificationErrorEnabled    = true
	DefaultNotificationExpiringEnabled = true
	DefaultWebhookEnabled              = false
	DefaultWebhookURL                  = ""
	DefaultDataRetentionDays           = 30
)

var allowedSettingKeys = map[string]struct{}{
	"health_check_interval":         {},
	"request_timeout":               {},
	"max_retry_attempts":            {},
	"notification_error_enabled":    {},
	"notification_expiring_enabled": {},
	"notification_expiring_days":    {},
	"webhook_enabled":               {},
	"webhook_url":                   {},
	"data_retention_days":           {},
}

func IsAllowedSettingKey(key string) bool {
	_, ok := allowedSettingKeys[key]
	return ok
}

func ParsePositiveInt(raw string, fallback int) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return fallback
}

func ParseNonNegativeInt(raw string, fallback int) int {
	if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
		return n
	}
	return fallback
}

func ParseBool(raw string, fallback bool) bool {
	switch raw {
	case "1", "true", "TRUE", "True":
		return true
	case "0", "false", "FALSE", "False":
		return false
	default:
		return fallback
	}
}

func BoolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
