package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
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

func (s *Store) GetTokenByNameAndPool(name string, poolID *int64) (*AccessToken, error) {
	var row rowScanner
	if poolID == nil {
		row = s.db.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE name = ? AND pool_id IS NULL`, name)
	} else {
		row = s.db.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE name = ? AND pool_id = ?`, name, *poolID)
	}

	token, err := scanTokenRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return token, err
}

func defaultPoolTokenName(poolLabel string) string {
	name := strings.TrimSpace(poolLabel)
	if name == "" {
		return "pool-token"
	}
	return name + "-token"
}

func (s *Store) CreateToken(t *AccessToken) (int64, error) {
	if t.PoolID == nil {
		return 0, fmt.Errorf("pool_id is required")
	}
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

func (s *Store) UpdateTokenName(id int64, name string) error {
	_, err := s.db.Exec(
		`UPDATE access_tokens SET name=?, updated_at=datetime('now') WHERE id=?`,
		name, id,
	)
	return err
}

func (s *Store) RegenerateToken(id int64) (*AccessToken, error) {
	value, err := generateToken()
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`UPDATE access_tokens SET token=?, updated_at=datetime('now') WHERE id=?`,
		value, id,
	)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
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

func (s *Store) EnsurePoolToken(poolID int64) (*AccessToken, error) {
	pool, err := s.GetPool(poolID)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, nil
	}
	tokens, err := s.ListTokensByPool(poolID)
	if err != nil {
		return nil, err
	}
	if len(tokens) > 0 {
		if !tokens[0].Enabled {
			if _, err := s.db.Exec(`UPDATE access_tokens SET enabled=1, updated_at=datetime('now') WHERE id=?`, tokens[0].ID); err != nil {
				return nil, err
			}
			tokens[0].Enabled = true
		}
		return &tokens[0], nil
	}
	poolIDCopy := poolID
	id, err := s.CreateToken(&AccessToken{
		Name:    defaultPoolTokenName(pool.Label),
		PoolID:  &poolIDCopy,
		Enabled: true,
	})
	if err != nil {
		return nil, err
	}
	return s.GetToken(id)
}

type reconcileTokenResult struct {
	Created int
	Skipped int
}

func (s *Store) ReconcilePoolTokens() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := reconcilePoolTokensTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func reconcilePoolTokensTx(tx *sql.Tx) (reconcileTokenResult, error) {
	var result reconcileTokenResult
	var tableCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('pools', 'access_tokens')`).Scan(&tableCount); err != nil {
		return result, err
	}
	if tableCount < 2 {
		return result, nil
	}

	if res, err := tx.Exec(`DELETE FROM access_tokens WHERE pool_id IS NULL`); err != nil {
		return result, err
	} else if n, err := res.RowsAffected(); err == nil {
		result.Skipped += int(n)
	}

	rows, err := tx.Query(`SELECT id, label FROM pools ORDER BY id`)
	if err != nil {
		return result, err
	}
	type poolRow struct {
		id    int64
		label string
	}
	var pools []poolRow
	for rows.Next() {
		var p poolRow
		if err := rows.Scan(&p.id, &p.label); err != nil {
			rows.Close()
			return result, err
		}
		pools = append(pools, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	for _, pool := range pools {
		tokenRows, err := tx.Query(`SELECT id FROM access_tokens WHERE pool_id = ? ORDER BY id`, pool.id)
		if err != nil {
			return result, err
		}
		var ids []int64
		for tokenRows.Next() {
			var id int64
			if err := tokenRows.Scan(&id); err != nil {
				tokenRows.Close()
				return result, err
			}
			ids = append(ids, id)
		}
		if err := tokenRows.Err(); err != nil {
			tokenRows.Close()
			return result, err
		}
		tokenRows.Close()

		if len(ids) == 0 {
			value, err := generateToken()
			if err != nil {
				return result, err
			}
			if _, err := tx.Exec(
				`INSERT INTO access_tokens (name, token, pool_id, enabled) VALUES (?, ?, ?, 1)`,
				defaultPoolTokenName(pool.label), value, pool.id,
			); err != nil {
				return result, err
			}
			result.Created++
			continue
		}

		keepID := ids[0]
		if _, err := tx.Exec(`UPDATE access_tokens SET enabled=1, updated_at=datetime('now') WHERE id=?`, keepID); err != nil {
			return result, err
		}
		for _, extraID := range ids[1:] {
			if _, err := tx.Exec(`DELETE FROM access_tokens WHERE id=?`, extraID); err != nil {
				return result, err
			}
			result.Skipped++
		}
	}

	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_access_tokens_pool_unique ON access_tokens(pool_id)`); err != nil {
		return result, err
	}
	return result, nil
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
	if len(t.Token) > 12 {
		t.TokenMasked = t.Token[:12] + "..." + t.Token[len(t.Token)-4:]
	} else {
		t.TokenMasked = t.Token
	}

	return &t, nil
}
