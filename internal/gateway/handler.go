package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lune/internal/auth"
	"lune/internal/router"
	"lune/internal/store"
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

	// read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		webutil.WriteGatewayError(w, 400, "bad_request", "failed to read request body")
		return
	}

	// parse model and stream fields
	modelAlias, isStream := parseRequestBody(body)
	if modelAlias == "" {
		webutil.WriteGatewayError(w, 400, "bad_request", "missing model field in request body")
		return
	}

	// resolve route
	resolved, err := h.router.Resolve(modelAlias)
	if err != nil {
		if errors.Is(err, router.ErrNoRoute) {
			webutil.WriteGatewayError(w, 404, "no_route", fmt.Sprintf("no route for model: %s", modelAlias))
			return
		}
		webutil.WriteGatewayError(w, 500, "internal", err.Error())
		return
	}

	// rewrite model in body if needed
	requestBody := body
	if resolved.TargetModel != modelAlias {
		requestBody = rewriteModel(body, resolved.TargetModel)
	}

	// retry loop
	maxRetries := h.getMaxRetries()
	var exclude []int64
	var lastErr error
	var lastStatusCode int

	for attempt := 0; attempt < maxRetries; attempt++ {
		selected, err := h.router.SelectAccount(resolved.PoolID, resolved.TargetModel, exclude)
		if err != nil {
			if errors.Is(err, router.ErrPoolDisabled) {
				webutil.WriteGatewayError(w, 503, "pool_disabled", "pool is disabled")
				h.logRequest(requestID, accessToken, modelAlias, resolved, 0, 0, start, isStream, r, false, err.Error(), Usage{}, "")
				return
			}
			if errors.Is(err, router.ErrNoHealthyAccount) {
				webutil.WriteGatewayError(w, 503, "no_healthy_account", "no healthy account available")
				h.logRequest(requestID, accessToken, modelAlias, resolved, 0, 0, start, isStream, r, false, err.Error(), Usage{}, "")
				return
			}
			webutil.WriteGatewayError(w, 500, "internal", err.Error())
			return
		}

		// resolve upstream target based on source_kind
		target := h.resolveTarget(selected.Account)

		timeout := h.getRequestTimeout()
		result := Forward(w, r, target, pathSuffix, requestBody, isStream, requestID, timeout)

		if result.Err != nil {
			// network/connection error
			exclude = append(exclude, selected.Account.ID)
			lastErr = result.Err
			h.updateHealth(selected.Account.ID, "error", result.Err.Error())

			if !IsRetryable(result.Err) {
				webutil.WriteGatewayError(w, 502, "upstream_failed", result.Err.Error())
				h.logRequest(requestID, accessToken, modelAlias, resolved, selected.Account.ID, 0, start, isStream, r, false, result.Err.Error(), Usage{}, selected.Account.SourceKind)
				return
			}
			continue
		}

		if IsRetryableStatus(result.StatusCode) && attempt < maxRetries-1 {
			// response already written for streaming, can't retry
			if isStream {
				h.logRequest(requestID, accessToken, modelAlias, resolved, selected.Account.ID, result.StatusCode, start, isStream, r, false, "upstream error", result.Usage, selected.Account.SourceKind)
				return
			}
			exclude = append(exclude, selected.Account.ID)
			lastStatusCode = result.StatusCode
			h.updateHealth(selected.Account.ID, "error", fmt.Sprintf("HTTP %d", result.StatusCode))
			continue
		}

		// success or non-retryable response — already written to client
		success := result.StatusCode >= 200 && result.StatusCode < 400
		if success {
			h.updateHealth(selected.Account.ID, "healthy", "")
		}

		// accumulate usage
		totalTokens := result.Usage.InputTokens + result.Usage.OutputTokens
		if totalTokens > 0 && accessToken != nil {
			go func() {
				_ = h.store.IncrementTokenUsage(accessToken.ID, totalTokens)
				h.cache.Invalidate()
			}()
		}

		h.logRequest(requestID, accessToken, modelAlias, resolved, selected.Account.ID, result.StatusCode, start, isStream, r, success, "", result.Usage, selected.Account.SourceKind)
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
	h.logRequest(requestID, accessToken, modelAlias, resolved, 0, lastStatusCode, start, isStream, r, false, errMsg, Usage{}, "")
}

func (h *Handler) handleModels(w http.ResponseWriter) {
	aliases := h.cache.GetEnabledModelAliases()
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	models := make([]model, 0, len(aliases))
	for _, alias := range aliases {
		models = append(models, model{
			ID:      alias,
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

func (h *Handler) logRequest(requestID string, token *store.AccessToken, alias string, resolved *router.ResolvedRoute, accountID int64, statusCode int, start time.Time, stream bool, r *http.Request, success bool, errMsg string, usage Usage, sourceKind string) {
	tokenName := ""
	if token != nil {
		tokenName = token.Name
	}
	targetModel := ""
	var poolID int64
	if resolved != nil {
		targetModel = resolved.TargetModel
		poolID = resolved.PoolID
	}

	log := &store.RequestLog{
		RequestID:       requestID,
		AccessTokenName: tokenName,
		ModelAlias:      alias,
		TargetModel:     targetModel,
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
	go func() { _ = h.store.InsertLog(log) }()
}

func (h *Handler) updateHealth(accountID int64, status, lastError string) {
	go func() {
		_ = h.store.UpdateAccountHealth(accountID, status, lastError)
		h.cache.Invalidate()
	}()
}

func (h *Handler) getMaxRetries() int {
	v := h.cache.GetSetting("max_retry_attempts")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 3
}

func (h *Handler) getRequestTimeout() time.Duration {
	v := h.cache.GetSetting("request_timeout")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 120 * time.Second
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

func rewriteModel(body []byte, targetModel string) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	modelJSON, _ := json.Marshal(targetModel)
	m["model"] = modelJSON
	rewritten, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return rewritten
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
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
