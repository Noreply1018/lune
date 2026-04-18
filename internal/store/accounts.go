package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

var accountColumns = `id, label, source_kind, base_url, api_key, provider,
	cpa_service_id, cpa_provider, cpa_account_key, cpa_email, cpa_plan_type, cpa_openai_id,
	cpa_expired_at, cpa_last_refresh_at, cpa_disabled,
	codex_quota_json, codex_quota_fetched_at,
	probe_models, last_probe_status, last_probe_at, last_probe_error,
	enabled, status, notes, quota_display, last_checked_at, last_error, created_at, updated_at`

var accountColumnsWithAlias = `a.id, a.label, a.source_kind, a.base_url, a.api_key, a.provider,
	a.cpa_service_id, a.cpa_provider, a.cpa_account_key, a.cpa_email, a.cpa_plan_type, a.cpa_openai_id,
	a.cpa_expired_at, a.cpa_last_refresh_at, a.cpa_disabled,
	a.codex_quota_json, a.codex_quota_fetched_at,
	a.probe_models, a.last_probe_status, a.last_probe_at, a.last_probe_error,
	a.enabled, a.status, a.notes, a.quota_display, a.last_checked_at, a.last_error, a.created_at, a.updated_at`

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountColumns + ` FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func (s *Store) GetAccount(id int64) (*Account, error) {
	row := s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id)
	a, err := scanAccountRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Store) CreateAccount(a *Account) (int64, error) {
	if a.SourceKind == "" {
		a.SourceKind = "openai_compat"
	}
	res, err := s.db.Exec(
		`INSERT INTO accounts (label, source_kind, base_url, api_key, provider,
			cpa_service_id, cpa_provider, cpa_account_key, cpa_email, cpa_plan_type, cpa_openai_id,
			cpa_expired_at, cpa_last_refresh_at, cpa_disabled,
			enabled, status, notes, quota_display)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Label, a.SourceKind, a.BaseURL, a.APIKey, a.Provider,
		a.CpaServiceID, a.CpaProvider, a.CpaAccountKey, a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID,
		a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
		a.Enabled, "healthy", a.Notes, a.QuotaDisplay,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAccount(id int64, a *Account) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET label=?, base_url=?, api_key=?, provider=?, enabled=?, notes=?, quota_display=?, updated_at=datetime('now') WHERE id=?`,
		a.Label, a.BaseURL, a.APIKey, a.Provider, a.Enabled, a.Notes, a.QuotaDisplay, id,
	)
	return err
}

func (s *Store) EnableAccount(id int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DisableAccount(id int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET enabled=0, status='disabled', updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DeleteAccount(id int64) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id)
	return err
}

func (s *Store) UpdateAccountHealth(id int64, status, lastError string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET status=?, last_error=?, last_checked_at=datetime('now'), updated_at=datetime('now') WHERE id=?`,
		status, lastError, id,
	)
	return err
}

func (s *Store) CountAccounts() (total int, byStatus map[string]int, err error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM accounts GROUP BY status`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	byStatus = make(map[string]int)
	for rows.Next() {
		var st string
		var c int
		if err := rows.Scan(&st, &c); err != nil {
			return 0, nil, err
		}
		byStatus[st] = c
		total += c
	}
	return total, byStatus, rows.Err()
}

func (s *Store) CountAccountsByCpaService(serviceID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE cpa_service_id = ?`, serviceID).Scan(&count)
	return count, err
}

func (s *Store) CountAccountsBySource() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT source_kind, COUNT(*) FROM accounts GROUP BY source_kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int)
	for rows.Next() {
		var kind string
		var c int
		if err := rows.Scan(&kind, &c); err != nil {
			return nil, err
		}
		m[kind] = c
	}
	return m, rows.Err()
}

