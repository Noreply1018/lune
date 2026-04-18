package store

type Account struct {
	ID         int64  `json:"id"`
	Label      string `json:"label"`
	SourceKind string `json:"source_kind"`

	// openai_compat fields
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key,omitempty"`
	Provider string `json:"provider"`

	// cpa fields
	CpaServiceID     *int64 `json:"cpa_service_id,omitempty"`
	CpaProvider      string `json:"cpa_provider,omitempty"`
	CpaAccountKey    string `json:"cpa_account_key,omitempty"`
	CpaEmail         string `json:"cpa_email,omitempty"`
	CpaPlanType      string `json:"cpa_plan_type,omitempty"`
	CpaOpenaiID      string `json:"cpa_openai_id,omitempty"`
	CpaExpiredAt     string `json:"cpa_expired_at,omitempty"`
	CpaLastRefreshAt string `json:"cpa_last_refresh_at,omitempty"`
	CpaDisabled      bool   `json:"cpa_disabled,omitempty"`

	// codex quota snapshot (updated by health loop)
	CodexQuotaJSON      string `json:"codex_quota_json,omitempty"`
	CodexQuotaFetchedAt string `json:"codex_quota_fetched_at,omitempty"`

	// probe configuration + last self-check result (direct accounts)
	ProbeModels     []string `json:"probe_models"`
	LastProbeStatus string   `json:"last_probe_status,omitempty"`
	LastProbeAt     *string  `json:"last_probe_at,omitempty"`
	LastProbeError  string   `json:"last_probe_error,omitempty"`

	// common fields
	Enabled       bool    `json:"enabled"`
	Status        string  `json:"status"`
	Notes         string  `json:"notes"`
	QuotaDisplay  string  `json:"quota_display"`
	LastCheckedAt *string `json:"last_checked_at"`
	LastError     string  `json:"last_error"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`

	// computed fields (not stored in DB)
	APIKeySet    bool            `json:"api_key_set"`
	APIKeyMasked string          `json:"api_key_masked"`
	Models       []string        `json:"models"`
	Runtime      *AccountRuntime `json:"runtime,omitempty"`
}

type AccountRuntime struct {
	BaseURL  string `json:"base_url"`
	AuthMode string `json:"auth_mode"`
}

type Pool struct {
	ID        int64  `json:"id"`
	Label     string `json:"label"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// aggregated fields (populated at response time)
	AccountCount         int      `json:"account_count"`
	HealthyAccountCount  int      `json:"healthy_account_count"`
	RoutableAccountCount int      `json:"routable_account_count"`
	Models               []string `json:"models"`
}

type PoolMember struct {
	ID        int64 `json:"id"`
	PoolID    int64 `json:"pool_id"`
	AccountID int64 `json:"account_id"`
	Position  int   `json:"position"`
	Enabled   bool  `json:"enabled"`

	// JOIN field
	Account *Account `json:"account,omitempty"`
}

type AccessToken struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Token      string  `json:"token,omitempty"`
	PoolID     *int64  `json:"pool_id"`
	Enabled    bool    `json:"enabled"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	LastUsedAt *string `json:"last_used_at"`

	// computed fields
	TokenMasked string `json:"token_masked"`
	IsGlobal    bool   `json:"is_global"`
	PoolLabel   string `json:"pool_label,omitempty"`
}

type RequestLog struct {
	ID              int64  `json:"id"`
	RequestID       string `json:"request_id"`
	AccessTokenName string `json:"access_token_name"`
	ModelRequested  string `json:"model_requested"`
	ModelActual     string `json:"model_actual"`
	PoolID          int64  `json:"pool_id"`
	AccountID       int64  `json:"account_id"`
	AccountLabel    string `json:"account_label"`
	StatusCode      int    `json:"status_code"`
	LatencyMs       int64  `json:"latency_ms"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	Stream          bool   `json:"stream"`
	RequestIP       string `json:"request_ip"`
	Success         bool   `json:"success"`
	ErrorMessage    string `json:"error_message"`
	SourceKind      string `json:"source_kind"`
	CreatedAt       string `json:"created_at"`
}

type AccountModel struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	ModelID   string `json:"model_id"`
	CreatedAt string `json:"created_at"`
}

type CpaService struct {
	ID            int64   `json:"id"`
	Label         string  `json:"label"`
	BaseURL       string  `json:"base_url"`
	APIKey        string  `json:"api_key,omitempty"`
	ManagementKey string  `json:"management_key,omitempty"`
	APIKeySet     bool    `json:"api_key_set"`
	APIKeyMasked  string  `json:"api_key_masked"`
	Enabled       bool    `json:"enabled"`
	Status        string  `json:"status"`
	LastCheckedAt *string `json:"last_checked_at"`
	LastError     string  `json:"last_error"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type UsageStats struct {
	TotalRequests     int64            `json:"total_requests"`
	SuccessRate       float64          `json:"success_rate"`
	TotalInputTokens  int64            `json:"total_input_tokens"`
	TotalOutputTokens int64            `json:"total_output_tokens"`
	ByAccount         []UsageByAccount `json:"by_account"`
	ByToken           []UsageByToken   `json:"by_token"`
}

type UsageByAccount struct {
	AccountID          int64   `json:"account_id"`
	AccountLabel       string  `json:"account_label"`
	Requests           int64   `json:"requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	SuccessRate        float64 `json:"success_rate"`
	InputTokens        int64   `json:"input_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
}

type UsageByToken struct {
	TokenName    string `json:"token_name"`
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

type UsageLogPage struct {
	Items    []RequestLog `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

type Overview struct {
	PoolsTotal        int     `json:"pools_total"`
	PoolsHealthy      int     `json:"pools_healthy"`
	AccountsTotal     int     `json:"accounts_total"`
	AccountsHealthy   int     `json:"accounts_healthy"`
	ModelsTotal       int     `json:"models_total"`
	RequestsToday     int64   `json:"requests_today"`
	SuccessRateToday  float64 `json:"success_rate_today"`
	GlobalTokenID     *int64  `json:"global_token_id,omitempty"`
	GlobalTokenMasked string  `json:"global_token_masked"`
	Alerts            []Alert `json:"alerts"`
}

type Alert struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	PoolID  int64  `json:"pool_id"`
}

type SystemNotification struct {
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	AccountID *int64 `json:"account_id,omitempty"`
	ServiceID *int64 `json:"service_id,omitempty"`
	// Label carries the human-readable name of the source (account label or
	// cpa_service label) so template placeholders like {{ .Vars.account_label }}
	// and {{ .Vars.service_label }} can be populated by the dispatcher.
	Label string `json:"label,omitempty"`
	// LastError mirrors the source's last_error field for account_error and
	// cpa_service_error so templates can surface the underlying cause.
	LastError string `json:"last_error,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type DataRetentionSummary struct {
	RetentionDays               int     `json:"retention_days"`
	TotalLogs                   int64   `json:"total_logs"`
	OldestLogAt                 *string `json:"oldest_log_at"`
	NewestLogAt                 *string `json:"newest_log_at"`
	TotalNotificationDeliveries int64   `json:"total_notification_deliveries"`
	TotalNotificationOutbox     int64   `json:"total_notification_outbox"`
}

// LatencyBucket holds percentile latencies for a single time bucket.
type LatencyBucket struct {
	Bucket string  `json:"bucket"`
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	Count  int     `json:"count"`
}
