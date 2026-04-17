package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLegacyWebhookMigrationPreservesUnicodeURL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '4');
CREATE TABLE notification_channels (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	config TEXT NOT NULL DEFAULT '{}',
	subscriptions TEXT NOT NULL DEFAULT '[]',
	title_template TEXT NOT NULL DEFAULT '',
	body_template TEXT NOT NULL DEFAULT '',
	retry_max_attempts INTEGER NOT NULL DEFAULT 5,
	retry_schedule_seconds TEXT NOT NULL DEFAULT '[30,120,600,1800,7200]',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO system_config (key, value) VALUES ('webhook_url', ?), ('webhook_enabled', '1')`, "https://example.com/通知?name=上海"); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	st, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store with migration: %v", err)
	}
	defer st.Close()

	channels, err := st.ListNotificationChannels()
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 migrated channel, got %d", len(channels))
	}
	var cfg struct {
		Schema int    `json:"schema"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(channels[0].Config, &cfg); err != nil {
		t.Fatalf("decode migrated config: %v", err)
	}
	if cfg.URL != "https://example.com/通知?name=上海" {
		t.Fatalf("expected unicode URL to survive migration, got %q", cfg.URL)
	}
}

func TestMigrateV2PreservesCpaServicesWithoutManagementKeyColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-v2.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '2');
CREATE TABLE cpa_services (
	id INTEGER PRIMARY KEY,
	label TEXT NOT NULL,
	base_url TEXT NOT NULL,
	api_key TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	status TEXT NOT NULL DEFAULT 'unknown',
	last_checked_at TEXT,
	last_error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO cpa_services (label, base_url, api_key, enabled, status, last_error, created_at, updated_at)
VALUES ('Legacy CPA', 'https://cpa.example.com', 'legacy-key', 1, 'healthy', '', '2026-04-01 00:00:00', '2026-04-01 00:00:00');
`); err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	st, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store with migration: %v", err)
	}
	defer st.Close()

	rows, err := st.DB().Query(`SELECT label, base_url, api_key, management_key FROM cpa_services`)
	if err != nil {
		t.Fatalf("query migrated services: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("expected migrated cpa service to exist")
	}
	var label, baseURL, apiKey, managementKey string
	if err := rows.Scan(&label, &baseURL, &apiKey, &managementKey); err != nil {
		t.Fatalf("scan migrated service: %v", err)
	}
	if label != "Legacy CPA" || baseURL != "https://cpa.example.com" || apiKey != "legacy-key" {
		t.Fatalf("unexpected migrated values: label=%q baseURL=%q apiKey=%q", label, baseURL, apiKey)
	}
	if managementKey != "" {
		t.Fatalf("expected empty management key for legacy schema, got %q", managementKey)
	}
}

func TestMigrateV3CreatesNotificationSchemaWithoutRebuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-v3.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '3');
`); err != nil {
		t.Fatalf("seed v3 schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	st, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store with migration: %v", err)
	}
	defer st.Close()

	if st.SchemaVersion() != v3SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", v3SchemaVersion, st.SchemaVersion())
	}
	if _, err := st.DB().Exec(`INSERT INTO notification_channels (name, type, enabled, config, subscriptions) VALUES ('ops','generic_webhook',1,'{}','[]')`); err != nil {
		t.Fatalf("expected notification schema to exist, insert failed: %v", err)
	}
}

func TestMigrateV5RebuildsNotificationForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-v5.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '5');
CREATE TABLE notification_channels (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	config TEXT NOT NULL DEFAULT '{}',
	subscriptions TEXT NOT NULL DEFAULT '[]',
	title_template TEXT NOT NULL DEFAULT '',
	body_template TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE notification_outbox (
	id INTEGER PRIMARY KEY,
	channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
	event TEXT NOT NULL,
	severity TEXT NOT NULL,
	payload TEXT NOT NULL,
	dedup_key TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	attempt INTEGER NOT NULL DEFAULT 0,
	next_attempt_at TEXT NOT NULL DEFAULT (datetime('now')),
	last_error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE notification_deliveries (
	id INTEGER PRIMARY KEY,
	channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
	channel_name TEXT NOT NULL DEFAULT '',
	channel_type TEXT NOT NULL DEFAULT '',
	event TEXT NOT NULL,
	severity TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	payload_summary TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	upstream_code TEXT NOT NULL DEFAULT '',
	upstream_message TEXT NOT NULL DEFAULT '',
	latency_ms INTEGER NOT NULL DEFAULT 0,
	attempt INTEGER NOT NULL DEFAULT 1,
	dedup_key TEXT NOT NULL DEFAULT '',
	triggered_by TEXT NOT NULL DEFAULT 'system',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`); err != nil {
		t.Fatalf("seed v5 schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	st, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store with migration: %v", err)
	}
	defer st.Close()

	if st.SchemaVersion() != v3SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", v3SchemaVersion, st.SchemaVersion())
	}
	rows, err := st.DB().Query(`PRAGMA foreign_key_list(notification_deliveries)`)
	if err != nil {
		t.Fatalf("query foreign keys: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatalf("expected notification_deliveries to have no foreign keys after migration")
	}
}

func TestMigrateV6AddsNotificationRetryColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-v6.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '6');
CREATE TABLE notification_channels (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	config TEXT NOT NULL DEFAULT '{}',
	subscriptions TEXT NOT NULL DEFAULT '[]',
	title_template TEXT NOT NULL DEFAULT '',
	body_template TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO notification_channels (name, type, enabled, config, subscriptions) VALUES ('ops', 'generic_webhook', 1, '{}', '[]');
`); err != nil {
		t.Fatalf("seed v6 schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	st, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store with migration: %v", err)
	}
	defer st.Close()

	if st.SchemaVersion() != v3SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", v3SchemaVersion, st.SchemaVersion())
	}
	rows, err := st.DB().Query(`PRAGMA table_info(notification_channels)`)
	if err != nil {
		t.Fatalf("query table info: %v", err)
	}
	defer rows.Close()

	foundAttempts := false
	foundSchedule := false
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
			t.Fatalf("scan table info: %v", err)
		}
		if name == "retry_max_attempts" {
			foundAttempts = true
		}
		if name == "retry_schedule_seconds" {
			foundSchedule = true
		}
	}
	if !foundAttempts || !foundSchedule {
		t.Fatalf("expected retry columns after migration, attempts=%v schedule=%v", foundAttempts, foundSchedule)
	}
}
