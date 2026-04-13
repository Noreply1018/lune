package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lune/internal/cpa"
	"lune/internal/store"
	"lune/internal/webutil"
)

type Handler struct {
	store      *store.Store
	cache      *store.RoutingCache
	cpaAuthDir string
	sessions   *cpa.SessionStore
	logger     *log.Logger
}

func NewHandler(s *store.Store, c *store.RoutingCache, cpaAuthDir string) *Handler {
	return &Handler{
		store:      s,
		cache:      c,
		cpaAuthDir: cpaAuthDir,
		sessions:   cpa.NewSessionStore(),
		logger:     log.Default(),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	handle := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, wrap(fn))
	}

	// Accounts
	handle("GET /admin/api/accounts", h.listAccounts)
	handle("POST /admin/api/accounts", h.createAccount)
	handle("PUT /admin/api/accounts/{id}", h.updateAccount)
	handle("POST /admin/api/accounts/{id}/enable", h.enableAccount)
	handle("POST /admin/api/accounts/{id}/disable", h.disableAccount)
	handle("DELETE /admin/api/accounts/{id}", h.deleteAccount)
	handle("POST /admin/api/accounts/test-connection", h.testConnection)

	// Pools
	handle("GET /admin/api/pools", h.listPools)
	handle("POST /admin/api/pools", h.createPool)
	handle("PUT /admin/api/pools/{id}", h.updatePool)
	handle("POST /admin/api/pools/{id}/enable", h.enablePool)
	handle("POST /admin/api/pools/{id}/disable", h.disablePool)
	handle("DELETE /admin/api/pools/{id}", h.deletePool)

	// Routes
	handle("GET /admin/api/routes", h.listRoutes)
	handle("POST /admin/api/routes", h.createRoute)
	handle("PUT /admin/api/routes/{id}", h.updateRoute)
	handle("DELETE /admin/api/routes/{id}", h.deleteRoute)

	// Tokens
	handle("GET /admin/api/tokens", h.listTokens)
	handle("POST /admin/api/tokens", h.createToken)
	handle("PUT /admin/api/tokens/{id}", h.updateToken)
	handle("POST /admin/api/tokens/{id}/enable", h.enableToken)
	handle("POST /admin/api/tokens/{id}/disable", h.disableToken)
	handle("DELETE /admin/api/tokens/{id}", h.deleteToken)

	// Settings
	handle("GET /admin/api/settings", h.getSettings)
	handle("PUT /admin/api/settings", h.updateSettings)

	// Stats & Export
	handle("GET /admin/api/overview", h.getOverview)
	handle("GET /admin/api/usage", h.getUsage)
	handle("GET /admin/api/export", h.getExport)
	handle("GET /admin/api/usage/latency", h.getLatencyStats)

	// CPA Service
	handle("GET /admin/api/cpa/service", h.getCpaService)
	handle("PUT /admin/api/cpa/service", h.upsertCpaService)
	handle("DELETE /admin/api/cpa/service", h.deleteCpaService)
	handle("POST /admin/api/cpa/service/test", h.testCpaService)
	handle("POST /admin/api/cpa/service/enable", h.enableCpaService)
	handle("POST /admin/api/cpa/service/disable", h.disableCpaService)

	// CPA Login Sessions
	handle("POST /admin/api/accounts/cpa/login-sessions", h.createLoginSession)
	handle("GET /admin/api/accounts/cpa/login-sessions/{id}", h.getLoginSession)
	handle("POST /admin/api/accounts/cpa/login-sessions/{id}/cancel", h.cancelLoginSession)

	// CPA Import
	handle("GET /admin/api/cpa/service/remote-accounts", h.listRemoteAccounts)
	handle("POST /admin/api/accounts/cpa/import", h.importCpaAccount)
	handle("POST /admin/api/accounts/cpa/import/batch", h.batchImportCpaAccounts)
}

