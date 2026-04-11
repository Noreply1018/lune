package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"lune/internal/config"
	"lune/internal/execution"

	_ "modernc.org/sqlite"
)

type RequestLog struct {
	ID             int64     `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	ModelAlias     string    `json:"model_alias"`
	PlatformID     string    `json:"platform_id"`
	TargetModel    string    `json:"target_model"`
	AccessToken    string    `json:"access_token"`
	StatusCode     int       `json:"status_code"`
	LatencyMS      int64     `json:"latency_ms"`
	Success        bool      `json:"success"`
	ErrorMessage   string    `json:"error_message"`
	ResponseStream bool      `json:"response_stream"`
}

type AccountRecord struct {
	ID             string     `json:"id"`
	PlatformID     string     `json:"platform_id"`
	Label          string     `json:"label"`
	CredentialType string     `json:"credential_type"`
	CredentialEnv  string     `json:"credential_env"`
	EgressProxyEnv string     `json:"egress_proxy_env"`
	PlanType       string     `json:"plan_type"`
	Enabled        bool       `json:"enabled"`
	Status         string     `json:"status"`
	RiskScore      float64    `json:"risk_score"`
	CooldownUntil  *time.Time `json:"cooldown_until,omitempty"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastError      string     `json:"last_error"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type AccountPoolRecord struct {
	ID         string    `json:"id"`
	PlatformID string    `json:"platform_id"`
	Strategy   string    `json:"strategy"`
	Enabled    bool      `json:"enabled"`
	Members    []string  `json:"members"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type UsageLedgerEntry struct {
	ID               int64     `json:"id"`
	RequestID        string    `json:"request_id"`
	CreatedAt        time.Time `json:"created_at"`
	AccessTokenName  string    `json:"access_token_name"`
	ModelAlias       string    `json:"model_alias"`
	PlatformID       string    `json:"platform_id"`
	AccountID        string    `json:"account_id"`
	TargetModel      string    `json:"target_model"`
	StatusCode       int       `json:"status_code"`
	LatencyMS        int64     `json:"latency_ms"`
	Success          bool      `json:"success"`
	APICostUnits     int64     `json:"api_cost_units"`
	AccountCostUnits int64     `json:"account_cost_units"`
	AccountCostType  string    `json:"account_cost_type"`
	ErrorMessage     string    `json:"error_message"`
}

type TokenAccount struct {
	Name           string    `json:"name"`
	QuotaCalls     int64     `json:"quota_calls"`
	UsedCalls      int64     `json:"used_calls"`
	CostPerRequest int64     `json:"cost_per_request"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (t TokenAccount) RemainingCalls() int64 {
	if t.QuotaCalls <= 0 {
		return -1
	}
	remaining := t.QuotaCalls - t.UsedCalls
	if remaining < 0 {
		return 0
	}
	return remaining
}

type Store struct {
	db *sql.DB
}

type LedgerStore interface {
	RecordAttempt(context.Context, execution.Record) error
	RecordSuccess(context.Context, execution.Record) error
	RecordFailure(context.Context, execution.Record) error
	UpdateAccountExecutionState(context.Context, execution.Record) error
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS request_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  model_alias TEXT NOT NULL,
  platform_id TEXT NOT NULL,
  target_model TEXT NOT NULL,
  access_token TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL,
  success INTEGER NOT NULL,
  error_message TEXT NOT NULL,
  response_stream INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at DESC);
CREATE TABLE IF NOT EXISTS token_accounts (
  name TEXT PRIMARY KEY,
  quota_calls INTEGER NOT NULL,
  used_calls INTEGER NOT NULL,
  cost_per_request INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  platform_id TEXT NOT NULL,
  label TEXT NOT NULL,
  credential_type TEXT NOT NULL,
  credential_env TEXT NOT NULL,
  egress_proxy_env TEXT NOT NULL,
  plan_type TEXT NOT NULL,
  enabled INTEGER NOT NULL,
  status TEXT NOT NULL,
  risk_score REAL NOT NULL,
  cooldown_until TEXT NOT NULL,
  last_success_at TEXT NOT NULL,
  last_error TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS account_pools (
  id TEXT PRIMARY KEY,
  platform_id TEXT NOT NULL,
  strategy TEXT NOT NULL,
  enabled INTEGER NOT NULL,
  members_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS usage_ledger (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  access_token_name TEXT NOT NULL,
  model_alias TEXT NOT NULL,
  platform_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  target_model TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL,
  success INTEGER NOT NULL,
  api_cost_units INTEGER NOT NULL,
  account_cost_units INTEGER NOT NULL,
  account_cost_type TEXT NOT NULL,
  error_message TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_ledger_created_at ON usage_ledger(created_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	hasProviderID, err := s.columnExists("request_logs", "provider_id")
	if err != nil {
		return err
	}
	hasPlatformID, err := s.columnExists("request_logs", "platform_id")
	if err != nil {
		return err
	}
	if hasProviderID && !hasPlatformID {
		if _, err := s.db.Exec(`ALTER TABLE request_logs RENAME COLUMN provider_id TO platform_id`); err != nil {
			return err
		}
	}

	hasEgressProxyEnv, err := s.columnExists("accounts", "egress_proxy_env")
	if err != nil {
		return err
	}
	if !hasEgressProxyEnv {
		if _, err := s.db.Exec(`ALTER TABLE accounts ADD COLUMN egress_proxy_env TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) SyncAccessTokens(ctx context.Context, tokens []config.AccessToken) error {
	if s == nil || s.db == nil {
		return nil
	}

	for _, token := range tokens {
		cost := token.CostPerRequest
		if cost <= 0 {
			cost = 1
		}

		_, err := s.db.ExecContext(ctx, `
INSERT INTO token_accounts (name, quota_calls, used_calls, cost_per_request, updated_at)
VALUES (?, ?, 0, ?, ?)
ON CONFLICT(name) DO UPDATE SET
  quota_calls = excluded.quota_calls,
  cost_per_request = excluded.cost_per_request,
  updated_at = excluded.updated_at
`,
			token.Name,
			token.QuotaCalls,
			cost,
			time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) SyncAccounts(ctx context.Context, accounts []config.Account) error {
	if s == nil || s.db == nil {
		return nil
	}

	for _, account := range accounts {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO accounts (
  id, platform_id, label, credential_type, credential_env, egress_proxy_env, plan_type,
  enabled, status, risk_score, cooldown_until, last_success_at, last_error, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  platform_id = excluded.platform_id,
  label = excluded.label,
  credential_type = excluded.credential_type,
  credential_env = excluded.credential_env,
  egress_proxy_env = excluded.egress_proxy_env,
  plan_type = excluded.plan_type,
  enabled = excluded.enabled,
  status = excluded.status,
  risk_score = excluded.risk_score,
  updated_at = excluded.updated_at
`,
			account.ID,
			account.Platform,
			account.Label,
			account.CredentialType,
			account.CredentialEnv,
			account.EgressProxyEnv,
			account.PlanType,
			boolToInt(account.Enabled),
			account.Status,
			account.RiskScore,
			"",
			"",
			"",
			time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SyncAccountPools(ctx context.Context, pools []config.AccountPool) error {
	if s == nil || s.db == nil {
		return nil
	}

	for _, pool := range pools {
		membersJSON, err := json.Marshal(pool.Members)
		if err != nil {
			return err
		}

		_, err = s.db.ExecContext(ctx, `
INSERT INTO account_pools (id, platform_id, strategy, enabled, members_json, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  platform_id = excluded.platform_id,
  strategy = excluded.strategy,
  enabled = excluded.enabled,
  members_json = excluded.members_json,
  updated_at = excluded.updated_at
`,
			pool.ID,
			pool.Platform,
			pool.Strategy,
			boolToInt(pool.Enabled),
			string(membersJSON),
			time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertRequestLog(ctx context.Context, log RequestLog) error {
	if s == nil || s.db == nil {
		return nil
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO request_logs (
		  created_at, method, path, model_alias, platform_id, target_model,
		  access_token, status_code, latency_ms, success, error_message, response_stream
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.CreatedAt.UTC().Format(time.RFC3339),
		log.Method,
		log.Path,
		log.ModelAlias,
		log.PlatformID,
		log.TargetModel,
		log.AccessToken,
		log.StatusCode,
		log.LatencyMS,
		boolToInt(log.Success),
		log.ErrorMessage,
		boolToInt(log.ResponseStream),
	)
	return err
}

func (s *Store) ListRequestLogs(ctx context.Context, limit int) ([]RequestLog, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
	  id, created_at, method, path, model_alias, platform_id, target_model,
	  access_token, status_code, latency_ms, success, error_message, response_stream
	  FROM request_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var item RequestLog
		var createdAt string
		var success int
		var responseStream int
		if err := rows.Scan(
			&item.ID,
			&createdAt,
			&item.Method,
			&item.Path,
			&item.ModelAlias,
			&item.PlatformID,
			&item.TargetModel,
			&item.AccessToken,
			&item.StatusCode,
			&item.LatencyMS,
			&success,
			&item.ErrorMessage,
			&responseStream,
		); err != nil {
			return nil, err
		}

		parsed, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}

		item.CreatedAt = parsed
		item.Success = success == 1
		item.ResponseStream = responseStream == 1
		logs = append(logs, item)
	}

	if logs == nil {
		logs = []RequestLog{}
	}

	return logs, rows.Err()
}

func (s *Store) GetTokenAccount(ctx context.Context, name string) (TokenAccount, error) {
	if s == nil || s.db == nil {
		return TokenAccount{}, sql.ErrNoRows
	}

	var account TokenAccount
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT name, quota_calls, used_calls, cost_per_request, updated_at
FROM token_accounts
WHERE name = ?`, name).Scan(
		&account.Name,
		&account.QuotaCalls,
		&account.UsedCalls,
		&account.CostPerRequest,
		&updatedAt,
	)
	if err != nil {
		return TokenAccount{}, err
	}

	parsed, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return TokenAccount{}, err
	}
	account.UpdatedAt = parsed
	return account, nil
}

func (s *Store) ListTokenAccounts(ctx context.Context) ([]TokenAccount, error) {
	if s == nil || s.db == nil {
		return []TokenAccount{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT name, quota_calls, used_calls, cost_per_request, updated_at
FROM token_accounts
ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TokenAccount
	for rows.Next() {
		var item TokenAccount
		var updatedAt string
		if err := rows.Scan(&item.Name, &item.QuotaCalls, &item.UsedCalls, &item.CostPerRequest, &updatedAt); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt = parsed
		items = append(items, item)
	}
	if items == nil {
		items = []TokenAccount{}
	}
	return items, rows.Err()
}

func (s *Store) ListAccounts(ctx context.Context) ([]AccountRecord, error) {
	if s == nil || s.db == nil {
		return []AccountRecord{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, platform_id, label, credential_type, credential_env, egress_proxy_env, plan_type,
       enabled, status, risk_score, cooldown_until, last_success_at, last_error, updated_at
FROM accounts
ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AccountRecord
	for rows.Next() {
		var item AccountRecord
		var enabled int
		var cooldownUntil string
		var lastSuccessAt string
		var updatedAt string
		if err := rows.Scan(
			&item.ID,
			&item.PlatformID,
			&item.Label,
			&item.CredentialType,
			&item.CredentialEnv,
			&item.EgressProxyEnv,
			&item.PlanType,
			&enabled,
			&item.Status,
			&item.RiskScore,
			&cooldownUntil,
			&lastSuccessAt,
			&item.LastError,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		if ts, err := parseOptionalTime(cooldownUntil); err != nil {
			return nil, err
		} else {
			item.CooldownUntil = ts
		}
		if ts, err := parseOptionalTime(lastSuccessAt); err != nil {
			return nil, err
		} else {
			item.LastSuccessAt = ts
		}
		item.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []AccountRecord{}
	}
	return items, rows.Err()
}

func (s *Store) ListAccountPools(ctx context.Context) ([]AccountPoolRecord, error) {
	if s == nil || s.db == nil {
		return []AccountPoolRecord{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, platform_id, strategy, enabled, members_json, updated_at
FROM account_pools
ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AccountPoolRecord
	for rows.Next() {
		var item AccountPoolRecord
		var enabled int
		var membersJSON string
		var updatedAt string
		if err := rows.Scan(&item.ID, &item.PlatformID, &item.Strategy, &enabled, &membersJSON, &updatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		if membersJSON != "" {
			if err := json.Unmarshal([]byte(membersJSON), &item.Members); err != nil {
				return nil, err
			}
		}
		item.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []AccountPoolRecord{}
	}
	return items, rows.Err()
}

func (s *Store) CanConsume(ctx context.Context, name string) (TokenAccount, bool, error) {
	account, err := s.GetTokenAccount(ctx, name)
	if err != nil {
		return TokenAccount{}, false, err
	}
	if account.QuotaCalls <= 0 {
		return account, true, nil
	}
	return account, account.RemainingCalls() >= account.CostPerRequest, nil
}

func (s *Store) ConsumeRequest(ctx context.Context, name string) error {
	if s == nil || s.db == nil {
		return nil
	}

	account, err := s.GetTokenAccount(ctx, name)
	if err != nil {
		return err
	}

	if account.QuotaCalls > 0 && account.RemainingCalls() < account.CostPerRequest {
		return fmt.Errorf("token quota exhausted")
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE token_accounts
SET used_calls = used_calls + ?, updated_at = ?
WHERE name = ?`,
		account.CostPerRequest,
		time.Now().UTC().Format(time.RFC3339),
		name,
	)
	return err
}

func (s *Store) InsertUsageLedgerEntry(ctx context.Context, entry UsageLedgerEntry) error {
	if s == nil || s.db == nil {
		return nil
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO usage_ledger (
  request_id, created_at, access_token_name, model_alias, platform_id, account_id,
  target_model, status_code, latency_ms, success, api_cost_units, account_cost_units,
  account_cost_type, error_message
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.RequestID,
		entry.CreatedAt.UTC().Format(time.RFC3339),
		entry.AccessTokenName,
		entry.ModelAlias,
		entry.PlatformID,
		entry.AccountID,
		entry.TargetModel,
		entry.StatusCode,
		entry.LatencyMS,
		boolToInt(entry.Success),
		entry.APICostUnits,
		entry.AccountCostUnits,
		entry.AccountCostType,
		entry.ErrorMessage,
	)
	return err
}

func (s *Store) ListUsageLedgerEntries(ctx context.Context, limit int) ([]UsageLedgerEntry, error) {
	if s == nil || s.db == nil {
		return []UsageLedgerEntry{}, nil
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, request_id, created_at, access_token_name, model_alias, platform_id, account_id,
       target_model, status_code, latency_ms, success, api_cost_units, account_cost_units,
       account_cost_type, error_message
FROM usage_ledger
ORDER BY id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []UsageLedgerEntry
	for rows.Next() {
		var item UsageLedgerEntry
		var createdAt string
		var success int
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&createdAt,
			&item.AccessTokenName,
			&item.ModelAlias,
			&item.PlatformID,
			&item.AccountID,
			&item.TargetModel,
			&item.StatusCode,
			&item.LatencyMS,
			&success,
			&item.APICostUnits,
			&item.AccountCostUnits,
			&item.AccountCostType,
			&item.ErrorMessage,
		); err != nil {
			return nil, err
		}

		parsed, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, err
		}
		item.CreatedAt = parsed
		item.Success = success == 1
		items = append(items, item)
	}

	if items == nil {
		items = []UsageLedgerEntry{}
	}
	return items, rows.Err()
}

func (s *Store) RecordAttempt(ctx context.Context, record execution.Record) error {
	return s.UpdateAccountExecutionState(ctx, record)
}

func (s *Store) RecordSuccess(ctx context.Context, record execution.Record) error {
	if err := s.InsertRequestLog(ctx, RequestLog{
		CreatedAt:      record.CreatedAt,
		Method:         record.Method,
		Path:           record.Endpoint,
		ModelAlias:     record.ModelAlias,
		PlatformID:     record.PlatformID,
		TargetModel:    record.TargetModel,
		AccessToken:    record.AccessTokenName,
		StatusCode:     record.StatusCode,
		LatencyMS:      record.LatencyMS,
		Success:        true,
		ErrorMessage:   "",
		ResponseStream: record.Stream,
	}); err != nil {
		return err
	}

	if err := s.InsertUsageLedgerEntry(ctx, UsageLedgerEntry{
		RequestID:        record.RequestID,
		CreatedAt:        record.CreatedAt,
		AccessTokenName:  record.AccessTokenName,
		ModelAlias:       record.ModelAlias,
		PlatformID:       record.PlatformID,
		AccountID:        record.AccountID,
		TargetModel:      record.TargetModel,
		StatusCode:       record.StatusCode,
		LatencyMS:        record.LatencyMS,
		Success:          true,
		APICostUnits:     record.APICostUnits,
		AccountCostUnits: record.AccountCostUnits,
		AccountCostType:  record.AccountCostType,
		ErrorMessage:     "",
	}); err != nil {
		return err
	}

	return s.UpdateAccountExecutionState(ctx, record)
}

func (s *Store) RecordFailure(ctx context.Context, record execution.Record) error {
	if err := s.InsertRequestLog(ctx, RequestLog{
		CreatedAt:      record.CreatedAt,
		Method:         record.Method,
		Path:           record.Endpoint,
		ModelAlias:     record.ModelAlias,
		PlatformID:     record.PlatformID,
		TargetModel:    record.TargetModel,
		AccessToken:    record.AccessTokenName,
		StatusCode:     record.StatusCode,
		LatencyMS:      record.LatencyMS,
		Success:        false,
		ErrorMessage:   record.ErrorMessage,
		ResponseStream: record.Stream,
	}); err != nil {
		return err
	}

	if err := s.InsertUsageLedgerEntry(ctx, UsageLedgerEntry{
		RequestID:        record.RequestID,
		CreatedAt:        record.CreatedAt,
		AccessTokenName:  record.AccessTokenName,
		ModelAlias:       record.ModelAlias,
		PlatformID:       record.PlatformID,
		AccountID:        record.AccountID,
		TargetModel:      record.TargetModel,
		StatusCode:       record.StatusCode,
		LatencyMS:        record.LatencyMS,
		Success:          false,
		APICostUnits:     record.APICostUnits,
		AccountCostUnits: record.AccountCostUnits,
		AccountCostType:  record.AccountCostType,
		ErrorMessage:     record.ErrorMessage,
	}); err != nil {
		return err
	}

	return s.UpdateAccountExecutionState(ctx, record)
}

func (s *Store) UpdateAccountExecutionState(ctx context.Context, record execution.Record) error {
	if s == nil || s.db == nil || record.AccountID == "" {
		return nil
	}

	var cooldownUntil string
	if record.CooldownUntil != nil {
		cooldownUntil = record.CooldownUntil.UTC().Format(time.RFC3339)
	}

	var lastSuccessAt string
	if record.LastSuccessAt != nil {
		lastSuccessAt = record.LastSuccessAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE accounts
SET cooldown_until = ?,
    last_success_at = CASE WHEN ? = '' THEN last_success_at ELSE ? END,
    last_error = ?,
    updated_at = ?
WHERE id = ?`,
		cooldownUntil,
		lastSuccessAt,
		lastSuccessAt,
		record.LastError,
		time.Now().UTC().Format(time.RFC3339),
		record.AccountID,
	)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func parseOptionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s *Store) columnExists(table string, column string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
