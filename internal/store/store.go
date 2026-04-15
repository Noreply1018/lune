package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

type Store struct {
	db          *sql.DB
	schemaMu    sync.Mutex
	schemaCache map[string]map[string]bool
}

const v3SchemaVersion = 4

const v3Schema = `
CREATE TABLE IF NOT EXISTS system_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS cpa_services (
    id              INTEGER PRIMARY KEY,
    label           TEXT NOT NULL,
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL DEFAULT '',
    management_key  TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pools (
    id         INTEGER PRIMARY KEY,
    label      TEXT NOT NULL UNIQUE,
    priority   INTEGER NOT NULL DEFAULT 0,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS accounts (
    id                  INTEGER PRIMARY KEY,
    label               TEXT NOT NULL,
    source_kind         TEXT NOT NULL DEFAULT 'openai_compat',
    base_url            TEXT NOT NULL DEFAULT '',
    api_key             TEXT NOT NULL DEFAULT '',
    provider            TEXT NOT NULL DEFAULT '',
    cpa_service_id      INTEGER REFERENCES cpa_services(id),
    cpa_provider        TEXT NOT NULL DEFAULT '',
    cpa_account_key     TEXT NOT NULL DEFAULT '',
    cpa_email           TEXT NOT NULL DEFAULT '',
    cpa_plan_type       TEXT NOT NULL DEFAULT '',
    cpa_openai_id       TEXT NOT NULL DEFAULT '',
    cpa_expired_at      TEXT NOT NULL DEFAULT '',
    cpa_last_refresh_at TEXT NOT NULL DEFAULT '',
    cpa_disabled        INTEGER NOT NULL DEFAULT 0,
    enabled             INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'unknown',
    notes               TEXT NOT NULL DEFAULT '',
    quota_display       TEXT NOT NULL DEFAULT '',
    last_checked_at     TEXT,
    last_error          TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pool_members (
    id         INTEGER PRIMARY KEY,
    pool_id    INTEGER NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL DEFAULT 0,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(pool_id, account_id)
);

CREATE TABLE IF NOT EXISTS account_models (
    id         INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model_id   TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, model_id)
);

CREATE TABLE IF NOT EXISTS access_tokens (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    token        TEXT NOT NULL UNIQUE,
    pool_id      INTEGER REFERENCES pools(id) ON DELETE CASCADE,
    enabled      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS request_logs (
    id                INTEGER PRIMARY KEY,
    request_id        TEXT NOT NULL,
    access_token_name TEXT NOT NULL DEFAULT '',
    model_requested   TEXT NOT NULL DEFAULT '',
    model_actual      TEXT NOT NULL DEFAULT '',
    pool_id           INTEGER,
    account_id        INTEGER,
    status_code       INTEGER NOT NULL DEFAULT 0,
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    input_tokens      INTEGER NOT NULL DEFAULT 0,
    output_tokens     INTEGER NOT NULL DEFAULT 0,
    stream            INTEGER NOT NULL DEFAULT 0,
    request_ip        TEXT NOT NULL DEFAULT '',
    success           INTEGER NOT NULL DEFAULT 1,
    error_message     TEXT NOT NULL DEFAULT '',
    source_kind       TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_pool_id ON request_logs(pool_id);
`

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

	s := &Store{db: db, schemaCache: make(map[string]map[string]bool)}
	if err := s.migrateV3(dbPath); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// migrateV3 detects the current schema version and either creates fresh v3
