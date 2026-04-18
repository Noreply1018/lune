package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lune/internal/cpa"
	"lune/internal/health"
	"lune/internal/notify"
	"lune/internal/store"
	"lune/internal/syscfg"
	"lune/internal/webutil"
)

type Handler struct {
	store            *store.Store
	cache            *store.RoutingCache
	cpaAuthDir       string
	cpaManagementKey string
	sessions         *cpa.SessionStore
	healthChecker    *health.Checker
	notifier         *notify.Service
}

func NewHandler(s *store.Store, c *store.RoutingCache, cpaAuthDir, cpaManagementKey string, hc *health.Checker, notifier *notify.Service) *Handler {
	sessionPath := ""
	if cpaAuthDir != "" {
		sessionPath = filepath.Join(cpaAuthDir, ".login-sessions.json")
	}

	h := &Handler{
		store:            s,
		cache:            c,
		cpaAuthDir:       cpaAuthDir,
		cpaManagementKey: cpaManagementKey,
		sessions:         cpa.NewSessionStore(sessionPath),
		healthChecker:    hc,
		notifier:         notifier,
	}
	h.resumeActiveLoginSessions()
	return h
}

func (h *Handler) internalError(w http.ResponseWriter, err error) {
	slog.Error("admin", "err", err)
	webutil.WriteAdminError(w, 500, "internal", "internal server error")
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

	// Account model discovery
	handle("POST /admin/api/accounts/{id}/discover-models", h.discoverModels)
	handle("GET /admin/api/accounts/{id}/models", h.getAccountModels)

	// Self-check configuration + result (direct accounts)
	handle("PUT /admin/api/accounts/{id}/probe-models", h.updateProbeModels)
	handle("POST /admin/api/accounts/{id}/probe-result", h.recordProbeResult)

	// Pools
	handle("GET /admin/api/pools", h.listPools)
	handle("POST /admin/api/pools", h.createPool)
	handle("GET /admin/api/pools/{id}", h.getPoolDetail)
	handle("PUT /admin/api/pools/{id}", h.updatePool)
	handle("POST /admin/api/pools/{id}/enable", h.enablePool)
	handle("POST /admin/api/pools/{id}/disable", h.disablePool)
	handle("DELETE /admin/api/pools/{id}", h.deletePool)

	// Pool members
	handle("POST /admin/api/pools/{id}/members", h.addPoolMember)
	handle("DELETE /admin/api/pools/{id}/members/{member_id}", h.removePoolMember)
	handle("PUT /admin/api/pools/{id}/members/reorder", h.reorderPoolMembers)
	handle("PUT /admin/api/pools/{id}/members/{member_id}", h.updatePoolMember)

	// Pool tokens
	handle("GET /admin/api/pools/{id}/tokens", h.listPoolTokens)

	// Tokens
	handle("GET /admin/api/tokens", h.listTokens)
	handle("POST /admin/api/tokens", h.createToken)
	handle("POST /admin/api/tokens/global/reveal", h.revealGlobalToken)
	handle("POST /admin/api/tokens/{id}/reveal", h.revealToken)
	handle("PUT /admin/api/tokens/{id}", h.updateToken)
	handle("POST /admin/api/tokens/{id}/enable", h.enableToken)
	handle("POST /admin/api/tokens/{id}/disable", h.disableToken)
	handle("POST /admin/api/tokens/{id}/regenerate", h.regenerateToken)
	handle("DELETE /admin/api/tokens/{id}", h.deleteToken)

	// Settings
	handle("GET /admin/api/settings", h.getSettings)
	handle("PUT /admin/api/settings", h.updateSettings)
	handle("GET /admin/api/settings/notifications", h.listNotifications)
	handle("GET /admin/api/settings/data-retention", h.getDataRetention)
	handle("GET /admin/api/settings/data-retention/preview", h.previewDataRetention)
	handle("POST /admin/api/settings/data-retention/prune", h.pruneDataRetention)
	handle("POST /admin/api/import/preview", h.importPreview)
	handle("GET /admin/api/notifications", h.getNotifications)
	handle("PUT /admin/api/notifications/settings", h.updateNotificationSettings)
	handle("PUT /admin/api/notifications/subscriptions/{event}", h.updateNotificationSubscription)
	handle("POST /admin/api/notifications/test", h.testNotification)
	handle("GET /admin/api/notifications/deliveries", h.listNotificationDeliveries)
	handle("GET /admin/api/notifications/event-types", h.listNotificationEventTypes)

	// Stats & Export
	handle("GET /admin/api/overview", h.getOverview)
	handle("GET /admin/api/usage", h.getUsage)
	handle("GET /admin/api/export", h.getExport)
	handle("POST /admin/api/import", h.importConfig)
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
	handle("GET /admin/api/accounts/cpa/login-sessions/active", h.getActiveLoginSession)
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
		h.internalError(w, err)
		return
	}
	for i := range accounts {
		h.fillAccountResponse(&accounts[i])
	}
	webutil.WriteList(w, accounts, len(accounts))
}

type createAccountRequest struct {
	store.Account
	PoolID int64 `json:"pool_id"`
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
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
	if req.PoolID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "pool_id is required")
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
		req.BaseURL = ""
		req.APIKey = ""
	default:
		webutil.WriteAdminError(w, 400, "bad_request", "source_kind must be openai_compat or cpa")
		return
	}

	id, err := h.store.CreateAccount(&req.Account)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			webutil.WriteAdminError(w, 409, "duplicate", "a CPA account with this service and provider already exists")
			return
		}
		h.internalError(w, err)
		return
	}
	req.Account.ID = id

	// add to pool
	if _, err := h.store.AddPoolMember(req.PoolID, id); err != nil {
		h.internalError(w, err)
		return
	}

	h.cache.Invalidate()
	h.fillAccountResponse(&req.Account)

	// trigger async model discovery
	acc := req.Account
	go h.healthChecker.DiscoverModels(context.Background(), acc)

	webutil.WriteData(w, 201, req.Account)
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	existing, err := h.store.GetAccount(id)
	if err != nil {
		h.internalError(w, err)
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
		req.Provider = existing.Provider
		req.QuotaDisplay = existing.QuotaDisplay
	}

	if err := h.store.UpdateAccount(id, &req); err != nil {
		h.internalError(w, err)
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
		h.internalError(w, err)
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
		h.internalError(w, err)
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
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Account Model Discovery ---

func (h *Handler) discoverModels(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	acc, err := h.store.GetAccount(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if acc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "account not found")
		return
	}

	models, err := h.healthChecker.DiscoverModels(r.Context(), *acc)
	if err != nil {
		webutil.WriteAdminError(w, 502, "discovery_failed", fmt.Sprintf("model discovery failed: %v", err))
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]any{"models": models})
}

