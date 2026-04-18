package gateway

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lune/internal/auth"
	"lune/internal/router"
	"lune/internal/store"
	"lune/internal/syscfg"
	"lune/internal/webutil"
)

type Handler struct {
	router *router.Router
	cache  *store.RoutingCache
	store  *store.Store
}

func NewHandler(rt *router.Router, cache *store.RoutingCache, st *store.Store) *Handler {
	return &Handler{router: rt, cache: cache, store: st}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := generateRequestID()
	accessToken := auth.AccessTokenFromContext(r.Context())

	// determine path suffix
	pathSuffix := extractPathSuffix(r.URL.Path)

	// GET /v1/models — handled locally
	if pathSuffix == "models" && r.Method == http.MethodGet {
		h.handleModels(w)
		return
	}

	// read body (limit 10 MB to prevent memory exhaustion)
	const maxBodySize = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			webutil.WriteGatewayError(w, 413, "request_too_large", "request body exceeds 10MB limit")
			return
		}
		webutil.WriteGatewayError(w, 400, "bad_request", "failed to read request body")
		return
	}

	// parse model and stream fields
	model, isStream := parseRequestBody(body)
	if model == "" {
		webutil.WriteGatewayError(w, 400, "bad_request", "missing model field in request body")
		return
	}

	// v3: extract token's pool_id and optional force-account header
	var tokenPoolID *int64
	if accessToken != nil && accessToken.PoolID != nil {
		tokenPoolID = accessToken.PoolID
	}

	var forceAccountID *int64
	if v := r.Header.Get("X-Lune-Account-Id"); v != "" && tokenPoolID == nil {
		// force-account only allowed for Global Tokens (no pool scope)
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			forceAccountID = &id
		}
	}

	// initial route resolution
	resolved, err := h.router.Resolve(model, tokenPoolID, forceAccountID)
	if err != nil {
		if errors.Is(err, router.ErrNoRoute) {
			webutil.WriteGatewayError(w, 404, "no_route", fmt.Sprintf("no route for model: %s", model))
			h.logRequest(requestID, accessToken, model, nil, 0, start, isStream, r, false, err.Error(), Usage{}, "")
			return
		}
		if errors.Is(err, router.ErrPoolDisabled) {
			webutil.WriteGatewayError(w, 503, "pool_disabled", "pool is disabled")
			h.logRequest(requestID, accessToken, model, nil, 0, start, isStream, r, false, err.Error(), Usage{}, "")
			return
		}
		if errors.Is(err, router.ErrNoHealthyAccount) {
			webutil.WriteGatewayError(w, 503, "no_healthy_account", "no healthy account available")
			h.logRequest(requestID, accessToken, model, nil, 0, start, isStream, r, false, err.Error(), Usage{}, "")
			return
		}
		if errors.Is(err, router.ErrModelNotOnAccount) {
			var accID int64
			if forceAccountID != nil {
				accID = *forceAccountID
			}
			webutil.WriteGatewayError(w, 404, "model_not_on_account",
				fmt.Sprintf("account %d does not list model: %s", accID, model))
			h.logRequest(requestID, accessToken, model, nil, 0, start, isStream, r, false, err.Error(), Usage{}, "")
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
	if forceAccountID != nil {
		maxRetries = 1
	}
	var exclude []int64
	var lastErr error
	var lastStatusCode int

	for attempt := 0; attempt < maxRetries; attempt++ {
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

		// resolve upstream target based on source_kind
		target := h.resolveTarget(resolved.Account)

		timeout := h.getRequestTimeout()
		result := Forward(w, r, target, pathSuffix, body, isStream, requestID, timeout)

		if result.Err != nil {
			// network/connection error
			exclude = append(exclude, resolved.AccountID)
			lastErr = result.Err
			h.updateHealth(resolved.AccountID, "error", result.Err.Error())

			if !IsRetryable(result.Err) {
				webutil.WriteGatewayError(w, 502, "upstream_failed", result.Err.Error())
				h.logRequest(requestID, accessToken, model, resolved, 0, start, isStream, r, false, result.Err.Error(), Usage{}, resolved.Account.SourceKind)
				return
			}
			continue
		}

		if IsRetryableStatus(result.StatusCode) && attempt < maxRetries-1 {
			// streaming response already written, can't retry
			if isStream {
				h.logRequest(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, false, "upstream error", result.Usage, resolved.Account.SourceKind)
				return
			}
			exclude = append(exclude, resolved.AccountID)
			lastStatusCode = result.StatusCode
			h.updateHealth(resolved.AccountID, "error", fmt.Sprintf("HTTP %d", result.StatusCode))

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
		if success {
			h.updateHealth(resolved.AccountID, "healthy", "")
			// v3: update token last_used_at (no quota tracking)
			if accessToken != nil {
				go func() {
					_ = h.store.UpdateTokenLastUsed(accessToken.ID)
				}()
			}
		}

		h.logRequest(requestID, accessToken, model, resolved, result.StatusCode, start, isStream, r, success, "", result.Usage, resolved.Account.SourceKind)
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
	h.logRequest(requestID, accessToken, model, nil, lastStatusCode, start, isStream, r, false, errMsg, Usage{}, "")
}

func (h *Handler) handleModels(w http.ResponseWriter) {
	// v3: aggregate models from account_models via cache
	modelNames := h.cache.GetAllModels()
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

func (h *Handler) resolveTarget(account store.Account) UpstreamTarget {
	if account.SourceKind == "cpa" && account.CpaServiceID != nil {
		svc := h.cache.GetCpaService(*account.CpaServiceID)
		if svc != nil {
			return UpstreamTarget{
				BaseURL:   strings.TrimRight(svc.BaseURL, "/") + "/api/provider/" + account.CpaProvider + "/v1",
				APIKey:    svc.APIKey,
				AccountID: account.ID,
			}
		}
	}
	return UpstreamTarget{
		BaseURL:   account.BaseURL,
		APIKey:    account.APIKey,
		AccountID: account.ID,
	}
}

func (h *Handler) logRequest(requestID string, token *store.AccessToken, model string, resolved *router.ResolvedRoute, statusCode int, start time.Time, stream bool, r *http.Request, success bool, errMsg string, usage Usage, sourceKind string) {
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

	log := &store.RequestLog{
		RequestID:       requestID,
		AccessTokenName: tokenName,
		ModelRequested:  model,
		ModelActual:     modelActual,
		PoolID:          poolID,
		AccountID:       accountID,
		StatusCode:      statusCode,
		LatencyMs:       time.Since(start).Milliseconds(),
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		Stream:          stream,
		RequestIP:       clientIP(r),
		Success:         success,
		ErrorMessage:    errMsg,
		SourceKind:      sourceKind,
	}
	go func() {
		if err := h.store.InsertLog(log); err != nil {
			slog.Error("failed to insert request log", "request_id", log.RequestID, "err", err)
		}
	}()
}

func (h *Handler) updateHealth(accountID int64, status, lastError string) {
	go func() {
		_ = h.store.UpdateAccountHealth(accountID, status, lastError)
		h.cache.Invalidate()
	}()
}

func (h *Handler) getMaxRetries() int {
	return syscfg.ParsePositiveInt(h.cache.GetSetting("max_retry_attempts"), syscfg.DefaultMaxRetryAttempts)
}

func (h *Handler) getRequestTimeout() time.Duration {
	return time.Duration(syscfg.ParsePositiveInt(h.cache.GetSetting("request_timeout"), syscfg.DefaultRequestTimeout)) * time.Second
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

func parseRequestBody(body []byte) (model string, stream bool) {
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", false
	}
	return req.Model, req.Stream
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
