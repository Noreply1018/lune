package store

import "database/sql"

func (s *Store) InsertLog(l *RequestLog) error {
	sourceKind := l.SourceKind
	if sourceKind == "" {
		sourceKind = "openai_compat"
	}
	_, err := s.db.Exec(
		`INSERT INTO request_logs (request_id, access_token_name, model_requested, model_actual, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message, source_kind) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.RequestID, l.AccessTokenName, l.ModelRequested, l.ModelActual, l.PoolID, l.AccountID, l.StatusCode, l.LatencyMs, l.InputTokens, l.OutputTokens, l.Stream, l.RequestIP, l.Success, l.ErrorMessage, sourceKind,
	)
	return err
}

func scanLogRow(rows *sql.Rows) (RequestLog, error) {
	var l RequestLog
	var stream, success int
	var poolID, accountID sql.NullInt64
	if err := rows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelRequested, &l.ModelActual, &poolID, &accountID, &l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success, &l.ErrorMessage, &l.SourceKind, &l.CreatedAt); err != nil {
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
	return l, nil
}

func (s *Store) ListLogs(limit, offset int) ([]RequestLog, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, request_id, access_token_name, model_requested, model_actual, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message, source_kind, created_at FROM request_logs ORDER BY id DESC LIMIT ? OFFSET ?`,
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
		`SELECT id, request_id, access_token_name, model_requested, model_actual, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message, source_kind, created_at FROM request_logs WHERE pool_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
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