// --- Accounts ---

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.store.ListAccounts()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	// mask api keys and fill runtime
	for i := range accounts {
		h.fillAccountResponse(&accounts[i])
	}
	webutil.WriteList(w, accounts, len(accounts))
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req store.Account
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.SourceKind == "" {
		req.SourceKind = "openai_compat"
	}
	if req.Label == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "label is required")
		return
	}
	switch req.SourceKind {
	case "openai_compat":
		if req.BaseURL == "" || req.APIKey == "" {
			webutil.WriteAdminError(w, 400, "bad_request", "base_url and api_key are required for openai_compat accounts")
			return
		}
	case "cpa":
		if req.CpaServiceID == nil || req.CpaProvider == "" {
			webutil.WriteAdminError(w, 400, "bad_request", "cpa_service_id and cpa_provider are required for cpa accounts")
			return
		}
		// clear fields that don't apply
		req.BaseURL = ""
		req.APIKey = ""
		req.QuotaTotal = 0
		req.QuotaUsed = 0
		req.QuotaUnit = ""
	default:
		webutil.WriteAdminError(w, 400, "bad_request", "source_kind must be openai_compat or cpa")
		return
	}

	id, err := h.store.CreateAccount(&req)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			webutil.WriteAdminError(w, 409, "duplicate", "a CPA account with this service and provider already exists")
			return
		}
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	h.fillAccountResponse(&req)
	webutil.WriteData(w, 201, req)
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	existing, err := h.store.GetAccount(id)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	if existing == nil {
		webutil.WriteAdminError(w, 404, "not_found", "account not found")
		return
	}

	var req store.Account
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}

	// preserve source_kind-dependent fields that can't be changed
	if existing.SourceKind == "cpa" {
		req.BaseURL = existing.BaseURL
		req.APIKey = existing.APIKey
		req.QuotaTotal = existing.QuotaTotal
		req.QuotaUsed = existing.QuotaUsed
		req.QuotaUnit = existing.QuotaUnit
		req.Provider = existing.Provider
	}

	if err := h.store.UpdateAccount(id, &req); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	req.SourceKind = existing.SourceKind
	req.CpaServiceID = existing.CpaServiceID
	req.CpaProvider = existing.CpaProvider
	req.CpaAccountKey = existing.CpaAccountKey
	h.fillAccountResponse(&req)
	webutil.WriteData(w, 200, req)
}

func (h *Handler) enableAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.EnableAccount(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) disableAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DisableAccount(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteAccount(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Pools ---

func (h *Handler) listPools(w http.ResponseWriter, r *http.Request) {
	pools, err := h.store.ListPools()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	webutil.WriteList(w, pools, len(pools))
}

func (h *Handler) createPool(w http.ResponseWriter, r *http.Request) {
	var req store.Pool
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.Label == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "label is required")
		return
	}
	if req.Strategy == "" {
		req.Strategy = "priority-first-healthy"
	}
	id, err := h.store.CreatePool(&req)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	webutil.WriteData(w, 201, req)
}

func (h *Handler) updatePool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req store.Pool
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdatePool(id, &req); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	webutil.WriteData(w, 200, req)
}

func (h *Handler) enablePool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.EnablePool(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) disablePool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DisablePool(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) deletePool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeletePool(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Routes ---

func (h *Handler) listRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := h.store.ListRoutes()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	webutil.WriteList(w, routes, len(routes))
}

func (h *Handler) createRoute(w http.ResponseWriter, r *http.Request) {
	var req store.ModelRoute
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.Alias == "" || req.TargetModel == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "alias and target_model are required")
		return
	}
	if req.PoolID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "pool_id is required")
		return
	}
	id, err := h.store.CreateRoute(&req)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	webutil.WriteData(w, 201, req)
}

func (h *Handler) updateRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req store.ModelRoute
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdateRoute(id, &req); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	webutil.WriteData(w, 200, req)
}

