package store

func (s *Store) RefreshAccountModels(accountID int64, modelIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM account_models WHERE account_id = ?`, accountID); err != nil {
		return err
	}

	if len(modelIDs) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO account_models (account_id, model_id) VALUES (?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, mid := range modelIDs {
			if _, err := stmt.Exec(accountID, mid); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *Store) ListAccountModels(accountID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT model_id FROM account_models WHERE account_id = ? ORDER BY model_id`, accountID)
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

// ListAllModels returns distinct model IDs from all enabled accounts in enabled pools.
func (s *Store) ListAllModels() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT am.model_id
		FROM account_models am
		JOIN accounts a ON a.id = am.account_id
		JOIN pool_members pm ON pm.account_id = a.id
		JOIN pools p ON p.id = pm.pool_id
		WHERE a.enabled = 1 AND p.enabled = 1 AND pm.enabled = 1
		ORDER BY am.model_id`)
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

func (s *Store) CountDistinctModels() (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT am.model_id)
		FROM account_models am
		JOIN accounts a ON a.id = am.account_id
		JOIN pool_members pm ON pm.account_id = a.id
		JOIN pools p ON p.id = pm.pool_id
		WHERE a.enabled = 1 AND p.enabled = 1 AND pm.enabled = 1`).Scan(&n)
	return n, err
}
