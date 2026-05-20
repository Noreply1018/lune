package gateway

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"lune/internal/auth"
	"lune/internal/health"
	"lune/internal/router"
	"lune/internal/store"
	"lune/internal/syscfg"
	"lune/internal/webutil"
)

const servingCooldownDuration = 5 * time.Minute

type Handler struct {
	router        *router.Router
	cache         *store.RoutingCache
	store         *store.Store
	tmpDir        string
	runtimeBinder runtimeBinder
	logWG         sync.WaitGroup
}

type runtimeBinder interface {
	ResolveRuntimeBinding(ctx context.Context, acc store.Account, waitAuthIndex bool, readOnly bool) (*health.RuntimeBinding, error)
}

type providerPinningCapable interface {
	ProviderPinningSupported() bool
}

type runtimeBindingLog struct {
	AuthIndex  string
	AuthID     string
	AccountKey string
	Status     string
	Reason     string
}

type runtimeBindingUnavailableError struct {
	status string
	reason string
}

func (e *runtimeBindingUnavailableError) Error() string {
	if e == nil || e.reason == "" {
		return "runtime_auth_binding_unavailable"
	}
	return e.reason
}

func (e *runtimeBindingUnavailableError) Status() string {
	if e == nil {
		return ""
	}
	return e.status
}

func (e *runtimeBindingUnavailableError) Reason() string {
	if e == nil {
		return ""
	}
	return e.reason
}

func NewHandler(rt *router.Router, cache *store.RoutingCache, st *store.Store, tmpDir string, binders ...runtimeBinder) *Handler {
	var binder runtimeBinder
	if len(binders) > 0 {
		binder = binders[0]
	}
	return &Handler{router: rt, cache: cache, store: st, tmpDir: tmpDir, runtimeBinder: binder}
}

