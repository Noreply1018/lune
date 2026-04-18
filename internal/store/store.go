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

const v3SchemaVersion = 12

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
    codex_quota_json    TEXT NOT NULL DEFAULT '',
    codex_quota_fetched_at TEXT NOT NULL DEFAULT '',
    probe_models        TEXT NOT NULL DEFAULT '[]',
    last_probe_status   TEXT NOT NULL DEFAULT '',
    last_probe_at       TEXT,
    last_probe_error    TEXT NOT NULL DEFAULT '',
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
    attempt_count     INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_pool_id ON request_logs(pool_id);

CREATE TABLE IF NOT EXISTS notification_settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    enabled             INTEGER NOT NULL DEFAULT 0,
    webhook_url         TEXT    NOT NULL DEFAULT '',
    mention_mobile_list TEXT    NOT NULL DEFAULT '[]',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS notification_subscriptions (
    event           TEXT PRIMARY KEY,
    subscribed      INTEGER NOT NULL DEFAULT 1,
    body_template   TEXT    NOT NULL,
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS notification_outbox (
    id               INTEGER PRIMARY KEY,
    channel_id       INTEGER NOT NULL DEFAULT 1,
    event            TEXT NOT NULL,
    severity         TEXT NOT NULL,
    payload          TEXT NOT NULL,
    dedup_key        TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending',
    attempt          INTEGER NOT NULL DEFAULT 0,
    next_attempt_at  TEXT NOT NULL DEFAULT (datetime('now')),
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id                INTEGER PRIMARY KEY,
    channel_id        INTEGER NOT NULL DEFAULT 1,
    channel_name      TEXT NOT NULL DEFAULT 'wechat_work_bot',
    channel_type      TEXT NOT NULL DEFAULT 'wechat_work_bot',
    event             TEXT NOT NULL,
    severity          TEXT NOT NULL,
    title             TEXT NOT NULL DEFAULT '',
    payload_summary   TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL,
    upstream_code     TEXT NOT NULL DEFAULT '',
    upstream_message  TEXT NOT NULL DEFAULT '',
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    attempt           INTEGER NOT NULL DEFAULT 1,
    dedup_key         TEXT NOT NULL DEFAULT '',
    triggered_by      TEXT NOT NULL DEFAULT 'system',
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_pending ON notification_outbox(status, next_attempt_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_outbox_active_unique ON notification_outbox(dedup_key) WHERE status IN ('pending', 'retrying');
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_created ON notification_deliveries(created_at);

INSERT OR IGNORE INTO notification_settings (id) VALUES (1);

INSERT OR IGNORE INTO notification_subscriptions (event, subscribed, body_template) VALUES
    ('account_expiring',  1, '账号 {{ .Vars.account_label }} 将在 {{ .Vars.expires_at }} 过期。'),
    ('account_error',     1, '账号 {{ .Vars.account_label }} 最近错误：{{ .Vars.last_error }}'),
    ('cpa_service_error', 1, 'CPA 服务 {{ .Vars.service_label }} 最近错误：{{ .Vars.last_error }}'),
    ('test',              1, '这是一条用于验证渠道可达性的真实消息，可忽略。');
`

func New(dbPath string) (*Store, error) {
	dsn := dbPath + "?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
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
		return nil // already at latest
	}

	if ver >= 3 && ver < v3SchemaVersion {
		slog.Info("migrating schema", "from_version", ver, "to_version", v3SchemaVersion)
		if ver < 8 {
			if err := s.migrateCodexQuotaColumns(); err != nil {
				return fmt.Errorf("migrate codex quota columns: %w", err)
			}
		}
		if ver < 9 {
			if err := s.migrateNotificationSingleton(); err != nil {
				return fmt.Errorf("migrate notification singleton: %w", err)
			}
		}
		if ver < 10 {
			if err := s.migrateNotificationTextOnly(); err != nil {
				return fmt.Errorf("migrate notification text-only: %w", err)
			}
		}
		if ver < 11 {
			if err := s.migrateProbeColumns(); err != nil {
				return fmt.Errorf("migrate probe columns: %w", err)
			}
		}
		if ver < 12 {
			if err := s.migrateAttemptCountColumn(); err != nil {
				return fmt.Errorf("migrate attempt_count column: %w", err)
			}
		}
		return s.SetSetting("schema_version", strconv.Itoa(v3SchemaVersion))
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

// migrateNotificationSingleton collapses the legacy multi-channel notification
// schema (v3–v8) down to a single WeChat-Work-Bot config. Historical channels,
// pending outbox entries, and indexes are all dropped; the new singleton tables
// are created and seeded with the built-in event defaults.
func (s *Store) migrateNotificationSingleton() error {
	// Drop obsolete indexes first so they don't collide with the new schema.
	dropStmts := []string{
		`DROP INDEX IF EXISTS idx_notification_channels_name_unique`,
		`DROP INDEX IF EXISTS idx_notification_outbox_active_unique`,
		`DROP INDEX IF EXISTS idx_notification_deliveries_recent`,
	}
	for _, stmt := range dropStmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("drop legacy notification index: %w", err)
		}
	}

	// Old outbox rows reference the soon-to-be-dropped channels table via
	// their foreign key; clear them before dropping channels.
	if _, err := s.db.Exec(`DELETE FROM notification_outbox`); err != nil {
		return fmt.Errorf("clear legacy notification outbox: %w", err)
	}

	if _, err := s.db.Exec(`DROP TABLE IF EXISTS notification_channels`); err != nil {
		return fmt.Errorf("drop legacy notification_channels: %w", err)
	}

	// Recreate outbox / deliveries without the FK so channel_id can be a
	// plain constant "1" going forward. The simplest path is to drop and
	// re-create both tables since old rows have been cleared.
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS notification_outbox`); err != nil {
		return fmt.Errorf("drop legacy notification_outbox: %w", err)
	}
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS notification_deliveries`); err != nil {
		return fmt.Errorf("drop legacy notification_deliveries: %w", err)
	}

	// Apply the full v3 schema; CREATE TABLE IF NOT EXISTS statements are
	// no-ops for tables that already exist, and the notification_* tables
	// we just dropped get recreated with the new singleton shape.
	if _, err := s.db.Exec(v3Schema); err != nil {
		return fmt.Errorf("apply notification singleton schema: %w", err)
	}

	// Clear legacy legacy-webhook flag so the old path cannot reassert itself.
	if _, err := s.db.Exec(`DELETE FROM system_config WHERE key IN ('webhook_url', 'webhook_enabled', 'notification_legacy_migrated')`); err != nil {
		return fmt.Errorf("clear legacy webhook settings: %w", err)
	}

	s.schemaMu.Lock()
	delete(s.schemaCache, "notification_outbox")
	delete(s.schemaCache, "notification_deliveries")
	delete(s.schemaCache, "notification_settings")
	delete(s.schemaCache, "notification_subscriptions")
	s.schemaMu.Unlock()
	return nil
}

// migrateNotificationTextOnly drops the format and title_template columns
// introduced by earlier iterations and promotes the body defaults to the new
// concrete-field templates. Rows whose body_template still equals the old
// `{{ .Message }}` passthrough are upgraded to the new default; any body the
// admin has customised is preserved.
func (s *Store) migrateNotificationTextOnly() error {
	// Drop the format column if it's still present (requires SQLite 3.35+,
	// which matches modernc.org/sqlite's bundled engine).
	hasFormat, err := s.hasColumn("notification_settings", "format")
	if err != nil {
		return fmt.Errorf("probe notification_settings.format: %w", err)
	}
	if hasFormat {
		if _, err := s.db.Exec(`ALTER TABLE notification_settings DROP COLUMN format`); err != nil {
			return fmt.Errorf("drop notification_settings.format: %w", err)
		}
	}

	hasTitle, err := s.hasColumn("notification_subscriptions", "title_template")
	if err != nil {
		return fmt.Errorf("probe notification_subscriptions.title_template: %w", err)
	}
	if hasTitle {
		if _, err := s.db.Exec(`ALTER TABLE notification_subscriptions DROP COLUMN title_template`); err != nil {
			return fmt.Errorf("drop notification_subscriptions.title_template: %w", err)
		}
	}

	// Safely upgrade rows that still carry the old `{{ .Message }}` passthrough
	// to the richer concrete-field defaults.
	bodyUpgrades := []struct{ event, body string }{
		{"account_expiring", "账号 {{ .Vars.account_label }} 将在 {{ .Vars.expires_at }} 过期。"},
		{"account_error", "账号 {{ .Vars.account_label }} 最近错误：{{ .Vars.last_error }}"},
		{"cpa_service_error", "CPA 服务 {{ .Vars.service_label }} 最近错误：{{ .Vars.last_error }}"},
	}
	for _, upgrade := range bodyUpgrades {
		if _, err := s.db.Exec(
			`UPDATE notification_subscriptions
			 SET body_template = ?, updated_at = datetime('now')
			 WHERE event = ? AND body_template = '{{ .Message }}'`,
			upgrade.body, upgrade.event,
		); err != nil {
			return fmt.Errorf("upgrade %s body_template: %w", upgrade.event, err)
		}
	}

	s.schemaMu.Lock()
	delete(s.schemaCache, "notification_settings")
	delete(s.schemaCache, "notification_subscriptions")
	s.schemaMu.Unlock()
	return nil
}

func (s *Store) migrateCodexQuotaColumns() error {
	// Partial pre-v3 migration paths (and narrowly-scoped tests) may reach here
	// before an `accounts` table exists. In that case the fresh-schema block
	// will eventually recreate the table with the columns already baked in, so
	// there's nothing to alter.
	exists, err := s.tableExists("accounts")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	hasJSON, err := s.hasColumn("accounts", "codex_quota_json")
	if err != nil {
		return err
	}
	if !hasJSON {
		if _, err := s.db.Exec(
			`ALTER TABLE accounts ADD COLUMN codex_quota_json TEXT NOT NULL DEFAULT ''`,
		); err != nil {
			return err
		}
	}

	hasFetchedAt, err := s.hasColumn("accounts", "codex_quota_fetched_at")
	if err != nil {
		return err
	}
	if !hasFetchedAt {
		if _, err := s.db.Exec(
			`ALTER TABLE accounts ADD COLUMN codex_quota_fetched_at TEXT NOT NULL DEFAULT ''`,
		); err != nil {
			return err
		}
	}

	s.schemaMu.Lock()
	delete(s.schemaCache, "accounts")
	s.schemaMu.Unlock()
	return nil
}

// migrateProbeColumns adds the probe_models configuration and last-probe result
// columns to existing accounts tables. Called when upgrading from ver <= 10.
func (s *Store) migrateProbeColumns() error {
	exists, err := s.tableExists("accounts")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	adds := []struct{ col, ddl string }{
		{"probe_models", `ALTER TABLE accounts ADD COLUMN probe_models TEXT NOT NULL DEFAULT '[]'`},
		{"last_probe_status", `ALTER TABLE accounts ADD COLUMN last_probe_status TEXT NOT NULL DEFAULT ''`},
		{"last_probe_at", `ALTER TABLE accounts ADD COLUMN last_probe_at TEXT`},
		{"last_probe_error", `ALTER TABLE accounts ADD COLUMN last_probe_error TEXT NOT NULL DEFAULT ''`},
	}
	for _, a := range adds {
		has, err := s.hasColumn("accounts", a.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := s.db.Exec(a.ddl); err != nil {
			return fmt.Errorf("add column %s: %w", a.col, err)
		}
	}

	s.schemaMu.Lock()
	delete(s.schemaCache, "accounts")
	s.schemaMu.Unlock()
	return nil
}

// migrateAttemptCountColumn adds the attempt_count column to request_logs so
// the gateway can surface "retry saved the day" telemetry. Idempotent.
func (s *Store) migrateAttemptCountColumn() error {
	exists, err := s.tableExists("request_logs")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	has, err := s.hasColumn("request_logs", "attempt_count")
	if err != nil {
		return err
	}
	if !has {
		if _, err := s.db.Exec(`ALTER TABLE request_logs ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 1`); err != nil {
			return fmt.Errorf("add column attempt_count: %w", err)
		}
	}
	s.schemaMu.Lock()
	delete(s.schemaCache, "request_logs")
	s.schemaMu.Unlock()
	return nil
}

type cpaServiceBackup struct {
	Label         string
	BaseURL       string
	APIKey        string
	ManagementKey string
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

	hasManagementKey, err := s.hasColumn("cpa_services", "management_key")
	if err != nil {
		return nil, err
	}
	query := `SELECT label, base_url, api_key, '' AS management_key, enabled, status, last_checked_at, last_error, created_at, updated_at FROM cpa_services`
	if hasManagementKey {
		query = `SELECT label, base_url, api_key, management_key, enabled, status, last_checked_at, last_error, created_at, updated_at FROM cpa_services`
	}
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []cpaServiceBackup
	for rows.Next() {
		var d cpaServiceBackup
		if err := rows.Scan(&d.Label, &d.BaseURL, &d.APIKey, &d.ManagementKey, &d.Enabled, &d.Status, &d.LastCheckedAt, &d.LastError, &d.CreatedAt, &d.UpdatedAt); err != nil {
			continue
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) tableExists(name string) (bool, error) {
	var tableName string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&tableName)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tableName != "", nil
}

func (s *Store) restoreCpaServices(data []cpaServiceBackup) error {
	stmt, err := s.db.Prepare(`INSERT INTO cpa_services (label, base_url, api_key, management_key, enabled, status, last_checked_at, last_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range data {
		if _, err := stmt.Exec(d.Label, d.BaseURL, d.APIKey, d.ManagementKey, d.Enabled, d.Status, d.LastCheckedAt, d.LastError, d.CreatedAt, d.UpdatedAt); err != nil {
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