// schema or migrates from v2 (backup + rebuild, preserving cpa_services data).
func (s *Store) migrateV3(dbPath string) error {
	// Ensure system_config exists for version tracking
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`)
	if err != nil {
		return err
	}

	ver := s.schemaVersion()

	if ver >= v3SchemaVersion {
		return nil // already at v3
	}

	if ver == 0 {
		// Fresh database — create v3 schema directly
		slog.Info("creating v3 schema (fresh database)")
		return s.createV3Schema()
	}

	// v1/v2/v3(old) → v3(new): backup and rebuild
	slog.Info("detected pre-v3 schema, migrating", "current_version", ver)

	// Preserve cpa_services data before rebuild
	cpaData, err := s.backupCpaServices()
	if err != nil {
		slog.Warn("could not backup cpa_services, will skip", "err", err)
	}

	// Backup the database file
	backupPath := dbPath + ".v2.bak"
	if err := s.backupDBFile(dbPath, backupPath); err != nil {
		slog.Warn("database backup failed, continuing anyway", "err", err)
	} else {
		slog.Info("database backed up", "path", backupPath)
	}

	// Drop all existing tables
	if err := s.dropAllTables(); err != nil {
		return fmt.Errorf("drop tables: %w", err)
	}

	// Create fresh v3 schema
	if err := s.createV3Schema(); err != nil {
		return fmt.Errorf("create v3 schema: %w", err)
	}

	// Restore cpa_services data
	if len(cpaData) > 0 {
		if err := s.restoreCpaServices(cpaData); err != nil {
			slog.Warn("could not restore cpa_services", "err", err)
		} else {
			slog.Info("cpa_services data restored", "count", len(cpaData))
		}
	}

	slog.Info("v3 migration complete")
	return nil
}

func (s *Store) createV3Schema() error {
	if _, err := s.db.Exec(v3Schema); err != nil {
		return err
	}
	return s.SetSetting("schema_version", strconv.Itoa(v3SchemaVersion))
}

type cpaServiceBackup struct {
	Label         string
	BaseURL       string
	APIKey        string
	Enabled       int
	Status        string
	LastCheckedAt *string
	LastError     string
	CreatedAt     string
	UpdatedAt     string
}

func (s *Store) backupCpaServices() ([]cpaServiceBackup, error) {
	// Check if cpa_services table exists
	var tableName string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='cpa_services'`).Scan(&tableName)
	if err != nil {
		return nil, nil // table doesn't exist, nothing to backup
	}

	rows, err := s.db.Query(`SELECT label, base_url, api_key, enabled, status, last_checked_at, last_error, created_at, updated_at FROM cpa_services`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []cpaServiceBackup
	for rows.Next() {
		var d cpaServiceBackup
		if err := rows.Scan(&d.Label, &d.BaseURL, &d.APIKey, &d.Enabled, &d.Status, &d.LastCheckedAt, &d.LastError, &d.CreatedAt, &d.UpdatedAt); err != nil {
			continue
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

func (s *Store) restoreCpaServices(data []cpaServiceBackup) error {
	stmt, err := s.db.Prepare(`INSERT INTO cpa_services (label, base_url, api_key, enabled, status, last_checked_at, last_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range data {
		if _, err := stmt.Exec(d.Label, d.BaseURL, d.APIKey, d.Enabled, d.Status, d.LastCheckedAt, d.LastError, d.CreatedAt, d.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) backupDBFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func (s *Store) dropAllTables() error {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name != 'sqlite_sequence'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		tables = append(tables, name)
	}

	// Disable FK temporarily for clean drop — pin to single connection
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Close()

	conn.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`)
	for _, t := range tables {
		if _, err := conn.ExecContext(context.Background(), `DROP TABLE IF EXISTS "`+t+`"`); err != nil {
			return fmt.Errorf("drop %s: %w", t, err)
		}
	}
	conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)

	// Clear schema cache
	s.schemaMu.Lock()
	s.schemaCache = make(map[string]map[string]bool)
	s.schemaMu.Unlock()

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

// tableColumns returns the set of column names for a table (cached).
func (s *Store) tableColumns(table string) (map[string]bool, error) {
	s.schemaMu.Lock()
	defer s.schemaMu.Unlock()

	if cols, ok := s.schemaCache[table]; ok {
		out := make(map[string]bool, len(cols))
		for k, v := range cols {
			out[k] = v
		}
		return out, nil
	}

	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var (
			cid      int
			name     string
			typ      string
			notnull  int
			defaultV any
			primaryK int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultV, &primaryK); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.schemaCache[table] = cols

	out := make(map[string]bool, len(cols))
	for k, v := range cols {
		out[k] = v
	}
	return out, nil
}