func (h *Handler) waitForLogWrites() {
	h.logWG.Wait()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := generateRequestID()
	accessToken := auth.AccessTokenFromContext(r.Context())
	diagnostic := DiagnosticFromContext(r.Context())
	if !diagnostic && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Lune-Diagnostic")), "true") {
		r = r.WithContext(ContextWithDiagnostic(r.Context()))
		diagnostic = true
	}

	// determine path suffix
	pathSuffix := extractPathSuffix(r.URL.Path)

	var tokenPoolID *int64
	if accessToken != nil && accessToken.PoolID != nil {
		tokenPoolID = accessToken.PoolID
	}

	// GET /v1/models — handled locally
	if pathSuffix == "models" && r.Method == http.MethodGet {
		h.handleModels(w, tokenPoolID)
		return
	}

	maxBodyBytes := int64(h.getGatewayMaxBodyMB()) << 20
	memoryBodyBytes := int64(h.getGatewayMemoryBodyMB()) << 20
	body, err := NewReplayBody(r, maxBodyBytes, memoryBodyBytes, h.tmpDir)
	if err != nil {
		if errors.Is(err, ErrBodyTooLarge) {
			msg := fmt.Sprintf("request body exceeds %dMB limit", h.getGatewayMaxBodyMB())
			webutil.WriteGatewayError(w, 413, "request_too_large", msg)
			h.logRequest(requestID, accessToken, "", nil, 413, start, false, r, false, msg, Usage{}, "gateway", 0)
			return
		}
		webutil.WriteGatewayError(w, 400, "bad_request", "failed to read request body")
		h.logRequest(requestID, accessToken, "", nil, 400, start, false, r, false, "failed to read request body", Usage{}, "gateway", 0)
		return
	}
	defer body.Close()
	slog.Debug("gateway request body prepared", "request_id", requestID, "size_bytes", body.Size(), "storage", body.Storage())

	// parse model and stream fields
	env, err := ParseRequestEnvelope(body)
	if err != nil {
		webutil.WriteGatewayError(w, 400, "bad_request", "malformed JSON request body")
		h.logRequest(requestID, accessToken, "", nil, 400, start, false, r, false, "malformed JSON request body", Usage{}, "gateway", 0)
		return
	}
	model, isStream := env.Model, env.Stream
	if model == "" {
		webutil.WriteGatewayError(w, 400, "bad_request", "missing model field in request body")
		h.logRequest(requestID, accessToken, "", nil, 400, start, isStream, r, false, "missing model field in request body", Usage{}, "gateway", 0)
		return
	}

	var forceAccountID *int64
	if v := r.Header.Get("X-Lune-Account-Id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			forceAccountID = &id
		}
	}
	forceAccount := forceAccountID != nil
	statefulProbe := forceAccount && !diagnostic && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Lune-Probe-Mode")), "stateful")
	if forceAccount {
		r = r.WithContext(ContextWithForceAccount(r.Context()))
	}
	if statefulProbe {
		r = r.WithContext(ContextWithStatefulProbe(r.Context()))
	}
	diagnosticRoute := diagnostic || statefulProbe

	// initial route resolution
	resolved, err := h.router.ResolveWithOptions(model, tokenPoolID, forceAccountID, router.ResolveOptions{Diagnostic: diagnosticRoute})
	if err != nil {
		initialTrace := h.initialRouteTrace(model, tokenPoolID, diagnosticRoute)
		if errors.Is(err, router.ErrNoRoute) {
			webutil.WriteGatewayError(w, 404, "no_route", fmt.Sprintf("no route for model: %s", model))
			h.logRequestWithBindingTrace(requestID, accessToken, model, nil, 404, start, isStream, r, false, err.Error(), Usage{}, "", 0, runtimeBindingLog{}, diagnostic, initialTrace)
			return
		}
		if errors.Is(err, router.ErrPoolDisabled) {
			webutil.WriteGatewayError(w, 503, "pool_disabled", "pool is disabled")
			h.logRequestWithBindingTrace(requestID, accessToken, model, nil, 503, start, isStream, r, false, err.Error(), Usage{}, "", 0, runtimeBindingLog{}, diagnostic, initialTrace)
			return
		}
		if errors.Is(err, router.ErrNoHealthyAccount) {
			webutil.WriteGatewayError(w, 503, "no_healthy_account", "no healthy account available")
			h.logRequestWithBindingTrace(requestID, accessToken, model, nil, 503, start, isStream, r, false, err.Error(), Usage{}, "", 0, runtimeBindingLog{}, diagnostic, initialTrace)
			return
		}
		if errors.Is(err, router.ErrRuntimeBinding) {
			reason := "provider_pinning_unsupported"
			webutil.WriteGatewayError(w, 503, "runtime_auth_binding_unavailable", reason)
			h.logRequestWithBindingTrace(requestID, accessToken, model, nil, 503, start, isStream, r, false, "runtime_auth_binding_unavailable: "+reason, Usage{}, "cpa", 0, runtimeBindingLog{}, diagnostic, initialTrace)
			return
		}
		if errors.Is(err, router.ErrModelNotOnAccount) {
			var accID int64
			if forceAccountID != nil {
				accID = *forceAccountID
			}
			webutil.WriteGatewayError(w, 404, "model_not_on_account",
				fmt.Sprintf("account %d does not list model: %s", accID, model))
			h.logRequest(requestID, accessToken, model, nil, 404, start, isStream, r, false, err.Error(), Usage{}, "", 0)
			return
		}
		webutil.WriteGatewayError(w, 500, "internal", err.Error())
		return
	}

	// retry loop with exponential backoff.
	// When the client forces a specific account (X-Lune-Account-Id), retries
	// would fall back to SelectNextAccount which ignores forceAccountID and
	// could route the retry to a different account, masking real failures
	// under a false "success" from an unrelated account. Single-shot in that
	// case so MiniChat / per-account probes reflect the actual target.
	maxRetries := h.getMaxRetries()
	if forceAccountID != nil || diagnostic {
		maxRetries = 1
	}
	var exclude []int64
	var lastErr error
	var lastStatusCode int
	// Remember the last account we actually forwarded to so that the
	// "all retries exhausted" log below still carries pool/account context
	// instead of an anonymous failure.
	var lastResolved *router.ResolvedRoute
	var lastBindingLog runtimeBindingLog
	var routeTrace []map[string]any
	attemptsUsed := 0

	for attempt := 0; attempt < maxRetries; attempt++ {
		attemptsUsed = attempt + 1
		// Exponential backoff for retries
		if attempt > 0 {
			base := time.Duration(1<<(attempt-1)) * 200 * time.Millisecond
			jitter := time.Duration(rand.N(int64(base / 2)))
			time.Sleep(base + jitter)

			// Re-resolve with exclude list
			resolved, err = h.router.SelectNextAccount(model, tokenPoolID, exclude)
			if err != nil {
				if errors.Is(err, router.ErrNoHealthyAccount) {
					break // exhausted all accounts
				}
				break
			}
		}
		lastResolved = resolved
		candidates := h.router.ExplainCandidates(model, resolved.PoolID, exclude, router.ResolveOptions{Diagnostic: diagnosticRoute})
		routeTrace = append(routeTrace, map[string]any{
			"attempt":             attempt + 1,
			"routing_policy":      currentRoutingPolicy(h.cache, resolved.PoolID),
			"selected_account_id": resolved.AccountID,
			"force_account":       forceAccount,
			"stateful_probe":      statefulProbe,
			"diagnostic":          diagnostic,
			"candidates":          candidates,
		})

		bindingLog := runtimeBindingLog{}
		targetBinding, bindErr := h.resolveForwardRuntimeBinding(r.Context(), resolved.Account)
		if bindErr != nil {
			status, reason := health.RuntimeBindingErrorState(bindErr)
			if reason == "" {
				reason = "runtime_auth_binding_unavailable"
			}
			bindingLog = runtimeBindingLog{
				AuthIndex:  "",
				AuthID:     "",
				AccountKey: resolved.Account.CpaAccountKey,
				Status:     status,
				Reason:     reason,
			}
			if targetBinding != nil {
				bindingLog.AuthIndex = targetBinding.AuthIndex
				bindingLog.AuthID = targetBinding.AuthID
				bindingLog.AccountKey = targetBinding.AccountKey
			}
			slog.Warn("gateway CPA runtime binding unavailable",
				"request_id", requestID,
				"account_id", resolved.AccountID,
				"cpa_account_key", resolved.Account.CpaAccountKey,
				"status", status,
				"reason", reason,
				"err", bindErr,
			)
			webutil.WriteGatewayError(w, 503, "runtime_auth_binding_unavailable", reason)
			h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, 503, start, isStream, r, false, "runtime_auth_binding_unavailable: "+reason, Usage{}, resolved.Account.SourceKind, 0, bindingLog, diagnostic, routeTrace)
			return
		}
		if targetBinding != nil {
			bindingLog = runtimeBindingLog{
				AuthIndex:  targetBinding.AuthIndex,
				AuthID:     targetBinding.AuthID,
				AccountKey: targetBinding.AccountKey,
				Status:     "confirmed",
			}
		}
		lastBindingLog = bindingLog

		// resolve upstream target based on source_kind
		target := h.resolveTarget(resolved.Account, targetBinding)

		timeout := h.getRequestTimeout()
		result := Forward(w, r, target, pathSuffix, body, isStream, requestID, timeout)

		if result.Err != nil {
			// network/connection error
			exclude = append(exclude, resolved.AccountID)
			lastErr = result.Err
			if result.HealthImpact && !diagnostic && !statefulProbe {
				h.recordServingFailure(resolved.AccountID, result.Err.Error())
			}
			if resolved.Account.SourceKind == "cpa" && (!diagnostic || statefulProbe) {
				h.recordAccountDiagnosticObservation(resolved.Account, "transient_error", "transient_error", "gateway request failed", 0, "upstream_request_failed", result.Err.Error())
			}

			if !IsRetryable(result.Err) {
				webutil.WriteGatewayError(w, 502, "upstream_failed", result.Err.Error())
				h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, 0, start, isStream, r, false, result.Err.Error(), Usage{}, resolved.Account.SourceKind, attemptsUsed, bindingLog, diagnostic, routeTrace)
				return
			}
			continue
		}

		if IsRetryableStatus(result.StatusCode) {
			errMsg := upstreamErrorMessage(result, fmt.Sprintf("HTTP %d", result.StatusCode))
			if resolved.Account.SourceKind == "cpa" && isCpaAccountBannedSignal(result.StatusCode, result.Body, errMsg) {
				if !diagnostic || statefulProbe {
					h.recordAccountDiagnosticObservation(resolved.Account, "banned", "upstream_banned_signal", "account banned by upstream", result.StatusCode, "upstream_banned", errMsg)
				}
				result.WriteResponse(w)
				h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, false, errMsg, result.Usage, resolved.Account.SourceKind, attemptsUsed, bindingLog, diagnostic, routeTrace)
				return
			}
			if resolved.Account.SourceKind == "cpa" && isCpaAccountUpstreamAuthFailure(result.StatusCode, result.Body) {
				if !diagnostic || statefulProbe {
					msg := fmt.Sprintf("HTTP %d", result.StatusCode)
					h.updateCpaCredential(resolved.AccountID, "needs_login", gatewayCredentialReason(result.Body), msg)
					h.recordAccountDiagnosticObservation(resolved.Account, "auth_invalid", "auth_invalid_signal", "account authentication failed", result.StatusCode, "auth_invalid", errMsg)
				}
				result.WriteResponse(w)
				h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, false, errMsg, result.Usage, resolved.Account.SourceKind, attemptsUsed, bindingLog, diagnostic, routeTrace)
				return
			}
			if result.StatusCode == http.StatusTooManyRequests && (!diagnostic || statefulProbe) {
				h.recordCodexQuotaRateLimitEvidence(resolved.Account, result.Body, errMsg)
				if bodyHasQuotaLimitSignal(result.Body) || textHasQuotaLimitSignal(errMsg) {
					h.recordAccountDiagnosticObservation(resolved.Account, "quota_exhausted", "quota_exhausted_signal", "quota exhausted by model request", result.StatusCode, "quota_exhausted", errMsg)
				} else {
					h.recordAccountDiagnosticObservation(resolved.Account, "unknown", "transient_error", "rate limited by model request", result.StatusCode, "rate_limited", errMsg)
				}
			} else if resolved.Account.SourceKind == "cpa" && (!diagnostic || statefulProbe) {
				h.recordAccountDiagnosticObservation(resolved.Account, "unknown", "transient_error", "transient upstream error", result.StatusCode, "upstream_transient_error", errMsg)
			}
			if result.HealthImpact && !diagnostic && !statefulProbe {
				h.recordServingFailure(resolved.AccountID, errMsg)
			}
			lastStatusCode = result.StatusCode

			if isStream {
				result.WriteResponse(w)
				h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, false, errMsg, result.Usage, resolved.Account.SourceKind, attemptsUsed, bindingLog, diagnostic, routeTrace)
				return
			}
			if attempt >= maxRetries-1 {
				result.WriteResponse(w)
				h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, false, errMsg, result.Usage, resolved.Account.SourceKind, attemptsUsed, bindingLog, diagnostic, routeTrace)
				return
			}
			exclude = append(exclude, resolved.AccountID)

			// Handle 429 with Retry-After
			if result.StatusCode == 429 {
				if retryAfter := result.Headers.Get("Retry-After"); retryAfter != "" {
					if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 && secs <= 30 {
						time.Sleep(time.Duration(secs) * time.Second)
					}
				}
			}
			continue
		}

		// success or non-retryable response — flush to client
		result.WriteResponse(w)
		success := result.StatusCode >= 200 && result.StatusCode < 400
		errMsg := ""
		if result.Stream != nil && result.Stream.Failed {
			success = false
			errMsg = result.Stream.ErrorMessage
			if errMsg == "" && result.Stream.Err != nil {
				errMsg = result.Stream.Err.Error()
			}
			if errMsg == "" {
				errMsg = "stream failed"
			}
		} else if !success {
			errMsg = upstreamErrorMessage(result, "")
		}
		if success {
			if !diagnostic && !statefulProbe {
				h.recordServingSuccess(resolved.AccountID)
				if resolved.Account.SourceKind == "cpa" && strings.EqualFold(resolved.Account.CpaCredentialStatus, "auth_suspect") {
					h.updateCpaCredential(resolved.AccountID, "ok", "", "")
				}
				// v3: update token last_used_at (no quota tracking)
				if accessToken != nil {
					go func() {
						_ = h.store.UpdateTokenLastUsed(accessToken.ID)
					}()
				}
			}
			if resolved.Account.SourceKind == "cpa" && strings.EqualFold(resolved.Account.CpaProvider, "codex") && (!diagnostic || statefulProbe) {
				if strings.EqualFold(resolved.Account.CpaQuotaStatus, "error") && isQuotaProbeAuthFailureText(resolved.Account.CpaQuotaLastError) {
					// Preserve quota/auth-failure evidence; only enrich diagnostic state.
				} else {
					_ = h.store.ClearAccountCodexModelRequestQuotaEvidence(resolved.AccountID)
				}
				_ = h.store.UpdateAccountCpaAccessStatus(resolved.AccountID, "eligible", "model_request_success", "", time.Now().UTC().Format(time.RFC3339))
				if strings.EqualFold(resolved.Account.CpaQuotaStatus, "error") && isQuotaProbeAuthFailureText(resolved.Account.CpaQuotaLastError) {
					h.recordAccountDiagnosticObservation(resolved.Account, "quota_probe_auth_failed_but_usable", "succeeded", "quota probe authentication failed but model request succeeded", result.StatusCode, "quota_probe_auth_failed_but_usable", "")
				} else {
					h.recordAccountDiagnosticObservation(resolved.Account, "usable", "succeeded", "model request succeeded", result.StatusCode, "", "")
				}
				h.cache.Invalidate()
			}
		} else if resolved.Account.SourceKind == "cpa" && isCpaAccountBannedSignal(result.StatusCode, result.Body, errMsg) {
			if !diagnostic || statefulProbe {
				h.recordAccountDiagnosticObservation(resolved.Account, "banned", "upstream_banned_signal", "account banned by upstream", result.StatusCode, "upstream_banned", errMsg)
			}
		} else if resolved.Account.SourceKind == "cpa" && isCpaAccountUpstreamAuthFailure(result.StatusCode, result.Body) {
			if !diagnostic || statefulProbe {
				msg := fmt.Sprintf("HTTP %d", result.StatusCode)
				h.updateCpaCredential(resolved.AccountID, "needs_login", gatewayCredentialReason(result.Body), msg)
				h.recordAccountDiagnosticObservation(resolved.Account, "auth_invalid", "auth_invalid_signal", "account authentication failed", result.StatusCode, "auth_invalid", errMsg)
			}
		} else if resolved.Account.SourceKind == "cpa" && (!diagnostic || statefulProbe) {
			h.recordAccountDiagnosticObservation(resolved.Account, "unknown", "transient_error", "upstream request failed", result.StatusCode, "upstream_request_failed", errMsg)
		} else if resolved.Account.SourceKind != "cpa" && isGatewayAuthFailure(result.StatusCode, result.Body) {
			if !diagnostic && !statefulProbe {
				h.updateHealth(resolved.AccountID, "error", "upstream authentication failed")
			}
		} else if result.Stream != nil && result.Stream.Failed {
			if !diagnostic && !statefulProbe && result.Stream.HealthImpact {
				h.recordServingFailure(resolved.AccountID, errMsg)
			}
			if resolved.Account.SourceKind == "cpa" && (!diagnostic || statefulProbe) {
				h.recordAccountDiagnosticObservation(resolved.Account, "unknown", "transient_error", "stream failed", result.StatusCode, "stream_failed", errMsg)
			}
		}

		h.logRequestWithBindingTrace(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, success, errMsg, result.Usage, resolved.Account.SourceKind, attemptsUsed, bindingLog, diagnostic, routeTrace)
		return
	}

	// all retries exhausted
	errMsg := "all upstream attempts failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	} else if lastStatusCode > 0 {
		errMsg = fmt.Sprintf("upstream returned HTTP %d", lastStatusCode)
	}
	webutil.WriteGatewayError(w, 502, "upstream_failed", errMsg)
	lastSourceKind := ""
	if lastResolved != nil {
		lastSourceKind = lastResolved.Account.SourceKind
	}
	h.logRequestWithBindingTrace(requestID, accessToken, model, lastResolved, lastStatusCode, start, isStream, r, false, errMsg, Usage{}, lastSourceKind, attemptsUsed, lastBindingLog, diagnostic, routeTrace)
}

