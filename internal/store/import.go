package store

import (
	"database/sql"
	"fmt"

	"lune/internal/syscfg"
)

type ConfigImportPayload struct {
	Pools        []Pool            `json:"pools"`
	AccessTokens []AccessToken     `json:"access_tokens"`
	Settings     map[string]string `json:"settings"`

	// Metadata carried by exports. Read-only from the import path's
	// perspective — ImportConfig never writes these back into the DB,
	// they exist purely so Preview and the UI can surface "where did
	// this file come from".
	SchemaVersion  string `json:"schema_version,omitempty"`
	SourceHost     string `json:"source_host,omitempty"`
	ExportedAt     string `json:"exported_at,omitempty"`
	IncludeSecrets bool   `json:"include_secrets,omitempty"`

	// Fields that are present in export output but not imported today.
	// Kept so the preview can report "N accounts ignored" accurately.
	Accounts    []Account    `json:"accounts,omitempty"`
	CpaServices []CpaService `json:"cpa_services,omitempty"`
}

type ConfigImportResult struct {
	CreatedPools    int `json:"created_pools"`
	UpdatedPools    int `json:"updated_pools"`
	CreatedTokens   int `json:"created_tokens"`
	SkippedTokens   int `json:"skipped_tokens"`
	UpdatedSettings int `json:"updated_settings"`
}

type ConfigImportPreview struct {
	SchemaVersion   string `json:"schema_version"`
	SourceHost      string `json:"source_host"`
	ExportedAt      string `json:"exported_at"`
	IncludeSecrets  bool   `json:"include_secrets"`
	CreatedPools    int    `json:"created_pools"`
	UpdatedPools    int    `json:"updated_pools"`
	CreatedTokens   int    `json:"created_tokens"`
	SkippedTokens   int    `json:"skipped_tokens"`
	UpdatedSettings int    `json:"updated_settings"`
	IgnoredAccounts int    `json:"ignored_accounts"`
	IgnoredServices int    `json:"ignored_services"`
}

func (s *Store) ImportConfig(payload ConfigImportPayload) (*ConfigImportResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := runImport(tx, payload)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// PreviewImport runs the same logic as ImportConfig but always rolls back,
// so the UI can show "will create 2 pools, skip 4 tokens, update 6 settings"
// without touching the database.
func (s *Store) PreviewImport(payload ConfigImportPayload) (*ConfigImportPreview, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	// Always roll back — preview must never mutate.
	defer tx.Rollback()

	result, err := runImport(tx, payload)
	if err != nil {
		return nil, err
	}

	preview := &ConfigImportPreview{
		SchemaVersion:   payload.SchemaVersion,
		SourceHost:      payload.SourceHost,
		ExportedAt:      payload.ExportedAt,
		IncludeSecrets:  payload.IncludeSecrets,
		CreatedPools:    result.CreatedPools,
		UpdatedPools:    result.UpdatedPools,
		CreatedTokens:   result.CreatedTokens,
		SkippedTokens:   result.SkippedTokens,
		UpdatedSettings: result.UpdatedSettings,
		IgnoredAccounts: len(payload.Accounts),
		IgnoredServices: len(payload.CpaServices),
	}
	return preview, nil
}

func runImport(tx *sql.Tx, payload ConfigImportPayload) (*ConfigImportResult, error) {
	result := &ConfigImportResult{}
	poolIDByLabel := make(map[string]int64, len(payload.Pools))

	for _, importedPool := range payload.Pools {
		if importedPool.Label == "" {
			continue
		}

		existing, err := getPoolByLabelTx(tx, importedPool.Label)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if _, err := tx.Exec(
				`UPDATE pools SET priority=?, enabled=?, updated_at=datetime('now') WHERE id=?`,
				importedPool.Priority, importedPool.Enabled, existing.ID,
			); err != nil {
				return nil, err
			}
			poolIDByLabel[importedPool.Label] = existing.ID
			result.UpdatedPools++
			continue
		}

		res, err := tx.Exec(
			`INSERT INTO pools (label, priority, enabled) VALUES (?, ?, ?)`,
			importedPool.Label, importedPool.Priority, importedPool.Enabled,
		)
		if err != nil {
			return nil, err
		}
		poolID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		poolIDByLabel[importedPool.Label] = poolID
		result.CreatedPools++
	}

	for _, importedToken := range payload.AccessTokens {
		if importedToken.Name == "" {
			continue
		}
		if importedToken.PoolID == nil {
			result.SkippedTokens++
			continue
		}

		var mappedPoolID *int64
		if importedToken.PoolLabel == "" {
			return nil, fmt.Errorf("token %q references a pool but has no pool_label", importedToken.Name)
		}
		poolID, ok := poolIDByLabel[importedToken.PoolLabel]
		if !ok {
			existingPool, err := getPoolByLabelTx(tx, importedToken.PoolLabel)
			if err != nil {
				return nil, err
			}
			if existingPool == nil {
				return nil, fmt.Errorf("pool %q not found for token %q", importedToken.PoolLabel, importedToken.Name)
			}
			poolID = existingPool.ID
			poolIDByLabel[importedToken.PoolLabel] = poolID
		}
		mappedPoolID = &poolID

		var poolTokenCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM access_tokens WHERE pool_id = ?`, poolID).Scan(&poolTokenCount); err != nil {
			return nil, err
		}
		if poolTokenCount > 0 {
			result.SkippedTokens++
			continue
		}

		existing, err := getTokenByNameAndPoolTx(tx, importedToken.Name, mappedPoolID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			result.SkippedTokens++
			continue
		}

		tokenValue, err := generateToken()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(
			`INSERT INTO access_tokens (name, token, pool_id, enabled) VALUES (?, ?, ?, ?)`,
			importedToken.Name, tokenValue, mappedPoolID, importedToken.Enabled,
		); err != nil {
			return nil, err
		}
		result.CreatedTokens++
	}

	reconcileResult, err := reconcilePoolTokensTx(tx)
	if err != nil {
		return nil, err
	}
	result.CreatedTokens += reconcileResult.Created
	result.SkippedTokens += reconcileResult.Skipped

	for key, value := range payload.Settings {
		if key == "admin_token" || !syscfg.IsAllowedSettingKey(key) {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO system_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key, value,
		); err != nil {
			return nil, err
		}
		result.UpdatedSettings++
	}

	return result, nil
}

func getPoolByLabelTx(tx *sql.Tx, label string) (*Pool, error) {
	row := tx.QueryRow(`
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

	pool, err := scanPoolRowWithCounts(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return pool, err
}

func getTokenByNameAndPoolTx(tx *sql.Tx, name string, poolID *int64) (*AccessToken, error) {
	var (
		row *sql.Row
	)
	if poolID == nil {
		row = tx.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE name = ? AND pool_id IS NULL`, name)
	} else {
		row = tx.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE name = ? AND pool_id = ?`, name, *poolID)
	}

	token, err := scanTokenRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return token, err
}
