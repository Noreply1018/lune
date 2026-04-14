package store

import "time"

type Overview struct {
	TotalAccounts    int                     `json:"total_accounts"`
	HealthyAccounts  int                     `json:"healthy_accounts"`
	TotalPools       int                     `json:"total_pools"`
	TotalTokens      int                     `json:"total_tokens"`
	Requests24h      int64                   `json:"requests_24h"`
	SuccessRate24h   float64                 `json:"success_rate_24h"`
	TokenUsage24h    OverviewTokenUsage      `json:"token_usage_24h"`
	AccountHealth    []OverviewAccountHealth `json:"account_health"`
	RecentRequests   []RequestLog            `json:"recent_requests"`
	CpaStatus        *OverviewCpaStatus      `json:"cpa_status"`
	AccountsBySource map[string]int          `json:"accounts_by_source"`
}

type OverviewTokenUsage struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

type OverviewAccountHealth struct {
	ID            int64   `json:"id"`
	Label         string  `json:"label"`
	Status        string  `json:"status"`
	LastCheckedAt *string `json:"last_checked_at"`
	LastError     *string `json:"last_error"`
}

type OverviewCpaStatus struct {
	Connected        bool    `json:"connected"`
	Label            string  `json:"label"`
	Status           string  `json:"status"`
	AccountsTotal    int     `json:"accounts_total"`
	AccountsHealthy  int     `json:"accounts_healthy"`
	AccountsError    int     `json:"accounts_error"`
	AccountsExpiring int     `json:"accounts_expiring"`
	LastCheckedAt    *string `json:"last_checked_at"`
}

func (s *Store) GetOverview() (*Overview, error) {
	o := &Overview{
		AccountHealth:  []OverviewAccountHealth{},
		RecentRequests: []RequestLog{},
	}

	// accounts
	total, byStatus, err := s.CountAccounts()
	if err != nil {
		return nil, err
	}
	o.TotalAccounts = total
	o.HealthyAccounts = byStatus["healthy"]

	// accounts by source
	o.AccountsBySource, _ = s.CountAccountsBySource()

	// pools
	s.db.QueryRow(`SELECT COUNT(*) FROM pools`).Scan(&o.TotalPools)

	// tokens
	s.db.QueryRow(`SELECT COUNT(*) FROM access_tokens`).Scan(&o.TotalTokens)

	// requests (24h)
	now := time.Now().UTC()
	h24 := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE created_at >= ?`, h24).Scan(&o.Requests24h)

	// success rate (24h)
	s.db.QueryRow(
		`SELECT COALESCE(CAST(SUM(success) AS REAL) / NULLIF(COUNT(*), 0), 0) FROM request_logs WHERE created_at >= ?`,
		h24,
	).Scan(&o.SuccessRate24h)

	// token usage (24h) - split input/output
	s.db.QueryRow(
		`SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM request_logs WHERE created_at >= ?`,
		h24,
	).Scan(&o.TokenUsage24h.Input, &o.TokenUsage24h.Output)

	// account health
	healthRows, err := s.db.Query(`SELECT id, label, status, last_checked_at, last_error FROM accounts ORDER BY id`)
	if err == nil {
		defer healthRows.Close()
		for healthRows.Next() {
			var ah OverviewAccountHealth
			var idRaw any
			var lastChecked, lastErr *string
			if err := healthRows.Scan(&idRaw, &ah.Label, &ah.Status, &lastChecked, &lastErr); err != nil {
				continue
			}
			ah.ID = normalizeAccountID(idRaw)
			ah.LastCheckedAt = lastChecked
			ah.LastError = lastErr
			o.AccountHealth = append(o.AccountHealth, ah)
		}
	}

	// recent requests (last 10)
	recentRows, err := s.db.Query(
		`SELECT rl.id, rl.request_id, rl.access_token_name, rl.model_alias, rl.target_model,
			rl.pool_id, rl.account_id, COALESCE(a.label, '') AS account_label,
			rl.status_code, rl.latency_ms, rl.input_tokens, rl.output_tokens,
			rl.stream, rl.request_ip, rl.success, rl.error_message, rl.source_kind, rl.created_at
		FROM request_logs rl
		LEFT JOIN accounts a ON a.id = rl.account_id
		ORDER BY rl.id DESC LIMIT 10`,
	)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			var l RequestLog
			var stream, success int
			if err := recentRows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelAlias,
				&l.TargetModel, &l.PoolID, &l.AccountID, &l.AccountLabel,
				&l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens,
				&stream, &l.RequestIP, &success, &l.ErrorMessage, &l.SourceKind, &l.CreatedAt); err != nil {
				continue
			}
			l.Stream = stream != 0
			l.Success = success != 0
			o.RecentRequests = append(o.RecentRequests, l)
		}
	}

	// CPA status
	cpaSvc, err := s.GetCpaService()
	if err == nil && cpaSvc != nil {
		cpaStatus := &OverviewCpaStatus{
			Connected:     true,
			Label:         cpaSvc.Label,
			Status:        cpaSvc.Status,
			LastCheckedAt: cpaSvc.LastCheckedAt,
		}
		rows, err := s.db.Query(`SELECT status, COUNT(*) FROM accounts WHERE source_kind = 'cpa' GROUP BY status`)
		if err == nil {
			for rows.Next() {
				var st string
				var c int
				rows.Scan(&st, &c)
				cpaStatus.AccountsTotal += c
				switch st {
				case "healthy", "degraded":
					cpaStatus.AccountsHealthy += c
				case "error":
					cpaStatus.AccountsError += c
				}
			}
			rows.Close()
		}
		var expiring int
		s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE source_kind = 'cpa' AND cpa_expired_at IS NOT NULL AND cpa_expired_at != '' AND cpa_expired_at <= datetime('now', '+7 days')`).Scan(&expiring)
		cpaStatus.AccountsExpiring = expiring
		o.CpaStatus = cpaStatus
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
		where += " AND (" + prefix + "model_alias = ? OR " + prefix + "target_model = ?)"
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

	query := `SELECT rl.id, rl.request_id, rl.access_token_name, rl.model_alias, rl.target_model, rl.pool_id, rl.account_id,
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
		if err := rows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelAlias, &l.TargetModel, &l.PoolID, &l.AccountID, &l.AccountLabel, &l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success, &l.ErrorMessage, &l.SourceKind, &l.CreatedAt); err != nil {
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
		Logs: UsageLogPage{
			Items: []RequestLog{},
		},
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(rl.input_tokens), 0), COALESCE(SUM(rl.output_tokens), 0)
		 FROM request_logs rl
		 WHERE `+where, args...,
	).Scan(&stats.TotalRequests, &stats.TotalInputTokens, &stats.TotalOutputTokens); err != nil {
		return nil, err
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

// LatencyBucket holds percentile latencies for a single time bucket.
type LatencyBucket struct {
	Bucket string  `json:"bucket"`
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	Count  int     `json:"count"`
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
		where += " AND (model_alias = ? OR target_model = ?)"
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