func (h *Handler) initialRouteTrace(model string, tokenPoolID *int64, diagnostic bool) []map[string]any {
	if tokenPoolID == nil {
		return nil
	}
	candidates := h.router.ExplainCandidates(model, *tokenPoolID, nil, router.ResolveOptions{Diagnostic: diagnostic})
	if len(candidates) == 0 {
		return nil
	}
	return []map[string]any{{
		"attempt":             0,
		"routing_policy":      currentRoutingPolicy(h.cache, *tokenPoolID),
		"selected_account_id": nil,
		"candidates":          candidates,
	}}
}

func (h *Handler) handleModels(w http.ResponseWriter, tokenPoolID *int64) {
	modelNames := []string{}
	if tokenPoolID != nil {
		if models, err := h.store.GetPoolModels(*tokenPoolID); err == nil {
			modelNames = models
		}
	}
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	models := make([]model, 0, len(modelNames))
	for _, name := range modelNames {
		models = append(models, model{
			ID:      name,
			Object:  "model",
			OwnedBy: "lune",
		})
	}
	webutil.WriteJSON(w, 200, map[string]any{
		"object": "list",
		"data":   models,
	})
}

func (h *Handler) resolveForwardRuntimeBinding(ctx context.Context, account store.Account) (*RuntimeBinding, error) {
	if account.SourceKind != "cpa" {
		return nil, nil
	}
	if h.runtimeBinder == nil {
		return nil, fmt.Errorf("runtime_auth_binding_unavailable")
	}
	binding, err := h.runtimeBinder.ResolveRuntimeBinding(ctx, account, true, true)
	if err != nil {
		return nil, err
	}
	if binding == nil || strings.TrimSpace(binding.AuthIndex) == "" {
		return nil, fmt.Errorf("runtime_auth_binding_unavailable")
	}
	targetBinding := &RuntimeBinding{
		AccountKey: binding.AccountKey,
		AuthID:     binding.AuthID,
		AuthIndex:  binding.AuthIndex,
		OpenAIID:   binding.OpenAIID,
	}
	if capable, ok := h.runtimeBinder.(providerPinningCapable); ok && capable.ProviderPinningSupported() {
		return targetBinding, nil
	}
	return targetBinding, &runtimeBindingUnavailableError{status: "unsupported", reason: "provider_pinning_unsupported"}
}

