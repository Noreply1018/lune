package store

func (s *Store) InsertLog(l *RequestLog) error {
	_, err := s.db.Exec(
		`INSERT INTO request_logs (request_id, access_token_name, model_alias, target_model, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.RequestID, l.AccessTokenName, l.ModelAlias, l.TargetModel, l.PoolID, l.AccountID, l.StatusCode, l.LatencyMs, l.InputTokens, l.OutputTokens, l.Stream, l.RequestIP, l.Success, l.ErrorMessage,
	)
	return err
}

func (s *Store) ListLogs(limit, offset int) ([]RequestLog, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, request_id, access_token_name, model_alias, target_model, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message, created_at FROM request_logs ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var l RequestLog
		var stream, success int
		if err := rows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelAlias, &l.TargetModel, &l.PoolID, &l.AccountID, &l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success, &l.ErrorMessage, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		l.Stream = stream != 0
		l.Success = success != 0
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