func (h *Handler) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteRoute(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Tokens ---

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListTokens()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	for i := range tokens {
		tokens[i].Token = maskKey(tokens[i].Token)
	}
	webutil.WriteList(w, tokens, len(tokens))
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var req store.AccessToken
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.Name == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "name is required")
		return
	}
	if req.Token == "" {
		req.Token = generateToken()
	}
	id, err := h.store.CreateToken(&req)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	// return full token on creation
	webutil.WriteData(w, 201, req)
}

func (h *Handler) updateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req store.AccessToken
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdateToken(id, &req); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	req.Token = maskKey(req.Token)
	webutil.WriteData(w, 200, req)
}

func (h *Handler) enableToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.EnableToken(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) disableToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DisableToken(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteToken(id); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Settings ---

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	// hide admin_token
	if _, ok := settings["admin_token"]; ok {
		settings["admin_token"] = maskKey(settings["admin_token"])
	}
	webutil.WriteData(w, 200, settings)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var pairs map[string]string
	if err := json.NewDecoder(r.Body).Decode(&pairs); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdateSettings(pairs); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Overview, Usage, Export ---

func (h *Handler) getOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.store.GetOverview()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	webutil.WriteData(w, 200, overview)
}

func (h *Handler) getUsage(w http.ResponseWriter, r *http.Request) {
	filter := store.UsageFilter{
		From:       r.URL.Query().Get("from"),
		To:         r.URL.Query().Get("to"),
		TokenName:  r.URL.Query().Get("token_name"),
		SourceKind: r.URL.Query().Get("source_kind"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	logs, total, err := h.store.GetUsage(filter)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	webutil.WriteList(w, logs, total)
}

func (h *Handler) getExport(w http.ResponseWriter, r *http.Request) {
	accounts, _ := h.store.ListAccounts()
	for i := range accounts {
		h.fillAccountResponse(&accounts[i])
	}
	pools, _ := h.store.ListPools()
	routes, _ := h.store.ListRoutes()
	tokens, _ := h.store.ListTokens()
	for i := range tokens {
		tokens[i].Token = maskKey(tokens[i].Token)
	}
	settings, _ := h.store.GetSettings()
	if _, ok := settings["admin_token"]; ok {
		settings["admin_token"] = maskKey(settings["admin_token"])
	}
	cpaServices, _ := h.store.ListCpaServices()
	for i := range cpaServices {
		cpaServices[i].APIKeyMasked = maskKey(cpaServices[i].APIKey)
		cpaServices[i].APIKeySet = cpaServices[i].APIKey != ""
		cpaServices[i].APIKey = ""
	}

	webutil.WriteData(w, 200, map[string]any{
		"exported_at":   time.Now().UTC().Format(time.RFC3339),
		"accounts":      accounts,
		"pools":         pools,
		"model_routes":  routes,
		"access_tokens": tokens,
		"settings":      settings,
		"cpa_services":  cpaServices,
	})
}

func (h *Handler) getLatencyStats(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	period := r.URL.Query().Get("period")
	bucket := r.URL.Query().Get("bucket")
	if period == "" {
		period = "24h"
	}
	if bucket == "" {
		bucket = "1h"
	}

	var opts []int64
	if acStr := r.URL.Query().Get("account"); acStr != "" {
		if v, err := strconv.ParseInt(acStr, 10, 64); err == nil {
			opts = append(opts, v)
		}
	}

	stats, err := h.store.GetLatencyStats(model, period, bucket, opts...)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	webutil.WriteData(w, 200, stats)
}

// --- CPA Service ---

func (h *Handler) getCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	if svc == nil {
		webutil.WriteData(w, 200, nil)
		return
	}
	svc.APIKeyMasked = maskKey(svc.APIKey)
	svc.APIKeySet = svc.APIKey != ""
	svc.APIKey = ""
	webutil.WriteData(w, 200, svc)
}

func (h *Handler) upsertCpaService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label   string `json:"label"`
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.Label == "" || req.BaseURL == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "label and base_url are required")
		return
	}

	existing, err := h.store.GetCpaService()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if existing != nil {
		// update
		svc := &store.CpaService{
			Label:   req.Label,
			BaseURL: strings.TrimRight(req.BaseURL, "/"),
			APIKey:  req.APIKey,
			Enabled: enabled,
		}
		if req.APIKey == "" {
			svc.APIKey = existing.APIKey
		}
		if err := h.store.UpdateCpaService(existing.ID, svc); err != nil {
			webutil.WriteAdminError(w, 500, "internal", err.Error())
			return
		}
		h.cache.Invalidate()
		svc.ID = existing.ID
		svc.APIKeyMasked = maskKey(svc.APIKey)
		svc.APIKeySet = svc.APIKey != ""
		svc.APIKey = ""
		webutil.WriteData(w, 200, svc)
	} else {
		// create
		svc := &store.CpaService{
			Label:   req.Label,
			BaseURL: strings.TrimRight(req.BaseURL, "/"),
			APIKey:  req.APIKey,
			Enabled: enabled,
		}
		id, err := h.store.CreateCpaService(svc)
		if err != nil {
			if strings.Contains(err.Error(), "only one") {
				webutil.WriteAdminError(w, 409, "already_exists", err.Error())
				return
			}
			webutil.WriteAdminError(w, 500, "internal", err.Error())
			return
		}
		h.cache.Invalidate()
		svc.ID = id
		svc.APIKeyMasked = maskKey(svc.APIKey)
		svc.APIKeySet = svc.APIKey != ""
		svc.APIKey = ""
		webutil.WriteData(w, 201, svc)
	}
}