func (h *Handler) getAccountModels(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	models, err := h.store.ListAccountModels(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, map[string]any{"models": models})
}

// updateProbeModels replaces the user-configured probe-model list on an
// account. The frontend sends the entire desired list every save, so we do a
// simple overwrite — no append/remove semantics.
func (h *Handler) updateProbeModels(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	acc, err := h.store.GetAccount(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if acc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "account not found")
		return
	}
	// De-duplicate and strip blanks so the stored list is canonical; the UI is
	// free to send noisy input.
	seen := make(map[string]bool, len(req.Models))
	cleaned := make([]string, 0, len(req.Models))
	for _, m := range req.Models {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		cleaned = append(cleaned, m)
	}
	if err := h.store.UpdateAccountProbeModels(id, cleaned); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]any{"probe_models": cleaned})
}

// recordProbeResult is written to by the Pool-detail self-check button. The
// frontend ran the actual probe(s) against /v1/chat/completions and we only
// persist the summary so StatusBadge + DirectAccountSignal can reflect it.
func (h *Handler) recordProbeResult(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		Status  string `json:"status"`
		LastErr string `json:"last_error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	switch req.Status {
	case "healthy", "degraded", "error":
	default:
		webutil.WriteAdminError(w, 400, "bad_request", "status must be healthy|degraded|error")
		return
	}
	acc, err := h.store.GetAccount(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if acc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "account not found")
		return
	}
	if err := h.store.UpdateAccountProbeResult(id, req.Status, req.LastErr); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]any{"ok": true})
}

// --- Pools ---

func (h *Handler) listPools(w http.ResponseWriter, r *http.Request) {
	pools, err := h.store.ListPools()
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteList(w, pools, len(pools))
}

func (h *Handler) createPool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label    string `json:"label"`
		Priority int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.Label == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "label is required")
		return
	}
	id, _, err := h.store.CreatePoolWithDefaultToken(req.Label, req.Priority, true)
	if err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()

	pool, err := h.store.GetPool(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 201, pool)
}

func (h *Handler) getPoolDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	pool, err := h.store.GetPool(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if pool == nil {
		webutil.WriteAdminError(w, 404, "not_found", "pool not found")
		return
	}

	members, err := h.store.ListPoolMembers(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	for i := range members {
		if members[i].Account != nil {
			h.fillAccountResponse(members[i].Account)
		}
	}

	tokens, err := h.store.ListTokensByPool(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if tokens == nil {
		tokens = []store.AccessToken{}
	}
	for i := range tokens {
		tokens[i].TokenMasked = maskKey(tokens[i].Token)
		tokens[i].Token = ""
		tokens[i].IsGlobal = tokens[i].PoolID == nil
		tokens[i].PoolLabel = pool.Label
	}

	models, err := h.store.GetPoolModels(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if models == nil {
		models = []string{}
	}

	stats, err := h.store.GetPoolStats(id, "24h")
	if err != nil {
		h.internalError(w, err)
		return
	}
	if stats.ByAccount == nil {
		stats.ByAccount = []store.UsageByAccount{}
	}
	if stats.ByToken == nil {
		stats.ByToken = []store.UsageByToken{}
	}

	webutil.WriteData(w, 200, struct {
		Pool    *store.Pool         `json:"pool"`
		Members []store.PoolMember  `json:"members"`
		Tokens  []store.AccessToken `json:"tokens"`
		Models  []string            `json:"models"`
		Stats   *store.UsageStats   `json:"stats"`
	}{
		Pool:    pool,
		Members: members,
		Tokens:  tokens,
		Models:  models,
		Stats:   stats,
	})
}

func (h *Handler) updatePool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		Label    string `json:"label"`
		Priority int    `json:"priority"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdatePool(id, req.Label, req.Priority, req.Enabled); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()

	pool, err := h.store.GetPool(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, pool)
}

