package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"lune/internal/auth"
	"lune/internal/config"
	"lune/internal/metrics"
	"lune/internal/platform"
	"lune/internal/runtimeconfig"
	"lune/internal/store"
	"lune/internal/webutil"
)

const (
	defaultPlatformID   = "upstream"
	defaultModelAlias   = "default-gpt"
	defaultTargetModel  = "gpt-4o"
	defaultPoolStrategy = "sticky-first-healthy"
)

type Handler struct {
	config   *runtimeconfig.Manager
	store    *store.Store
	metrics  *metrics.Collector
	registry *platform.Registry
}

type overviewPayload struct {
	NeedsBootstrap    bool    `json:"needs_bootstrap"`
	AccountsTotal     int     `json:"accounts_total"`
	ActiveAccounts    int     `json:"active_accounts"`
	PoolsTotal        int     `json:"pools_total"`
	APIKeysTotal      int     `json:"api_keys_total"`
	DefaultModelAlias string  `json:"default_model_alias"`
	TotalRequests     int64   `json:"total_requests"`
	SuccessRequests   int64   `json:"success_requests"`
	FailedRequests    int64   `json:"failed_requests"`
	SuccessRate       float64 `json:"success_rate"`
	AverageLatencyMS  float64 `json:"average_latency_ms"`
	RecentFailures    int     `json:"recent_failures"`
}

type upsertAccountRequest struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	CredentialType string `json:"credential_type"`
	CredentialEnv  string `json:"credential_env"`
	EgressProxyEnv string `json:"egress_proxy_env"`
	PlanType       string `json:"plan_type"`
	Enabled        bool   `json:"enabled"`
	Status         string `json:"status"`
}

type upsertPoolRequest struct {
	ID       string   `json:"id"`
	Strategy string   `json:"strategy"`
	Enabled  bool     `json:"enabled"`
	Members  []string `json:"members"`
}

type upsertAPIKeyRequest struct {
	Name           string `json:"name"`
	Token          string `json:"token"`
	Enabled        bool   `json:"enabled"`
	QuotaCalls     int64  `json:"quota_calls"`
	CostPerRequest int64  `json:"cost_per_request"`
}

type apiKeyView struct {
	Name           string `json:"name"`
	MaskedToken    string `json:"masked_token"`
	Enabled        bool   `json:"enabled"`
	QuotaCalls     int64  `json:"quota_calls"`
	UsedCalls      int64  `json:"used_calls"`
	RemainingCalls int64  `json:"remaining_calls"`
	CostPerRequest int64  `json:"cost_per_request"`
}

type usageSummary struct {
	TotalEntries     int64            `json:"total_entries"`
	Successful       int64            `json:"successful"`
	Failed           int64            `json:"failed"`
	ByAccount        map[string]int64 `json:"by_account"`
	ByToken          map[string]int64 `json:"by_token"`
	LatestRequestIDs []string         `json:"latest_request_ids"`
}

func NewHandler(cfg *runtimeconfig.Manager, st *store.Store, metricCollector *metrics.Collector, registry *platform.Registry) *Handler {
	return &Handler{
		config:   cfg,
		store:    st,
		metrics:  metricCollector,
		registry: registry,
	}
}

