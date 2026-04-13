package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

var accountColumns = `id, label, base_url, api_key, enabled, status, quota_total, quota_used, quota_unit, notes, model_allowlist, last_checked_at, last_error, created_at, updated_at, source_kind, provider, cpa_service_id, cpa_provider, cpa_account_key`

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
	a, err := scanAccount(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Store) CreateAccount(a *Account) (int64, error) {
	allowlist, _ := json.Marshal(a.ModelAllowlist)
	if a.ModelAllowlist == nil {
		allowlist = []byte("[]")
	}
	if a.SourceKind == "" {
		a.SourceKind = "openai_compat"
	}
	res, err := s.db.Exec(
		`INSERT INTO accounts (label, base_url, api_key, enabled, status, quota_total, quota_used, quota_unit, notes, model_allowlist, source_kind, provider, cpa_service_id, cpa_provider, cpa_account_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Label, a.BaseURL, a.APIKey, a.Enabled, "healthy", a.QuotaTotal, a.QuotaUsed, a.QuotaUnit, a.Notes, string(allowlist),
		a.SourceKind, a.Provider, a.CpaServiceID, a.CpaProvider, a.CpaAccountKey,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAccount(id int64, a *Account) error {
	allowlist, _ := json.Marshal(a.ModelAllowlist)
	if a.ModelAllowlist == nil {
		allowlist = []byte("[]")
	}
	_, err := s.db.Exec(
		`UPDATE accounts SET label=?, base_url=?, api_key=?, enabled=?, quota_total=?, quota_used=?, quota_unit=?, notes=?, model_allowlist=?, provider=?, updated_at=datetime('now') WHERE id=?`,
		a.Label, a.BaseURL, a.APIKey, a.Enabled, a.QuotaTotal, a.QuotaUsed, a.QuotaUnit, a.Notes, string(allowlist), a.Provider, id,
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

// --- scan helpers ---

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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccountRow(row rowScanner) (*Account, error) {
	var a Account
	var enabled int
	var allowlistJSON string
	var createdAt, updatedAt string
	var cpaServiceID sql.NullInt64
	err := row.Scan(&a.ID, &a.Label, &a.BaseURL, &a.APIKey, &enabled, &a.Status, &a.QuotaTotal, &a.QuotaUsed, &a.QuotaUnit, &a.Notes, &allowlistJSON, &a.LastCheckedAt, &a.LastError, &createdAt, &updatedAt,
		&a.SourceKind, &a.Provider, &cpaServiceID, &a.CpaProvider, &a.CpaAccountKey)
	if err != nil {
		return nil, err
	}
	a.Enabled = enabled != 0
	_ = json.Unmarshal([]byte(allowlistJSON), &a.ModelAllowlist)
	if a.ModelAllowlist == nil {
		a.ModelAllowlist = []string{}
	}
	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	a.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	if cpaServiceID.Valid {
		id := cpaServiceID.Int64
		a.CpaServiceID = &id
	}
	if a.SourceKind == "" {
		a.SourceKind = "openai_compat"
	}
	return &a, nil
}

func scanAccount(row *sql.Row) (*Account, error) {
	return scanAccountRow(row)
}
