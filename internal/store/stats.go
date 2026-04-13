package store

import "time"

type Overview struct {
	Accounts         OverviewAccounts        `json:"accounts"`
	Pools            OverviewCount            `json:"pools"`
	Routes           int                      `json:"routes"`
	Tokens           OverviewCount            `json:"tokens"`
	Requests         OverviewPeriod           `json:"requests"`
	TokenUsed        OverviewPeriod           `json:"token_used"`
	CpaStatus        *OverviewCpaStatus       `json:"cpa_status"`
	AccountsBySource map[string]int           `json:"accounts_by_source"`
}

type OverviewAccounts struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
}

type OverviewCount struct {
	Total    int `json:"total"`
	Enabled  int `json:"enabled"`
	Disabled int `json:"disabled"`
}

type OverviewPeriod struct {
	Last24h int64 `json:"last_24h"`
	Last7d  int64 `json:"last_7d"`
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
	o := &Overview{}

	// accounts
	total, byStatus, err := s.CountAccounts()
	if err != nil {
		return nil, err
	}
	o.Accounts = OverviewAccounts{Total: total, ByStatus: byStatus}

	// accounts by source
	o.AccountsBySource, _ = s.CountAccountsBySource()

	// pools
	var poolTotal, poolEnabled int
	s.db.QueryRow(`SELECT COUNT(*) FROM pools`).Scan(&poolTotal)
	s.db.QueryRow(`SELECT COUNT(*) FROM pools WHERE enabled=1`).Scan(&poolEnabled)
	o.Pools = OverviewCount{Total: poolTotal, Enabled: poolEnabled, Disabled: poolTotal - poolEnabled}

	// routes
	routeCount, _ := s.CountRoutes()
	o.Routes = routeCount

	// tokens
	var tokenTotal, tokenEnabled int
	s.db.QueryRow(`SELECT COUNT(*) FROM access_tokens`).Scan(&tokenTotal)
	s.db.QueryRow(`SELECT COUNT(*) FROM access_tokens WHERE enabled=1`).Scan(&tokenEnabled)
	o.Tokens = OverviewCount{Total: tokenTotal, Enabled: tokenEnabled, Disabled: tokenTotal - tokenEnabled}

	// requests
	now := time.Now().UTC()
	h24 := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	d7 := now.Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE created_at >= ?`, h24).Scan(&o.Requests.Last24h)
	s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE created_at >= ?`, d7).Scan(&o.Requests.Last7d)

	// token usage
	s.db.QueryRow(`SELECT COALESCE(SUM(input_tokens + output_tokens), 0) FROM request_logs WHERE created_at >= ?`, h24).Scan(&o.TokenUsed.Last24h)
	s.db.QueryRow(`SELECT COALESCE(SUM(input_tokens + output_tokens), 0) FROM request_logs WHERE created_at >= ?`, d7).Scan(&o.TokenUsed.Last7d)

	// CPA status
	cpaSvc, err := s.GetCpaService()
	if err == nil && cpaSvc != nil {
		cpaStatus := &OverviewCpaStatus{
			Connected:     true,
			Label:         cpaSvc.Label,
			Status:        cpaSvc.Status,
			LastCheckedAt: cpaSvc.LastCheckedAt,
		}
		// count CPA accounts by status
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
		// count expiring accounts (expired_at within 7 days)
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
	SourceKind string `json:"source_kind"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

func (s *Store) GetUsage(f UsageFilter) ([]RequestLog, int, error) {
	where := "1=1"
	var args []any

	if f.From != "" {
		where += " AND created_at >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		where += " AND created_at <= ?"
		args = append(args, f.To)
	}
	if f.TokenName != "" {
		where += " AND access_token_name = ?"
		args = append(args, f.TokenName)
	}
	if f.SourceKind != "" {
		where += " AND source_kind = ?"
		args = append(args, f.SourceKind)
	}

	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM request_logs WHERE "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset

	query := "SELECT id, request_id, access_token_name, model_alias, target_model, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message, source_kind, created_at FROM request_logs WHERE " + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var l RequestLog
		var stream, success int
		if err := rows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelAlias, &l.TargetModel, &l.PoolID, &l.AccountID, &l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success, &l.ErrorMessage, &l.SourceKind, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		l.Stream = stream != 0
		l.Success = success != 0
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// LatencyBucket holds percentile latencies for a single time bucket.
type LatencyBucket struct {
	Bucket string  `json:"bucket"`
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	Count  int     `json:"count"`
}

func (s *Store) GetLatencyStats(model, period, bucket string) ([]LatencyBucket, error) {
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
