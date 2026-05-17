package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxRequestLogErrorMessageBytes = 4096

var (
	bearerTokenPattern     = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
	apiKeyPattern          = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9._-]{8,}|sess-[A-Za-z0-9._-]{8,}|eyJ[A-Za-z0-9._-]{12,})\b`)
	authHeaderPattern      = regexp.MustCompile(`(?i)(authorization|x-api-key|api[_-]?key|access[_-]?token|refresh[_-]?token)\s*[:=]\s*[^,\s}]+`)
	requestPayloadPattern  = regexp.MustCompile(`(?is)("(?:messages|prompt|input|body|request_body)"\s*:\s*)(\[[^\n]*?\]|\{[^\n]*?\}|"[^"]*"|[^,}\n]+)`)
	whitespaceErrorPattern = regexp.MustCompile(`\s+`)
)

func (s *Store) InsertLog(l *RequestLog) error {
	sourceKind := l.SourceKind
	if sourceKind == "" {
		sourceKind = "openai_compat"
	}
	l.ErrorMessage = sanitizeRequestLogError(l.ErrorMessage)
	if !l.Success && strings.TrimSpace(l.ErrorMessage) != "" {
		l.ErrorFingerprint = requestLogErrorFingerprint(l, sourceKind)
		if l.ErrorRepeatCount <= 0 {
			l.ErrorRepeatCount = 1
		}
		if l.ErrorLastSeenAt == "" {
			l.ErrorLastSeenAt = time.Now().UTC().Format("2006-01-02 15:04:05")
		}
		if l.ErrorFingerprint != "" {
			if updated, err := s.foldRecentRequestLogError(l); err != nil {
				return err
			} else if updated {
				return nil
			}
		}
	} else if l.ErrorRepeatCount <= 0 {
		l.ErrorRepeatCount = 1
	}
	// attempt_count = 0  保留语义：请求没跑进重试循环就失败（路由阶段拒绝）。
	// >=1 表示真的发起过上游尝试。负数才 clamp 到 0，避免脏数据。
	attempts := l.AttemptCount
	if attempts < 0 {
		attempts = 0
	}
	accountLabelSnapshot := strings.TrimSpace(l.AccountLabel)
	if accountLabelSnapshot == "" && l.AccountID > 0 {
		if acc, err := s.GetAccount(l.AccountID); err == nil && acc != nil {
			accountLabelSnapshot = acc.Label
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO request_logs (
			request_id, access_token_name, model_requested, model_actual, pool_id, account_id, account_label_snapshot,
			status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success,
			error_message, error_fingerprint, error_repeat_count, error_last_seen_at, source_kind, attempt_count, diagnostic,
			runtime_auth_index, runtime_auth_id, runtime_account_key, runtime_binding_status, runtime_binding_reason, route_trace
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.RequestID, l.AccessTokenName, l.ModelRequested, l.ModelActual, l.PoolID, l.AccountID, accountLabelSnapshot,
		l.StatusCode, l.LatencyMs, l.InputTokens, l.OutputTokens, l.Stream, l.RequestIP, l.Success,
		l.ErrorMessage, l.ErrorFingerprint, l.ErrorRepeatCount, l.ErrorLastSeenAt, sourceKind, attempts, boolToInt(l.Diagnostic),
		l.RuntimeAuthIndex, l.RuntimeAuthID, l.RuntimeAccountKey, l.RuntimeBindingStatus, l.RuntimeBindingReason, sanitizeRequestLogError(l.RouteTrace),
	)
	return err
}

func (s *Store) foldRecentRequestLogError(l *RequestLog) (bool, error) {
	since := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	var id int64
	err := s.db.QueryRow(
		`SELECT id
		 FROM request_logs
		 WHERE error_fingerprint = ?
		   AND diagnostic = ?
		   AND created_at >= ?
		 ORDER BY id DESC
		 LIMIT 1`,
		l.ErrorFingerprint, boolToInt(l.Diagnostic), since,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(
		`UPDATE request_logs
		 SET error_repeat_count = error_repeat_count + 1,
		     error_last_seen_at = datetime('now'),
		     latency_ms = ?
		 WHERE id = ?`,
		l.LatencyMs, id,
	)
	return err == nil, err
}

func scanLogRow(rows *sql.Rows) (RequestLog, error) {
	var l RequestLog
	var stream, success, diagnostic int
	var poolID, accountID sql.NullInt64
	if err := rows.Scan(
		&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelRequested, &l.ModelActual, &poolID, &accountID,
		&l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success,
		&l.ErrorMessage, &l.ErrorFingerprint, &l.ErrorRepeatCount, &l.ErrorLastSeenAt, &l.SourceKind, &l.AttemptCount, &diagnostic,
		&l.RuntimeAuthIndex, &l.RuntimeAuthID, &l.RuntimeAccountKey, &l.RuntimeBindingStatus, &l.RuntimeBindingReason,
		&l.RouteTrace,
		&l.CreatedAt,
	); err != nil {
		return l, err
	}
	if poolID.Valid {
		l.PoolID = poolID.Int64
	}
	if accountID.Valid {
		l.AccountID = accountID.Int64
	}
	l.Stream = stream != 0
	l.Success = success != 0
	l.Diagnostic = diagnostic != 0
	return l, nil
}

func (s *Store) ListLogs(limit, offset int) ([]RequestLog, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, request_id, access_token_name, model_requested, model_actual, pool_id, account_id,
			status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success,
			error_message, error_fingerprint, error_repeat_count, error_last_seen_at, source_kind, attempt_count, diagnostic,
			runtime_auth_index, runtime_auth_id, runtime_account_key, runtime_binding_status, runtime_binding_reason, route_trace,
			created_at
		 FROM request_logs ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		l, err := scanLogRow(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

func (s *Store) ListLogsByPool(poolID int64, limit, offset int) ([]RequestLog, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE pool_id = ?`, poolID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, request_id, access_token_name, model_requested, model_actual, pool_id, account_id,
			status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success,
			error_message, error_fingerprint, error_repeat_count, error_last_seen_at, source_kind, attempt_count, diagnostic,
			runtime_auth_index, runtime_auth_id, runtime_account_key, runtime_binding_status, runtime_binding_reason, route_trace,
			created_at
		 FROM request_logs WHERE pool_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		poolID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		l, err := scanLogRow(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

func sanitizeRequestLogError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	message = bearerTokenPattern.ReplaceAllString(message, "Bearer [redacted]")
	message = apiKeyPattern.ReplaceAllString(message, "[redacted]")
	message = authHeaderPattern.ReplaceAllString(message, "$1=[redacted]")
	message = requestPayloadPattern.ReplaceAllString(message, `${1}"[redacted]"`)
	return truncateUTF8Bytes(message, maxRequestLogErrorMessageBytes)
}

func requestLogErrorFingerprint(l *RequestLog, sourceKind string) string {
	if l == nil {
		return ""
	}
	normalizedError := strings.ToLower(strings.TrimSpace(l.ErrorMessage))
	normalizedError = whitespaceErrorPattern.ReplaceAllString(normalizedError, " ")
	if normalizedError == "" {
		return ""
	}
	parts := []string{
		sourceKind,
		l.ModelRequested,
		l.ModelActual,
		strconvInt64(l.PoolID),
		strconvInt64(l.AccountID),
		strconv.Itoa(l.StatusCode),
		strconv.FormatBool(l.Diagnostic),
		normalizedError,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	end := 0
	for i := range s {
		if i > maxBytes {
			break
		}
		end = i
	}
	if end == 0 {
		return s[:maxBytes]
	}
	return strings.TrimSpace(s[:end])
}

func strconvInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