func (s *Store) FindAccountByCpaKey(serviceID int64, accountKey string) (*Account, error) {
	row := s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE cpa_service_id = ? AND cpa_account_key = ?`, serviceID, accountKey)
	a, err := scanAccountRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Store) ListCpaAccountsWithKey() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountColumns + ` FROM accounts WHERE source_kind = 'cpa' AND cpa_account_key != '' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func (s *Store) UpdateAccountCpaMetadata(id int64, expiredAt, lastRefreshAt string, disabled bool) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET cpa_expired_at=?, cpa_last_refresh_at=?, cpa_disabled=?, updated_at=datetime('now') WHERE id=?`,
		expiredAt, lastRefreshAt, disabled, id,
	)
	return err
}

func (s *Store) UpdateAccountCodexQuota(id int64, quotaJSON, fetchedAt string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET codex_quota_json=?, codex_quota_fetched_at=? WHERE id=?`,
		quotaJSON, fetchedAt, id,
	)
	return err
}

// UpdateAccountProbeModels persists the user's probe-model selection.
// A nil slice is stored as "[]" so the column never holds NULL or an empty
// string — scanAccountRow relies on that invariant.
func (s *Store) UpdateAccountProbeModels(id int64, models []string) error {
	if models == nil {
		models = []string{}
	}
	buf, err := json.Marshal(models)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE accounts SET probe_models=?, updated_at=datetime('now') WHERE id=?`,
		string(buf), id,
	)
	return err
}

// UpdateAccountProbeResult writes the outcome of a self-check run. `status`
// mirrors the health-badge vocabulary ("healthy" | "degraded" | "error"), and
// `errMsg` is persisted verbatim so the detail drawer can surface the raw
// upstream error.
func (s *Store) UpdateAccountProbeResult(id int64, status, errMsg string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(
		`UPDATE accounts SET last_probe_status=?, last_probe_error=?, last_probe_at=?, updated_at=datetime('now') WHERE id=?`,
		status, errMsg, now, id,
	)
	return err
}

// --- scan helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccounts(rows *sql.Rows) ([]Account, error) {
	var accs []Account
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, err
		}
		accs = append(accs, *a)
	}
	return accs, rows.Err()
}

func scanAccountRow(row rowScanner) (*Account, error) {
	var a Account
	var enabled, cpaDisabled int
	var cpaServiceID sql.NullInt64
	var lastCheckedAt sql.NullString
	var createdAt, updatedAt sql.NullString
	var probeModelsJSON string
	var lastProbeAt sql.NullString

	err := row.Scan(
		&a.ID, &a.Label, &a.SourceKind, &a.BaseURL, &a.APIKey, &a.Provider,
		&cpaServiceID, &a.CpaProvider, &a.CpaAccountKey, &a.CpaEmail, &a.CpaPlanType, &a.CpaOpenaiID,
		&a.CpaExpiredAt, &a.CpaLastRefreshAt, &cpaDisabled,
		&a.CodexQuotaJSON, &a.CodexQuotaFetchedAt,
		&probeModelsJSON, &a.LastProbeStatus, &lastProbeAt, &a.LastProbeError,
		&enabled, &a.Status, &a.Notes, &a.QuotaDisplay, &lastCheckedAt, &a.LastError, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	a.Enabled = enabled != 0
	a.CpaDisabled = cpaDisabled != 0

	if cpaServiceID.Valid {
		id := cpaServiceID.Int64
		a.CpaServiceID = &id
	}
	if lastCheckedAt.Valid {
		a.LastCheckedAt = &lastCheckedAt.String
	}
	if createdAt.Valid {
		a.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		a.UpdatedAt = updatedAt.String
	}
	if a.SourceKind == "" {
		a.SourceKind = "openai_compat"
	}

	a.ProbeModels = []string{}
	if probeModelsJSON != "" {
		// Silently tolerate legacy/garbage payloads — the probe config is
		// advisory; a broken value shouldn't take the account read offline.
		_ = json.Unmarshal([]byte(probeModelsJSON), &a.ProbeModels)
		if a.ProbeModels == nil {
			a.ProbeModels = []string{}
		}
	}
	if lastProbeAt.Valid {
		s := lastProbeAt.String
		a.LastProbeAt = &s
	}

	return &a, nil
}