func (h *Handler) enablePool(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.EnablePool(id); err != nil {
		h.internalError(w, err)
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
		h.internalError(w, err)
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
	if err := h.store.DeletePoolWithOrphans(id); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Pool Members ---

func (h *Handler) addPoolMember(w http.ResponseWriter, r *http.Request) {
	poolID, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		AccountID int64 `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.AccountID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "account_id is required")
		return
	}

	memberID, err := h.store.AddPoolMember(poolID, req.AccountID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()

	acc, _ := h.store.GetAccount(req.AccountID)
	if acc != nil {
		go h.healthChecker.DiscoverModels(context.Background(), *acc)
	}

	webutil.WriteData(w, 201, map[string]any{"id": memberID})
}

func (h *Handler) removePoolMember(w http.ResponseWriter, r *http.Request) {
	memberID, err := strconv.ParseInt(r.PathValue("member_id"), 10, 64)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid member_id")
		return
	}
	if err := h.store.RemovePoolMember(memberID); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) reorderPoolMembers(w http.ResponseWriter, r *http.Request) {
	poolID, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		MemberIDs []int64 `json:"member_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.ReorderPoolMembers(poolID, req.MemberIDs); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) updatePoolMember(w http.ResponseWriter, r *http.Request) {
	memberID, err := strconv.ParseInt(r.PathValue("member_id"), 10, 64)
	if err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid member_id")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if err := h.store.UpdatePoolMember(memberID, req.Enabled); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

// --- Pool Tokens ---

func (h *Handler) listPoolTokens(w http.ResponseWriter, r *http.Request) {
	poolID, ok := parseID(w, r)
	if !ok {
		return
	}
	tokens, err := h.store.ListTokensByPool(poolID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	pool, err := h.store.GetPool(poolID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	for i := range tokens {
		tokens[i].TokenMasked = maskKey(tokens[i].Token)
		tokens[i].Token = ""
		tokens[i].IsGlobal = tokens[i].PoolID == nil
		if tokens[i].PoolID != nil && pool != nil {
			tokens[i].PoolLabel = pool.Label
		}
	}
	if tokens == nil {
		tokens = []store.AccessToken{}
	}
	webutil.WriteList(w, tokens, len(tokens))
}

// --- Tokens ---

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListTokens()
	if err != nil {
		h.internalError(w, err)
		return
	}
	pools, err := h.store.ListPools()
	if err != nil {
		h.internalError(w, err)
		return
	}
	poolLabelByID := make(map[int64]string, len(pools))
	for _, pool := range pools {
		poolLabelByID[pool.ID] = pool.Label
	}
	for i := range tokens {
		tokens[i].TokenMasked = maskKey(tokens[i].Token)
		if tokens[i].PoolID != nil {
			tokens[i].PoolLabel = poolLabelByID[*tokens[i].PoolID]
		}
		tokens[i].Token = ""
	}
	webutil.WriteList(w, tokens, len(tokens))
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var req store.AccessToken
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "name is required")
		return
	}
	// Force server-side generation: never trust a client-supplied value for
	// the token column, otherwise any admin-authenticated caller could plant
	// a known secret and bypass the random-token entropy contract.
	req.Token = ""
	id, err := h.store.CreateToken(&req)
	if err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	// Fetch the persisted row so the response carries real created_at /
	// updated_at / token_masked values; then reattach the plaintext token
	// for the one-shot reveal-on-create UX.
	created, err := h.store.GetToken(id)
	if err != nil || created == nil {
		h.internalError(w, err)
		return
	}
	created.Token = req.Token
	created.IsGlobal = created.PoolID == nil
	webutil.WriteData(w, 201, created)
}

func (h *Handler) revealGlobalToken(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListGlobalTokens()
	if err != nil {
		h.internalError(w, err)
		return
	}
	var token *store.AccessToken
	for i := range tokens {
		if tokens[i].Enabled {
			token = &tokens[i]
			break
		}
	}
	if token == nil {
		webutil.WriteAdminError(w, 404, "not_found", "global token not found")
		return
	}
	token.TokenMasked = maskKey(token.Token)
	token.IsGlobal = true
	webutil.WriteData(w, 200, token)
}

func (h *Handler) revealToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	token, err := h.store.GetToken(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if token == nil {
		webutil.WriteAdminError(w, 404, "not_found", "token not found")
		return
	}
	token.TokenMasked = maskKey(token.Token)
	token.IsGlobal = token.PoolID == nil
	if token.PoolID != nil {
		if pool, err := h.store.GetPool(*token.PoolID); err == nil && pool != nil {
			token.PoolLabel = pool.Label
		}
	}
	webutil.WriteData(w, 200, token)
}

func (h *Handler) updateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	existing, err := h.store.GetToken(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if existing == nil {
		webutil.WriteAdminError(w, 404, "not_found", "token not found")
		return
	}
	var req store.AccessToken
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "name is required")
		return
	}
	if err := h.store.UpdateToken(id, &req); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	// UpdateToken only writes name/pool_id/enabled. The real token value is
	// unchanged, so echo back the database's masked form rather than masking
	// whatever the client happened to put in req.Token.
	existing.Name = req.Name
	existing.PoolID = req.PoolID
	existing.Enabled = req.Enabled
	existing.IsGlobal = existing.PoolID == nil
	existing.Token = ""
	webutil.WriteData(w, 200, existing)
}

func (h *Handler) enableToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.store.EnableToken(id); err != nil {
		h.internalError(w, err)
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
		h.internalError(w, err)
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
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) regenerateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	token, err := h.store.RegenerateToken(id)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if token == nil {
		webutil.WriteAdminError(w, 404, "not_found", "token not found")
		return
	}
	h.cache.Invalidate()
	token.TokenMasked = maskKey(token.Token)
	token.IsGlobal = token.PoolID == nil
	if token.PoolID != nil {
		if pool, err := h.store.GetPool(*token.PoolID); err == nil && pool != nil {
			token.PoolLabel = pool.Label
		}
	}
	webutil.WriteData(w, 200, token)
}

// --- Settings ---

