package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"
)

var accountColumns = `id, label, base_url, api_key, enabled, status, quota_total, quota_used, quota_unit, notes, model_allowlist, last_checked_at, last_error, created_at, updated_at, source_kind, provider, cpa_service_id, cpa_provider, cpa_account_key, cpa_email, cpa_plan_type, cpa_openai_id, cpa_expired_at, cpa_last_refresh_at, cpa_disabled`

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
		`INSERT INTO accounts (label, base_url, api_key, enabled, status, quota_total, quota_used, quota_unit, notes, model_allowlist, source_kind, provider, cpa_service_id, cpa_provider, cpa_account_key, cpa_email, cpa_plan_type, cpa_openai_id, cpa_expired_at, cpa_last_refresh_at, cpa_disabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Label, a.BaseURL, a.APIKey, a.Enabled, "healthy", a.QuotaTotal, a.QuotaUsed, a.QuotaUnit, a.Notes, string(allowlist),
		a.SourceKind, a.Provider, a.CpaServiceID, a.CpaProvider, a.CpaAccountKey,
		a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID, a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
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
	res, err := s.db.Exec(`UPDATE accounts SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	return s.execByLegacyID(`UPDATE accounts SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
}

func (s *Store) DisableAccount(id int64) error {
	res, err := s.db.Exec(`UPDATE accounts SET enabled=0, status='disabled', updated_at=datetime('now') WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	return s.execByLegacyID(`UPDATE accounts SET enabled=0, status='disabled', updated_at=datetime('now') WHERE id=?`, id)
}

func (s *Store) DeleteAccount(id int64) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	// fallback: scan raw IDs for legacy string-keyed rows
	return s.execByLegacyID(`DELETE FROM accounts WHERE id=?`, id)
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
	var idRaw any
	var enabled, cpaDisabled int
	var allowlistJSON string
	var createdAt, updatedAt sql.NullString
	var cpaServiceID sql.NullInt64
	var cpaExpiredAt, cpaLastRefreshAt sql.NullString
	err := row.Scan(&idRaw, &a.Label, &a.BaseURL, &a.APIKey, &enabled, &a.Status, &a.QuotaTotal, &a.QuotaUsed, &a.QuotaUnit, &a.Notes, &allowlistJSON, &a.LastCheckedAt, &a.LastError, &createdAt, &updatedAt,
		&a.SourceKind, &a.Provider, &cpaServiceID, &a.CpaProvider, &a.CpaAccountKey,
		&a.CpaEmail, &a.CpaPlanType, &a.CpaOpenaiID, &cpaExpiredAt, &cpaLastRefreshAt, &cpaDisabled)
	if err != nil {
		return nil, err
	}
	a.ID = normalizeAccountID(idRaw)
	a.Enabled = enabled != 0
	a.CpaDisabled = cpaDisabled != 0
	_ = json.Unmarshal([]byte(allowlistJSON), &a.ModelAllowlist)
	if a.ModelAllowlist == nil {
		a.ModelAllowlist = []string{}
	}
	if createdAt.Valid {
		a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt.String)
	}
	if updatedAt.Valid {
		a.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt.String)
	}
	if cpaServiceID.Valid {
		id := cpaServiceID.Int64
		a.CpaServiceID = &id
	}
	if cpaExpiredAt.Valid {
		a.CpaExpiredAt = &cpaExpiredAt.String
	}
	if cpaLastRefreshAt.Valid {
		a.CpaLastRefreshAt = &cpaLastRefreshAt.String
	}
	if a.SourceKind == "" {
		a.SourceKind = "openai_compat"
	}
	return &a, nil
}

func normalizeAccountID(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case []byte:
		return normalizeAccountID(string(x))
	case string:
		if x == "" {
			return 0
		}
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(x))
		return int64(h.Sum64() & 0x7fffffffffffffff)
	default:
		if n, err := strconv.ParseInt(fmt.Sprint(v), 10, 64); err == nil {
			return n
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(fmt.Sprint(v)))
		return int64(h.Sum64() & 0x7fffffffffffffff)
	}
}

func scanAccount(row *sql.Row) (*Account, error) {
	return scanAccountRow(row)
}

func (s *Store) UpdateAccountCpaMetadata(id int64, expiredAt, lastRefreshAt *string, disabled bool) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET cpa_expired_at=?, cpa_last_refresh_at=?, cpa_disabled=?, updated_at=datetime('now') WHERE id=?`,
		expiredAt, lastRefreshAt, disabled, id,
	)
	return err
}

func (s *Store) ListCpaAccountsWithKey() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountColumns + ` FROM accounts WHERE source_kind = 'cpa' AND cpa_account_key != '' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func (s *Store) FindAccountByCpaKey(serviceID int64, accountKey string) (*Account, error) {
	row := s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE cpa_service_id = ? AND cpa_account_key = ?`, serviceID, accountKey)
	a, err := scanAccountRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

// execByLegacyID handles legacy accounts whose id column stores a string value.
// normalizeAccountID hashes such strings to int64, but the DB still holds the original string.
// This scans all raw IDs, finds the one whose hash matches, and executes the query with the original value.
// It also handles JS Number precision loss: large int64 values get truncated when passed through
// JavaScript's JSON.parse/stringify cycle (IEEE 754 double has ~15-17 significant digits).
func (s *Store) execByLegacyID(query string, targetID int64) error {
	rows, err := s.db.Query(`SELECT id FROM accounts`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rawID any
		if err := rows.Scan(&rawID); err != nil {
			continue
		}
		normalized := normalizeAccountID(rawID)
		// exact match
		if normalized == targetID {
			_, err := s.db.Exec(query, rawID)
			return err
		}
		// JS Number precision loss: int64 values > 2^53 lose low-order bits when
		// round-tripped through JavaScript Number. Two different int64 values may
		// map to the same JS Number. We consider a match if the difference is
		// small enough to be explained by IEEE 754 double rounding.
		if normalized > 1<<53 || normalized < -(1<<53) {
			diff := normalized - targetID
			if diff < 0 {
				diff = -diff
			}
			// max ULP gap for numbers in this range is ~1024; use 2048 as safety margin
			if diff < 2048 {
				_, err := s.db.Exec(query, rawID)
				return err
			}
		}
	}
	return nil
}
