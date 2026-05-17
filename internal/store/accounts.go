package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

var accountColumns = `id, label, source_kind, base_url, api_key, provider,
	cpa_service_id, cpa_provider, cpa_account_key, cpa_email, cpa_plan_type, cpa_openai_id,
	cpa_expired_at, cpa_last_refresh_at, cpa_disabled,
	cpa_credential_status, cpa_credential_reason, cpa_credential_last_error, cpa_credential_checked_at,
	cpa_subscription_expires_at, cpa_subscription_fetched_at, cpa_subscription_last_error, cpa_subscription_status,
	cpa_access_status, cpa_access_reason, cpa_access_last_error, cpa_access_checked_at,
	codex_quota_json, codex_quota_fetched_at, cpa_quota_status, cpa_quota_last_error, cpa_quota_checked_at,
	cpa_quota_backoff_until, cpa_quota_backoff_count,
	serving_status, failure_count, last_failure_at, last_success_at, cooldown_until,
	probe_models, last_probe_status, last_probe_at, last_probe_error,
	enabled, status, notes, quota_display, last_checked_at, last_error, created_at, updated_at`

var accountColumnsWithAlias = `a.id, a.label, a.source_kind, a.base_url, a.api_key, a.provider,
	a.cpa_service_id, a.cpa_provider, a.cpa_account_key, a.cpa_email, a.cpa_plan_type, a.cpa_openai_id,
	a.cpa_expired_at, a.cpa_last_refresh_at, a.cpa_disabled,
	a.cpa_credential_status, a.cpa_credential_reason, a.cpa_credential_last_error, a.cpa_credential_checked_at,
	a.cpa_subscription_expires_at, a.cpa_subscription_fetched_at, a.cpa_subscription_last_error, a.cpa_subscription_status,
	a.cpa_access_status, a.cpa_access_reason, a.cpa_access_last_error, a.cpa_access_checked_at,
	a.codex_quota_json, a.codex_quota_fetched_at, a.cpa_quota_status, a.cpa_quota_last_error, a.cpa_quota_checked_at,
	a.cpa_quota_backoff_until, a.cpa_quota_backoff_count,
	a.serving_status, a.failure_count, a.last_failure_at, a.last_success_at, a.cooldown_until,
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
			cpa_credential_status, cpa_credential_reason, cpa_credential_last_error, cpa_credential_checked_at,
			cpa_subscription_expires_at, cpa_subscription_fetched_at, cpa_subscription_last_error, cpa_subscription_status,
			cpa_access_status, cpa_access_reason, cpa_access_last_error, cpa_access_checked_at,
			cpa_quota_status, cpa_quota_last_error, cpa_quota_checked_at,
			serving_status, failure_count, last_failure_at, last_success_at, cooldown_until,
			enabled, status, notes, quota_display)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Label, a.SourceKind, a.BaseURL, a.APIKey, a.Provider,
		a.CpaServiceID, a.CpaProvider, a.CpaAccountKey, a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID,
		a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
		defaultCpaCredentialStatus(a.CpaCredentialStatus), a.CpaCredentialReason, a.CpaCredentialLastError, a.CpaCredentialCheckedAt,
		a.CpaSubscriptionExpiresAt, a.CpaSubscriptionFetchedAt, a.CpaSubscriptionLastError, defaultCpaSubscriptionStatus(a.CpaSubscriptionStatus),
		initialCpaAccessStatus(a), initialCpaAccessReason(a), a.CpaAccessLastError, initialCpaAccessCheckedAt(a),
		defaultCpaQuotaStatus(a.CpaQuotaStatus), a.CpaQuotaLastError, a.CpaQuotaCheckedAt,
		defaultServingStatus(a.ServingStatus), a.FailureCount, a.LastFailureAt, a.LastSuccessAt, a.CooldownUntil,
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

func (s *Store) UpdateAccountHealthIfUnchanged(id int64, status, lastError string, previous Account) (bool, error) {
	lastCheckedAt := ""
	if previous.LastCheckedAt != nil {
		lastCheckedAt = *previous.LastCheckedAt
	}
	res, err := s.db.Exec(
		`UPDATE accounts
		 SET status=?, last_error=?, last_checked_at=datetime('now'), updated_at=datetime('now')
		 WHERE id=?
		   AND status=?
		   AND last_error=?
		   AND COALESCE(last_checked_at, '')=?`,
		status, lastError, id,
		previous.Status, previous.LastError, lastCheckedAt,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
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

func (s *Store) UpdateCpaAccountFromImport(id int64, a *Account) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET
			label=?, cpa_provider=?, cpa_email=?, cpa_plan_type=?, cpa_openai_id=?,
			cpa_expired_at=?, cpa_last_refresh_at=?, cpa_disabled=?,
			cpa_credential_status=?, cpa_credential_reason=?, cpa_credential_last_error=?, cpa_credential_checked_at=?,
			cpa_subscription_expires_at=?, cpa_subscription_fetched_at=?, cpa_subscription_last_error=?, cpa_subscription_status=?,
			cpa_access_status=?, cpa_access_reason=?, cpa_access_last_error=?, cpa_access_checked_at=?,
			enabled=?, status='healthy', last_error='', last_checked_at=datetime('now'), notes=?, updated_at=datetime('now')
		 WHERE id=?`,
		a.Label, a.CpaProvider, a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID,
		a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
		defaultCpaCredentialStatus(a.CpaCredentialStatus), a.CpaCredentialReason, a.CpaCredentialLastError, a.CpaCredentialCheckedAt,
		a.CpaSubscriptionExpiresAt, a.CpaSubscriptionFetchedAt, a.CpaSubscriptionLastError, defaultCpaSubscriptionStatus(a.CpaSubscriptionStatus),
		initialCpaAccessStatus(a), initialCpaAccessReason(a), a.CpaAccessLastError, initialCpaAccessCheckedAt(a),
		a.Enabled, a.Notes, id,
	)
	return err
}