func (h *Handler) deleteCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	if svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}
	count, err := h.store.CountAccountsByCpaService(svc.ID)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	if count > 0 {
		webutil.WriteAdminError(w, 409, "has_dependent_accounts",
			fmt.Sprintf("Cannot delete CPA service: %d accounts are linked to it. Remove them first.", count))
		return
	}
	if err := h.store.DeleteCpaService(svc.ID); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) testConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.BaseURL == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "base_url is required")
		return
	}

	baseURL := strings.TrimRight(req.BaseURL, "/")
	client := &http.Client{Timeout: 10 * time.Second}

	result := struct {
		Reachable bool     `json:"reachable"`
		LatencyMs int64    `json:"latency_ms"`
		Models    []string `json:"models"`
		Error     string   `json:"error"`
	}{}

	start := time.Now()
	modelsReq, _ := http.NewRequest("GET", baseURL+"/models", nil)
	if req.APIKey != "" {
		modelsReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	modelsResp, err := client.Do(modelsReq)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		webutil.WriteData(w, 200, result)
		return
	}
	defer modelsResp.Body.Close()

	if modelsResp.StatusCode != 200 {
		result.Error = fmt.Sprintf("HTTP %d", modelsResp.StatusCode)
		webutil.WriteData(w, 200, result)
		return
	}

	result.Reachable = true

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(modelsResp.Body).Decode(&body); err == nil {
		for _, m := range body.Data {
			result.Models = append(result.Models, m.ID)
		}
	}

	webutil.WriteData(w, 200, result)
}