func (h *Handler) resolveTarget(account store.Account, binding *RuntimeBinding) UpstreamTarget {
	if account.SourceKind == "cpa" && account.CpaServiceID != nil {
		svc := h.cache.GetCpaService(*account.CpaServiceID)
		if svc != nil {
			return UpstreamTarget{
				BaseURL:        strings.TrimRight(svc.BaseURL, "/") + "/api/provider/" + account.CpaProvider + "/v1",
				APIKey:         svc.APIKey,
				AccountID:      account.ID,
				RuntimeBinding: binding,
			}
		}
	}
	return UpstreamTarget{
		BaseURL:   account.BaseURL,
		APIKey:    account.APIKey,
		AccountID: account.ID,
	}
}

func (h *Handler) logRequest(requestID string, token *store.AccessToken, model string, resolved *router.ResolvedRoute, statusCode int, start time.Time, stream bool, r *http.Request, success bool, errMsg string, usage Usage, sourceKind string, attemptCount int) {
	diagnostic := false
	if r != nil {
		diagnostic = DiagnosticFromContext(r.Context())
	}
	h.logRequestWithBinding(requestID, token, model, resolved, statusCode, start, stream, r, success, errMsg, usage, sourceKind, attemptCount, runtimeBindingLog{}, diagnostic)
}