func (s *Store) UpsertCpaAccountAndPoolMemberFromImport(poolID int64, a *Account) (*Account, int64, bool, error) {
	if a == nil || a.CpaServiceID == nil {
		return nil, 0, false, sql.ErrNoRows
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, false, err
	}
	defer tx.Rollback()

	existing, err := scanAccountRow(tx.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE cpa_service_id = ? AND cpa_account_key = ?`, *a.CpaServiceID, a.CpaAccountKey))
	created := false
	if err == sql.ErrNoRows {
		res, err := tx.Exec(
			`INSERT INTO accounts (label, source_kind, base_url, api_key, provider,
				cpa_service_id, cpa_provider, cpa_account_key, cpa_email, cpa_plan_type, cpa_openai_id,
				cpa_expired_at, cpa_last_refresh_at, cpa_disabled,
				cpa_credential_status, cpa_credential_reason, cpa_credential_last_error, cpa_credential_checked_at,
				cpa_subscription_expires_at, cpa_subscription_fetched_at, cpa_subscription_last_error, cpa_subscription_status,
				cpa_access_status, cpa_access_reason, cpa_access_last_error, cpa_access_checked_at,
				cpa_quota_status, cpa_quota_last_error, cpa_quota_checked_at,
				serving_status, failure_count, last_failure_at, last_success_at, cooldown_until,
				enabled, status, notes, quota_display)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.Label, firstNonEmpty(a.SourceKind, "cpa"), a.BaseURL, a.APIKey, a.Provider,
			a.CpaServiceID, a.CpaProvider, a.CpaAccountKey, a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID,
			a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
			defaultCpaCredentialStatus(a.CpaCredentialStatus), a.CpaCredentialReason, a.CpaCredentialLastError, a.CpaCredentialCheckedAt,
			a.CpaSubscriptionExpiresAt, a.CpaSubscriptionFetchedAt, a.CpaSubscriptionLastError, defaultCpaSubscriptionStatus(a.CpaSubscriptionStatus),
			initialCpaAccessStatus(a), initialCpaAccessReason(a), a.CpaAccessLastError, initialCpaAccessCheckedAt(a),
			defaultCpaQuotaStatus(a.CpaQuotaStatus), a.CpaQuotaLastError, a.CpaQuotaCheckedAt,
			defaultServingStatus(a.ServingStatus), a.FailureCount, a.LastFailureAt, a.LastSuccessAt, a.CooldownUntil,
			a.Enabled, "healthy", a.Notes, a.QuotaDisplay,
		)
		if err != nil {
			return nil, 0, false, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, 0, false, err
		}
		a.ID = id
		created = true
	} else if err != nil {
		return nil, 0, false, err
	} else {
		a.ID = existing.ID
		_, err := tx.Exec(
			`UPDATE accounts SET
				label=?, cpa_provider=?, cpa_email=?, cpa_plan_type=?, cpa_openai_id=?,
				cpa_expired_at=?, cpa_last_refresh_at=?, cpa_disabled=?,
				cpa_credential_status=?, cpa_credential_reason=?, cpa_credential_last_error=?, cpa_credential_checked_at=?,
				cpa_subscription_expires_at=?, cpa_subscription_fetched_at=?, cpa_subscription_last_error=?, cpa_subscription_status=?,
				cpa_access_status=?, cpa_access_reason=?, cpa_access_last_error=?, cpa_access_checked_at=?,
				enabled=?, status='healthy', last_error='', last_checked_at=datetime('now'), notes=?, updated_at=datetime('now')
			 WHERE id=?`,
			a.Label, a.CpaProvider, a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID,
			a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
			defaultCpaCredentialStatus(a.CpaCredentialStatus), a.CpaCredentialReason, a.CpaCredentialLastError, a.CpaCredentialCheckedAt,
			a.CpaSubscriptionExpiresAt, a.CpaSubscriptionFetchedAt, a.CpaSubscriptionLastError, defaultCpaSubscriptionStatus(a.CpaSubscriptionStatus),
			initialCpaAccessStatus(a), initialCpaAccessReason(a), a.CpaAccessLastError, initialCpaAccessCheckedAt(a),
			a.Enabled, a.Notes, existing.ID,
		)
		if err != nil {
			return nil, 0, false, err
		}
	}

	var maxPos sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(position) FROM pool_members WHERE pool_id = ?`, poolID).Scan(&maxPos); err != nil {
		return nil, 0, false, err
	}
	nextPos := 0
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO pool_members (pool_id, account_id, position, enabled) VALUES (?, ?, ?, 1)`, poolID, a.ID, nextPos); err != nil {
		return nil, 0, false, err
	}
	var memberID int64
	if err := tx.QueryRow(`SELECT id FROM pool_members WHERE pool_id = ? AND account_id = ?`, poolID, a.ID).Scan(&memberID); err != nil {
		return nil, 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, false, err
	}
	imported, err := s.GetAccount(a.ID)
	return imported, memberID, created, err
}

