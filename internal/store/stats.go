package store

import (
	"fmt"
	"time"
)

func (s *Store) GetOverview() (*Overview, error) {
	o := &Overview{
		Alerts: []Alert{},
	}

	// pools total / healthy
	poolRows, err := s.db.Query(`
		SELECT p.enabled,
			(SELECT COUNT(*)
			 FROM pool_members pm
			 JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1) AS account_count,
			(SELECT COUNT(*) FROM pool_members pm JOIN accounts a ON a.id = pm.account_id
			 WHERE pm.pool_id = p.id AND pm.enabled = 1 AND a.enabled = 1 AND a.status IN ('healthy', 'degraded')) AS healthy_account_count
		FROM pools p`)
	if err == nil {
		defer poolRows.Close()
		for poolRows.Next() {
			var enabled int
			var accountCount int
			var healthyAccountCount int
			if err := poolRows.Scan(&enabled, &accountCount, &healthyAccountCount); err != nil {
				continue
			}
			if enabled == 0 {
				continue
			}
			o.PoolsTotal++
			if healthyAccountCount > 0 {
				o.PoolsHealthy++
			}
		}
	}

	// accounts total / healthy
	total, byStatus, err := s.CountAccounts()
	if err == nil {
		o.AccountsTotal = total
		o.AccountsHealthy = byStatus["healthy"] + byStatus["degraded"]
	}

	// models total (distinct model_id from account_models)
	s.db.QueryRow(`SELECT COUNT(DISTINCT model_id) FROM account_models`).Scan(&o.ModelsTotal)

	// requests today + success rate
	now := time.Now().UTC()
	todayStart := now.Format("2006-01-02") + " 00:00:00"

	s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE created_at >= ?`, todayStart).Scan(&o.RequestsToday)
	s.db.QueryRow(
		`SELECT COALESCE(CAST(SUM(success) AS REAL) / NULLIF(COUNT(*), 0), 0) FROM request_logs WHERE created_at >= ?`,
		todayStart,
	).Scan(&o.SuccessRateToday)

	// global token metadata (first token where pool_id IS NULL)
	var tokenID int64
	var tokenVal string
	err = s.db.QueryRow(`SELECT id, token FROM access_tokens WHERE pool_id IS NULL AND enabled = 1 ORDER BY id LIMIT 1`).Scan(&tokenID, &tokenVal)
	if err == nil && tokenVal != "" {
		o.GlobalTokenID = &tokenID
		if len(tokenVal) > 12 {
			o.GlobalTokenMasked = tokenVal[:12] + "..." + tokenVal[len(tokenVal)-4:]
		} else {
			o.GlobalTokenMasked = tokenVal
		}
	}

	// alerts: accounts expiring within 7 days
	expiringRows, err := s.db.Query(
		`SELECT id, label, cpa_expired_at FROM accounts WHERE source_kind = 'cpa' AND cpa_expired_at != ''`,
	)
	if err == nil {
		defer expiringRows.Close()
		now := time.Now().UTC()
		cutoff := now.AddDate(0, 0, 7)
		for expiringRows.Next() {
			var id int64
			var label, expiredAt string
			if err := expiringRows.Scan(&id, &label, &expiredAt); err != nil {
				continue
			}
			expiry, err := parseCpaExpiry(expiredAt)
			if err != nil || expiry.Before(now) || expiry.After(cutoff) {
				continue
			}
			o.Alerts = append(o.Alerts, Alert{
				Type:    "expiring",
				Message: fmt.Sprintf("Account %q expires at %s", label, expiredAt),
			})
		}
	}

	// alerts: accounts with error status
	errorRows, err := s.db.Query(
		`SELECT id, label, last_error FROM accounts WHERE status = 'error'`,
	)
	if err == nil {
		defer errorRows.Close()
		for errorRows.Next() {
			var id int64
			var label, lastError string
			if err := errorRows.Scan(&id, &label, &lastError); err != nil {
				continue
			}
			msg := fmt.Sprintf("Account %q has error status", label)
			if lastError != "" {
				msg += ": " + lastError
			}
			o.Alerts = append(o.Alerts, Alert{
				Type:    "error",
				Message: msg,
			})
		}
	}

	return o, nil
}

type UsageFilter struct {
	From       string `json:"from"`
	To         string `json:"to"`
	TokenName  string `json:"token_name"`
	AccountID  int64  `json:"account_id"`
	Model      string `json:"model"`
	SourceKind string `json:"source_kind"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

func buildUsageWhere(f UsageFilter, alias string) (string, []any) {
	where := "1=1"
	var args []any
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	if f.From != "" {
		where += " AND " + prefix + "created_at >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		where += " AND " + prefix + "created_at <= ?"
		args = append(args, f.To)
	}
	if f.TokenName != "" {
		where += " AND " + prefix + "access_token_name = ?"
		args = append(args, f.TokenName)
	}
	if f.AccountID > 0 {
		where += " AND " + prefix + "account_id = ?"
		args = append(args, f.AccountID)
	}
	if f.Model != "" {
		where += " AND (" + prefix + "model_requested = ? OR " + prefix + "model_actual = ?)"
		args = append(args, f.Model, f.Model)
	}
	if f.SourceKind != "" {
		where += " AND " + prefix + "source_kind = ?"
		args = append(args, f.SourceKind)
	}
	return where, args
}

func (s *Store) GetUsage(f UsageFilter) ([]RequestLog, int, error) {
	where, args := buildUsageWhere(f, "rl")
	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM request_logs rl WHERE "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset

	query := `SELECT rl.id, rl.request_id, rl.access_token_name, rl.model_requested, rl.model_actual, rl.pool_id, rl.account_id,
		COALESCE(a.label, '') AS account_label, rl.status_code, rl.latency_ms, rl.input_tokens, rl.output_tokens,
		rl.stream, rl.request_ip, rl.success, rl.error_message, rl.source_kind, rl.created_at
		FROM request_logs rl
		LEFT JOIN accounts a ON a.id = rl.account_id
		WHERE ` + where + ` ORDER BY rl.id DESC LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), limit, offset)

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var l RequestLog
		var stream, success int
		if err := rows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelRequested, &l.ModelActual, &l.PoolID, &l.AccountID, &l.AccountLabel, &l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success, &l.ErrorMessage, &l.SourceKind, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		l.Stream = stream != 0
		l.Success = success != 0
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

func (s *Store) GetUsageSummary(f UsageFilter) (*UsageStats, error) {
	where, args := buildUsageWhere(f, "rl")
	stats := &UsageStats{
		ByAccount: []UsageByAccount{},
		ByToken:   []UsageByToken{},
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 WHERE `+where, args...,
	).Scan(&stats.TotalRequests, &stats.TotalInputTokens, &stats.TotalOutputTokens); err != nil {
		return nil, err
	}

	// success rate
	if stats.TotalRequests > 0 {
		var successCount int64
		s.db.QueryRow(
			`SELECT COALESCE(SUM(rl.success), 0) FROM request_logs rl WHERE `+where, args...,
		).Scan(&successCount)
		stats.SuccessRate = float64(successCount) / float64(stats.TotalRequests)
	}

	accountRows, err := s.db.Query(
		`SELECT rl.account_id, COALESCE(a.label, '') AS account_label, COUNT(*),
			COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 LEFT JOIN accounts a ON a.id = rl.account_id
		 WHERE `+where+`
		 GROUP BY rl.account_id, a.label
		 ORDER BY COUNT(*) DESC, rl.account_id ASC
		 LIMIT 20`, args...,
	)
	if err != nil {
		return nil, err
	}
	defer accountRows.Close()
	for accountRows.Next() {
		var row UsageByAccount
		if err := accountRows.Scan(&row.AccountID, &row.AccountLabel, &row.Requests, &row.InputTokens, &row.OutputTokens); err != nil {
			return nil, err
		}
		stats.ByAccount = append(stats.ByAccount, row)
	}
	if err := accountRows.Err(); err != nil {
		return nil, err
	}

	tokenRows, err := s.db.Query(
		`SELECT COALESCE(rl.access_token_name, '') AS token_name, COUNT(*),
			COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 WHERE `+where+`
		 GROUP BY rl.access_token_name
		 ORDER BY COUNT(*) DESC, token_name ASC
		 LIMIT 20`, args...,
	)
	if err != nil {
		return nil, err
	}
	defer tokenRows.Close()
	for tokenRows.Next() {
		var row UsageByToken
		if err := tokenRows.Scan(&row.TokenName, &row.Requests, &row.InputTokens, &row.OutputTokens); err != nil {
			return nil, err
		}
		stats.ByToken = append(stats.ByToken, row)
	}
	if err := tokenRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *Store) GetPoolStats(poolID int64, window string) (*UsageStats, error) {
	now := time.Now().UTC()
	var since string
	switch window {
	case "today":
		since = now.Format("2006-01-02") + " 00:00:00"
	case "24h":
		since = now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	case "7d":
		since = now.Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	case "30d":
		since = now.Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	default:
		since = now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	}

	f := UsageFilter{
		From: since,
	}
	where, args := buildUsageWhere(f, "rl")
	where += " AND rl.pool_id = ?"
	args = append(args, poolID)

	stats := &UsageStats{
		ByAccount: []UsageByAccount{},
		ByToken:   []UsageByToken{},
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 WHERE `+where, args...,
	).Scan(&stats.TotalRequests, &stats.TotalInputTokens, &stats.TotalOutputTokens); err != nil {
		return nil, err
	}

	if stats.TotalRequests > 0 {
		var successCount int64
		s.db.QueryRow(
			`SELECT COALESCE(SUM(rl.success), 0) FROM request_logs rl WHERE `+where, args...,
		).Scan(&successCount)
		stats.SuccessRate = float64(successCount) / float64(stats.TotalRequests)
	}

	accountRows, err := s.db.Query(
		`SELECT rl.account_id, COALESCE(a.label, '') AS account_label, COUNT(*),
			COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 LEFT JOIN accounts a ON a.id = rl.account_id
		 WHERE `+where+`
		 GROUP BY rl.account_id, a.label
		 ORDER BY COUNT(*) DESC, rl.account_id ASC
		 LIMIT 20`, args...,
	)
	if err != nil {
		return nil, err
	}
	defer accountRows.Close()
	for accountRows.Next() {
		var row UsageByAccount
		if err := accountRows.Scan(&row.AccountID, &row.AccountLabel, &row.Requests, &row.InputTokens, &row.OutputTokens); err != nil {
			return nil, err
		}
		stats.ByAccount = append(stats.ByAccount, row)
	}
	if err := accountRows.Err(); err != nil {
		return nil, err
	}

	tokenRows, err := s.db.Query(
		`SELECT COALESCE(rl.access_token_name, '') AS token_name, COUNT(*),
			COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 WHERE `+where+`
		 GROUP BY rl.access_token_name
		 ORDER BY COUNT(*) DESC, token_name ASC
		 LIMIT 20`, args...,
	)
	if err != nil {
		return nil, err
	}
	defer tokenRows.Close()
	for tokenRows.Next() {
		var row UsageByToken
		if err := tokenRows.Scan(&row.TokenName, &row.Requests, &row.InputTokens, &row.OutputTokens); err != nil {
			return nil, err
		}
		stats.ByToken = append(stats.ByToken, row)
	}
	if err := tokenRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *Store) GetLatencyStats(model, period, bucket string, accountID ...int64) ([]LatencyBucket, error) {
	now := time.Now().UTC()
	var since time.Time
	switch period {
	case "1h":
		since = now.Add(-1 * time.Hour)
	case "24h":
		since = now.Add(-24 * time.Hour)
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
	default:
		since = now.Add(-24 * time.Hour)
	}

	var bucketExpr string
	switch bucket {
	case "5m":
		bucketExpr = "strftime('%Y-%m-%d %H:', created_at) || printf('%02d', (cast(strftime('%M', created_at) as integer) / 5) * 5)"
	case "1h":
		bucketExpr = "strftime('%Y-%m-%d %H:00', created_at)"
	case "1d":
		bucketExpr = "strftime('%Y-%m-%d', created_at)"
	default:
		bucketExpr = "strftime('%Y-%m-%d %H:00', created_at)"
	}

	where := "created_at >= ? AND latency_ms > 0"
	args := []any{since.Format("2006-01-02 15:04:05")}
	if model != "" {
		where += " AND (model_requested = ? OR model_actual = ?)"
		args = append(args, model, model)
	}
	if len(accountID) > 0 && accountID[0] > 0 {
		where += " AND account_id = ?"
		args = append(args, accountID[0])
	}

	query := "SELECT " + bucketExpr + " as bucket, latency_ms FROM request_logs WHERE " + where + " ORDER BY bucket, latency_ms"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type bucketEntry struct {
		values []int64
	}
	bucketMap := make(map[string]*bucketEntry)
	var order []string

	for rows.Next() {
		var b string
		var lat int64
		if err := rows.Scan(&b, &lat); err != nil {
			return nil, err
		}
		e, exists := bucketMap[b]
		if !exists {
			e = &bucketEntry{}
			bucketMap[b] = e
			order = append(order, b)
		}
		e.values = append(e.values, lat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]LatencyBucket, 0, len(order))
	for _, b := range order {
		vals := bucketMap[b].values
		// vals are already sorted (ORDER BY latency_ms)
		result = append(result, LatencyBucket{
			Bucket: b,
			P50:    percentile(vals, 0.50),
			P95:    percentile(vals, 0.95),
			P99:    percentile(vals, 0.99),
			Count:  len(vals),
		})
	}
	return result, nil
}

func percentile(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return float64(sorted[idx])
}
