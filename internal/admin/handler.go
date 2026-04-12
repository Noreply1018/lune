package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

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
}

// --- Accounts ---

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.store.ListAccounts()
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	// mask api keys
	for i := range accounts {
		accounts[i].APIKey = maskKey(accounts[i].APIKey)
	}
	webutil.WriteList(w, accounts, len(accounts))
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req store.Account
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.Label == "" || req.BaseURL == "" || req.APIKey == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "label, base_url, and api_key are required")
		return
	}
	id, err := h.store.CreateAccount(&req)
	if err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	req.APIKey = maskKey(req.APIKey)
	webutil.WriteData(w, 201, req)
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req store.Account
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdateAccount(id, &req); err != nil {
		webutil.WriteAdminError(w, 500, "internal", err.Error())
		return
	}
	h.cache.Invalidate()
	req.ID = id
	req.APIKey = maskKey(req.APIKey)
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

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "sk-lune-" + hex.EncodeToString(b)
}