func (s *Store) RestoreCpaAccountImportSnapshot(a *Account) error {
	if a == nil {
		return nil
	}
	var lastCheckedAt any
	if a.LastCheckedAt != nil {
		lastCheckedAt = *a.LastCheckedAt
	}
	_, err := s.db.Exec(
		`UPDATE accounts SET
			label=?, cpa_provider=?, cpa_email=?, cpa_plan_type=?, cpa_openai_id=?,
			cpa_expired_at=?, cpa_last_refresh_at=?, cpa_disabled=?,
			cpa_credential_status=?, cpa_credential_reason=?, cpa_credential_last_error=?, cpa_credential_checked_at=?,
			cpa_subscription_expires_at=?, cpa_subscription_fetched_at=?, cpa_subscription_last_error=?, cpa_subscription_status=?,
			cpa_access_status=?, cpa_access_reason=?, cpa_access_last_error=?, cpa_access_checked_at=?,
			enabled=?, status=?, last_error=?, last_checked_at=?, notes=?, updated_at=datetime('now')
		 WHERE id=?`,
		a.Label, a.CpaProvider, a.CpaEmail, a.CpaPlanType, a.CpaOpenaiID,
		a.CpaExpiredAt, a.CpaLastRefreshAt, a.CpaDisabled,
		defaultCpaCredentialStatus(a.CpaCredentialStatus), a.CpaCredentialReason, a.CpaCredentialLastError, a.CpaCredentialCheckedAt,
		a.CpaSubscriptionExpiresAt, a.CpaSubscriptionFetchedAt, a.CpaSubscriptionLastError, defaultCpaSubscriptionStatus(a.CpaSubscriptionStatus),
		defaultCpaAccessStatus(a.CpaAccessStatus), a.CpaAccessReason, a.CpaAccessLastError, a.CpaAccessCheckedAt,
		a.Enabled, a.Status, a.LastError, lastCheckedAt, a.Notes, a.ID,
	)
	return err
}

func (s *Store) UpdateAccountCpaCredentialStatus(id int64, status, reason, lastError, checkedAt string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET cpa_credential_status=?, cpa_credential_reason=?, cpa_credential_last_error=?, cpa_credential_checked_at=?, updated_at=datetime('now') WHERE id=?`,
		defaultCpaCredentialStatus(status), reason, lastError, checkedAt, id,
	)
	return err
}

func (s *Store) UpdateAccountCpaAccessStatus(id int64, status, reason, lastError, checkedAt string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET cpa_access_status=?, cpa_access_reason=?, cpa_access_last_error=?, cpa_access_checked_at=?, updated_at=datetime('now') WHERE id=?`,
		defaultCpaAccessStatus(status), reason, lastError, checkedAt, id,
	)
	return err
}