func (h *Handler) testCpaService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}

	// allow testing with body params or from stored service
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		json.Unmarshal(body, &req)
	}
	if req.BaseURL == "" {
		svc, err := h.store.GetCpaService()
		if err != nil || svc == nil {
			webutil.WriteAdminError(w, 400, "bad_request", "no CPA service configured and no base_url provided")
			return
		}
		req.BaseURL = svc.BaseURL
		req.APIKey = svc.APIKey
	}

	baseURL := strings.TrimRight(req.BaseURL, "/")
	client := &http.Client{Timeout: 10 * time.Second}

	result := struct {
		Reachable bool     `json:"reachable"`
		LatencyMs int64    `json:"latency_ms"`
		Providers []string `json:"providers"`
		Error     string   `json:"error"`
	}{}

	// test healthz
	start := time.Now()
	healthReq, _ := http.NewRequest("GET", baseURL+"/healthz", nil)
	if req.APIKey != "" {
		healthReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	healthResp, err := client.Do(healthReq)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		webutil.WriteData(w, 200, result)
		return
	}
	healthResp.Body.Close()

	if healthResp.StatusCode != 200 {
		result.Error = fmt.Sprintf("healthz returned HTTP %d", healthResp.StatusCode)
		webutil.WriteData(w, 200, result)
		return
	}

	result.Reachable = true

	// get models to extract providers
	modelsReq, _ := http.NewRequest("GET", baseURL+"/v1/models", nil)
	if req.APIKey != "" {
		modelsReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	modelsResp, err := client.Do(modelsReq)
	if err == nil {
		defer modelsResp.Body.Close()
		var modelsBody struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(modelsResp.Body).Decode(&modelsBody); err == nil {
			providerSet := make(map[string]bool)
			for _, m := range modelsBody.Data {
				// extract provider from model IDs like "codex/gpt-4o" or use first segment
				parts := strings.SplitN(m.ID, "/", 2)
				if len(parts) == 2 {
					providerSet[parts[0]] = true
				}
			}
			for p := range providerSet {
				result.Providers = append(result.Providers, p)
			}
		}
	}

	webutil.WriteData(w, 200, result)
}

func (h *Handler) enableCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	if svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}
	if err := h.store.EnableCpaService(svc.ID); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) disableCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	if svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}
	if err := h.store.DisableCpaService(svc.ID); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- helpers ---

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid id")
		return 0, false
	}
	return id, true
}

func maskKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:3] + "..." + key[len(key)-4:]
}

func (h *Handler) fillAccountResponse(a *store.Account) {
	a.APIKeyMasked = maskKey(a.APIKey)
	a.APIKeySet = a.APIKey != ""
	a.APIKey = ""

	switch a.SourceKind {
	case "cpa":
		if a.CpaServiceID != nil {
			svc := h.cache.GetCpaService(*a.CpaServiceID)
			if svc != nil {
				a.Runtime = &store.AccountRuntime{
					BaseURL:  strings.TrimRight(svc.BaseURL, "/") + "/api/provider/" + a.CpaProvider + "/v1",
					AuthMode: "bearer",
				}
			}
		}
	default:
		a.Runtime = &store.AccountRuntime{
			BaseURL:  a.BaseURL,
			AuthMode: "bearer",
		}
	}
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "sk-lune-" + hex.EncodeToString(b)
}

// --- CPA Login Sessions ---