type systemSettingsResponse struct {
	AdminTokenMasked         string `json:"admin_token_masked"`
	HealthCheckInterval      int    `json:"health_check_interval"`
	RequestTimeout           int    `json:"request_timeout"`
	MaxRetryAttempts         int    `json:"max_retry_attempts"`
	NotificationExpiringDays int    `json:"notification_expiring_days"`
	DataRetentionDays        int    `json:"data_retention_days"`
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}

	resp := systemSettingsResponse{
		AdminTokenMasked:         maskKey(settings["admin_token"]),
		HealthCheckInterval:      syscfg.ParsePositiveInt(settings["health_check_interval"], syscfg.DefaultHealthCheckInterval),
		RequestTimeout:           syscfg.ParsePositiveInt(settings["request_timeout"], syscfg.DefaultRequestTimeout),
		MaxRetryAttempts:         syscfg.ParsePositiveInt(settings["max_retry_attempts"], syscfg.DefaultMaxRetryAttempts),
		NotificationExpiringDays: syscfg.ParsePositiveInt(settings["notification_expiring_days"], syscfg.DefaultNotificationExpiringDays),
		DataRetentionDays:        syscfg.ParseNonNegativeInt(settings["data_retention_days"], syscfg.DefaultDataRetentionDays),
	}
	webutil.WriteData(w, 200, resp)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	pairs := make(map[string]string)
	for k, v := range raw {
		if isDeprecatedNotificationSetting(k) {
			continue
		}
		if !syscfg.IsAllowedSettingKey(k) {
			webutil.WriteAdminError(w, 400, "bad_request", fmt.Sprintf("unsupported setting key: %s", k))
			return
		}
		s := strings.TrimSpace(string(v))
		if s == "null" {
			webutil.WriteAdminError(w, 400, "bad_request", fmt.Sprintf("setting %s cannot be null", k))
			return
		}
		if s == "" {
			webutil.WriteAdminError(w, 400, "bad_request", fmt.Sprintf("setting %s cannot be empty", k))
			return
		}
		value, err := normalizeSettingValue(k, v)
		if err != nil {
			webutil.WriteAdminError(w, 400, "bad_request", err.Error())
			return
		}
		pairs[k] = value
	}
	if err := h.store.UpdateSettings(pairs); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func normalizeSettingValue(key string, raw json.RawMessage) (string, error) {
	var num float64
	if err := json.Unmarshal(raw, &num); err != nil {
		return "", fmt.Errorf("setting %s must be a number", key)
	}
	if num != float64(int(num)) {
		return "", fmt.Errorf("setting %s must be an integer", key)
	}
	value := int(num)
	switch key {
	case "health_check_interval", "request_timeout", "max_retry_attempts", "notification_expiring_days":
		if value <= 0 {
			return "", fmt.Errorf("setting %s must be greater than 0", key)
		}
	case "data_retention_days":
		if value < 0 {
			return "", fmt.Errorf("setting %s must be 0 or greater", key)
		}
	}
	return strconv.Itoa(value), nil
}

func isDeprecatedNotificationSetting(key string) bool {
	switch key {
	case "notification_error_enabled", "notification_expiring_enabled",
		"webhook_enabled", "webhook_url":
		return true
	default:
		return false
	}
}

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.store.ListSystemNotifications()
	if err != nil {
		h.internalError(w, err)
		return
	}
	if notifications == nil {
		notifications = []store.SystemNotification{}
	}
	webutil.WriteList(w, notifications, len(notifications))
}

func (h *Handler) getDataRetention(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}
	retentionDays := syscfg.ParseNonNegativeInt(settings["data_retention_days"], syscfg.DefaultDataRetentionDays)
	summary, err := h.store.GetDataRetentionSummary(retentionDays)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, summary)
}

func (h *Handler) previewDataRetention(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}
	retentionDays := syscfg.ParseNonNegativeInt(settings["data_retention_days"], syscfg.DefaultDataRetentionDays)
	preview, err := h.store.GetDataRetentionPreview(retentionDays)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, preview)
}

func (h *Handler) pruneDataRetention(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}
	retentionDays := syscfg.ParseNonNegativeInt(settings["data_retention_days"], syscfg.DefaultDataRetentionDays)
	deleted, err := h.store.PruneRequestLogs(retentionDays)
	if err != nil {
		h.internalError(w, err)
		return
	}
	deletedDeliveries, deletedOutbox, err := h.store.PruneNotificationHistory(retentionDays)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if retentionDays > 0 {
		if err := h.store.RecordPruneRun(deleted, deletedDeliveries, deletedOutbox); err != nil {
			h.internalError(w, err)
			return
		}
	}
	summary, err := h.store.GetDataRetentionSummary(retentionDays)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, map[string]any{
		"retention_days":                  summary.RetentionDays,
		"deleted_logs":                    deleted,
		"deleted_notification_deliveries": deletedDeliveries,
		"deleted_notification_outbox":     deletedOutbox,
		"total_logs":                      summary.TotalLogs,
		"oldest_log_at":                   summary.OldestLogAt,
		"newest_log_at":                   summary.NewestLogAt,
		"total_notification_deliveries":   summary.TotalNotificationDeliveries,
		"total_notification_outbox":       summary.TotalNotificationOutbox,
	})
}

// --- Overview, Usage, Export ---

func (h *Handler) getOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.store.GetOverview()
	if err != nil {
		h.internalError(w, err)
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

	if filter.TokenName == "" {
		filter.TokenName = r.URL.Query().Get("token")
	}
	filter.Model = r.URL.Query().Get("model")
	if acStr := r.URL.Query().Get("account"); acStr != "" {
		if v, err := strconv.ParseInt(acStr, 10, 64); err == nil && v > 0 {
			filter.AccountID = v
		}
	}

	if filter.From == "" && filter.To == "" {
		switch r.URL.Query().Get("range") {
		case "1h":
			filter.From = time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
		case "24h", "":
			filter.From = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
		case "7d":
			filter.From = time.Now().UTC().Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")
		case "30d":
			filter.From = time.Now().UTC().Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
		case "all":
			// no lower bound
		}
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			webutil.WriteAdminError(w, 400, "bad_request", "invalid page parameter")
			return
		}
		page = n
	}
	pageSize := 50
	if v := r.URL.Query().Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			webutil.WriteAdminError(w, 400, "bad_request", "invalid page_size parameter")
			return
		}
		pageSize = n
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			webutil.WriteAdminError(w, 400, "bad_request", "invalid limit parameter")
			return
		}
		pageSize = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			webutil.WriteAdminError(w, 400, "bad_request", "invalid offset parameter")
			return
		}
		filter.Offset = n
	} else {
		filter.Offset = (page - 1) * pageSize
	}
	filter.Limit = pageSize

	summary, err := h.store.GetUsageSummary(filter)
	if err != nil {
		h.internalError(w, err)
		return
	}
	logs, total, err := h.store.GetUsage(filter)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if logs == nil {
		logs = []store.RequestLog{}
	}
	resp := struct {
		*store.UsageStats
		Logs store.UsageLogPage `json:"logs"`
	}{
		UsageStats: summary,
		Logs: store.UsageLogPage{
			Items:    logs,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	}
	webutil.WriteData(w, 200, resp)
}

