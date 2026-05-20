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
	CpaServiceID             *int64 `json:"cpa_service_id,omitempty"`
	CpaProvider              string `json:"cpa_provider,omitempty"`
	CpaAccountKey            string `json:"cpa_account_key,omitempty"`
	CpaEmail                 string `json:"cpa_email,omitempty"`
	CpaPlanType              string `json:"cpa_plan_type,omitempty"`
	CpaOpenaiID              string `json:"cpa_openai_id,omitempty"`
	CpaExpiredAt             string `json:"cpa_expired_at,omitempty"`
	CpaLastRefreshAt         string `json:"cpa_last_refresh_at,omitempty"`
	CpaDisabled              bool   `json:"cpa_disabled,omitempty"`
	CpaCredentialStatus      string `json:"cpa_credential_status,omitempty"`
	CpaCredentialReason      string `json:"cpa_credential_reason,omitempty"`
	CpaCredentialLastError   string `json:"cpa_credential_last_error,omitempty"`
	CpaCredentialCheckedAt   string `json:"cpa_credential_checked_at,omitempty"`
	CpaSubscriptionExpiresAt string `json:"cpa_subscription_expires_at,omitempty"`
	CpaSubscriptionFetchedAt string `json:"cpa_subscription_fetched_at,omitempty"`
	CpaSubscriptionLastError string `json:"cpa_subscription_last_error,omitempty"`
	CpaSubscriptionStatus    string `json:"cpa_subscription_status,omitempty"`
	CpaAccessStatus          string `json:"cpa_access_status,omitempty"`
	CpaAccessReason          string `json:"cpa_access_reason,omitempty"`
	CpaAccessLastError       string `json:"cpa_access_last_error,omitempty"`
	CpaAccessCheckedAt       string `json:"cpa_access_checked_at,omitempty"`

	// codex quota snapshot (updated by health loop)
	CodexQuotaJSON       string `json:"codex_quota_json,omitempty"`
	CodexQuotaFetchedAt  string `json:"codex_quota_fetched_at,omitempty"`
	CpaQuotaStatus       string `json:"cpa_quota_status,omitempty"`
	CpaQuotaLastError    string `json:"cpa_quota_last_error,omitempty"`
	CpaQuotaCheckedAt    string `json:"cpa_quota_checked_at,omitempty"`
	CpaQuotaBackoffUntil string `json:"cpa_quota_backoff_until,omitempty"`
	CpaQuotaBackoffCount int    `json:"cpa_quota_backoff_count,omitempty"`

	// serving circuit breaker state (updated by gateway traffic)
	ServingStatus string `json:"serving_status,omitempty"`
	FailureCount  int    `json:"failure_count,omitempty"`
	LastFailureAt string `json:"last_failure_at,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	CooldownUntil string `json:"cooldown_until,omitempty"`

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
	APIKeySet            bool               `json:"api_key_set"`
	APIKeyMasked         string             `json:"api_key_masked"`
	Models               []string           `json:"models"`
	Runtime              *AccountRuntime    `json:"runtime,omitempty"`
	Diagnostic           *AccountDiagnostic `json:"diagnostic,omitempty"`
	DiagnosticLoadFailed bool               `json:"-"`

	DiagnosticStatus  string `json:"diagnostic_status,omitempty"`
	SchedulerStatus   string `json:"scheduler_status,omitempty"`
	LastDiagnosedAt   string `json:"last_diagnosed_at,omitempty"`
	DiagnosticSummary string `json:"diagnostic_summary,omitempty"`
	CpaAccountKeyHash string `json:"cpa_account_key_hash,omitempty"`
}

type AccountRuntime struct {
	BaseURL                  string `json:"base_url"`
	AuthMode                 string `json:"auth_mode"`
	ProviderPinningSupported bool   `json:"provider_pinning_supported"`
}

type Pool struct {
	ID            int64  `json:"id"`
	Label         string `json:"label"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
	RoutingPolicy string `json:"routing_policy"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`

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
	PoolLabel   string `json:"pool_label,omitempty"`
}