func (h *Handler) createLoginSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceID int64 `json:"service_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}

	if h.cpaAuthDir == "" {
		webutil.WriteAdminError(w, 400, "not_configured", "cpa_auth_dir is not configured")
		return
	}

	svc, err := h.store.GetCpaServiceByID(req.ServiceID)
	if err != nil || svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "CPA service not found")
		return
	}

	dcr, err := cpa.RequestDeviceCode(r.Context())
	if err != nil {
		webutil.WriteAdminError(w, 502, "upstream_error", fmt.Sprintf("device code request failed: %v", err))
		return
	}

	session, err := h.sessions.CreateSession(req.ServiceID, dcr)
	if err != nil {
		webutil.WriteAdminError(w, 409, "active_session", err.Error())
		return
	}

	// start background polling goroutine
	ctx, cancel := context.WithDeadline(context.Background(), session.ExpiresAt)
	session.CancelFunc = cancel

	go h.pollLoginSession(ctx, session, svc)

	webutil.WriteData(w, 201, map[string]any{
		"id":                   session.ID,
		"status":               session.Status,
		"verification_uri":     session.VerificationURI,
		"user_code":            session.UserCode,
		"expires_at":           session.ExpiresAt.Format(time.RFC3339),
		"poll_interval_seconds": session.PollInterval,
	})
}

func (h *Handler) pollLoginSession(ctx context.Context, session *cpa.LoginSession, svc *store.CpaService) {
	interval := time.Duration(session.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// check if cancelled or expired
			s := h.sessions.GetSession(session.ID)
			if s != nil && s.Status == "pending" {
				h.sessions.UpdateStatus(session.ID, "expired", "expired_token", "Device code expired")
			}
			return
		case <-ticker.C:
			s := h.sessions.GetSession(session.ID)
			if s == nil || s.Status == "cancelled" {
				return
			}

			tokenResp, err := cpa.PollForToken(ctx, session.DeviceCode)
			if err != nil {
				if errors.Is(err, cpa.ErrAuthorizationPending) {
					continue
				}
				if errors.Is(err, cpa.ErrSlowDown) {
					ticker.Reset(interval + 5*time.Second)
					continue
				}
				if errors.Is(err, cpa.ErrExpiredToken) {
					h.sessions.UpdateStatus(session.ID, "expired", "expired_token", "Device code expired")
					return
				}
				if errors.Is(err, cpa.ErrAccessDenied) {
					h.sessions.UpdateStatus(session.ID, "failed", "access_denied", "User denied authorization")
					return
				}
				h.sessions.UpdateStatus(session.ID, "failed", "poll_error", err.Error())
				return
			}

			// success — process the token
			h.sessions.UpdateStatus(session.ID, "authorized", "", "")
			h.finalizeLogin(session, svc, tokenResp)
			return
		}
	}
}

func (h *Handler) finalizeLogin(session *cpa.LoginSession, svc *store.CpaService, tokenResp *cpa.TokenResponse) {
	info, err := cpa.ParseAccountInfo(tokenResp.AccessToken)
	if err != nil {
		h.sessions.UpdateStatus(session.ID, "failed", "jwt_parse_error", fmt.Sprintf("Failed to parse JWT: %v", err))
		return
	}

	planType := info.PlanType
	if planType == "" {
		planType = "unknown"
	}
	accountKey := fmt.Sprintf("codex-%s-%s", info.Email, planType)

	expiredAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	lastRefresh := time.Now().Format(time.RFC3339)

	// write credential file to cpa-auth directory
	authFile := &cpa.CpaAuthFile{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		AccountID:    info.AccountID,
		Email:        info.Email,
		Disabled:     false,
		Expired:      expiredAt,
		LastRefresh:  lastRefresh,
		Type:         "codex",
	}
	if err := cpa.WriteAuthFile(h.cpaAuthDir, authFile, accountKey); err != nil {
		h.sessions.UpdateStatus(session.ID, "failed", "file_write_error", fmt.Sprintf("Failed to write credential file: %v", err))
		return
	}

	// create local account record (no tokens stored in DB)
	label := fmt.Sprintf("codex - %s (%s)", info.Email, planType)
	account := &store.Account{
		Label:         label,
		SourceKind:    "cpa",
		CpaServiceID:  &svc.ID,
		CpaProvider:   "codex",
		CpaAccountKey: accountKey,
		CpaEmail:      info.Email,
		CpaPlanType:   planType,
		CpaOpenaiID:   info.AccountID,
		CpaExpiredAt:  &expiredAt,
		CpaLastRefreshAt: &lastRefresh,
		Enabled:       true,
	}

	id, err := h.store.CreateAccount(account)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			// account already exists — update session with existing account
			existing, _ := h.store.FindAccountByCpaKey(svc.ID, accountKey)
			if existing != nil {
				h.fillAccountResponse(existing)
				h.sessions.CompleteSession(session.ID, existing.ID, existing)
				h.cache.Invalidate()
				return
			}
		}
		h.sessions.UpdateStatus(session.ID, "failed", "account_create_error", fmt.Sprintf("Failed to create account: %v", err))
		return
	}

	account.ID = id
	h.fillAccountResponse(account)
	h.sessions.CompleteSession(session.ID, id, account)
	h.cache.Invalidate()
}

func (h *Handler) getLoginSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session := h.sessions.GetSession(id)
	if session == nil {
		webutil.WriteAdminError(w, 404, "not_found", "login session not found")
		return
	}

	resp := map[string]any{
		"id":     session.ID,
		"status": session.Status,
	}

	switch session.Status {
	case "pending":
		resp["verification_uri"] = session.VerificationURI
		resp["user_code"] = session.UserCode
		resp["expires_at"] = session.ExpiresAt.Format(time.RFC3339)
		resp["poll_interval_seconds"] = session.PollInterval
	case "succeeded":
		if session.AccountID != nil {
			resp["account_id"] = *session.AccountID
		}
		if session.Account != nil {
			resp["account"] = session.Account
		}
	case "failed", "expired":
		resp["error_code"] = session.ErrorCode
		resp["error_message"] = session.ErrorMessage
	}

	webutil.WriteData(w, 200, resp)
}

func (h *Handler) cancelLoginSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.sessions.CancelSession(id); err != nil {
		webutil.WriteAdminError(w, 404, "not_found", err.Error())
		return
	}
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- CPA Import ---

func (h *Handler) listRemoteAccounts(w http.ResponseWriter, r *http.Request) {
	if h.cpaAuthDir == "" {
		webutil.WriteAdminError(w, 400, "not_configured", "cpa_auth_dir is not configured")
		return
	}

	svc, err := h.store.GetCpaService()
	if err != nil || svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}

	scanned, err := cpa.ScanAuthDirKeyed(h.cpaAuthDir)
	if err != nil {
		webutil.WriteAdminError(w, 500, "scan_error", fmt.Sprintf("Failed to scan cpa-auth directory: %v", err))
		return
	}

	type remoteAccount struct {
		AccountKey      string  `json:"account_key"`
		Email           string  `json:"email"`
		PlanType        string  `json:"plan_type"`
		Provider        string  `json:"provider"`
		AccountID       string  `json:"account_id"`
		ExpiredAt       *string `json:"expired_at"`
		Disabled        bool    `json:"disabled"`
		AlreadyImported bool    `json:"already_imported"`
	}

	var result []remoteAccount
	for _, s := range scanned {
		var expiredAt *string
		if s.File.Expired != "" {
			expiredAt = &s.File.Expired
		}

		alreadyImported := false
		if existing, _ := h.store.FindAccountByCpaKey(svc.ID, s.Key); existing != nil {
			alreadyImported = true
		}

		// derive plan_type from JWT or filename
		planType := ""
		if info, err := cpa.ParseAccountInfo(s.File.AccessToken); err == nil {
			planType = info.PlanType
		}

		result = append(result, remoteAccount{
			AccountKey:      s.Key,
			Email:           s.File.Email,
			PlanType:        planType,
			Provider:        s.File.Type,
			AccountID:       s.File.AccountID,
			ExpiredAt:       expiredAt,
			Disabled:        s.File.Disabled,
			AlreadyImported: alreadyImported,
		})
	}

	webutil.WriteData(w, 200, result)
}

func (h *Handler) importCpaAccount(w http.ResponseWriter, r *http.Request) {
	if h.cpaAuthDir == "" {
		webutil.WriteAdminError(w, 400, "not_configured", "cpa_auth_dir is not configured")
		return
	}

	var req struct {
		ServiceID      int64    `json:"service_id"`
		AccountKey     string   `json:"account_key"`
		Label          string   `json:"label"`
		Enabled        bool     `json:"enabled"`
		Notes          string   `json:"notes"`
		ModelAllowlist []string `json:"model_allowlist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.AccountKey == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "account_key is required")
		return
	}

	svc, err := h.store.GetCpaServiceByID(req.ServiceID)
	if err != nil || svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "CPA service not found")
		return
	}

	f, err := cpa.ReadAuthFile(h.cpaAuthDir, req.AccountKey)
	if err != nil {
		webutil.WriteAdminError(w, 404, "not_found", fmt.Sprintf("Credential file not found: %v", err))
		return
	}

	// parse plan_type from JWT if available
	planType := ""
	openaiID := f.AccountID
	if info, err := cpa.ParseAccountInfo(f.AccessToken); err == nil {
		planType = info.PlanType
		if info.AccountID != "" {
			openaiID = info.AccountID
		}
	}

	label := req.Label
	if label == "" {
		label = fmt.Sprintf("%s - %s (%s)", f.Type, f.Email, planType)
	}

	var expiredAt, lastRefreshAt *string
	if f.Expired != "" {
		expiredAt = &f.Expired
	}
	if f.LastRefresh != "" {
		lastRefreshAt = &f.LastRefresh
	}

	account := &store.Account{
		Label:            label,
		SourceKind:       "cpa",
		CpaServiceID:     &svc.ID,
		CpaProvider:      f.Type,
		CpaAccountKey:    req.AccountKey,
		CpaEmail:         f.Email,
		CpaPlanType:      planType,
		CpaOpenaiID:      openaiID,
		CpaExpiredAt:     expiredAt,
		CpaLastRefreshAt: lastRefreshAt,
		CpaDisabled:      f.Disabled,
		Enabled:          req.Enabled,
		Notes:            req.Notes,
		ModelAllowlist:   req.ModelAllowlist,
	}

	id, err := h.store.CreateAccount(account)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			webutil.WriteAdminError(w, 409, "duplicate", "This account has already been imported")
			return
		}
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}

	h.cache.Invalidate()
	account.ID = id
	h.fillAccountResponse(account)
	webutil.WriteData(w, 201, account)
}