func (h *Handler) getExport(w http.ResponseWriter, r *http.Request) {
	includeSecrets := syscfg.ParseBool(r.URL.Query().Get("include_secrets"), false)

	accounts, err := h.store.ListAccounts()
	if err != nil {
		h.internalError(w, err)
		return
	}
	for i := range accounts {
		if includeSecrets {
			h.fillAccountResponseKeepSecret(&accounts[i])
		} else {
			h.fillAccountResponse(&accounts[i])
		}
	}
	pools, err := h.store.ListPools()
	if err != nil {
		h.internalError(w, err)
		return
	}
	poolLabelByID := make(map[int64]string, len(pools))
	for _, pool := range pools {
		poolLabelByID[pool.ID] = pool.Label
	}
	tokens, err := h.store.ListTokens()
	if err != nil {
		h.internalError(w, err)
		return
	}
	for i := range tokens {
		tokens[i].TokenMasked = maskKey(tokens[i].Token)
		if tokens[i].PoolID != nil {
			tokens[i].PoolLabel = poolLabelByID[*tokens[i].PoolID]
		}
		if !includeSecrets {
			tokens[i].Token = ""
		}
	}
	settings, err := h.store.GetSettings()
	if err != nil {
		h.internalError(w, err)
		return
	}
	// admin_token is the keys-to-the-kingdom for this gateway — it's
	// intentionally masked even in "complete" exports. A leaked file then
	// only leaks upstream provider/CPA credentials (which the user can
	// rotate out-of-band), not the control plane itself.
	if _, ok := settings["admin_token"]; ok {
		settings["admin_token"] = maskKey(settings["admin_token"])
	}
	cpaServices, err := h.store.ListCpaServices()
	if err != nil {
		h.internalError(w, err)
		return
	}
	for i := range cpaServices {
		cpaServices[i].APIKeyMasked = maskKey(cpaServices[i].APIKey)
		cpaServices[i].APIKeySet = cpaServices[i].APIKey != ""
		if !includeSecrets {
			cpaServices[i].APIKey = ""
			cpaServices[i].ManagementKey = ""
		}
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")
	filename := fmt.Sprintf("lune-export-%s-%s.json", stamp, hostname)

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	webutil.WriteData(w, 200, map[string]any{
		"schema_version":  exportSchemaVersion,
		"source_host":     hostname,
		"exported_at":     now.Format(time.RFC3339),
		"include_secrets": includeSecrets,
		"accounts":        accounts,
		"pools":           pools,
		"access_tokens":   tokens,
		"settings":        settings,
		"cpa_services":    cpaServices,
	})
}

// fillAccountResponseKeepSecret mirrors fillAccountResponse but leaves the
// plaintext APIKey in place so "complete" exports actually carry enough
// state to restore openai_compat accounts on a new machine. Only the
// export handler should use this — every other API path must scrub keys.
func (h *Handler) fillAccountResponseKeepSecret(a *store.Account) {
	a.APIKeyMasked = maskKey(a.APIKey)
	a.APIKeySet = a.APIKey != ""

	models, _ := h.store.ListAccountModels(a.ID)
	a.Models = models
	if a.Models == nil {
		a.Models = []string{}
	}
	if a.SourceKind == "cpa" && a.CpaServiceID != nil {
		svc := h.cache.GetCpaService(*a.CpaServiceID)
		if svc != nil {
			a.Runtime = &store.AccountRuntime{
				BaseURL:  svc.BaseURL,
				AuthMode: "cpa",
			}
		}
	}
}

const exportSchemaVersion = "3"

func (h *Handler) importConfig(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		Data *store.ConfigImportPayload `json:"data"`
		store.ConfigImportPayload
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}

	payload := raw.ConfigImportPayload
	if raw.Data != nil {
		payload = *raw.Data
	}
	if payload.SchemaVersion != "" && payload.SchemaVersion != exportSchemaVersion {
		webutil.WriteAdminError(
			w, 400, "schema_mismatch",
			fmt.Sprintf("file schema_version=%q is not compatible with %q", payload.SchemaVersion, exportSchemaVersion),
		)
		return
	}
	validatedSettings := make(map[string]string, len(payload.Settings))
	for key, value := range payload.Settings {
		if key == "admin_token" || !syscfg.IsAllowedSettingKey(key) {
			continue
		}
		normalizedValue, err := normalizeImportedSettingValue(key, value)
		if err != nil {
			webutil.WriteAdminError(w, 400, "bad_request", err.Error())
			return
		}
		validatedSettings[key] = normalizedValue
	}
	payload.Settings = validatedSettings

	result, err := h.store.ImportConfig(payload)
	if err != nil {
		webutil.WriteAdminError(w, 400, "import_failed", err.Error())
		return
	}

	h.cache.Invalidate()
	webutil.WriteData(w, 200, result)
}

func (h *Handler) importPreview(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		Data *store.ConfigImportPayload `json:"data"`
		store.ConfigImportPayload
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}

	payload := raw.ConfigImportPayload
	if raw.Data != nil {
		payload = *raw.Data
	}
	if payload.SchemaVersion != "" && payload.SchemaVersion != exportSchemaVersion {
		webutil.WriteAdminError(
			w, 400, "schema_mismatch",
			fmt.Sprintf("file schema_version=%q is not compatible with %q", payload.SchemaVersion, exportSchemaVersion),
		)
		return
	}
	// Preview runs the same validation pipeline as import, otherwise the
	// numbers wouldn't match reality.
	validatedSettings := make(map[string]string, len(payload.Settings))
	for key, value := range payload.Settings {
		if key == "admin_token" || !syscfg.IsAllowedSettingKey(key) {
			continue
		}
		normalizedValue, err := normalizeImportedSettingValue(key, value)
		if err != nil {
			webutil.WriteAdminError(w, 400, "bad_request", err.Error())
			return
		}
		validatedSettings[key] = normalizedValue
	}
	payload.Settings = validatedSettings

	preview, err := h.store.PreviewImport(payload)
	if err != nil {
		webutil.WriteAdminError(w, 400, "preview_failed", err.Error())
		return
	}
	webutil.WriteData(w, 200, preview)
}

