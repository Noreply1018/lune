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
	// channels table, and seed the singleton settings + 5 subscription rows.
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
	if len(subs) != 5 {
		t.Fatalf("expected 5 seeded subscriptions, got %d", len(subs))
	}
	// The v8→v9 path seeds fresh defaults directly; these must already be the
	// new concrete-field templates, never the old `{{ .Message }}` passthrough.
	wantBodies := map[string]string{
		"account_expiring":     "账号 {{ .Vars.account_label }} 将在 {{ .Vars.expires_at }} 过期。",
		"cpa_credential_error": "账号 {{ .Vars.account_label }} 的 CPA 登录态失效：{{ .Vars.last_error }}。请重新登录。",
		"account_error":        "账号 {{ .Vars.account_label }} 最近错误：{{ .Vars.last_error }}",
		"cpa_service_error":    "CPA runtime {{ .Vars.service_label }} 最近错误：{{ .Vars.last_error }}",
		"test":                 "这是一条用于验证渠道可达性的真实消息，可忽略。",
	}
	for _, sub := range subs {
		want, ok := wantBodies[sub.Event]
		if !ok {
			t.Fatalf("unexpected seeded event %q", sub.Event)
		}
		if sub.BodyTemplate != want {
			t.Fatalf("unexpected seeded body for %q:\nwant=%q\n got=%q", sub.Event, want, sub.BodyTemplate)
		}
	}
}

func TestMigrateV9ToV10DropsFormatAndUpgradesDefaultBodies(t *testing.T) {
	// Seed a v9 database whose notification tables still carry `format` (on
	// settings) and `title_template` (on subscriptions). Three subs sit on the
	// old `{{ .Message }}` passthrough body — they must be upgraded. One sub
	// carries an admin-customized body — it must survive untouched.
	dbPath := filepath.Join(t.TempDir(), "legacy-v9.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '9');
CREATE TABLE notification_settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    enabled             INTEGER NOT NULL DEFAULT 0,
    webhook_url         TEXT    NOT NULL DEFAULT '',
    format              TEXT    NOT NULL DEFAULT 'markdown',
    mention_mobile_list TEXT    NOT NULL DEFAULT '[]',
    created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO notification_settings (id, enabled, webhook_url, format, mention_mobile_list)
VALUES (1, 1, 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=seed', 'markdown', '[]');
CREATE TABLE notification_subscriptions (
    event           TEXT PRIMARY KEY,
    subscribed      INTEGER NOT NULL DEFAULT 1,
    title_template  TEXT    NOT NULL,
    body_template   TEXT    NOT NULL,
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO notification_subscriptions (event, subscribed, title_template, body_template) VALUES
    ('account_expiring',  1, 'Lune [{{ .Severity }}] {{ .Event }}', '{{ .Message }}'),
    ('account_error',     1, 'Lune [{{ .Severity }}] {{ .Event }}', 'CUSTOM body kept as-is'),
    ('cpa_service_error', 1, 'Lune [{{ .Severity }}] {{ .Event }}', '{{ .Message }}'),
    ('test',              1, 'Lune [{{ .Severity }}] {{ .Event }}', '{{ .Message }}');
`); err != nil {
		t.Fatalf("seed v9 schema: %v", err)
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

	hasFormat, err := st.hasColumn("notification_settings", "format")
	if err != nil {
		t.Fatalf("probe format column: %v", err)
	}
	if hasFormat {
		t.Fatalf("expected notification_settings.format to be dropped after v10 migration")
	}
	hasTitle, err := st.hasColumn("notification_subscriptions", "title_template")
	if err != nil {
		t.Fatalf("probe title_template column: %v", err)
	}
	if hasTitle {
		t.Fatalf("expected notification_subscriptions.title_template to be dropped after v10 migration")
	}

	subs, err := st.ListNotificationSubscriptions()
	if err != nil {
		t.Fatalf("list subs: %v", err)
	}
	bodies := map[string]string{}
	for _, sub := range subs {
		bodies[sub.Event] = sub.BodyTemplate
	}
	wantUpgraded := map[string]string{
		"account_expiring":  "账号 {{ .Vars.account_label }} 将在 {{ .Vars.expires_at }} 过期。",
		"cpa_service_error": "CPA runtime {{ .Vars.service_label }} 最近错误：{{ .Vars.last_error }}",
	}
	for event, want := range wantUpgraded {
		if got := bodies[event]; got != want {
			t.Fatalf("expected %q body upgraded to new default:\nwant=%q\n got=%q", event, want, got)
		}
	}
	// Customized body must be preserved — migration only rewrites the old
	// `{{ .Message }}` passthrough.
	if got := bodies["account_error"]; got != "CUSTOM body kept as-is" {
		t.Fatalf("expected customised body preserved, got %q", got)
	}
	// `test` body was `{{ .Message }}` in v9 and is *not* in the upgrade list
	// (there is no concrete-field template for it), so the passthrough should
	// survive unchanged — verifies the migration is selective.
	if got := bodies["test"]; got != "{{ .Message }}" {
		t.Fatalf("expected test body untouched (not in upgrade list), got %q", got)
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
	if count != 5 {
		t.Fatalf("expected 5 seeded subscription rows, got %d", count)
	}
}

func TestMigrateV12AccessTokensBecomePoolScoped(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-v12-tokens.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE system_config (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
INSERT INTO system_config (key, value) VALUES ('schema_version', '12');
CREATE TABLE pools (
    id INTEGER PRIMARY KEY,
    label TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO pools (id, label, priority, enabled) VALUES (1, 'Primary', 0, 1), (2, 'Secondary', 1, 1), (3, 'Missing Token Pool', 2, 1);
CREATE TABLE access_tokens (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    pool_id INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT
);
INSERT INTO access_tokens (id, name, token, pool_id, enabled, created_at, updated_at)
VALUES
    (1, 'global-old', 'sk-lune-global-old', NULL, 1, '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
    (2, 'primary-a', 'sk-lune-primary-a', 1, 0, '2026-01-02 00:00:00', '2026-01-02 00:00:00'),
    (3, 'primary-b', 'sk-lune-primary-b', 1, 1, '2026-01-03 00:00:00', '2026-01-03 00:00:00'),
    (4, 'secondary', 'sk-lune-secondary', 2, 1, '2026-01-04 00:00:00', '2026-01-04 00:00:00');
`); err != nil {
		t.Fatalf("seed v12 schema: %v", err)
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

	var poolIDNotNull int
	rows, err := st.DB().Query(`PRAGMA table_info(access_tokens)`)
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
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
			rows.Close()
			t.Fatalf("scan table info: %v", err)
		}
		if name == "pool_id" {
			poolIDNotNull = notNull
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("table info rows: %v", err)
	}
	rows.Close()
	if poolIDNotNull != 1 {
		t.Fatalf("expected access_tokens.pool_id to be NOT NULL after migration")
	}

	var total int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM access_tokens`).Scan(&total); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected one token per pool, got %d", total)
	}
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM access_tokens WHERE pool_id IS NULL`).Scan(&total); err != nil {
		t.Fatalf("count global tokens: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected no global tokens after migration, got %d", total)
	}

	if _, err := st.DB().Exec(`INSERT INTO access_tokens (name, token, pool_id) VALUES ('bad', 'sk-lune-bad', NULL)`); err == nil {
		t.Fatalf("expected NULL pool_id insert to fail")
	}
	if _, err := st.DB().Exec(`INSERT INTO access_tokens (name, token, pool_id) VALUES ('dup', 'sk-lune-dup', 1)`); err == nil {
		t.Fatalf("expected duplicate pool_id insert to fail")
	}
}
