package cpa

import (
	"strconv"
	"strings"
	"time"
)

func SubscriptionActiveUntilFromTokens(idToken, accessToken string) string {
	for _, token := range []string{idToken, accessToken} {
		if strings.TrimSpace(token) == "" {
			continue
		}
		claims, err := parseJWTClaims(token)
		if err != nil {
			continue
		}
		if value, ok := claims["chatgpt_subscription_active_until"].(string); ok {
			if normalized := NormalizeSubscriptionActiveUntil(value); normalized != "" {
				return normalized
			}
		}
		if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
			if value, ok := auth["chatgpt_subscription_active_until"].(string); ok {
				if normalized := NormalizeSubscriptionActiveUntil(value); normalized != "" {
					return normalized
				}
			}
		}
	}
	return ""
}

func NormalizeSubscriptionActiveUntil(value string) string {
	s := strings.TrimSpace(value)
	if s == "" {
		return ""
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if t, ok := parseUnixLike(n); ok {
			return t.UTC().Format(time.RFC3339)
		}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func parseUnixLike(n float64) (time.Time, bool) {
	if n <= 0 {
		return time.Time{}, false
	}
	if n > 1e12 {
		return time.UnixMilli(int64(n)), true
	}
	if n > 1e9 {
		return time.Unix(int64(n), 0), true
	}
	return time.Time{}, false
}
