package store

import (
	"time"
)

func (s *Store) ListRoutes() ([]ModelRoute, error) {
	rows, err := s.db.Query(
		`SELECT r.id, r.alias, r.pool_id, COALESCE(p.label, '') AS pool_label, r.target_model, r.enabled, r.created_at, r.updated_at
		 FROM model_routes r
		 LEFT JOIN pools p ON p.id = r.pool_id
		 ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []ModelRoute
	for rows.Next() {
		r, err := scanRouteRow(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, *r)
	}
	return routes, rows.Err()
}

func (s *Store) CreateRoute(r *ModelRoute) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO model_routes (alias, pool_id, target_model, enabled) VALUES (?, ?, ?, ?)`,
		r.Alias, r.PoolID, r.TargetModel, r.Enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateRoute(id int64, r *ModelRoute) error {
	_, err := s.db.Exec(
		`UPDATE model_routes SET alias=?, pool_id=?, target_model=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
		r.Alias, r.PoolID, r.TargetModel, r.Enabled, id,
	)
	return err
}

func (s *Store) DeleteRoute(id int64) error {
	_, err := s.db.Exec(`DELETE FROM model_routes WHERE id=?`, id)
	return err
}

func (s *Store) CountRoutes() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM model_routes`).Scan(&n)
	return n, err
}

func scanRouteRow(row rowScanner) (*ModelRoute, error) {
	var r ModelRoute
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&r.ID, &r.Alias, &r.PoolID, &r.PoolLabel, &r.TargetModel, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	r.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &r, nil
}
