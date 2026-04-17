package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

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

func TestMigrateFromLegacyNotificationSchemaInstallsSingletonTables(t *testing.T) {
	// Seed a database at the pre-singleton schema (v8) that had notification_channels,
	// outbox, deliveries tables. Opening the store should migrate to v9, drop the
	// channels table, and seed the singleton settings + 4 subscription rows.
	dbPath := filepath.Join(t.TempDir(), "legacy-v8.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '8');
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
CREATE TABLE notification_outbox (
	id INTEGER PRIMARY KEY,
	channel_id INTEGER NOT NULL DEFAULT 1,
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
	channel_id INTEGER NOT NULL DEFAULT 1,
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
		t.Fatalf("seed v8 schema: %v", err)
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

	settings, err := st.GetNotificationSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Enabled {
		t.Fatalf("expected default settings to be disabled after migration, got %+v", settings)
	}

	subs, err := st.ListNotificationSubscriptions()
	if err != nil {
		t.Fatalf("list subs: %v", err)
	}
	if len(subs) != 4 {
		t.Fatalf("expected 4 seeded subscriptions, got %d", len(subs))
	}
}

func TestFreshDatabaseOpensAtCurrentSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fresh.db")
	st, err := New(dbPath)
	if err != nil {
		t.Fatalf("open fresh store: %v", err)
	}
	defer st.Close()

	if st.SchemaVersion() != v3SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", v3SchemaVersion, st.SchemaVersion())
	}

	var count int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM notification_settings WHERE id = 1`).Scan(&count); err != nil {
		t.Fatalf("query notification_settings: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected singleton settings row, got count %d", count)
	}
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM notification_subscriptions`).Scan(&count); err != nil {
		t.Fatalf("query notification_subscriptions: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 seeded subscription rows, got %d", count)
	}
}