func (h *Handler) Route(w http.ResponseWriter, r *http.Request) {
	clean := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case clean == "/admin":
		h.Dashboard(w, r)
	case clean == "/admin/api/overview":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.OverviewAPI)).ServeHTTP(w, r)
	case clean == "/admin/api/accounts":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.AccountsAPI)).ServeHTTP(w, r)
	case strings.HasPrefix(clean, "/admin/api/accounts/"):
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.AccountActionsAPI)).ServeHTTP(w, r)
	case clean == "/admin/api/pools":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.PoolsAPI)).ServeHTTP(w, r)
	case clean == "/admin/api/api-keys":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.APIKeysAPI)).ServeHTTP(w, r)
	case clean == "/admin/api/logs":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Logs)).ServeHTTP(w, r)
	case clean == "/admin/api/usage":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.UsageAPI)).ServeHTTP(w, r)
	case clean == "/admin/api/config":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Config)).ServeHTTP(w, r)
	case clean == "/admin/api/config/validate":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.ValidateConfig)).ServeHTTP(w, r)
	case clean == "/admin/overview":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.OverviewAPI)).ServeHTTP(w, r)
	case clean == "/admin/platforms":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Platforms)).ServeHTTP(w, r)
	case clean == "/admin/models":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Models)).ServeHTTP(w, r)
	case clean == "/admin/tokens":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.LegacyTokens)).ServeHTTP(w, r)
	case clean == "/admin/accounts":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.LegacyAccounts)).ServeHTTP(w, r)
	case clean == "/admin/account-pools":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.LegacyAccountPools)).ServeHTTP(w, r)
	case clean == "/admin/logs":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Logs)).ServeHTTP(w, r)
	case clean == "/admin/metrics":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Metrics)).ServeHTTP(w, r)
	case clean == "/admin/test":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.TestPlatforms)).ServeHTTP(w, r)
	case clean == "/admin/config":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.Config)).ServeHTTP(w, r)
	case clean == "/admin/config/validate":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.ValidateConfig)).ServeHTTP(w, r)
	case clean == "/admin/config/schema":
		auth.RequireAdminFunc(h.adminToken, http.HandlerFunc(h.ConfigSchema)).ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) adminToken() string {
	return h.config.Current().Auth.AdminToken
}

func (h *Handler) currentConfig() config.Config {
	return h.config.Current()
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.Redirect(w, r, "/admin", http.StatusPermanentRedirect)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) OverviewAPI(w http.ResponseWriter, r *http.Request) {
	cfg := h.currentConfig()
	metricSnapshot := metrics.Snapshot{}
	if h.metrics != nil {
		metricSnapshot = h.metrics.Snapshot()
	}
	logs, _ := h.listLogs(r.Context(), 25)
	recentFailures := 0
	for _, item := range logs {
		if !item.Success {
			recentFailures++
		}
	}

	successRate := 0.0
	if metricSnapshot.TotalRequests > 0 {
		successRate = float64(metricSnapshot.Successful) / float64(metricSnapshot.TotalRequests)
	}

	activeAccounts := 0
	for _, account := range cfg.Accounts {
		if account.Enabled && isRunnableAccountStatus(account.Status) {
			activeAccounts++
		}
	}

	overview := overviewPayload{
		NeedsBootstrap:    len(cfg.Accounts) == 0 || len(cfg.AccountPools) == 0 || len(cfg.Auth.AccessTokens) == 0,
		AccountsTotal:     len(cfg.Accounts),
		ActiveAccounts:    activeAccounts,
		PoolsTotal:        len(cfg.AccountPools),
		APIKeysTotal:      len(cfg.Auth.AccessTokens),
		DefaultModelAlias: h.defaultModelAlias(cfg),
		TotalRequests:     metricSnapshot.TotalRequests,
		SuccessRequests:   metricSnapshot.Successful,
		FailedRequests:    metricSnapshot.Failed,
		SuccessRate:       successRate,
		AverageLatencyMS:  metricSnapshot.AverageLatencyMS,
		RecentFailures:    recentFailures,
	}

	webutil.WriteJSON(w, http.StatusOK, map[string]any{"overview": overview})
}

func (h *Handler) Platforms(w http.ResponseWriter, r *http.Request) {
	statuses := []platform.Status(nil)
	if h.registry != nil {
		statuses = h.registry.Snapshot()
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"platforms": statuses,
	})
}

func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"models": h.currentConfig().Models,
	})
}

func (h *Handler) LegacyTokens(w http.ResponseWriter, r *http.Request) {
	items, err := h.apiKeyViews(r.Context())
	if err != nil {
		webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{"tokens": items})
}

func (h *Handler) LegacyAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.accountViews(r.Context())
	if err != nil {
		webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{"accounts": items})
}

func (h *Handler) LegacyAccountPools(w http.ResponseWriter, r *http.Request) {
	items, err := h.poolViews(r.Context())
	if err != nil {
		webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{"account_pools": items})
}

