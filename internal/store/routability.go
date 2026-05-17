package store

import (
	"strings"
	"time"
)

type RoutabilityOptions struct {
	Diagnostic                 bool
	CpaRuntimeBindingSupported bool
	Now                        time.Time
}

type RoutabilityDecision struct {
	Routable              bool
	Reason                string
	QuotaWarn             bool
	RuntimeBindingBlocked bool
	Penalty               int
}

func EvaluateAccountRoutability(acc *Account, opts RoutabilityOptions) RoutabilityDecision {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if acc == nil {
		return blocked("missing_account")
	}
	if !acc.Enabled {
		return blocked("account_disabled")
	}
	if acc.Status != "healthy" && acc.Status != "degraded" {
		return blocked("account_unhealthy")
	}
	if !opts.Diagnostic && strings.EqualFold(acc.ServingStatus, "cooldown") {
		if cooldownUntil, ok := parseRouteTime(acc.CooldownUntil); !ok || cooldownUntil.After(now) {
			return blocked("serving_cooldown")
		}
	}
	if !opts.Diagnostic && strings.EqualFold(acc.ServingStatus, "error") {
		return blocked("serving_error")
	}
	if acc.SourceKind != "cpa" {
		return RoutabilityDecision{Routable: true, Reason: "routable"}
	}

	decision := evaluateCpaRoutability(acc, opts)
	if !decision.Routable {
		return decision
	}
	if !opts.CpaRuntimeBindingSupported {
		decision.Routable = false
		decision.Reason = "runtime_auth_binding_unavailable"
		decision.RuntimeBindingBlocked = true
	}
	return decision
}

func evaluateCpaRoutability(acc *Account, opts RoutabilityOptions) RoutabilityDecision {
	switch strings.ToLower(acc.CpaCredentialStatus) {
	case "needs_login", "refresh_failed", "runtime_pending", "runtime_error", "unknown", "":
		return blocked("credential_" + firstNonEmpty(strings.ToLower(acc.CpaCredentialStatus), "unknown"))
	}

	decision := RoutabilityDecision{Routable: true, Reason: "routable"}
	if strings.EqualFold(acc.CpaCredentialStatus, "auth_suspect") {
		decision.Penalty++
	}
	if !opts.Diagnostic && strings.EqualFold(acc.CpaQuotaStatus, "blocked") {
		return blocked("quota_blocked")
	}
	if !opts.Diagnostic &&
		strings.EqualFold(acc.CpaProvider, "codex") &&
		strings.EqualFold(acc.CpaQuotaStatus, "error") &&
		strings.HasPrefix(acc.CpaQuotaLastError, "HTTP 429 from model request") {
		return blocked("model_request_429")
	}
	if strings.EqualFold(acc.CpaQuotaStatus, "error") || strings.EqualFold(acc.CpaQuotaStatus, "unknown") || strings.EqualFold(acc.CpaQuotaStatus, "pending") {
		decision.QuotaWarn = true
		decision.Penalty++
	}
	if strings.EqualFold(acc.CpaProvider, "codex") && !codexAccessEligible(acc) {
		return blocked("access_" + firstNonEmpty(strings.ToLower(acc.CpaAccessStatus), "unknown"))
	}
	return decision
}

func codexAccessEligible(acc *Account) bool {
	return strings.EqualFold(acc.CpaAccessStatus, "eligible")
}

func blocked(reason string) RoutabilityDecision {
	return RoutabilityDecision{Reason: reason}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseRouteTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
