package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lune/internal/store"
	"lune/internal/webutil"
)

type Handler struct {
	store *store.Store
	cache *store.RoutingCache
}

func NewHandler(s *store.Store, c *store.RoutingCache) *Handler {
	return &Handler{store: s, cache: c}
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

	// CPA Service
	handle("GET /admin/api/cpa/service", h.getCpaService)
	handle("PUT /admin/api/cpa/service", h.upsertCpaService)
	handle("DELETE /admin/api/cpa/service", h.deleteCpaService)
	handle("POST /admin/api/cpa/service/test", h.testCpaService)
	handle("POST /admin/api/cpa/service/enable", h.enableCpaService)
	handle("POST /admin/api/cpa/service/disable", h.disableCpaService)
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