func normalizeImportedSettingValue(key, value string) (string, error) {
	value = strings.TrimSpace(value)
	n, err := strconv.Atoi(value)
	if err != nil {
		return "", fmt.Errorf("setting %s must be an integer", key)
	}
	switch key {
	case "health_check_interval", "request_timeout", "max_retry_attempts", "notification_expiring_days":
		if n <= 0 {
			return "", fmt.Errorf("setting %s must be greater than 0", key)
		}
	case "data_retention_days":
		if n < 0 {
			return "", fmt.Errorf("setting %s must be 0 or greater", key)
		}
	}
	return strconv.Itoa(n), nil
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
	var poolID int64
	if acStr := r.URL.Query().Get("account"); acStr != "" {
		if v, err := strconv.ParseInt(acStr, 10, 64); err == nil {
			opts = append(opts, v)
		}
	}
	if poolStr := r.URL.Query().Get("pool"); poolStr != "" {
		if v, err := strconv.ParseInt(poolStr, 10, 64); err == nil {
			poolID = v
		}
	}

	var accountID int64
	if len(opts) > 0 {
		accountID = opts[0]
	}

	stats, err := h.store.GetLatencyStats(model, period, bucket, accountID, poolID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	webutil.WriteData(w, 200, stats)
}

// --- CPA Service ---

func (h *Handler) getCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		h.internalError(w, err)
		return
	}
	if svc == nil {
		webutil.WriteData(w, 200, nil)
		return
	}
	svc.APIKeyMasked = maskKey(svc.APIKey)
	svc.APIKeySet = svc.APIKey != ""
	svc.APIKey = ""
	svc.ManagementKey = ""
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
	req.Label = strings.TrimSpace(req.Label)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.Label == "" || req.BaseURL == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "label and base_url are required")
		return
	}

	existing, err := h.store.GetCpaService()
	if err != nil {
		h.internalError(w, err)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if existing != nil {
		svc := &store.CpaService{
			Label:         req.Label,
			BaseURL:       strings.TrimRight(req.BaseURL, "/"),
			APIKey:        req.APIKey,
			ManagementKey: existing.ManagementKey,
			Enabled:       enabled,
		}
		if req.APIKey == "" {
			svc.APIKey = existing.APIKey
		}
		if err := h.store.UpdateCpaService(existing.ID, svc); err != nil {
			h.internalError(w, err)
			return
		}
		h.cache.Invalidate()
		svc.ID = existing.ID
		svc.APIKeyMasked = maskKey(svc.APIKey)
		svc.APIKeySet = svc.APIKey != ""
		svc.APIKey = ""
		svc.ManagementKey = ""
		webutil.WriteData(w, 200, svc)
	} else {
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
			h.internalError(w, err)
			return
		}
		h.cache.Invalidate()
		svc.ID = id
		svc.APIKeyMasked = maskKey(svc.APIKey)
		svc.APIKeySet = svc.APIKey != ""
		svc.APIKey = ""
		svc.ManagementKey = ""
		webutil.WriteData(w, 201, svc)
	}
}

func (h *Handler) deleteCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		h.internalError(w, err)
		return
	}
	if svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}
	count, err := h.store.CountAccountsByCpaService(svc.ID)
	if err != nil {
		h.internalError(w, err)
		return
	}
	if count > 0 {
		webutil.WriteAdminError(w, 409, "has_dependent_accounts",
			fmt.Sprintf("Cannot delete CPA service: %d accounts are linked to it. Remove them first.", count))
		return
	}
	if err := h.store.DeleteCpaService(svc.ID); err != nil {
		h.internalError(w, err)
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

	modelsReq, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		result.Error = fmt.Sprintf("invalid base_url: %v", err)
		webutil.WriteData(w, 200, result)
		return
	}
	if req.APIKey != "" {
		modelsReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	start := time.Now()
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

	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
			return
		}
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

	healthReq, err := http.NewRequest("GET", baseURL+"/healthz", nil)
	if err != nil {
		result.Error = fmt.Sprintf("invalid base_url: %v", err)
		webutil.WriteData(w, 200, result)
		return
	}
	if req.APIKey != "" {
		healthReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	start := time.Now()
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

	modelsReq, err := http.NewRequest("GET", baseURL+"/v1/models", nil)
	if err != nil {
		// Reachable is already true from /healthz; skip the enrichment step
		// and return what we have rather than crashing on a nil request.
		webutil.WriteData(w, 200, result)
		return
	}
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
		h.internalError(w, err)
		return
	}
	if svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}
	if err := h.store.EnableCpaService(svc.ID); err != nil {
		h.internalError(w, err)
		return
	}
	h.cache.Invalidate()
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) disableCpaService(w http.ResponseWriter, r *http.Request) {
	svc, err := h.store.GetCpaService()
	if err != nil {
		h.internalError(w, err)
		return
	}
	if svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "no CPA service configured")
		return
	}
	if err := h.store.DisableCpaService(svc.ID); err != nil {
		h.internalError(w, err)
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

	// Fill models from account_models
	models, _ := h.store.ListAccountModels(a.ID)
	a.Models = models
	if a.Models == nil {
		a.Models = []string{}
	}

	// Fill runtime for CPA accounts
	if a.SourceKind == "cpa" && a.CpaServiceID != nil {
		svc := h.cache.GetCpaService(*a.CpaServiceID)
		if svc != nil {
			a.Runtime = &store.AccountRuntime{
				BaseURL:  strings.TrimRight(svc.BaseURL, "/") + "/api/provider/" + a.CpaProvider + "/v1",
				AuthMode: "cpa",
			}
		}
	} else {
		a.Runtime = &store.AccountRuntime{
			BaseURL:  a.BaseURL,
			AuthMode: "bearer",
		}
	}
}

// --- CPA Login Sessions (Device Code Flow) ---