func (h *Handler) AccountsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.accountViews(r.Context())
		if err != nil {
			webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{
			"accounts": items,
		})
	case http.MethodPost:
		var req upsertAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "账号 JSON 不合法"})
			return
		}
		applied, err := h.applyAccountMutation(r.Context(), req, true)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "账号已创建", "config": applied})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) AccountActionsAPI(w http.ResponseWriter, r *http.Request) {
	clean := strings.TrimSuffix(r.URL.Path, "/")
	rest := strings.TrimPrefix(clean, "/admin/api/accounts/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req upsertAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "账号 JSON 不合法"})
			return
		}
		req.ID = id
		applied, err := h.applyAccountMutation(r.Context(), req, false)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "账号已更新", "config": applied})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	switch parts[1] {
	case "disable":
		applied, err := h.toggleAccountEnabled(r.Context(), id, false)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "账号已停用", "config": applied})
	case "enable":
		applied, err := h.toggleAccountEnabled(r.Context(), id, true)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "账号已启用", "config": applied})
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) PoolsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.poolViews(r.Context())
		if err != nil {
			webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"pools": items})
	case http.MethodPost:
		var req upsertPoolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "号池 JSON 不合法"})
			return
		}
		applied, err := h.applyPoolMutation(r.Context(), req, true)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "号池已创建", "config": applied})
	case http.MethodPut:
		var req upsertPoolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "号池 JSON 不合法"})
			return
		}
		applied, err := h.applyPoolMutation(r.Context(), req, false)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "号池已更新", "config": applied})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) APIKeysAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.apiKeyViews(r.Context())
		if err != nil {
			webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"api_keys": items})
	case http.MethodPost:
		var req upsertAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "API key JSON 不合法"})
			return
		}
		applied, err := h.applyAPIKeyMutation(r.Context(), req, true)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "API key 已创建", "config": applied})
	case http.MethodPut:
		var req upsertAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "API key JSON 不合法"})
			return
		}
		applied, err := h.applyAPIKeyMutation(r.Context(), req, false)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"message": "API key 已更新", "config": applied})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	logs, err := h.listLogs(r.Context(), 200)
	if err != nil {
		webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (h *Handler) UsageAPI(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		webutil.WriteJSON(w, http.StatusOK, map[string]any{"usage": usageSummary{ByAccount: map[string]int64{}, ByToken: map[string]int64{}}})
		return
	}
	entries, err := h.store.ListUsageLedgerEntries(r.Context(), 200)
	if err != nil {
		webutil.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	payload := usageSummary{
		ByAccount: map[string]int64{},
		ByToken:   map[string]int64{},
	}
	for _, entry := range entries {
		payload.TotalEntries++
		if entry.Success {
			payload.Successful++
		} else {
			payload.Failed++
		}
		payload.ByAccount[entry.AccountID] += entry.AccountCostUnits
		payload.ByToken[entry.AccessTokenName] += entry.APICostUnits
		if len(payload.LatestRequestIDs) < 10 {
			payload.LatestRequestIDs = append(payload.LatestRequestIDs, entry.RequestID)
		}
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{"usage": payload})
}

func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	overview := overviewPayload{}
	cfg := h.currentConfig()
	overview.AccountsTotal = len(cfg.Accounts)
	if h.metrics != nil {
		metricSnapshot := h.metrics.Snapshot()
		overview.TotalRequests = metricSnapshot.TotalRequests
		overview.SuccessRequests = metricSnapshot.Successful
		overview.FailedRequests = metricSnapshot.Failed
		overview.AverageLatencyMS = metricSnapshot.AverageLatencyMS
		if metricSnapshot.TotalRequests > 0 {
			overview.SuccessRate = float64(metricSnapshot.Successful) / float64(metricSnapshot.TotalRequests)
		}
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{"metrics": overview})
}

func (h *Handler) TestPlatforms(w http.ResponseWriter, r *http.Request) {
	if h.registry != nil {
		h.registry.CheckAll(r.Context())
	}
	h.Platforms(w, r)
}