func (s *Store) UpdateAccountCodexQuota(id int64, quotaJSON, fetchedAt string) error {
	_, err := s.db.Exec(
		`UPDATE accounts
		 SET codex_quota_json=?,
		     codex_quota_fetched_at=?,
		     cpa_quota_status=CASE
		       WHEN cpa_quota_last_error LIKE 'HTTP 429 from model request%' THEN cpa_quota_status
		       ELSE 'ok'
		     END,
		     cpa_quota_last_error=CASE
		       WHEN cpa_quota_last_error LIKE 'HTTP 429 from model request%' THEN cpa_quota_last_error
		       ELSE ''
		     END,
		     cpa_quota_checked_at=?,
		     cpa_quota_backoff_until='',
		     cpa_quota_backoff_count=0,
		     updated_at=datetime('now')
		 WHERE id=?`,
		quotaJSON, fetchedAt, fetchedAt, id,
	)
	return err
}

func (s *Store) ClearAccountCodexModelRequestQuotaEvidence(id int64) error {
	_, err := s.db.Exec(
		`UPDATE accounts
		 SET cpa_quota_status=CASE
		       WHEN cpa_quota_status IN ('error', 'blocked') AND cpa_quota_last_error LIKE 'HTTP 429 from model request%' THEN 'ok'
		       ELSE cpa_quota_status
		     END,
		     cpa_quota_last_error=CASE
		       WHEN cpa_quota_last_error LIKE 'HTTP 429 from model request%' THEN ''
		       ELSE cpa_quota_last_error
		     END,
		     updated_at=datetime('now')
		 WHERE id=?`,
		id,
	)
	return err
}

func (s *Store) UpdateAccountCodexQuotaStatus(id int64, status, lastError, checkedAt string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET cpa_quota_status=?, cpa_quota_last_error=?, cpa_quota_checked_at=?, updated_at=datetime('now') WHERE id=?`,
		defaultCpaQuotaStatus(status), lastError, checkedAt, id,
	)
	return err
}

func (s *Store) UpdateAccountCodexQuotaBackoff(id int64, until string, count int) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET cpa_quota_backoff_until=?, cpa_quota_backoff_count=?, updated_at=datetime('now') WHERE id=?`,
		until, count, id,
	)
	return err
}

func (s *Store) MarkAccountServingSuccess(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE accounts
		 SET serving_status='healthy', failure_count=0, last_success_at=?, cooldown_until='',
		     status='healthy', last_error='', last_checked_at=datetime('now'), updated_at=datetime('now')
		 WHERE id=?`,
		now, id,
	)
	return err
}

func (s *Store) MarkAccountServingFailure(id int64, lastError string, cooldownUntil time.Time) error {
	until := cooldownUntil.UTC().Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE accounts
		 SET serving_status='cooldown', failure_count=failure_count + 1, last_failure_at=?, cooldown_until=?,
		     last_error=?, last_checked_at=datetime('now'), updated_at=datetime('now')
		 WHERE id=?`,
		now, until, lastError, id,
	)
	return err
}

