package store

import (
	"database/sql"
	"fmt"
)

func (s *Store) ListCpaServices() ([]CpaService, error) {
	rows, err := s.db.Query(`SELECT id, label, base_url, api_key, enabled, status, last_checked_at, last_error, created_at, updated_at FROM cpa_services ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var svcs []CpaService
	for rows.Next() {
		svc, err := scanCpaServiceRow(rows)
		if err != nil {
			return nil, err
		}
		svcs = append(svcs, *svc)
	}
	return svcs, rows.Err()
}

func (s *Store) GetCpaService() (*CpaService, error) {
	row := s.db.QueryRow(`SELECT id, label, base_url, api_key, enabled, status, last_checked_at, last_error, created_at, updated_at FROM cpa_services LIMIT 1`)
	svc, err := scanCpaServiceRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return svc, err
}

func (s *Store) GetCpaServiceByID(id int64) (*CpaService, error) {
	row := s.db.QueryRow(`SELECT id, label, base_url, api_key, enabled, status, last_checked_at, last_error, created_at, updated_at FROM cpa_services WHERE id = ?`, id)
	svc, err := scanCpaServiceRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return svc, err
}

func (s *Store) CreateCpaService(svc *CpaService) (int64, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM cpa_services`).Scan(&count); err != nil {
		return 0, err
	}
	if count >= 1 {
		return 0, fmt.Errorf("only one CPA service is allowed")
	}
	res, err := s.db.Exec(
		`INSERT INTO cpa_services (label, base_url, api_key, enabled) VALUES (?, ?, ?, ?)`,
		svc.Label, svc.BaseURL, svc.APIKey, svc.Enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateCpaService(id int64, svc *CpaService) error {
	_, err := s.db.Exec(
		`UPDATE cpa_services SET label=?, base_url=?, api_key=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
		svc.Label, svc.BaseURL, svc.APIKey, svc.Enabled, id,
	)
	return err
}

func (s *Store) DeleteCpaService(id int64) error {
	_, err := s.db.Exec(`DELETE FROM cpa_services WHERE id=?`, id)
	return err
}

func (s *Store) EnableCpaService(id int64) error {
	_, err := s.db.Exec(`UPDATE cpa_services SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DisableCpaService(id int64) error {
	_, err := s.db.Exec(`UPDATE cpa_services SET enabled=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) UpdateCpaServiceHealth(id int64, status, lastError string) error {
	_, err := s.db.Exec(
		`UPDATE cpa_services SET status=?, last_error=?, last_checked_at=datetime('now'), updated_at=datetime('now') WHERE id=?`,
		status, lastError, id,
	)
	return err
}

func scanCpaServiceRow(row rowScanner) (*CpaService, error) {
	var svc CpaService
	var enabled int
	err := row.Scan(&svc.ID, &svc.Label, &svc.BaseURL, &svc.APIKey, &enabled, &svc.Status, &svc.LastCheckedAt, &svc.LastError, &svc.CreatedAt, &svc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	svc.Enabled = enabled != 0
	return &svc, nil
}
