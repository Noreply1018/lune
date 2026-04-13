package store

import (
	"database/sql"
	"fmt"
	"strconv"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	dsn := dbPath + "?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// --- migrations ---

var migrations = []string{
	// v1: all tables
	`
CREATE TABLE IF NOT EXISTS system_config (
	key   TEXT PRIMARY KEY,
	value TEXT
);

CREATE TABLE IF NOT EXISTS accounts (
	id              INTEGER PRIMARY KEY,
	label           TEXT NOT NULL,
	base_url        TEXT NOT NULL,
	api_key         TEXT NOT NULL,
	enabled         INTEGER NOT NULL DEFAULT 1,
	status          TEXT NOT NULL DEFAULT 'healthy',
	quota_total     REAL NOT NULL DEFAULT 0,
	quota_used      REAL NOT NULL DEFAULT 0,
	quota_unit      TEXT NOT NULL DEFAULT '',
	notes           TEXT NOT NULL DEFAULT '',
	model_allowlist TEXT NOT NULL DEFAULT '[]',
	last_checked_at TEXT,
	last_error      TEXT NOT NULL DEFAULT '',
	created_at      TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pools (
	id         INTEGER PRIMARY KEY,
	label      TEXT NOT NULL,
	strategy   TEXT NOT NULL DEFAULT 'priority-first-healthy',
	enabled    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pool_members (
	id         INTEGER PRIMARY KEY,
	pool_id    INTEGER NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
	account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	priority   INTEGER NOT NULL DEFAULT 1,
	weight     INTEGER NOT NULL DEFAULT 1,
	UNIQUE(pool_id, account_id)
);

CREATE TABLE IF NOT EXISTS model_routes (
	id           INTEGER PRIMARY KEY,
	alias        TEXT NOT NULL UNIQUE,
	pool_id      INTEGER NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
	target_model TEXT NOT NULL,
	enabled      INTEGER NOT NULL DEFAULT 1,
	created_at   TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS access_tokens (
	id           INTEGER PRIMARY KEY,
	name         TEXT NOT NULL,
	token        TEXT NOT NULL UNIQUE,
	enabled      INTEGER NOT NULL DEFAULT 1,
	quota_tokens INTEGER NOT NULL DEFAULT 0,
	used_tokens  INTEGER NOT NULL DEFAULT 0,
	created_at   TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
	last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS request_logs (
	id                INTEGER PRIMARY KEY,
	request_id        TEXT NOT NULL,
	access_token_name TEXT,
	model_alias       TEXT,
	target_model      TEXT,
	pool_id           INTEGER,
	account_id        INTEGER,
	status_code       INTEGER,
	latency_ms        INTEGER,
	input_tokens      INTEGER,
	output_tokens     INTEGER,
	stream            INTEGER,
	request_ip        TEXT,
	success           INTEGER,
	error_message     TEXT,
	created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
`,
	// v2: CPA as Provider
	`
CREATE TABLE IF NOT EXISTS cpa_services (
	id              INTEGER PRIMARY KEY,
	label           TEXT NOT NULL,
	base_url        TEXT NOT NULL,
	api_key         TEXT NOT NULL DEFAULT '',
	enabled         INTEGER NOT NULL DEFAULT 1,
	status          TEXT NOT NULL DEFAULT 'unknown',
	last_checked_at TEXT,
	last_error      TEXT NOT NULL DEFAULT '',
	created_at      TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

ALTER TABLE accounts ADD COLUMN source_kind     TEXT NOT NULL DEFAULT 'openai_compat';
ALTER TABLE accounts ADD COLUMN provider         TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_service_id   INTEGER REFERENCES cpa_services(id);
ALTER TABLE accounts ADD COLUMN cpa_provider     TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_account_key  TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_cpa_unique
	ON accounts(cpa_service_id, cpa_provider)
	WHERE source_kind = 'cpa' AND cpa_provider != '';

ALTER TABLE request_logs ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'openai_compat';
`,
	// v3: CPA Management Adapter
	`
ALTER TABLE accounts ADD COLUMN cpa_email           TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_plan_type       TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_openai_id       TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_expired_at      TEXT;
ALTER TABLE accounts ADD COLUMN cpa_last_refresh_at TEXT;
ALTER TABLE accounts ADD COLUMN cpa_disabled        INTEGER NOT NULL DEFAULT 0;

DROP INDEX IF EXISTS idx_accounts_cpa_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_cpa_key_unique
    ON accounts(cpa_service_id, cpa_account_key)
    WHERE source_kind = 'cpa' AND cpa_account_key != '';
`,
}

func (s *Store) migrate() error {
	// ensure system_config exists for version tracking
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS system_config (key TEXT PRIMARY KEY, value TEXT)`)
	if err != nil {
		return err
	}

	current := s.schemaVersion()
	for i := current; i < len(migrations); i++ {
		if _, err := s.db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration v%d: %w", i+1, err)
		}
		if err := s.SetSetting("schema_version", strconv.Itoa(i+1)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) schemaVersion() int {
	var v string
	err := s.db.QueryRow(`SELECT value FROM system_config WHERE key = 'schema_version'`).Scan(&v)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func (s *Store) SchemaVersion() int {
	return s.schemaVersion()
}

// --- system_config ---

func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO system_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

func (s *Store) GetSettings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM system_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

func (s *Store) UpdateSettings(pairs map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO system_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for k, v := range pairs {
		if _, err := stmt.Exec(k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
