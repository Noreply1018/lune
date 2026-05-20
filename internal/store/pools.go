package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const accountRoutableBaseWhereSQL = `a.enabled = 1
		AND a.status IN ('healthy', 'degraded')
		AND a.serving_status <> 'error'
		AND (a.serving_status <> 'cooldown' OR (a.cooldown_until <> '' AND datetime(a.cooldown_until) <= datetime('now')))
		AND NOT EXISTS (
			SELECT 1 FROM account_diagnostics ad
			WHERE ad.account_id = a.id
			  AND (
				lower(trim(ad.scheduler_status)) NOT IN ('eligible', 'eligible_with_warning')
				OR (
					trim(COALESCE(ad.scheduler_override, '')) = ''
					AND lower(trim(ad.stable_diagnostic_status)) IN ('banned', 'quota_exhausted', 'auth_invalid')
				)
			  )
		)
		AND (
			a.source_kind <> 'cpa'
			OR (
				COALESCE((SELECT value FROM system_config WHERE key='cpa_provider_pinning_supported'), '0') = '1'
				AND lower(a.cpa_credential_status) NOT IN ('needs_login', 'refresh_failed', 'runtime_pending', 'runtime_error', 'unknown', '')
				AND lower(a.cpa_quota_status) <> 'blocked'
				AND NOT (lower(a.cpa_provider) = 'codex' AND lower(a.cpa_quota_status) = 'error' AND a.cpa_quota_last_error LIKE 'HTTP 429 from model request%')
				AND (
					lower(a.cpa_provider) <> 'codex'
					OR lower(a.cpa_access_status) = 'eligible'
				)
			)
		)`

const accountRoutableWhereSQL = accountRoutableBaseWhereSQL

const routableAccountWhereSQL = `pm.enabled = 1 AND ` + accountRoutableWhereSQL

func (s *Store) ListPools() ([]Pool, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.label, p.priority, p.enabled, p.routing_policy, p.created_at, p.updated_at,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status = 'healthy') AS healthy_account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND ` + routableAccountWhereSQL + `) AS routable_account_count
		FROM pools p
		ORDER BY p.priority ASC, p.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pools := []Pool{}
	for rows.Next() {
		p, err := scanPoolRowWithCounts(rows)
		if err != nil {
			return nil, err
		}
		pools = append(pools, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// load models for each pool
	for i := range pools {
		models, err := s.GetPoolModels(pools[i].ID)
		if err != nil {
			return nil, err
		}
		pools[i].Models = models
	}

	return pools, nil
}

func (s *Store) GetPool(id int64) (*Pool, error) {
	row := s.db.QueryRow(`
		SELECT p.id, p.label, p.priority, p.enabled, p.routing_policy, p.created_at, p.updated_at,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status = 'healthy') AS healthy_account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND `+routableAccountWhereSQL+`) AS routable_account_count
		FROM pools p
		WHERE p.id = ?`, id)

	p, err := scanPoolRowWithCounts(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	models, err := s.GetPoolModels(p.ID)
	if err != nil {
		return nil, err
	}
	p.Models = models

	return p, nil
}

func (s *Store) GetPoolByLabel(label string) (*Pool, error) {
	row := s.db.QueryRow(`
		SELECT p.id, p.label, p.priority, p.enabled, p.routing_policy, p.created_at, p.updated_at,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status = 'healthy') AS healthy_account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND `+routableAccountWhereSQL+`) AS routable_account_count
		FROM pools p
		WHERE p.label = ?`, label)

	p, err := scanPoolRowWithCounts(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	models, err := s.GetPoolModels(p.ID)
	if err != nil {
		return nil, err
	}
	p.Models = models
	return p, nil
}

func (s *Store) CreatePool(label string, priority int, enabled bool) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO pools (label, priority, enabled) VALUES (?, ?, ?)`,
		label, priority, enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreatePoolWithDefaultToken creates a pool and a default pool-scoped access
// token in the same transaction. The token name is derived from `label` with a
// consistent suffix so Settings can present one credential per Pool. Returns
// the new pool ID and token ID. Note: `access_tokens.name` is NOT unique, and
// for a brand new pool there is no pre-existing token to collide with, so this
// always succeeds for the happy path. The only real failure modes are the usual
// transaction errors (I/O, token-value UNIQUE collision from rand failure).
func (s *Store) CreatePoolWithDefaultToken(label string, priority int, enabled bool) (poolID, tokenID int64, err error) {
	tok, err := generateToken()
	if err != nil {
		return 0, 0, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	poolRes, err := tx.Exec(
		`INSERT INTO pools (label, priority, enabled) VALUES (?, ?, ?)`,
		label, priority, enabled,
	)
	if err != nil {
		return 0, 0, err
	}
	poolID, err = poolRes.LastInsertId()
	if err != nil {
		return 0, 0, err
	}

	tokenName := defaultPoolTokenName(label)
	tokRes, err := tx.Exec(
		`INSERT INTO access_tokens (name, token, pool_id, enabled) VALUES (?, ?, ?, ?)`,
		tokenName, tok, poolID, true,
	)
	if err != nil {
		return 0, 0, err
	}
	tokenID, err = tokRes.LastInsertId()
	if err != nil {
		return 0, 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return poolID, tokenID, nil
}

func (s *Store) UpdatePool(id int64, label string, priority int, enabled bool) error {
	_, err := s.db.Exec(
		`UPDATE pools SET label=?, priority=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
		label, priority, enabled, id,
	)
	return err
}

func (s *Store) UpdatePoolWithRoutingPolicy(id int64, label string, priority int, enabled bool, routingPolicy string) error {
	routingPolicy = NormalizeRoutingPolicy(routingPolicy)
	if !ValidRoutingPolicy(routingPolicy) {
		return fmt.Errorf("invalid routing policy: %s", routingPolicy)
	}
	_, err := s.db.Exec(
		`UPDATE pools SET label=?, priority=?, enabled=?, routing_policy=?, updated_at=datetime('now') WHERE id=?`,
		label, priority, enabled, routingPolicy, id,
	)
	return err
}

func NormalizeRoutingPolicy(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "health_first"
	}
	return value
}

