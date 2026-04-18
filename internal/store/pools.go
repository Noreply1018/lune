package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

func (s *Store) ListPools() ([]Pool, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.label, p.priority, p.enabled, p.created_at, p.updated_at,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status = 'healthy') AS healthy_account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status IN ('healthy', 'degraded')) AS routable_account_count
		FROM pools p
		ORDER BY p.priority ASC, p.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []Pool
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
		SELECT p.id, p.label, p.priority, p.enabled, p.created_at, p.updated_at,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status = 'healthy') AS healthy_account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status IN ('healthy', 'degraded')) AS routable_account_count
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
		SELECT p.id, p.label, p.priority, p.enabled, p.created_at, p.updated_at,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status = 'healthy') AS healthy_account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status IN ('healthy', 'degraded')) AS routable_account_count
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

func (s *Store) UpdatePool(id int64, label string, priority int, enabled bool) error {
	_, err := s.db.Exec(
		`UPDATE pools SET label=?, priority=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
		label, priority, enabled, id,
	)
	return err
}

func (s *Store) EnablePool(id int64) error {
	_, err := s.db.Exec(`UPDATE pools SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DisablePool(id int64) error {
	_, err := s.db.Exec(`UPDATE pools SET enabled=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
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

	err := row.Scan(&p.ID, &p.Label, &p.Priority, &enabled, &createdAt, &updatedAt,
		&p.AccountCount, &p.HealthyAccountCount, &p.RoutableAccountCount)
	if err != nil {
		return nil, err
	}

	p.Enabled = enabled != 0
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
	var aEnabled, cpaDisabled int
	var cpaServiceID sql.NullInt64
	var lastCheckedAt sql.NullString
	var createdAt, updatedAt sql.NullString
	var probeModelsJSON string
	var lastProbeAt sql.NullString

	err := row.Scan(
		&m.ID, &m.PoolID, &m.AccountID, &m.Position, mEnabled,
		// account columns
		&a.ID, &a.Label, &a.SourceKind, &a.BaseURL, &a.APIKey, &a.Provider,
		&cpaServiceID, &a.CpaProvider, &a.CpaAccountKey, &a.CpaEmail, &a.CpaPlanType, &a.CpaOpenaiID,
		&a.CpaExpiredAt, &a.CpaLastRefreshAt, &cpaDisabled,
		&a.CodexQuotaJSON, &a.CodexQuotaFetchedAt,
		&probeModelsJSON, &a.LastProbeStatus, &lastProbeAt, &a.LastProbeError,
		&aEnabled, &a.Status, &a.Notes, &a.QuotaDisplay, &lastCheckedAt, &a.LastError, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	a.Enabled = aEnabled != 0
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
