package debugcmd

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const maxDebugRows = 200

type command struct {
	dataDir    string
	dbPath     string
	cpaAuthDir string
	cpaBaseURL string
	out        io.Writer
	errOut     io.Writer
}

func Run(args []string) error {
	cfg := commandFromEnv()
	cfg.out = os.Stdout
	cfg.errOut = os.Stderr
	return cfg.run(args)
}

func commandFromEnv() command {
	dataDir := firstNonEmpty(os.Getenv("LUNE_DATA_DIR"), "./data")
	cpaAuthDir := os.Getenv("LUNE_CPA_FILES_DIR")
	if cpaAuthDir == "" {
		cpaAuthDir = os.Getenv("LUNE_CPA_AUTH_DIR")
	}
	if cpaAuthDir == "" {
		cpaAuthDir = filepath.Join(dataDir, "cpa-auth")
	}
	return command{
		dataDir:    dataDir,
		dbPath:     filepath.Join(dataDir, "lune.db"),
		cpaAuthDir: cpaAuthDir,
		cpaBaseURL: os.Getenv("LUNE_CPA_BASE_URL"),
	}
}

func (c command) run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "summary":
		return c.writeJSON(c.summary())
	case "recent-operations":
		return c.writeJSON(c.recentOperations())
	case "operation":
		if len(args) != 2 {
			return fmt.Errorf("usage: lune debug operation <operation_id>")
		}
		return c.writeJSON(c.operation(args[1]))
	case "db-integrity":
		return c.writeJSON(c.dbIntegrity())
	case "cpa-auth":
		return c.writeJSON(c.cpaAuth())
	case "cpa-runtime":
		return c.writeJSON(c.cpaRuntime())
	case "account":
		if len(args) != 2 {
			return fmt.Errorf("usage: lune debug account <account_id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("account_id must be a positive integer")
		}
		return c.writeJSON(c.account(id))
	case "collect":
		return c.collect(args[1:])
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("usage: lune debug <summary|recent-operations|operation|db-integrity|cpa-auth|cpa-runtime|account|collect>")
}

func (c command) openReadOnlyDB() (*sql.DB, error) {
	if _, err := os.Stat(c.dbPath); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=foreign_keys(on)", c.dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (c command) writeJSON(payload any) error {
	enc := json.NewEncoder(c.out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func (c command) summary() map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error(), "db_path": c.dbPath}
	}
	defer db.Close()

	counts := map[string]int64{}
	for _, table := range []string{"accounts", "pools", "pool_members", "request_logs", "operations", "operation_items"} {
		counts[table] = countRows(db, table)
	}
	return map[string]any{
		"status":             "ok",
		"generated_at":       nowUTC(),
		"data_dir":           c.dataDir,
		"db_path":            c.dbPath,
		"schema_version":     readSetting(db, "schema_version"),
		"first_account_id":   firstInt64(db, "accounts", "id"),
		"first_operation_id": firstText(db, "operations", "operation_id"),
		"counts":             counts,
	}
}

func (c command) recentOperations() map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()

	rows, err := db.Query(`SELECT operation_id, operation_type, source, target_type, target_id, target_summary,
		status, error_code, safe_error_message, correlation_id, started_at, finished_at, created_at
		FROM operations ORDER BY datetime(created_at) DESC, id DESC LIMIT ?`, maxDebugRows)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer rows.Close()

	ops := []map[string]any{}
	for rows.Next() {
		op := scanOperationMap(rows)
		op["item_count"] = countOperationItems(db, fmt.Sprint(op["operation_id"]))
		ops = append(ops, op)
	}
	return map[string]any{"status": "ok", "operations": ops}
}

func (c command) operation(operationID string) map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()

	row := db.QueryRow(`SELECT operation_id, operation_type, source, target_type, target_id, target_summary,
		status, error_code, safe_error_message, correlation_id, started_at, finished_at, created_at
		FROM operations WHERE operation_id = ?`, operationID)
	op := scanOperationMap(row)
	if _, ok := op["_scan_error"]; ok {
		return map[string]any{"status": "not_found", "operation_id": operationID}
	}

	items, err := queryMaps(db, `SELECT item_index, client_file_name, account_key_hash, action, status, runtime_sync,
		error_code, safe_error_message, account_id, pool_member_id, stage, created_at
		FROM operation_items WHERE operation_id = ? ORDER BY item_index, id LIMIT 1000`, operationID)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error(), "operation": op}
	}
	redactRows(items)
	op["items"] = items
	return map[string]any{"status": "ok", "operation": op}
}