func ValidRoutingPolicy(value string) bool {
	switch value {
	case "health_first", "ordered":
		return true
	default:
		return false
	}
}

func (s *Store) EnablePool(id int64) error {
	_, err := s.db.Exec(`UPDATE pools SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DisablePool(id int64) error {
	_, err := s.db.Exec(`UPDATE pools SET enabled=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

// DeletePoolWithAccounts deletes a pool and all accounts that currently belong
// to it. Deleting the accounts also removes their memberships in any other
// pools through the pool_members account_id foreign key.
func (s *Store) DeletePoolWithAccounts(id int64) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = s.deletePoolWithAccounts(id); err == nil {
			return nil
		}
		if !isSQLiteBusy(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	return err
}

func (s *Store) deletePoolWithAccounts(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT account_id FROM pool_members WHERE pool_id = ?`, id)
	if err != nil {
		return err
	}
	var accountIDs []int64
	for rows.Next() {
		var aid int64
		if err := rows.Scan(&aid); err != nil {
			rows.Close()
			return err
		}
		accountIDs = append(accountIDs, aid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM pools WHERE id = ?`, id); err != nil {
		return err
	}

	for _, aid := range accountIDs {
		if _, err := tx.Exec(`DELETE FROM accounts WHERE id = ?`, aid); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}

// DeletePoolWithOrphans deletes a pool and cleans up orphan accounts
// (accounts that are not referenced by any remaining pool_member).
func (s *Store) DeletePoolWithOrphans(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Collect account IDs that belong to this pool (before deletion)
	rows, err := tx.Query(`SELECT account_id FROM pool_members WHERE pool_id = ?`, id)
	if err != nil {
		return err
	}
	var accountIDs []int64
	for rows.Next() {
		var aid int64
		if err := rows.Scan(&aid); err != nil {
			rows.Close()
			return err
		}
		accountIDs = append(accountIDs, aid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Delete the pool (CASCADE will remove pool_members)
	if _, err := tx.Exec(`DELETE FROM pools WHERE id = ?`, id); err != nil {
		return err
	}

	// Delete orphan accounts: those not in any remaining pool_member
	for _, aid := range accountIDs {
		var cnt int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pool_members WHERE account_id = ?`, aid).Scan(&cnt); err != nil {
			return err
		}
		if cnt == 0 {
			if _, err := tx.Exec(`DELETE FROM accounts WHERE id = ?`, aid); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *Store) CountPools() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pools`).Scan(&n)
	return n, err
}

// --- Pool Members ---

func (s *Store) ListPoolMembers(poolID int64) ([]PoolMember, error) {
	rows, err := s.db.Query(`
		SELECT pm.id, pm.pool_id, pm.account_id, pm.position, pm.enabled,
			`+accountColumnsWithAlias+`
		FROM pool_members pm
		JOIN accounts a ON a.id = pm.account_id
		WHERE pm.pool_id = ?
		ORDER BY pm.position, pm.id`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []PoolMember
	for rows.Next() {
		var m PoolMember
		var mEnabled int
		a, err := scanPoolMemberWithAccount(rows, &m, &mEnabled)
		if err != nil {
			return nil, err
		}
		m.Enabled = mEnabled != 0
		m.Account = a
		members = append(members, m)
	}
	if members == nil {
		members = []PoolMember{}
	}
	return members, rows.Err()
}

// AddPoolMember adds an account to a pool with auto-assigned position (max+1).
func (s *Store) AddPoolMember(poolID, accountID int64) (int64, error) {
	var maxPos sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(position) FROM pool_members WHERE pool_id = ?`, poolID).Scan(&maxPos)
	if err != nil {
		return 0, err
	}
	nextPos := 0
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	res, err := s.db.Exec(
		`INSERT INTO pool_members (pool_id, account_id, position, enabled) VALUES (?, ?, ?, 1)`,
		poolID, accountID, nextPos,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AddPoolMemberIdempotent(poolID, accountID int64) (int64, bool, error) {
	var maxPos sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(position) FROM pool_members WHERE pool_id = ?`, poolID).Scan(&maxPos); err != nil {
		return 0, false, err
	}
	nextPos := 0
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO pool_members (pool_id, account_id, position, enabled) VALUES (?, ?, ?, 1)`,
		poolID, accountID, nextPos,
	)
	if err != nil {
		return 0, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}

	var id int64
	if err := s.db.QueryRow(
		`SELECT id FROM pool_members WHERE pool_id = ? AND account_id = ?`,
		poolID, accountID,
	).Scan(&id); err != nil {
		return 0, false, err
	}
	return id, affected > 0, nil
}

// RemovePoolMember removes a member by its ID.
func (s *Store) RemovePoolMember(memberID int64) error {
	_, err := s.db.Exec(`DELETE FROM pool_members WHERE id = ?`, memberID)
	return err
}

// UpdatePoolMember updates the enabled state of a pool member.
func (s *Store) UpdatePoolMember(memberID int64, enabled bool) error {
	_, err := s.db.Exec(`UPDATE pool_members SET enabled = ? WHERE id = ?`, enabled, memberID)
	return err
}

// ReorderPoolMembers sets position values based on the order of memberIDs.
func (s *Store) ReorderPoolMembers(poolID int64, memberIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE pool_members SET position = ? WHERE id = ? AND pool_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, mid := range memberIDs {
		res, err := stmt.Exec(i, mid, poolID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("pool member %d not found in pool %d", mid, poolID)
		}
	}

	return tx.Commit()
}

// GetPoolModels returns distinct model_ids for all accounts that are members of a given pool.
func (s *Store) GetPoolModels(poolID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT am.model_id
		FROM account_models am
		JOIN pool_members pm ON pm.account_id = am.account_id
		JOIN accounts a ON a.id = am.account_id
		WHERE pm.pool_id = ? AND pm.enabled = 1 AND a.enabled = 1
		ORDER BY am.model_id`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	if models == nil {
		models = []string{}
	}
	return models, rows.Err()
}

// --- scan helpers ---

func scanPoolRowWithCounts(row rowScanner) (*Pool, error) {
	var p Pool
	var enabled int
	var createdAt, updatedAt sql.NullString

	err := row.Scan(&p.ID, &p.Label, &p.Priority, &enabled, &p.RoutingPolicy, &createdAt, &updatedAt,
		&p.AccountCount, &p.HealthyAccountCount, &p.RoutableAccountCount)
	if err != nil {
		return nil, err
	}

	p.Enabled = enabled != 0
	p.RoutingPolicy = NormalizeRoutingPolicy(p.RoutingPolicy)
	if createdAt.Valid {
		p.CreatedAt = createdAt.String
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.String
	}
	p.Models = []string{}
	return &p, nil
}

// scanPoolMemberWithAccount scans a row that has pool_member fields followed by full account columns.
func scanPoolMemberWithAccount(row rowScanner, m *PoolMember, mEnabled *int) (*Account, error) {
	var a Account
	accountState := newAccountScanState(&a)
	targets := []any{&m.ID, &m.PoolID, &m.AccountID, &m.Position, mEnabled}
	targets = append(targets, accountState.targets()...)

	if err := row.Scan(targets...); err != nil {
		return nil, err
	}
	accountState.apply()
	return &a, nil
}