type RequestLog struct {
	ID                   int64  `json:"id"`
	RequestID            string `json:"request_id"`
	AccessTokenName      string `json:"access_token_name"`
	ModelRequested       string `json:"model_requested"`
	ModelActual          string `json:"model_actual"`
	PoolID               int64  `json:"pool_id"`
	AccountID            int64  `json:"account_id"`
	AccountLabel         string `json:"account_label"`
	StatusCode           int    `json:"status_code"`
	LatencyMs            int64  `json:"latency_ms"`
	InputTokens          int64  `json:"input_tokens"`
	OutputTokens         int64  `json:"output_tokens"`
	Stream               bool   `json:"stream"`
	RequestIP            string `json:"request_ip"`
	Success              bool   `json:"success"`
	ErrorMessage         string `json:"error_message"`
	ErrorFingerprint     string `json:"error_fingerprint,omitempty"`
	ErrorRepeatCount     int    `json:"error_repeat_count,omitempty"`
	ErrorLastSeenAt      string `json:"error_last_seen_at,omitempty"`
	SourceKind           string `json:"source_kind"`
	AttemptCount         int    `json:"attempt_count"`
	Diagnostic           bool   `json:"diagnostic,omitempty"`
	ForceAccount         bool   `json:"force_account,omitempty"`
	StatefulProbe        bool   `json:"stateful_probe,omitempty"`
	TrafficKind          string `json:"traffic_kind,omitempty"`
	RuntimeAuthIndex     string `json:"runtime_auth_index"`
	RuntimeAuthID        string `json:"runtime_auth_id"`
	RuntimeAccountKey    string `json:"runtime_account_key"`
	RuntimeBindingStatus string `json:"runtime_binding_status"`
	RuntimeBindingReason string `json:"runtime_binding_reason"`
	RouteTrace           string `json:"route_trace,omitempty"`
	CreatedAt            string `json:"created_at"`
}

type AccountDiagnostic struct {
	ID                             int64                       `json:"id"`
	AccountID                      int64                       `json:"account_id"`
	AccountKeyHash                 string                      `json:"account_key_hash,omitempty"`
	Provider                       string                      `json:"provider,omitempty"`
	OperationID                    string                      `json:"operation_id,omitempty"`
	StartedAt                      string                      `json:"started_at"`
	FinishedAt                     string                      `json:"finished_at"`
	StableDiagnosticStatus         string                      `json:"stable_diagnostic_status"`
	PreviousStableDiagnosticStatus string                      `json:"previous_stable_diagnostic_status,omitempty"`
	LastProbeStatus                string                      `json:"last_probe_status"`
	SchedulerStatus                string                      `json:"scheduler_status"`
	SchedulerOverride              string                      `json:"scheduler_override,omitempty"`
	SafeSummary                    string                      `json:"safe_summary,omitempty"`
	CreatedAt                      string                      `json:"created_at"`
	UpdatedAt                      string                      `json:"updated_at"`
	Evidence                       []AccountDiagnosticEvidence `json:"evidence,omitempty"`
}

type AccountDiagnosticEvidence struct {
	ID                  int64  `json:"id"`
	DiagnosticID        int64  `json:"diagnostic_id"`
	ProbeType           string `json:"probe_type"`
	Stage               string `json:"stage,omitempty"`
	HTTPStatus          *int   `json:"http_status,omitempty"`
	UpstreamErrorCode   string `json:"upstream_error_code,omitempty"`
	NormalizedErrorCode string `json:"normalized_error_code,omitempty"`
	SafeMessage         string `json:"safe_message,omitempty"`
	ObservedAt          string `json:"observed_at"`
	RequestLogID        *int64 `json:"request_log_id,omitempty"`
}

type Operation struct {
	ID               int64           `json:"id"`
	OperationID      string          `json:"operation_id"`
	OperationType    string          `json:"operation_type"`
	Source           string          `json:"source"`
	TargetType       string          `json:"target_type"`
	TargetID         string          `json:"target_id"`
	TargetSummary    string          `json:"target_summary"`
	Status           string          `json:"status"`
	ErrorCode        string          `json:"error_code,omitempty"`
	SafeErrorMessage string          `json:"safe_error_message,omitempty"`
	CorrelationID    string          `json:"correlation_id,omitempty"`
	StartedAt        string          `json:"started_at"`
	FinishedAt       string          `json:"finished_at"`
	CreatedAt        string          `json:"created_at"`
	Items            []OperationItem `json:"items,omitempty"`
}