func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		webutil.WriteJSON(w, http.StatusOK, map[string]any{
			"config": h.currentConfig(),
		})
	case http.MethodPut:
		var candidate config.Config
		if err := json.NewDecoder(r.Body).Decode(&candidate); err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "配置 JSON 不合法"})
			return
		}
		candidate = normalizeControlPlaneConfig(candidate)
		applied, err := h.config.Apply(r.Context(), candidate)
		if err != nil {
			webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": humanizeConfigError(err)})
			return
		}
		h.refreshRegistry(r.Context())
		webutil.WriteJSON(w, http.StatusOK, map[string]any{
			"message": "配置已保存并热重载",
			"config":  applied,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) ValidateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var candidate config.Config
	if err := json.NewDecoder(r.Body).Decode(&candidate); err != nil {
		webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "配置 JSON 不合法"})
		return
	}

	prepared, err := h.config.ValidateOnly(normalizeControlPlaneConfig(candidate))
	if err != nil {
		webutil.WriteJSON(w, http.StatusBadRequest, map[string]any{
			"valid": false,
			"error": humanizeConfigError(err),
		})
		return
	}
	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"valid":  true,
		"config": prepared,
	})
}

func (h *Handler) ConfigSchema(w http.ResponseWriter, r *http.Request) {
	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"defaults": map[string]any{
			"server": map[string]any{
				"port":                              7788,
				"stream_smoothing":                  true,
				"stream_heartbeat":                  true,
				"request_timeout_seconds":           120,
				"shutdown_timeout_seconds":          10,
				"data_dir":                          "data",
				"platform_refresh_interval_seconds": 60,
			},
			"account": map[string]any{
				"credential_type": "api_key",
				"plan_type":       "plus",
				"enabled":         true,
				"status":          "healthy",
				"risk_score":      0,
			},
			"account_pool": map[string]any{
				"strategy": defaultPoolStrategy,
				"enabled":  true,
			},
			"api_key": map[string]any{
				"enabled":          true,
				"quota_calls":      1000,
				"cost_per_request": 1,
			},
		},
		"enums": map[string]any{
			"credential_types":   []string{"api_key"},
			"account_statuses":   []string{"healthy", "ready", "active", "disabled", "degraded", "blocked"},
			"account_plan_types": []string{"free", "plus", "pro", "team"},
			"pool_strategies":    []string{defaultPoolStrategy, "single", "fallback"},
		},
		"help": map[string]string{
			"account.credential_env": "填写环境变量名，不是凭据明文。例如 UPSTREAM_API_KEY，值为 upstream 引擎的 API key。",
			"account_pool.strategy":  "建议使用 sticky-first-healthy，优先复用最近可用账号。",
			"api_key.quota_calls":    "额度以本地调用次数估算，不依赖上游官方余额接口。",
		},
	})
}

func (h *Handler) accountViews(ctx context.Context) ([]store.AccountRecord, error) {
	if h.store != nil {
		if items, err := h.store.ListAccounts(ctx); err == nil {
			return items, nil
		}
	}
	cfg := h.currentConfig()
	items := make([]store.AccountRecord, 0, len(cfg.Accounts))
	now := time.Now().UTC()
	for _, account := range cfg.Accounts {
		items = append(items, store.AccountRecord{
			ID:             account.ID,
			PlatformID:     account.Platform,
			Label:          account.Label,
			CredentialType: account.CredentialType,
			CredentialEnv:  account.CredentialEnv,
			EgressProxyEnv: account.EgressProxyEnv,
			PlanType:       account.PlanType,
			Enabled:        account.Enabled,
			Status:         account.Status,
			RiskScore:      account.RiskScore,
			UpdatedAt:      now,
		})
	}
	return items, nil
}

func (h *Handler) poolViews(ctx context.Context) ([]store.AccountPoolRecord, error) {
	if h.store != nil {
		if items, err := h.store.ListAccountPools(ctx); err == nil {
			return items, nil
		}
	}
	cfg := h.currentConfig()
	items := make([]store.AccountPoolRecord, 0, len(cfg.AccountPools))
	now := time.Now().UTC()
	for _, pool := range cfg.AccountPools {
		items = append(items, store.AccountPoolRecord{
			ID:         pool.ID,
			PlatformID: pool.Platform,
			Strategy:   pool.Strategy,
			Enabled:    pool.Enabled,
			Members:    append([]string(nil), pool.Members...),
			UpdatedAt:  now,
		})
	}
	return items, nil
}

