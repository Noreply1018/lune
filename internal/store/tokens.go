package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

const tokenColumns = `id, name, token, pool_id, enabled, created_at, updated_at, last_used_at`

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return "sk-lune-" + hex.EncodeToString(b), nil
}

func (s *Store) ListTokens() ([]AccessToken, error) {
	rows, err := s.db.Query(`SELECT ` + tokenColumns + ` FROM access_tokens ORDER BY id`)
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
	row := s.db.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE id = ?`, id)
	t, err := scanTokenRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Store) CreateToken(t *AccessToken) (int64, error) {
	if t.Token == "" {
		tok, err := generateToken()
		if err != nil {
			return 0, err
		}
		t.Token = tok
	}
	res, err := s.db.Exec(
		`INSERT INTO access_tokens (name, token, pool_id, enabled) VALUES (?, ?, ?, ?)`,
		t.Name, t.Token, t.PoolID, t.Enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateToken(id int64, t *AccessToken) error {
	_, err := s.db.Exec(
		`UPDATE access_tokens SET name=?, pool_id=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
		t.Name, t.PoolID, t.Enabled, id,
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

func (s *Store) RegenerateToken(id int64) (*AccessToken, error) {
	value, err := generateToken()
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(
		`UPDATE access_tokens SET token=?, updated_at=datetime('now') WHERE id=?`,
		value, id,
	); err != nil {
		return nil, err
	}
	return s.GetToken(id)
}

func (s *Store) FindTokenByValue(tokenValue string) (*AccessToken, error) {
	row := s.db.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE token = ?`, tokenValue)
	t, err := scanTokenRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Store) CountTokens() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM access_tokens`).Scan(&n)
	return n, err
}

func (s *Store) UpdateTokenLastUsed(id int64) error {
	_, err := s.db.Exec(
		`UPDATE access_tokens SET last_used_at=datetime('now'), updated_at=datetime('now') WHERE id=?`,
		id,
	)
	return err
}

func (s *Store) ListTokensByPool(poolID int64) ([]AccessToken, error) {
	rows, err := s.db.Query(`SELECT `+tokenColumns+` FROM access_tokens WHERE pool_id = ? ORDER BY id`, poolID)
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

func (s *Store) ListGlobalTokens() ([]AccessToken, error) {
	rows, err := s.db.Query(`SELECT ` + tokenColumns + ` FROM access_tokens WHERE pool_id IS NULL ORDER BY id`)
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

// EnsureDefaultGlobalToken creates a default global token if no tokens exist.
// Returns the token (existing or newly created), or nil if tokens already exist.
func (s *Store) EnsureDefaultGlobalToken() (*AccessToken, error) {
	count, err := s.CountTokens()
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}

	tok, err := generateToken()
	if err != nil {
		return nil, err
	}

	t := &AccessToken{
		Name:    "default",
		Token:   tok,
		PoolID:  nil,
		Enabled: true,
	}
	id, err := s.CreateToken(t)
	if err != nil {
		return nil, err
	}
	return s.GetToken(id)
}

func scanTokenRow(row rowScanner) (*AccessToken, error) {
	var t AccessToken
	var enabled int
	var poolID sql.NullInt64
	var lastUsedAt sql.NullString

	err := row.Scan(&t.ID, &t.Name, &t.Token, &poolID, &enabled, &t.CreatedAt, &t.UpdatedAt, &lastUsedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	if poolID.Valid {
		v := poolID.Int64
		t.PoolID = &v
	}
	if lastUsedAt.Valid {
		t.LastUsedAt = &lastUsedAt.String
	}

	// computed fields
	t.IsGlobal = t.PoolID == nil
	if len(t.Token) > 12 {
		t.TokenMasked = t.Token[:12] + "..." + t.Token[len(t.Token)-4:]
	} else {
		t.TokenMasked = t.Token
	}

	return &t, nil
}