type OperationItem struct {
	ID               int64  `json:"id"`
	OperationID      string `json:"operation_id"`
	ItemIndex        int    `json:"item_index"`
	ClientFileName   string `json:"client_file_name"`
	AccountKeyHash   string `json:"account_key_hash,omitempty"`
	Action           string `json:"action,omitempty"`
	Status           string `json:"status"`
	RuntimeSync      string `json:"runtime_sync,omitempty"`
	ErrorCode        string `json:"error_code,omitempty"`
	SafeErrorMessage string `json:"safe_error_message,omitempty"`
	AccountID        int64  `json:"account_id,omitempty"`
	PoolMemberID     int64  `json:"pool_member_id,omitempty"`
	Stage            string `json:"stage,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type AccountModel struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	ModelID   string `json:"model_id"`
	CreatedAt string `json:"created_at"`
}

type CpaService struct {
	ID                       int64   `json:"id"`
	Label                    string  `json:"label"`
	BaseURL                  string  `json:"base_url"`
	APIKey                   string  `json:"api_key,omitempty"`
	ManagementKey            string  `json:"management_key,omitempty"`
	APIKeySet                bool    `json:"api_key_set"`
	APIKeyMasked             string  `json:"api_key_masked"`
	Enabled                  bool    `json:"enabled"`
	Status                   string  `json:"status"`
	LastCheckedAt            *string `json:"last_checked_at"`
	LastError                string  `json:"last_error"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
	RuntimeMode              string  `json:"runtime_mode,omitempty"`
	AuthDir                  string  `json:"auth_dir,omitempty"`
	ImagePinnedVersion       string  `json:"image_pinned_version,omitempty"`
	RunningVersion           string  `json:"running_version,omitempty"`
	CurrentVersion           string  `json:"current_version,omitempty"`
	LatestVersion            string  `json:"latest_version,omitempty"`
	UpdateAvailable          bool    `json:"update_available,omitempty"`
	ProviderPinningSupported bool    `json:"provider_pinning_supported"`
	ProviderPinningState     string  `json:"provider_pinning_state"`
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
	PoolsTotal       int     `json:"pools_total"`
	PoolsHealthy     int     `json:"pools_healthy"`
	AccountsTotal    int     `json:"accounts_total"`
	AccountsHealthy  int     `json:"accounts_healthy"`
	ModelsTotal      int     `json:"models_total"`
	RequestsToday    int64   `json:"requests_today"`
	SuccessRateToday float64 `json:"success_rate_today"`
	AvgLatencyToday  float64 `json:"avg_latency_today"`
	Alerts           []Alert `json:"alerts"`
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
	RetentionDays                  int     `json:"retention_days"`
	DatabaseSizeBytes              int64   `json:"database_size_bytes"`
	DatabaseWalSizeBytes           int64   `json:"database_wal_size_bytes"`
	DatabaseShmSizeBytes           int64   `json:"database_shm_size_bytes"`
	TotalLogs                      int64   `json:"total_logs"`
	OldestLogAt                    *string `json:"oldest_log_at"`
	NewestLogAt                    *string `json:"newest_log_at"`
	LogsSizeBytes                  int64   `json:"logs_size_bytes"`
	TotalNotificationDeliveries    int64   `json:"total_notification_deliveries"`
	NotificationDeliveriesOldestAt *string `json:"notification_deliveries_oldest_at"`
	NotificationDeliveriesNewestAt *string `json:"notification_deliveries_newest_at"`
	TotalNotificationOutbox        int64   `json:"total_notification_outbox"`
	OutboxPendingCount             int64   `json:"outbox_pending_count"`
	OutboxDroppedCount             int64   `json:"outbox_dropped_count"`
	LastPruneAt                    *string `json:"last_prune_at"`
	LastPruneDeletedLogs           int64   `json:"last_prune_deleted_logs"`
	LastPruneDeletedDeliveries     int64   `json:"last_prune_deleted_deliveries"`
	LastPruneDeletedOutbox         int64   `json:"last_prune_deleted_outbox"`
}

type DataRetentionPreview struct {
	RetentionDays         int   `json:"retention_days"`
	LogsToDelete          int64 `json:"logs_to_delete"`
	LogsToDeleteSizeBytes int64 `json:"logs_to_delete_size_bytes"`
	DeliveriesToDelete    int64 `json:"deliveries_to_delete"`
	OutboxToDelete        int64 `json:"outbox_to_delete"`
	OutboxSafetyDays      int   `json:"outbox_safety_days"`
}

// LatencyBucket holds percentile latencies for a single time bucket.
type LatencyBucket struct {
	Bucket string  `json:"bucket"`
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	Count  int     `json:"count"`
}
