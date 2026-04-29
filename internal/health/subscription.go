package health

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseCodexSubscriptionExpiry(body []byte, accountID string) (string, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("invalid subscription JSON: %w", err)
	}

	scopes := subscriptionScopes(root, accountID)
	for _, scope := range scopes {
		if expiresAt, ok := findSubscriptionExpiry(scope, nil); ok {
			return expiresAt.UTC().Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("subscription expiry not found")
}

func subscriptionScopes(root any, accountID string) []any {
	if accountID == "" {
		return []any{root}
	}
	var scopes []any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if valueMatches(x["account_id"], accountID) || valueMatches(x["id"], accountID) || valueMatches(x["chatgpt_account_id"], accountID) {
				scopes = append(scopes, x)
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(root)
	if len(scopes) == 0 {
		return []any{root}
	}
	return scopes
}

func valueMatches(v any, want string) bool {
	s, ok := v.(string)
	return ok && s == want
}

func findSubscriptionExpiry(v any, path []string) (time.Time, bool) {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			if t, ok := parseSubscriptionTime(key, value, path); ok {
				return t, true
			}
		}
		for key, value := range x {
			if t, ok := findSubscriptionExpiry(value, append(path, key)); ok {
				return t, true
			}
		}
	case []any:
		for _, value := range x {
			if t, ok := findSubscriptionExpiry(value, path); ok {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func parseSubscriptionTime(key string, value any, path []string) (time.Time, bool) {
	key = strings.ToLower(key)
	pathText := strings.ToLower(strings.Join(path, "."))
	explicit := key == "subscription_expires_at_timestamp" ||
		key == "subscription_expires_at" ||
		key == "subscription_expiry" ||
		key == "current_period_end" ||
		key == "billing_period_end" ||
		key == "next_billing_at" ||
		key == "next_renewal_at" ||
		key == "renews_at"
	contextual := key == "expires_at" && (strings.Contains(pathText, "subscription") ||
		strings.Contains(pathText, "account_plan") ||
		strings.Contains(pathText, "entitlement") ||
		strings.Contains(pathText, "billing"))
	if !explicit && !contextual {
		return time.Time{}, false
	}
	return parseAnyTime(value)
}

func parseAnyTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case float64:
		return parseUnixLike(x)
	case json.Number:
		n, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return time.Time{}, false
		}
		return parseUnixLike(n)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return time.Time{}, false
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return parseUnixLike(n)
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
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