func (h *Handler) logRequestWithBinding(requestID string, token *store.AccessToken, model string, resolved *router.ResolvedRoute, statusCode int, start time.Time, stream bool, r *http.Request, success bool, errMsg string, usage Usage, sourceKind string, attemptCount int, binding runtimeBindingLog, diagnostic bool) {
	h.logRequestWithBindingTrace(requestID, token, model, resolved, statusCode, start, stream, r, success, errMsg, usage, sourceKind, attemptCount, binding, diagnostic, nil)
}

func (h *Handler) logRequestWithBindingTrace(requestID string, token *store.AccessToken, model string, resolved *router.ResolvedRoute, statusCode int, start time.Time, stream bool, r *http.Request, success bool, errMsg string, usage Usage, sourceKind string, attemptCount int, binding runtimeBindingLog, diagnostic bool, routeTrace []map[string]any) {
	tokenName := ""
	if token != nil {
		tokenName = token.Name
	}
	modelActual := model
	var poolID, accountID int64
	if resolved != nil {
		modelActual = resolved.TargetModel
		poolID = resolved.PoolID
		accountID = resolved.AccountID
	}
	// attemptCount == 0 对应"还没跑进重试循环就失败"的路由拒绝场景。
	// >=1 才是真正发过上游请求的次数。store 层只 clamp 负值。
	if attemptCount < 0 {
		attemptCount = 0
	}
	trace := ""
	if len(routeTrace) > 0 {
		if data, err := json.Marshal(routeTrace); err == nil {
			trace = string(data)
		}
	}
	forceAccount := false
	statefulProbe := false
	if r != nil {
		forceAccount = ForceAccountFromContext(r.Context())
		statefulProbe = StatefulProbeFromContext(r.Context())
	}
	trafficKind := "ordinary"
	if diagnostic {
		trafficKind = "diagnostic"
	} else if statefulProbe {
		trafficKind = "stateful_probe"
	}

	log := &store.RequestLog{
		RequestID:            requestID,
		AccessTokenName:      tokenName,
		ModelRequested:       model,
		ModelActual:          modelActual,
		PoolID:               poolID,
		AccountID:            accountID,
		StatusCode:           statusCode,
		LatencyMs:            time.Since(start).Milliseconds(),
		InputTokens:          usage.InputTokens,
		OutputTokens:         usage.OutputTokens,
		Stream:               stream,
		RequestIP:            clientIP(r),
		Success:              success,
		ErrorMessage:         errMsg,
		SourceKind:           sourceKind,
		AttemptCount:         attemptCount,
		Diagnostic:           diagnostic,
		ForceAccount:         forceAccount,
		StatefulProbe:        statefulProbe,
		TrafficKind:          trafficKind,
		RuntimeAuthIndex:     binding.AuthIndex,
		RuntimeAuthID:        binding.AuthID,
		RuntimeAccountKey:    binding.AccountKey,
		RuntimeBindingStatus: binding.Status,
		RuntimeBindingReason: binding.Reason,
		RouteTrace:           trace,
	}
	h.logWG.Add(1)
	go func() {
		defer h.logWG.Done()
		if err := h.store.InsertLog(log); err != nil {
			slog.Error("failed to insert request log", "request_id", log.RequestID, "err", err)
		}
	}()
}