func (h *Handler) apiKeyViews(ctx context.Context) ([]apiKeyView, error) {
	cfg := h.currentConfig()
	used := map[string]store.TokenAccount{}
	if h.store != nil {
		items, err := h.store.ListTokenAccounts(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			used[item.Name] = item
		}
	}
	views := make([]apiKeyView, 0, len(cfg.Auth.AccessTokens))
	for _, token := range cfg.Auth.AccessTokens {
		ledger := used[token.Name]
		remaining := int64(-1)
		if ledger.Name != "" {
			remaining = ledger.RemainingCalls()
		}
		if ledger.Name == "" && token.QuotaCalls > 0 {
			remaining = token.QuotaCalls
		}
		views = append(views, apiKeyView{
			Name:           token.Name,
			MaskedToken:    maskToken(token.Token),
			Enabled:        token.Enabled,
			QuotaCalls:     token.QuotaCalls,
			UsedCalls:      ledger.UsedCalls,
			RemainingCalls: remaining,
			CostPerRequest: token.CostPerRequest,
		})
	}
	return views, nil
}

func (h *Handler) listLogs(ctx context.Context, limit int) ([]store.RequestLog, error) {
	if h.store == nil {
		return []store.RequestLog{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return h.store.ListRequestLogs(ctx, limit)
}

func (h *Handler) applyAccountMutation(ctx context.Context, req upsertAccountRequest, creating bool) (config.Config, error) {
	if strings.TrimSpace(req.Label) == "" {
		return config.Config{}, fmt.Errorf("账号标签不能为空")
	}
	if strings.TrimSpace(req.CredentialEnv) == "" {
		return config.Config{}, fmt.Errorf("凭据环境变量不能为空")
	}
	return h.mutateConfig(ctx, func(cfg *config.Config) error {
		cfg.Platforms = ensureDefaultPlatform(cfg.Platforms)
		account := config.Account{
			ID:             normalizeID(req.ID, req.Label),
			Platform:       defaultPlatformID,
			Label:          strings.TrimSpace(req.Label),
			CredentialType: defaultString(req.CredentialType, "api_key"),
			CredentialEnv:  strings.TrimSpace(req.CredentialEnv),
			EgressProxyEnv: strings.TrimSpace(req.EgressProxyEnv),
			PlanType:       defaultString(req.PlanType, "plus"),
			Enabled:        req.Enabled,
			Status:         defaultString(req.Status, "healthy"),
			RiskScore:      0,
		}
		if creating {
			if findAccountIndex(cfg.Accounts, account.ID) >= 0 {
				return fmt.Errorf("账号 %s 已存在", account.ID)
			}
			cfg.Accounts = append(cfg.Accounts, account)
			return nil
		}
		index := findAccountIndex(cfg.Accounts, account.ID)
		if index < 0 {
			return fmt.Errorf("账号 %s 不存在", account.ID)
		}
		cfg.Accounts[index] = account
		return nil
	})
}

func (h *Handler) toggleAccountEnabled(ctx context.Context, id string, enabled bool) (config.Config, error) {
	return h.mutateConfig(ctx, func(cfg *config.Config) error {
		index := findAccountIndex(cfg.Accounts, id)
		if index < 0 {
			return fmt.Errorf("账号 %s 不存在", id)
		}
		cfg.Accounts[index].Enabled = enabled
		if !enabled {
			cfg.Accounts[index].Status = "disabled"
		} else if cfg.Accounts[index].Status == "disabled" {
			cfg.Accounts[index].Status = "healthy"
		}
		return nil
	})
}

func (h *Handler) applyPoolMutation(ctx context.Context, req upsertPoolRequest, creating bool) (config.Config, error) {
	if strings.TrimSpace(req.ID) == "" {
		return config.Config{}, fmt.Errorf("号池 ID 不能为空")
	}
	return h.mutateConfig(ctx, func(cfg *config.Config) error {
		cfg.Platforms = ensureDefaultPlatform(cfg.Platforms)
		pool := config.AccountPool{
			ID:       strings.TrimSpace(req.ID),
			Platform: defaultPlatformID,
			Strategy: defaultString(req.Strategy, defaultPoolStrategy),
			Enabled:  req.Enabled,
			Members:  normalizedMembers(req.Members),
		}
		for _, member := range pool.Members {
			if findAccountIndex(cfg.Accounts, member) < 0 {
				return fmt.Errorf("号池成员 %s 不存在", member)
			}
		}
		if creating {
			if findPoolIndex(cfg.AccountPools, pool.ID) >= 0 {
				return fmt.Errorf("号池 %s 已存在", pool.ID)
			}
			cfg.AccountPools = append(cfg.AccountPools, pool)
			return nil
		}
		index := findPoolIndex(cfg.AccountPools, pool.ID)
		if index < 0 {
			return fmt.Errorf("号池 %s 不存在", pool.ID)
		}
		cfg.AccountPools[index] = pool
		return nil
	})
}

func (h *Handler) applyAPIKeyMutation(ctx context.Context, req upsertAPIKeyRequest, creating bool) (config.Config, error) {
	if strings.TrimSpace(req.Name) == "" {
		return config.Config{}, fmt.Errorf("API key 名称不能为空")
	}
	return h.mutateConfig(ctx, func(cfg *config.Config) error {
		if strings.TrimSpace(cfg.Auth.AdminToken) == "" {
			cfg.Auth.AdminToken = "change-me-admin-token"
		}
		index := findTokenIndex(cfg.Auth.AccessTokens, req.Name)
		if creating && index >= 0 {
			return fmt.Errorf("API key %s 已存在", req.Name)
		}
		if !creating && index < 0 {
			return fmt.Errorf("API key %s 不存在", req.Name)
		}
		tokenValue := strings.TrimSpace(req.Token)
		if !creating && tokenValue == "" {
			tokenValue = cfg.Auth.AccessTokens[index].Token
		}
		if tokenValue == "" {
			return fmt.Errorf("API key 令牌不能为空")
		}
		item := config.AccessToken{
			Name:           strings.TrimSpace(req.Name),
			Token:          tokenValue,
			Enabled:        req.Enabled,
			QuotaCalls:     req.QuotaCalls,
			CostPerRequest: req.CostPerRequest,
		}
		if item.CostPerRequest <= 0 {
			item.CostPerRequest = 1
		}
		if creating {
			cfg.Auth.AccessTokens = append(cfg.Auth.AccessTokens, item)
			return nil
		}
		cfg.Auth.AccessTokens[index] = item
		return nil
	})
}

func (h *Handler) mutateConfig(ctx context.Context, mutate func(*config.Config) error) (config.Config, error) {
	candidate := h.currentConfig()
	candidate = normalizeControlPlaneConfig(candidate)
	if err := mutate(&candidate); err != nil {
		return config.Config{}, err
	}
	candidate = normalizeControlPlaneConfig(candidate)
	applied, err := h.config.Apply(ctx, candidate)
	if err != nil {
		return config.Config{}, fmt.Errorf("%s", humanizeConfigError(err))
	}
	h.refreshRegistry(ctx)
	return applied, nil
}

func (h *Handler) refreshRegistry(ctx context.Context) {
	if h.registry != nil {
		h.registry.CheckAll(ctx)
	}
}

func normalizeControlPlaneConfig(cfg config.Config) config.Config {
	cfg.Platforms = ensureDefaultPlatform(cfg.Platforms)
	for i := range cfg.Accounts {
		if strings.TrimSpace(cfg.Accounts[i].Platform) == "" {
			cfg.Accounts[i].Platform = defaultPlatformID
		}
		if strings.TrimSpace(cfg.Accounts[i].Status) == "" {
			cfg.Accounts[i].Status = "healthy"
		}
		if strings.TrimSpace(cfg.Accounts[i].CredentialType) == "" {
			cfg.Accounts[i].CredentialType = "api_key"
		}
		if strings.TrimSpace(cfg.Accounts[i].PlanType) == "" {
			cfg.Accounts[i].PlanType = "plus"
		}
	}
	for i := range cfg.AccountPools {
		if strings.TrimSpace(cfg.AccountPools[i].Platform) == "" {
			cfg.AccountPools[i].Platform = defaultPlatformID
		}
		if strings.TrimSpace(cfg.AccountPools[i].Strategy) == "" {
			cfg.AccountPools[i].Strategy = defaultPoolStrategy
		}
	}
	if len(cfg.Models) == 0 && len(cfg.AccountPools) > 0 {
		defaultPool := firstPoolID(cfg.AccountPools)
		cfg.Models = []config.ModelRoute{{
			Alias:       defaultModelAlias,
			AccountPool: defaultPool,
			TargetKind:  "account_pool",
			TargetID:    defaultPool,
			TargetModel: defaultTargetModel,
			Fallbacks:   []string{},
		}}
	}
	if len(cfg.Models) > 0 {
		validPools := map[string]struct{}{}
		for _, pool := range cfg.AccountPools {
			validPools[pool.ID] = struct{}{}
		}
		for i := range cfg.Models {
			if strings.TrimSpace(cfg.Models[i].TargetKind) == "" {
				cfg.Models[i].TargetKind = "account_pool"
			}
			if strings.TrimSpace(cfg.Models[i].TargetID) == "" {
				cfg.Models[i].TargetID = cfg.Models[i].AccountPool
			}
			if strings.TrimSpace(cfg.Models[i].AccountPool) == "" {
				cfg.Models[i].AccountPool = cfg.Models[i].TargetID
			}
			if strings.TrimSpace(cfg.Models[i].TargetModel) == "" {
				cfg.Models[i].TargetModel = defaultTargetModel
			}
			if _, ok := validPools[cfg.Models[i].TargetID]; !ok && len(cfg.AccountPools) > 0 {
				cfg.Models[i].TargetID = firstPoolID(cfg.AccountPools)
				cfg.Models[i].AccountPool = cfg.Models[i].TargetID
			}
		}
	}
	return cfg
}

func ensureDefaultPlatform(platforms []config.Platform) []config.Platform {
	for _, item := range platforms {
		if item.ID == defaultPlatformID {
			return platforms
		}
	}
	return append(platforms, defaultPlatformConfig())
}

func defaultPlatformConfig() config.Platform {
	return config.Platform{
		ID:       defaultPlatformID,
		Type:     "openai",
		Adapter:  "openai-upstream",
		Enabled:  true,
		Priority: 100,
		Weight:   10,
		TimeoutS: 60,
	}
}

func firstPoolID(pools []config.AccountPool) string {
	for _, pool := range pools {
		if pool.Enabled {
			return pool.ID
		}
	}
	return pools[0].ID
}

func normalizedMembers(members []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(members))
	for _, member := range members {
		trimmed := strings.TrimSpace(member)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func findAccountIndex(accounts []config.Account, id string) int {
	for i, account := range accounts {
		if account.ID == id {
			return i
		}
	}
	return -1
}

func findPoolIndex(pools []config.AccountPool, id string) int {
	for i, pool := range pools {
		if pool.ID == id {
			return i
		}
	}
	return -1
}

func findTokenIndex(tokens []config.AccessToken, name string) int {
	for i, token := range tokens {
		if token.Name == name {
			return i
		}
	}
	return -1
}

func normalizeID(id string, fallback string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed != "" {
		return trimmed
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", ".", "-")
	return strings.ToLower(strings.Trim(replacer.Replace(strings.TrimSpace(fallback)), "-"))
}

func maskToken(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:4] + strings.Repeat("•", len(trimmed)-8) + trimmed[len(trimmed)-4:]
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func isRunnableAccountStatus(status string) bool {
	switch status {
	case "", "healthy", "ready", "active":
		return true
	default:
		return false
	}
}

func (h *Handler) defaultModelAlias(cfg config.Config) string {
	if len(cfg.Models) == 0 {
		return defaultModelAlias
	}
	return cfg.Models[0].Alias
}

func humanizeConfigError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	replacer := strings.NewReplacer(
		"invalid server.port", "服务端口不合法",
		"auth.admin_token is required", "管理员令牌不能为空",
		"at least one access token is required", "至少需要一个访问令牌",
		"at least one platform is required", "至少需要一个平台",
		"platform id is required", "平台 ID 不能为空",
		"duplicate platform id", "平台 ID 重复",
		"account id is required", "账号 ID 不能为空",
		"duplicate account id", "账号 ID 重复",
		"account pool id is required", "账号池 ID 不能为空",
		"duplicate account pool id", "账号池 ID 重复",
		"model alias is required", "模型别名不能为空",
		"duplicate model alias", "模型别名重复",
		"references unknown platform", "引用了不存在的平台",
		"references unknown account", "引用了不存在的账号",
		"references unknown account pool", "引用了不存在的账号池",
		"has unsupported target kind", "使用了不支持的目标类型",
	)
	return replacer.Replace(msg)
}