func (c command) dbIntegrity() map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()

	integrity := querySingleColumn(db, `PRAGMA integrity_check`)
	foreignKeys := queryMapsNoArgs(db, `PRAGMA foreign_key_check`)
	return map[string]any{
		"status":            "ok",
		"integrity_check":   integrity,
		"foreign_key_check": foreignKeys,
	}
}

func (c command) cpaAuth() map[string]any {
	entries, err := os.ReadDir(c.cpaAuthDir)
	if err != nil {
		return map[string]any{"status": "error", "auth_dir": c.cpaAuthDir, "error": err.Error()}
	}
	files := []map[string]any{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		files = append(files, map[string]any{
			"account_key_hash": hashValue(key),
			"file_name":        "ACCOUNT_KEY_REDACTED.json",
			"size_bytes":       info.Size(),
			"modified_at":      info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return map[string]any{"status": "ok", "auth_dir": c.cpaAuthDir, "files": files}
}

func (c command) cpaRuntime() map[string]any {
	if strings.TrimSpace(c.cpaBaseURL) == "" {
		return map[string]any{"status": "not_configured", "base_url": ""}
	}
	url := strings.TrimRight(c.cpaBaseURL, "/") + "/healthz"
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return map[string]any{"status": "error", "base_url": maskURL(c.cpaBaseURL), "error": sanitizeText(err.Error())}
	}
	defer resp.Body.Close()
	return map[string]any{
		"status":      "ok",
		"base_url":    maskURL(c.cpaBaseURL),
		"health_path": "/healthz",
		"http_status": resp.StatusCode,
	}
}

func (c command) account(id int64) map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()

	rows, err := queryMaps(db, `SELECT id, label, source_kind, provider, cpa_provider, cpa_account_key, cpa_email,
		cpa_plan_type, cpa_credential_status, cpa_subscription_status, cpa_access_status, cpa_quota_status,
		serving_status, enabled, status, last_checked_at, last_error, updated_at
		FROM accounts WHERE id = ?`, id)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	if len(rows) == 0 {
		return map[string]any{"status": "not_found", "account_id": id}
	}
	acct := rows[0]
	if key, _ := acct["cpa_account_key"].(string); key != "" {
		acct["cpa_account_key_hash"] = hashValue(key)
	}
	delete(acct, "cpa_account_key")
	if email, _ := acct["cpa_email"].(string); email != "" {
		acct["cpa_email"] = maskEmail(email)
	}
	if msg, _ := acct["last_error"].(string); msg != "" {
		acct["last_error"] = sanitizeText(msg)
	}
	redactRows([]map[string]any{acct})
	recentLogs, _ := queryMaps(db, `SELECT request_id, status_code, success, error_message, diagnostic, stateful_probe,
		traffic_kind, runtime_binding_status, runtime_binding_reason, created_at
		FROM request_logs WHERE account_id = ? ORDER BY datetime(created_at) DESC, id DESC LIMIT 20`, id)
	for _, log := range recentLogs {
		if msg, _ := log["error_message"].(string); msg != "" {
			log["error_message"] = sanitizeText(msg)
		}
	}
	redactRows(recentLogs)
	return map[string]any{"status": "ok", "account": acct, "recent_request_logs": recentLogs}
}

func (c command) collect(args []string) error {
	redact := false
	for _, arg := range args {
		switch arg {
		case "--redact":
			redact = true
		default:
			return fmt.Errorf("usage: lune debug collect --redact")
		}
	}
	if !redact {
		return fmt.Errorf("collect requires --redact")
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	outDir := filepath.Join(c.dataDir, "debug-collect-"+stamp)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	files := map[string]any{
		"summary.txt":                       c.summary(),
		"recent-operations.json":            c.recentOperations(),
		"accounts.redacted.json":            c.accountsForCollect(),
		"account-diagnostics.redacted.json": map[string]any{"status": "not_implemented", "message": "account diagnostics tables are not available yet"},
		"pools.redacted.json":               c.poolsForCollect(),
		"cpa-auth-files.redacted.json":      c.cpaAuth(),
		"runtime.redacted.json":             c.cpaRuntime(),
		"request-logs.redacted.json":        c.requestLogsForCollect(),
		"docker-env.redacted.txt":           redactedEnv(),
	}
	for name, payload := range files {
		if err := writeCollectFile(filepath.Join(outDir, name), payload); err != nil {
			return err
		}
	}
	archivePath := filepath.Join(c.dataDir, "lune-audit-"+stamp+".tar.gz")
	if err := tarGzDir(outDir, archivePath); err != nil {
		return err
	}
	_, err := fmt.Fprintln(c.out, archivePath)
	return err
}

func (c command) accountsForCollect() map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()
	rows, err := queryMaps(db, `SELECT id, label, source_kind, provider, cpa_provider, cpa_account_key, cpa_email,
		cpa_plan_type, cpa_credential_status, cpa_subscription_status, cpa_access_status, cpa_quota_status,
		serving_status, enabled, status, last_checked_at, updated_at FROM accounts ORDER BY id LIMIT ?`, maxDebugRows)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	for _, row := range rows {
		if key, _ := row["cpa_account_key"].(string); key != "" {
			row["cpa_account_key_hash"] = hashValue(key)
		}
		delete(row, "cpa_account_key")
		if email, _ := row["cpa_email"].(string); email != "" {
			row["cpa_email"] = maskEmail(email)
		}
	}
	redactRows(rows)
	return map[string]any{"status": "ok", "accounts": rows}
}

func (c command) poolsForCollect() map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()
	rows, err := queryMaps(db, `SELECT id, label, priority, enabled, routing_policy, created_at, updated_at FROM pools ORDER BY id LIMIT ?`, maxDebugRows)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	return map[string]any{"status": "ok", "pools": rows}
}