func currentRoutingPolicy(cache *store.RoutingCache, poolID int64) string {
	if cache == nil {
		return "health_first"
	}
	pool := cache.GetPool(poolID)
	if pool == nil {
		return "health_first"
	}
	return store.NormalizeRoutingPolicy(pool.RoutingPolicy)
}

func (h *Handler) updateHealth(accountID int64, status, lastError string) {
	go func() {
		_ = h.store.UpdateAccountHealth(accountID, status, lastError)
		h.cache.Invalidate()
	}()
}

func (h *Handler) recordServingSuccess(accountID int64) {
	_ = h.store.MarkAccountServingSuccess(accountID)
	h.cache.Invalidate()
}

func (h *Handler) recordServingFailure(accountID int64, lastError string) {
	if lastError == "" {
		lastError = "upstream request failed"
	}
	until := time.Now().UTC().Add(servingCooldownDuration)
	_ = h.store.MarkAccountServingFailure(accountID, lastError, until)
	h.cache.Invalidate()
}

func (h *Handler) recordCodexQuotaRateLimitEvidence(account store.Account, body []byte, errMsg string) {
	if account.SourceKind != "cpa" || !strings.EqualFold(account.CpaProvider, "codex") {
		return
	}
	status := "error"
	if bodyHasQuotaLimitSignal(body) || textHasQuotaLimitSignal(errMsg) {
		status = "blocked"
	}
	msg := "HTTP 429 from model request"
	if strings.TrimSpace(errMsg) != "" && !strings.EqualFold(strings.TrimSpace(errMsg), "HTTP 429") {
		msg = "HTTP 429 from model request: " + truncateStreamError(errMsg, 240)
	}
	_ = h.store.UpdateAccountCodexQuotaStatus(account.ID, status, msg, time.Now().UTC().Format(time.RFC3339))
	h.cache.Invalidate()
}