func (s *Store) UpdateAccountQuotaDisplay(id int64, quotaDisplay string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET quota_display=?, updated_at=datetime('now') WHERE id=?`,
		quotaDisplay, id,
	)
	return err
}

func (s *Store) UpdateAccountCpaSubscription(id int64, expiresAt, fetchedAt, lastError string) error {
	status := deriveCpaSubscriptionStatus(expiresAt, lastError)
	_, err := s.db.Exec(
		`UPDATE accounts
		 SET cpa_subscription_expires_at=?,
		     cpa_subscription_fetched_at=?,
		     cpa_subscription_last_error=?,
		     cpa_subscription_status=?,
		     cpa_access_status=CASE
		       WHEN ? = 'active' THEN 'eligible'
		       WHEN ? = 'expired' AND (cpa_access_status <> 'eligible' OR cpa_access_reason = 'subscription_active') THEN 'ineligible'
		       ELSE cpa_access_status
		     END,
		     cpa_access_reason=CASE
		       WHEN ? = 'active' THEN 'subscription_active'
		       WHEN ? = 'expired' AND (cpa_access_status <> 'eligible' OR cpa_access_reason = 'subscription_active') THEN 'subscription_expired'
		       ELSE cpa_access_reason
		     END,
		     cpa_access_last_error=CASE WHEN ? = 'active' THEN '' ELSE cpa_access_last_error END,
		     cpa_access_checked_at=CASE WHEN ? IN ('active', 'expired') THEN ? ELSE cpa_access_checked_at END,
		     updated_at=datetime('now')
		 WHERE id=?`,
		expiresAt, fetchedAt, lastError, status,
		status, status, status, status, status, status, fetchedAt, id,
	)
	return err
}

func (s *Store) UpdateAccountCpaSubscriptionStatus(id int64, status, lastError, checkedAt string) error {
	_, err := s.db.Exec(
		`UPDATE accounts
		 SET cpa_subscription_status=?,
		     cpa_subscription_last_error=?,
		     cpa_subscription_fetched_at=?,
		     cpa_access_status=CASE
		       WHEN ? = 'active' THEN 'eligible'
		       WHEN ? = 'expired' AND (cpa_access_status <> 'eligible' OR cpa_access_reason = 'subscription_active') THEN 'ineligible'
		       ELSE cpa_access_status
		     END,
		     cpa_access_reason=CASE
		       WHEN ? = 'active' THEN 'subscription_active'
		       WHEN ? = 'expired' AND (cpa_access_status <> 'eligible' OR cpa_access_reason = 'subscription_active') THEN 'subscription_expired'
		       ELSE cpa_access_reason
		     END,
		     cpa_access_last_error=CASE WHEN ? = 'active' THEN '' ELSE cpa_access_last_error END,
		     cpa_access_checked_at=CASE WHEN ? IN ('active', 'expired') THEN ? ELSE cpa_access_checked_at END,
		     updated_at=datetime('now')
		 WHERE id=?`,
		defaultCpaSubscriptionStatus(status), lastError, checkedAt,
		defaultCpaSubscriptionStatus(status), defaultCpaSubscriptionStatus(status),
		defaultCpaSubscriptionStatus(status), defaultCpaSubscriptionStatus(status),
		defaultCpaSubscriptionStatus(status), defaultCpaSubscriptionStatus(status), checkedAt, id,
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

	state := newAccountScanState(&a)
	if err := row.Scan(state.targets()...); err != nil {
		return nil, err
	}
	state.apply()
	return &a, nil
}

type accountScanState struct {
	account *Account

	enabled         int
	cpaDisabled     int
	cpaServiceID    sql.NullInt64
	lastCheckedAt   sql.NullString
	createdAt       sql.NullString
	updatedAt       sql.NullString
	probeModelsJSON string
	lastProbeAt     sql.NullString
}

func newAccountScanState(account *Account) *accountScanState {
	return &accountScanState{account: account}
}

func (s *accountScanState) targets() []any {
	a := s.account
	return []any{
		&a.ID, &a.Label, &a.SourceKind, &a.BaseURL, &a.APIKey, &a.Provider,
		&s.cpaServiceID, &a.CpaProvider, &a.CpaAccountKey, &a.CpaEmail, &a.CpaPlanType, &a.CpaOpenaiID,
		&a.CpaExpiredAt, &a.CpaLastRefreshAt, &s.cpaDisabled,
		&a.CpaCredentialStatus, &a.CpaCredentialReason, &a.CpaCredentialLastError, &a.CpaCredentialCheckedAt,
		&a.CpaSubscriptionExpiresAt, &a.CpaSubscriptionFetchedAt, &a.CpaSubscriptionLastError, &a.CpaSubscriptionStatus,
		&a.CpaAccessStatus, &a.CpaAccessReason, &a.CpaAccessLastError, &a.CpaAccessCheckedAt,
		&a.CodexQuotaJSON, &a.CodexQuotaFetchedAt, &a.CpaQuotaStatus, &a.CpaQuotaLastError, &a.CpaQuotaCheckedAt,
		&a.CpaQuotaBackoffUntil, &a.CpaQuotaBackoffCount,
		&a.ServingStatus, &a.FailureCount, &a.LastFailureAt, &a.LastSuccessAt, &a.CooldownUntil,
		&s.probeModelsJSON, &a.LastProbeStatus, &s.lastProbeAt, &a.LastProbeError,
		&s.enabled, &a.Status, &a.Notes, &a.QuotaDisplay, &s.lastCheckedAt, &a.LastError, &s.createdAt, &s.updatedAt,
	}
}

func (s *accountScanState) apply() {
	a := s.account
	a.Enabled = s.enabled != 0
	a.CpaDisabled = s.cpaDisabled != 0
	a.CpaCredentialStatus = defaultCpaCredentialStatus(a.CpaCredentialStatus)
	a.CpaSubscriptionStatus = defaultCpaSubscriptionStatus(a.CpaSubscriptionStatus)
	a.CpaAccessStatus = defaultCpaAccessStatus(a.CpaAccessStatus)
	a.CpaQuotaStatus = defaultCpaQuotaStatus(a.CpaQuotaStatus)
	a.ServingStatus = defaultServingStatus(a.ServingStatus)

	if s.cpaServiceID.Valid {
		id := s.cpaServiceID.Int64
		a.CpaServiceID = &id
	}
	if s.lastCheckedAt.Valid {
		a.LastCheckedAt = &s.lastCheckedAt.String
	}
	if s.createdAt.Valid {
		a.CreatedAt = s.createdAt.String
	}
	if s.updatedAt.Valid {
		a.UpdatedAt = s.updatedAt.String
	}
	if a.SourceKind == "" {
		a.SourceKind = "openai_compat"
	}

	a.ProbeModels = []string{}
	if s.probeModelsJSON != "" {
		// Silently tolerate legacy/garbage payloads — the probe config is
		// advisory; a broken value shouldn't take the account read offline.
		_ = json.Unmarshal([]byte(s.probeModelsJSON), &a.ProbeModels)
		if a.ProbeModels == nil {
			a.ProbeModels = []string{}
		}
	}
	if s.lastProbeAt.Valid {
		value := s.lastProbeAt.String
		a.LastProbeAt = &value
	}
}

func defaultCpaCredentialStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

func defaultCpaQuotaStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

func defaultCpaSubscriptionStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

func defaultCpaAccessStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

func initialCpaAccessStatus(a *Account) string {
	if a == nil {
		return "unknown"
	}
	if strings.TrimSpace(a.CpaAccessStatus) != "" {
		return defaultCpaAccessStatus(a.CpaAccessStatus)
	}
	if strings.EqualFold(a.CpaProvider, "codex") {
		switch strings.ToLower(a.CpaSubscriptionStatus) {
		case "active":
			return "eligible"
		case "expired":
			return "ineligible"
		}
	}
	return "unknown"
}

func initialCpaAccessReason(a *Account) string {
	if a == nil {
		return ""
	}
	if strings.TrimSpace(a.CpaAccessReason) != "" || strings.TrimSpace(a.CpaAccessStatus) != "" {
		return firstNonEmpty(a.CpaAccessReason)
	}
	if strings.EqualFold(a.CpaProvider, "codex") {
		switch strings.ToLower(a.CpaSubscriptionStatus) {
		case "active":
			return "subscription_active"
		case "expired":
			return "subscription_expired"
		}
	}
	return ""
}

func initialCpaAccessCheckedAt(a *Account) string {
	if a == nil {
		return ""
	}
	if strings.TrimSpace(a.CpaAccessCheckedAt) != "" || strings.TrimSpace(a.CpaAccessStatus) != "" {
		return firstNonEmpty(a.CpaAccessCheckedAt)
	}
	if strings.EqualFold(a.CpaProvider, "codex") &&
		(strings.EqualFold(a.CpaSubscriptionStatus, "active") || strings.EqualFold(a.CpaSubscriptionStatus, "expired")) {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return ""
}

func deriveCpaSubscriptionStatus(expiresAt, lastError string) string {
	lastError = strings.TrimSpace(lastError)
	if strings.EqualFold(lastError, "subscription metadata pending") {
		expiresAt = strings.TrimSpace(expiresAt)
		if t, ok := parseCpaSubscriptionTime(expiresAt); ok {
			if t.After(time.Now().UTC()) {
				return "active"
			}
			return "expired"
		}
		return "pending"
	}
	if lastError != "" {
		return "error"
	}
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return "unknown"
	}
	if t, ok := parseCpaSubscriptionTime(expiresAt); ok {
		if t.After(time.Now().UTC()) {
			return "active"
		}
		return "expired"
	}
	return "error"
}

func parseCpaSubscriptionTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func defaultServingStatus(status string) string {
	if status == "" {
		return "healthy"
	}
	return status
}