func (h *Handler) batchImportCpaAccounts(w http.ResponseWriter, r *http.Request) {
	if h.cpaAuthDir == "" {
		webutil.WriteAdminError(w, 400, "not_configured", "cpa_auth_dir is not configured")
		return
	}

	var req struct {
		ServiceID   int64    `json:"service_id"`
		AccountKeys []string `json:"account_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}

	svc, err := h.store.GetCpaServiceByID(req.ServiceID)
	if err != nil || svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "CPA service not found")
		return
	}

	var imported, skipped int
	var errs []string

	for _, key := range req.AccountKeys {
		f, err := cpa.ReadAuthFile(h.cpaAuthDir, key)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", key, err))
			continue
		}

		planType := ""
		openaiID := f.AccountID
		if info, err := cpa.ParseAccountInfo(f.AccessToken); err == nil {
			planType = info.PlanType
			if info.AccountID != "" {
				openaiID = info.AccountID
			}
		}

		var expiredAt, lastRefreshAt *string
		if f.Expired != "" {
			expiredAt = &f.Expired
		}
		if f.LastRefresh != "" {
			lastRefreshAt = &f.LastRefresh
		}

		account := &store.Account{
			Label:            fmt.Sprintf("%s - %s (%s)", f.Type, f.Email, planType),
			SourceKind:       "cpa",
			CpaServiceID:     &svc.ID,
			CpaProvider:      f.Type,
			CpaAccountKey:    key,
			CpaEmail:         f.Email,
			CpaPlanType:      planType,
			CpaOpenaiID:      openaiID,
			CpaExpiredAt:     expiredAt,
			CpaLastRefreshAt: lastRefreshAt,
			CpaDisabled:      f.Disabled,
			Enabled:          true,
		}

		_, err = h.store.CreateAccount(account)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				skipped++
			} else {
				errs = append(errs, fmt.Sprintf("%s: %v", key, err))
			}
			continue
		}
		imported++
	}

	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"errors":   errs,
	})
}