func (h *Handler) createLoginSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceID int64  `json:"service_id"`
		PoolID    int64  `json:"pool_id"`
		Provider  string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.PoolID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "pool_id is required")
		return
	}
	if req.Provider == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "provider is required")
		return
	}

	svc, err := h.store.GetCpaServiceByID(req.ServiceID)
	if err != nil || svc == nil {
		webutil.WriteAdminError(w, 404, "not_found", "CPA service not found")
		return
	}

	if h.cpaAuthDir == "" {
		webutil.WriteAdminError(w, 400, "not_configured", "cpa_auth_dir is not configured")
		return
	}

	if existing := h.sessions.GetActiveSession(req.ServiceID); existing != nil {
		if existing.PoolID == 0 && req.PoolID > 0 {
			h.sessions.UpdatePoolID(existing.ID, req.PoolID)
			existing.PoolID = req.PoolID
		}
		webutil.WriteData(w, 200, h.loginSessionResponse(existing))
		return
	}

	dcr, err := cpa.RequestDeviceCode(r.Context())
	if err != nil {
		webutil.WriteAdminError(w, 502, "upstream_error", fmt.Sprintf("device code request failed: %v", err))
		return
	}

	session, err := h.sessions.CreateSession(req.ServiceID, req.Provider, dcr)
	if err != nil {
		webutil.WriteAdminError(w, 409, "active_session", err.Error())
		return
	}
	session.PoolID = req.PoolID
	h.sessions.UpdatePoolID(session.ID, req.PoolID)

	ctx, cancel := context.WithDeadline(context.Background(), session.ExpiresAt)
	session.CancelFunc = cancel

	go h.pollLoginSession(ctx, session, svc)

	webutil.WriteData(w, 201, h.loginSessionResponse(session))
}

func (h *Handler) getActiveLoginSession(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.URL.Query().Get("service_id"), 10, 64)
	if err != nil || serviceID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "service_id is required")
		return
	}

	session := h.sessions.GetActiveSession(serviceID)
	if session == nil {
		webutil.WriteAdminError(w, 404, "not_found", "active login session not found")
		return
	}

	webutil.WriteData(w, 200, h.loginSessionResponse(session))
}

func (h *Handler) pollLoginSession(ctx context.Context, session *cpa.LoginSession, svc *store.CpaService) {
	interval := time.Duration(session.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
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

			tokenResp, err := cpa.PollForToken(ctx, session.DeviceAuthID, session.UserCode)
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
				if ctx.Err() == nil && isTransientCPAPollError(err) {
					slog.Warn("cpa login poll transient error", "session_id", session.ID, "err", err)
					continue
				}
				h.sessions.UpdateStatus(session.ID, "failed", "poll_error", err.Error())
				return
			}

			h.sessions.UpdateStatus(session.ID, "authorized", "", "")
			h.finalizeLogin(session, svc, tokenResp)
			return
		}
	}
}

func isTransientCPAPollError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	msg := err.Error()
	return strings.Contains(msg, "(HTTP 408)") ||
		strings.Contains(msg, "(HTTP 429)") ||
		strings.Contains(msg, "(HTTP 500)") ||
		strings.Contains(msg, "(HTTP 502)") ||
		strings.Contains(msg, "(HTTP 503)") ||
		strings.Contains(msg, "(HTTP 504)")
}

func (h *Handler) finalizeLogin(session *cpa.LoginSession, svc *store.CpaService, tokenResp *cpa.TokenResponse) {
	info, err := cpa.ParseAccountInfoFromTokens(tokenResp.IDToken, tokenResp.AccessToken)
	if err != nil {
		h.sessions.UpdateStatus(session.ID, "failed", "jwt_parse_error", fmt.Sprintf("Failed to parse JWT: %v", err))
		return
	}

	planType := info.PlanType
	if planType == "" {
		planType = "unknown"
	}
	provider := session.Provider
	if provider == "" {
		provider = "codex"
	}
	accountKey := fmt.Sprintf("%s-%s-%s", provider, info.Email, planType)

	expiredAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	lastRefresh := time.Now().Format(time.RFC3339)

	authFile := &cpa.CpaAuthFile{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		AccountID:    info.AccountID,
		Email:        info.Email,
		Disabled:     false,
		Expired:      expiredAt,
		LastRefresh:  lastRefresh,
		Type:         provider,
	}
	if err := cpa.WriteAuthFile(h.cpaAuthDir, authFile, accountKey); err != nil {
		h.sessions.UpdateStatus(session.ID, "failed", "file_write_error", fmt.Sprintf("Failed to write credential file: %v", err))
		return
	}

	existingBefore, _ := h.store.FindAccountByCpaKey(svc.ID, accountKey)
	account, err := h.upsertImportedCpaAccount(svc, accountKey, authFile, "", true, "")
	if err != nil {
		h.sessions.UpdateStatus(session.ID, "failed", "import_error", fmt.Sprintf("Failed to create account: %v", err))
		return
	}

	// auto-add to the pool specified in session
	if session.PoolID > 0 {
		if _, err := h.store.AddPoolMember(session.PoolID, account.ID); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				slog.Info("finalizeLogin: pool member already exists", "pool_id", session.PoolID, "account_id", account.ID)
			} else {
				if existingBefore == nil {
					if delErr := h.store.DeleteAccount(account.ID); delErr != nil {
						slog.Error("finalizeLogin: rollback orphan account failed", "account_id", account.ID, "err", delErr)
					}
				}
				h.sessions.UpdateStatus(session.ID, "failed", "pool_attach_error", fmt.Sprintf("Failed to add account to pool: %v", err))
				return
			}
		}
	}

	// trigger async model discovery
	if acc, err := h.store.GetAccount(account.ID); err == nil && acc != nil {
		go h.healthChecker.DiscoverModels(context.Background(), *acc)
	}

	h.sessions.CompleteSession(session.ID, account.ID, account)
	h.cache.Invalidate()
}

func (h *Handler) getLoginSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session := h.sessions.GetSession(id)
	if session == nil {
		webutil.WriteAdminError(w, 404, "not_found", "login session not found")
		return
	}
	webutil.WriteData(w, 200, h.loginSessionResponse(session))
}

