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