func (h *Handler) recordAccountDiagnosticObservation(account store.Account, stableStatus, probeStatus, safeSummary string, httpStatus int, normalizedCode, upstreamMessage string) {
	if account.ID <= 0 {
		return
	}
	current, err := h.store.GetAccountDiagnostic(account.ID)
	if err != nil {
		slog.Warn("load account diagnostic for observation", "account_id", account.ID, "err", err)
		return
	}
	previousStable := "unknown"
	if current != nil && strings.TrimSpace(current.StableDiagnosticStatus) != "" {
		previousStable = current.StableDiagnosticStatus
	}
	nextStable := strings.ToLower(strings.TrimSpace(stableStatus))
	preserveStable := strings.EqualFold(probeStatus, "transient_error")
	if nextStable == "" {
		nextStable = previousStable
	}
	if preserveStable {
		nextStable = previousStable
		if nextStable == "" {
			nextStable = "unknown"
		}
	}
	if err := store.ValidateDiagnosticStatus(nextStable); err != nil {
		nextStable = "unknown"
	}
	status := httpStatus
	var statusPtr *int
	if status > 0 {
		statusPtr = &status
	}
	if strings.TrimSpace(safeSummary) == "" {
		safeSummary = "routing observation recorded"
	}
	operationID := store.AccountKeyHash(fmt.Sprintf("account-diagnostic:%d:%s", account.ID, time.Now().UTC().Format(time.RFC3339Nano)))
	err = h.store.UpdateAccountDiagnosticWithEvidence(account.ID, store.AccountDiagnosticUpdate{
		OperationID:                    operationID,
		StableDiagnosticStatus:         nextStable,
		PreviousStableDiagnosticStatus: previousStable,
		LastProbeStatus:                firstNonEmptyString(probeStatus, "succeeded"),
		SafeSummary:                    safeSummary,
		PreserveStableStatus:           preserveStable,
	}, store.AccountDiagnosticEvidenceInput{
		ProbeType:           "routing_observation",
		Stage:               "model_request",
		HTTPStatus:          statusPtr,
		UpstreamErrorCode:   normalizedCode,
		NormalizedErrorCode: normalizedCode,
		SafeMessage:         safeDiagnosticObservationMessage(normalizedCode, safeSummary),
		ObservedAt:          time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		slog.Warn("record account diagnostic observation", "account_id", account.ID, "err", err)
		return
	}
	h.cache.Invalidate()
}

func safeDiagnosticObservationMessage(normalizedCode, fallback string) string {
	switch strings.TrimSpace(normalizedCode) {
	case "quota_exhausted":
		return "quota exhausted by model request"
	case "quota_probe_auth_failed_but_usable":
		return "quota probe authentication failed but account remains usable"
	case "rate_limited":
		return "rate limited by model request"
	case "auth_invalid":
		return "account authentication failed"
	case "upstream_banned":
		return "account banned by upstream"
	case "upstream_request_failed":
		return "upstream request failed"
	case "upstream_transient_error":
		return "transient upstream error"
	case "stream_failed":
		return "stream failed"
	}
	if strings.TrimSpace(fallback) == "" {
		return "routing observation recorded"
	}
	return truncateStreamError(fallback, 120)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (h *Handler) updateCpaCredential(accountID int64, status, reason, lastError string) {
	go func() {
		_ = h.store.UpdateAccountCpaCredentialStatus(accountID, status, reason, lastError, time.Now().UTC().Format(time.RFC3339))
		h.cache.Invalidate()
	}()
}

func isGatewayAuthFailure(statusCode int, body []byte) bool {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return true
	}
	text := strings.ToLower(string(body))
	return strings.Contains(text, "invalid_token") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "access denied") ||
		strings.Contains(text, "refresh token") ||
		strings.Contains(text, "expired token")
}