func (h *Handler) cancelLoginSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.sessions.CancelSession(id); err != nil {
		webutil.WriteAdminError(w, 404, "not_found", err.Error())
		return
	}
	webutil.WriteData(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) loginSessionResponse(session *cpa.LoginSession) map[string]any {
	resp := map[string]any{
		"id":         session.ID,
		"service_id": session.ServiceID,
		"pool_id":    session.PoolID,
		"provider":   session.Provider,
		"status":     session.Status,
	}

	switch session.Status {
	case "pending", "authorized":
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
		resp["expires_at"] = session.ExpiresAt.Format(time.RFC3339)
		resp["error_code"] = session.ErrorCode
		resp["error_message"] = session.ErrorMessage
	case "cancelled":
		resp["expires_at"] = session.ExpiresAt.Format(time.RFC3339)
	}

	return resp
}

func (h *Handler) resumeActiveLoginSessions() {
	for _, session := range h.sessions.ListActiveSessions() {
		if session.ExpiresAt.Before(time.Now()) {
			h.sessions.UpdateStatus(session.ID, "expired", "expired_token", "Device code expired")
			continue
		}

		svc, err := h.store.GetCpaServiceByID(session.ServiceID)
		if err != nil || svc == nil {
			h.sessions.UpdateStatus(session.ID, "failed", "service_missing", "CPA service not found")
			continue
		}

		ctx, cancel := context.WithDeadline(context.Background(), session.ExpiresAt)
		session.CancelFunc = cancel
		go h.pollLoginSession(ctx, session, svc)
	}
}

func (h *Handler) upsertImportedCpaAccount(svc *store.CpaService, accountKey string, f *cpa.CpaAuthFile, label string, enabled bool, notes string) (*store.Account, error) {
	planType := ""
	openaiID := f.AccountID
	if info, err := cpa.ParseAccountInfoFromTokens(f.IDToken, f.AccessToken); err == nil {
		planType = info.PlanType
		if info.AccountID != "" {
			openaiID = info.AccountID
		}
	}
	if label == "" {
		label = fmt.Sprintf("%s - %s (%s)", f.Type, f.Email, planType)
	}

	var expiredAt, lastRefreshAt string
	if f.Expired != "" {
		expiredAt = f.Expired
	}
	if f.LastRefresh != "" {
		lastRefreshAt = f.LastRefresh
	}

	account := &store.Account{
		Label:            label,
		SourceKind:       "cpa",
		CpaServiceID:     &svc.ID,
		CpaProvider:      f.Type,
		CpaAccountKey:    accountKey,
		CpaEmail:         f.Email,
		CpaPlanType:      planType,
		CpaOpenaiID:      openaiID,
		CpaExpiredAt:     expiredAt,
		CpaLastRefreshAt: lastRefreshAt,
		CpaDisabled:      f.Disabled,
		Enabled:          enabled,
		Notes:            notes,
	}

	id, err := h.store.CreateAccount(account)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			existing, findErr := h.store.FindAccountByCpaKey(svc.ID, accountKey)
			if findErr == nil && existing != nil {
				h.fillAccountResponse(existing)
				return existing, nil
			}
		}
		return nil, err
	}

	account.ID = id
	h.fillAccountResponse(account)
	return account, nil
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

		planType := ""
		if info, err := cpa.ParseAccountInfoFromTokens(s.File.IDToken, s.File.AccessToken); err == nil {
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
		ServiceID  int64  `json:"service_id"`
		AccountKey string `json:"account_key"`
		Label      string `json:"label"`
		Enabled    bool   `json:"enabled"`
		Notes      string `json:"notes"`
		PoolID     int64  `json:"pool_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.AccountKey == "" {
		webutil.WriteAdminError(w, 400, "bad_request", "account_key is required")
		return
	}
	if req.PoolID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "pool_id is required")
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

	existingBefore, err := h.store.FindAccountByCpaKey(svc.ID, req.AccountKey)
	if err != nil {
		h.internalError(w, err)
		return
	}

	account, err := h.upsertImportedCpaAccount(svc, req.AccountKey, f, req.Label, req.Enabled, req.Notes)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			webutil.WriteAdminError(w, 409, "duplicate", "This account has already been imported")
			return
		}
		h.internalError(w, err)
		return
	}

	// add to pool
	if _, err := h.store.AddPoolMember(req.PoolID, account.ID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			slog.Info("importCpaAccount: pool member already exists", "pool_id", req.PoolID, "account_id", account.ID)
		} else {
			if existingBefore == nil {
				if delErr := h.store.DeleteAccount(account.ID); delErr != nil {
					slog.Error("importCpaAccount: rollback orphan account failed", "account_id", account.ID, "err", delErr)
				}
			}
			webutil.WriteAdminError(w, 500, "internal", "failed to add imported account to pool")
			return
		}
	}

	// trigger async model discovery
	if acc, err := h.store.GetAccount(account.ID); err == nil && acc != nil {
		go h.healthChecker.DiscoverModels(context.Background(), *acc)
	}

	h.cache.Invalidate()
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
		PoolID      int64    `json:"pool_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.WriteAdminError(w, 400, "bad_request", "invalid JSON")
		return
	}
	if req.PoolID == 0 {
		webutil.WriteAdminError(w, 400, "bad_request", "pool_id is required")
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
		if info, err := cpa.ParseAccountInfoFromTokens(f.IDToken, f.AccessToken); err == nil {
			planType = info.PlanType
			if info.AccountID != "" {
				openaiID = info.AccountID
			}
		}

		var expiredAt, lastRefreshAt string
		if f.Expired != "" {
			expiredAt = f.Expired
		}
		if f.LastRefresh != "" {
			lastRefreshAt = f.LastRefresh
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

		accountID, err := h.store.CreateAccount(account)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				skipped++
			} else {
				errs = append(errs, fmt.Sprintf("%s: %v", key, err))
			}
			continue
		}

		// add to pool
		if _, err := h.store.AddPoolMember(req.PoolID, accountID); err != nil {
			if delErr := h.store.DeleteAccount(accountID); delErr != nil {
				slog.Error("batchImportCpaAccounts: rollback orphan account failed", "account_id", accountID, "err", delErr)
			}
			errs = append(errs, fmt.Sprintf("%s: failed to add imported account to pool: %v", key, err))
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
