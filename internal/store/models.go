package store

import "time"

type Account struct {
	ID             int64     `json:"id"`
	Label          string    `json:"label"`
	BaseURL        string    `json:"base_url"`
	APIKey         string    `json:"api_key"`
	Enabled        bool      `json:"enabled"`
	Status         string    `json:"status"`
	QuotaTotal     float64   `json:"quota_total"`
	QuotaUsed      float64   `json:"quota_used"`
	QuotaUnit      string    `json:"quota_unit"`
	Notes          string    `json:"notes"`
	ModelAllowlist []string  `json:"model_allowlist"`
	LastCheckedAt  *string   `json:"last_checked_at"`
	LastError      string    `json:"last_error"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Pool struct {
	ID        int64        `json:"id"`
	Label     string       `json:"label"`
	Strategy  string       `json:"strategy"`
	Enabled   bool         `json:"enabled"`
	Members   []PoolMember `json:"members"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type PoolMember struct {
	ID        int64 `json:"id"`
	PoolID    int64 `json:"pool_id"`
	AccountID int64 `json:"account_id"`
	Priority  int   `json:"priority"`
	Weight    int   `json:"weight"`
}

type ModelRoute struct {
	ID          int64     `json:"id"`
	Alias       string    `json:"alias"`
	PoolID      int64     `json:"pool_id"`
	TargetModel string    `json:"target_model"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AccessToken struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Token       string    `json:"token"`
	Enabled     bool      `json:"enabled"`
	QuotaTokens int64     `json:"quota_tokens"`
	UsedTokens  int64     `json:"used_tokens"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastUsedAt  *string   `json:"last_used_at"`
}

type RequestLog struct {
	ID              int64   `json:"id"`
	RequestID       string  `json:"request_id"`
	AccessTokenName string  `json:"access_token_name"`
	ModelAlias      string  `json:"model_alias"`
	TargetModel     string  `json:"target_model"`
	PoolID          int64   `json:"pool_id"`
	AccountID       int64   `json:"account_id"`
	StatusCode      int     `json:"status_code"`
	LatencyMs       int64   `json:"latency_ms"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	Stream          bool    `json:"stream"`
	RequestIP       string  `json:"request_ip"`
	Success         bool    `json:"success"`
	ErrorMessage    string  `json:"error_message"`
	CreatedAt       string  `json:"created_at"`
}