func (c command) requestLogsForCollect() map[string]any {
	db, err := c.openReadOnlyDB()
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer db.Close()
	rows, err := queryMaps(db, `SELECT request_id, account_id, status_code, success, error_message, diagnostic,
		stateful_probe, traffic_kind, runtime_binding_status, runtime_binding_reason, created_at
		FROM request_logs ORDER BY datetime(created_at) DESC, id DESC LIMIT ?`, maxDebugRows)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	for _, row := range rows {
		if msg, _ := row["error_message"].(string); msg != "" {
			row["error_message"] = sanitizeText(msg)
		}
	}
	redactRows(rows)
	return map[string]any{"status": "ok", "request_logs": rows}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOperationMap(row rowScanner) map[string]any {
	var operationID, operationType, source, targetType, targetID, targetSummary string
	var status, errorCode, safeErrorMessage, correlationID, startedAt, finishedAt, createdAt string
	if err := row.Scan(&operationID, &operationType, &source, &targetType, &targetID, &targetSummary,
		&status, &errorCode, &safeErrorMessage, &correlationID, &startedAt, &finishedAt, &createdAt); err != nil {
		return map[string]any{"_scan_error": err.Error()}
	}
	return map[string]any{
		"operation_id":       operationID,
		"operation_type":     operationType,
		"source":             source,
		"target_type":        targetType,
		"target_id":          targetID,
		"target_summary":     sanitizeText(targetSummary),
		"status":             status,
		"error_code":         errorCode,
		"safe_error_message": sanitizeText(safeErrorMessage),
		"correlation_id":     correlationID,
		"started_at":         startedAt,
		"finished_at":        finishedAt,
		"created_at":         createdAt,
	}
}

func queryMapsNoArgs(db *sql.DB, query string) []map[string]any {
	rows, err := db.Query(query)
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	defer rows.Close()
	maps, err := rowsToMaps(rows)
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	return maps
}

func queryMaps(db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToMaps(rows)
}

func rowsToMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, col := range cols {
			row[col] = normalizeDBValue(values[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func normalizeDBValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(x)
	default:
		return x
	}
}

func redactRows(rows []map[string]any) {
	for _, row := range rows {
		for key, value := range row {
			text, ok := value.(string)
			if !ok || text == "" {
				continue
			}
			switch key {
			case "client_file_name", "file_name":
				row[key] = maskFileName(text)
			case "safe_error_message", "error_message", "last_error", "runtime_binding_reason", "target_summary":
				row[key] = sanitizeText(text)
			case "cpa_email":
				row[key] = maskEmail(text)
			default:
				row[key] = sanitizeEmails(text)
			}
		}
	}
}

func querySingleColumn(db *sql.DB, query string) []string {
	rows, err := db.Query(query)
	if err != nil {
		return []string{"error: " + err.Error()}
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return []string{"error: " + err.Error()}
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return []string{"error: " + err.Error()}
	}
	return out
}

func countRows(db *sql.DB, table string) int64 {
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return -1
	}
	return count
}

func countOperationItems(db *sql.DB, operationID string) int64 {
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_items WHERE operation_id = ?`, operationID).Scan(&count); err != nil {
		return 0
	}
	return count
}

func readSetting(db *sql.DB, key string) string {
	var value string
	if err := db.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&value); err != nil {
		return ""
	}
	return value
}

func firstInt64(db *sql.DB, table, column string) int64 {
	var value int64
	if err := db.QueryRow(`SELECT ` + column + ` FROM ` + table + ` ORDER BY id LIMIT 1`).Scan(&value); err != nil {
		return 0
	}
	return value
}

func firstText(db *sql.DB, table, column string) string {
	var value string
	if err := db.QueryRow(`SELECT ` + column + ` FROM ` + table + ` ORDER BY id LIMIT 1`).Scan(&value); err != nil {
		return ""
	}
	return value
}

func writeCollectFile(path string, payload any) error {
	switch v := payload.(type) {
	case string:
		return os.WriteFile(path, []byte(v), 0644)
	default:
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, append(data, '\n'), 0644)
	}
}

func tarGzDir(srcDir, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}

func redactedEnv() string {
	keys := []string{"LUNE_DATA_DIR", "LUNE_CPA_FILES_DIR", "LUNE_CPA_AUTH_DIR", "LUNE_CPA_BASE_URL", "LUNE_EMBEDDED_CPA", "LUNE_CPA_PROVIDER_PINNING_SUPPORTED"}
	var b strings.Builder
	for _, key := range keys {
		value := os.Getenv(key)
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
			value = "REDACTED"
		}
		fmt.Fprintf(&b, "%s=%s\n", key, sanitizeText(value))
	}
	return b.String()
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at+1:]
	if len(local) <= 2 {
		return "***@" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + domain
}

func maskFileName(name string) string {
	if name == "" || name == "ACCOUNT_KEY_REDACTED.json" {
		return name
	}
	ext := filepath.Ext(name)
	if strings.Contains(name, "@") || strings.HasSuffix(strings.ToLower(name), ".json") {
		if ext == "" {
			ext = ".json"
		}
		return "ACCOUNT_KEY_REDACTED" + ext
	}
	return sanitizeEmails(name)
}

func maskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, os.Getenv("LUNE_CPA_API_KEY"), "REDACTED")
	raw = strings.ReplaceAll(raw, os.Getenv("LUNE_CPA_MANAGEMENT_KEY"), "REDACTED")
	return raw
}

func sanitizeText(value string) string {
	if value == "" {
		return ""
	}
	replacements := []string{
		os.Getenv("LUNE_ADMIN_TOKEN"),
		os.Getenv("LUNE_CPA_API_KEY"),
		os.Getenv("CPA_API_KEY"),
		os.Getenv("LUNE_CPA_MANAGEMENT_KEY"),
	}
	out := value
	for _, secret := range replacements {
		if secret != "" {
			out = strings.ReplaceAll(out, secret, "REDACTED")
		}
	}
	for _, marker := range []string{"refresh_token", "access_token", "id_token", "api_key", "management_key"} {
		if strings.Contains(strings.ToLower(out), marker) {
			return "REDACTED_SECRET_FIELD"
		}
	}
	return sanitizeEmails(out)
}

func sanitizeEmails(value string) string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '"' || r == '\'' || r == '<' || r == '>' || r == '(' || r == ')' || r == '[' || r == ']'
	})
	out := value
	for _, field := range fields {
		if strings.Contains(field, "@") {
			trimmed := strings.Trim(field, ".,;:")
			out = strings.ReplaceAll(out, trimmed, maskEmail(trimmed))
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
