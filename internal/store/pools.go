package store

import (
	"database/sql"
	"time"
)

func (s *Store) ListPools() ([]Pool, error) {
	rows, err := s.db.Query(`SELECT id, label, strategy, enabled, created_at, updated_at FROM pools ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []Pool
	for rows.Next() {
		p, err := scanPoolRow(rows)
		if err != nil {
			return nil, err
		}
		pools = append(pools, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// load members for each pool
	for i := range pools {
		members, err := s.listPoolMembers(pools[i].ID)
		if err != nil {
			return nil, err
		}
		pools[i].Members = members
	}
	return pools, nil
}

func (s *Store) GetPool(id int64) (*Pool, error) {
	row := s.db.QueryRow(`SELECT id, label, strategy, enabled, created_at, updated_at FROM pools WHERE id = ?`, id)
	p, err := scanPoolRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	members, err := s.listPoolMembers(p.ID)
	if err != nil {
		return nil, err
	}
	p.Members = members
	return p, nil
}

func (s *Store) CreatePool(p *Pool) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO pools (label, strategy, enabled) VALUES (?, ?, ?)`,
		p.Label, p.Strategy, p.Enabled,
	)
	if err != nil {
		return 0, err
	}
	poolID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := insertMembers(tx, poolID, p.Members); err != nil {
		return 0, err
	}
	return poolID, tx.Commit()
}

func (s *Store) UpdatePool(id int64, p *Pool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`UPDATE pools SET label=?, strategy=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
		p.Label, p.Strategy, p.Enabled, id,
	)
	if err != nil {
		return err
	}

	// replace members
	if _, err := tx.Exec(`DELETE FROM pool_members WHERE pool_id=?`, id); err != nil {
		return err
	}
	if err := insertMembers(tx, id, p.Members); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EnablePool(id int64) error {
	_, err := s.db.Exec(`UPDATE pools SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DisablePool(id int64) error {
	_, err := s.db.Exec(`UPDATE pools SET enabled=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DeletePool(id int64) error {
	_, err := s.db.Exec(`DELETE FROM pools WHERE id=?`, id)
	return err
}

func (s *Store) CountPools() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pools`).Scan(&n)
	return n, err
}

func (s *Store) listPoolMembers(poolID int64) ([]PoolMember, error) {
	rows, err := s.db.Query(
		`SELECT pm.id, pm.pool_id, pm.account_id, COALESCE(a.label, '') AS account_label, COALESCE(a.status, '') AS account_status, pm.priority, pm.weight
		 FROM pool_members pm
		 LEFT JOIN accounts a ON a.id = pm.account_id
		 WHERE pm.pool_id=? ORDER BY pm.priority, pm.id`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []PoolMember
	for rows.Next() {
		var m PoolMember
		if err := rows.Scan(&m.ID, &m.PoolID, &m.AccountID, &m.AccountLabel, &m.AccountStatus, &m.Priority, &m.Weight); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if members == nil {
		members = []PoolMember{}
	}
	return members, rows.Err()
}

func insertMembers(tx *sql.Tx, poolID int64, members []PoolMember) error {
	stmt, err := tx.Prepare(`INSERT INTO pool_members (pool_id, account_id, priority, weight) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range members {
		if _, err := stmt.Exec(poolID, m.AccountID, m.Priority, m.Weight); err != nil {
			return err
		}
	}
	return nil
}

func scanPoolRow(row rowScanner) (*Pool, error) {
	var p Pool
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Label, &p.Strategy, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled != 0
	p.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	p.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	p.Members = []PoolMember{}
	return &p, nil
}