func isCpaAccountUpstreamAuthFailure(statusCode int, body []byte) bool {
	text := strings.ToLower(string(body))
	if strings.Contains(text, "service key") ||
		strings.Contains(text, "api key") ||
		strings.Contains(text, "management key") ||
		strings.Contains(text, "invalid authorization") {
		return false
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return strings.Contains(text, "refresh token") ||
			strings.Contains(text, "invalid_grant") ||
			strings.Contains(text, "token_invalidated") ||
			strings.Contains(text, "upstream credential") ||
			strings.Contains(text, "upstream authentication") ||
			strings.Contains(text, "chatgpt") ||
			strings.Contains(text, "codex credential") ||
			strings.Contains(text, "auth_unavailable") ||
			strings.Contains(text, "no auth available") ||
			strings.Contains(text, "credential unavailable")
	}
	if statusCode >= 500 {
		return strings.Contains(text, "auth_unavailable") ||
			strings.Contains(text, "no auth available") ||
			strings.Contains(text, "credential unavailable") ||
			strings.Contains(text, "upstream credential") ||
			strings.Contains(text, "upstream authentication") ||
			strings.Contains(text, "codex credential")
	}
	return false
}

func isCpaAccountBannedSignal(statusCode int, body []byte, fallback string) bool {
	if statusCode < 400 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(firstNonEmptyString(extractUpstreamErrorMessage(body), string(body), fallback)))
	if text == "" || textHasQuotaLimitSignal(text) {
		return false
	}
	if strings.Contains(text, "policy violation") {
		return false
	}
	for _, phrase := range []string{
		"account banned",
		"account is banned",
		"account deactivated",
		"account disabled",
		"account is disabled",
		"account suspended",
		"abuse lock",
		"policy lock",
		"account locked",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	if strings.Contains(text, "access to this account has been disabled") {
		return true
	}
	return false
}

func bodyHasQuotaLimitSignal(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if msg := extractUpstreamErrorMessage(body); msg != "" && textHasQuotaLimitSignal(msg) {
		return true
	}
	return textHasQuotaLimitSignal(string(body))
}

func isQuotaProbeAuthFailureText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(text, "http 401") ||
		strings.Contains(text, "http 403") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "forbidden")
}

func textHasQuotaLimitSignal(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "quota") ||
		strings.Contains(text, "rate limit") ||
		strings.Contains(text, "ratelimit") ||
		strings.Contains(text, "usage limit") ||
		strings.Contains(text, "limit reached") ||
		strings.Contains(text, "too many requests")
}

func gatewayCredentialReason(body []byte) string {
	if strings.Contains(strings.ToLower(string(body)), "refresh") {
		return "refresh_failed"
	}
	return "auth_failed"
}

func upstreamErrorMessage(result *ProxyResult, fallback string) string {
	if result == nil {
		return fallback
	}
	if result.Stream != nil && result.Stream.ErrorMessage != "" {
		return result.Stream.ErrorMessage
	}
	if msg := extractUpstreamErrorMessage(result.Body); msg != "" {
		return msg
	}
	return fallback
}

func (h *Handler) getMaxRetries() int {
	return syscfg.ParsePositiveInt(h.cache.GetSetting("max_retry_attempts"), syscfg.DefaultMaxRetryAttempts)
}

func (h *Handler) getRequestTimeout() time.Duration {
	return time.Duration(syscfg.ParsePositiveInt(h.cache.GetSetting("request_timeout"), syscfg.DefaultRequestTimeout)) * time.Second
}

func (h *Handler) getGatewayMaxBodyMB() int {
	return syscfg.ParsePositiveInt(h.cache.GetSetting("gateway_max_body_mb"), syscfg.DefaultGatewayMaxBodyMB)
}

func (h *Handler) getGatewayMemoryBodyMB() int {
	maxMB := h.getGatewayMaxBodyMB()
	memoryMB := syscfg.ParsePositiveInt(h.cache.GetSetting("gateway_memory_body_mb"), syscfg.DefaultGatewayMemoryBodyMB)
	if memoryMB > maxMB {
		return maxMB
	}
	return memoryMB
}

func extractPathSuffix(path string) string {
	if strings.HasPrefix(path, "/openai/v1/") {
		return strings.TrimPrefix(path, "/openai/v1/")
	}
	if strings.HasPrefix(path, "/v1/") {
		return strings.TrimPrefix(path, "/v1/")
	}
	return strings.TrimPrefix(path, "/")
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}
