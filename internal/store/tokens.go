package store

import (
	"database/sql"
	"time"
)

func (s *Store) ListTokens() ([]AccessToken, error) {
	rows, err := s.db.Query(`SELECT id, name, token, enabled, quota_tokens, used_tokens, created_at, updated_at, last_used_at FROM access_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []AccessToken
	for rows.Next() {
		t, err := scanTokenRow(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, *t)
	}
	return tokens, rows.Err()
}

func (s *Store) GetToken(id int64) (*AccessToken, error) {
	row := s.db.QueryRow(`SELECT id, name, token, enabled, quota_tokens, used_tokens, created_at, updated_at, last_used_at FROM access_tokens WHERE id = ?`, id)
	t, err := scanTokenRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Store) CreateToken(t *AccessToken) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO access_tokens (name, token, enabled, quota_tokens) VALUES (?, ?, ?, ?)`,
		t.Name, t.Token, t.Enabled, t.QuotaTokens,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateToken(id int64, t *AccessToken) error {
	_, err := s.db.Exec(
		`UPDATE access_tokens SET name=?, enabled=?, quota_tokens=?, updated_at=datetime('now') WHERE id=?`,
		t.Name, t.Enabled, t.QuotaTokens, id,
	)
	return err
}

func (s *Store) EnableToken(id int64) error {
	_, err := s.db.Exec(`UPDATE access_tokens SET enabled=1, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DisableToken(id int64) error {
	_, err := s.db.Exec(`UPDATE access_tokens SET enabled=0, updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *Store) DeleteToken(id int64) error {
	_, err := s.db.Exec(`DELETE FROM access_tokens WHERE id=?`, id)
	return err
}

func (s *Store) FindTokenByValue(tokenValue string) (*AccessToken, error) {
	row := s.db.QueryRow(`SELECT id, name, token, enabled, quota_tokens, used_tokens, created_at, updated_at, last_used_at FROM access_tokens WHERE token = ?`, tokenValue)
	t, err := scanTokenRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Store) IncrementTokenUsage(id int64, tokens int64) error {
	_, err := s.db.Exec(
		`UPDATE access_tokens SET used_tokens = used_tokens + ?, last_used_at = datetime('now'), updated_at = datetime('now') WHERE id = ?`,
		tokens, id,
	)
	return err
}

func (s *Store) CountTokens() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM access_tokens`).Scan(&n)
	return n, err
}

func scanTokenRow(row rowScanner) (*AccessToken, error) {
	var t AccessToken
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&t.ID, &t.Name, &t.Token, &enabled, &t.QuotaTokens, &t.UsedTokens, &createdAt, &updatedAt, &t.LastUsedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	t.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &t, nil
}
