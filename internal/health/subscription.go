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
	if len(scopes) == 0 {
		return "", fmt.Errorf("subscription account not found")
	}
	var candidates []subscriptionCandidate
	for _, scope := range scopes {
		candidates = collectSubscriptionCandidates(scope, nil, candidates)
	}
	expiresAt, ok := bestSubscriptionCandidate(candidates)
	if !ok {
		return "", fmt.Errorf("subscription expiry not found")
	}
	return expiresAt.UTC().Format(time.RFC3339), nil
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
			for key, child := range x {
				if valueMatches(key, accountID) {
					scopes = append(scopes, child)
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(root)
	return scopes
}

func valueMatches(v any, want string) bool {
	s, ok := v.(string)
	return ok && strings.EqualFold(s, want)
}

type subscriptionCandidate struct {
	expiresAt time.Time
	priority  int
}

func bestSubscriptionCandidate(candidates []subscriptionCandidate) (time.Time, bool) {
	if len(candidates) == 0 {
		return time.Time{}, false
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.priority < best.priority {
			best = candidate
			continue
		}
		if candidate.priority == best.priority && candidate.expiresAt.After(best.expiresAt) {
			best = candidate
		}
	}
	return best.expiresAt, true
}

func collectSubscriptionCandidates(v any, path []string, candidates []subscriptionCandidate) []subscriptionCandidate {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			if t, priority, ok := parseSubscriptionTime(key, value, path); ok {
				candidates = append(candidates, subscriptionCandidate{expiresAt: t, priority: priority})
			}
		}
		for key, value := range x {
			candidates = collectSubscriptionCandidates(value, append(path, key), candidates)
		}
	case []any:
		for _, value := range x {
			candidates = collectSubscriptionCandidates(value, path, candidates)
		}
	}
	return candidates
}

func parseSubscriptionTime(key string, value any, path []string) (time.Time, int, bool) {
	key = strings.ToLower(key)
	pathText := strings.ToLower(strings.Join(path, "."))
	priority := subscriptionTimePriority(key, pathText)
	if priority == 0 {
		return time.Time{}, 0, false
	}
	t, ok := parseAnyTime(value)
	return t, priority, ok
}

func subscriptionTimePriority(key, pathText string) int {
	switch key {
	case "subscription_expires_at_timestamp", "subscription_expires_at", "subscription_expiry":
		return 1
	case "current_period_end", "billing_period_end":
		return 2
	case "next_billing_at", "next_renewal_at", "renews_at":
		return 3
	case "expires_at":
		if strings.Contains(pathText, "subscription") ||
			strings.Contains(pathText, "account_plan") ||
			strings.Contains(pathText, "entitlement") ||
			strings.Contains(pathText, "billing") {
			return 4
		}
	}
	return 0
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
