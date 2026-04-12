package store

import "time"

type Overview struct {
	Accounts  OverviewAccounts `json:"accounts"`
	Pools     OverviewCount    `json:"pools"`
	Routes    int              `json:"routes"`
	Tokens    OverviewCount    `json:"tokens"`
	Requests  OverviewPeriod   `json:"requests"`
	TokenUsed OverviewPeriod   `json:"token_used"`
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

func (s *Store) GetOverview() (*Overview, error) {
	o := &Overview{}

	// accounts
	total, byStatus, err := s.CountAccounts()
	if err != nil {
		return nil, err
	}
	o.Accounts = OverviewAccounts{Total: total, ByStatus: byStatus}

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

	return o, nil
}

type UsageFilter struct {
	From      string `json:"from"`
	To        string `json:"to"`
	TokenName string `json:"token_name"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
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

	query := "SELECT id, request_id, access_token_name, model_alias, target_model, pool_id, account_id, status_code, latency_ms, input_tokens, output_tokens, stream, request_ip, success, error_message, created_at FROM request_logs WHERE " + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
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
		if err := rows.Scan(&l.ID, &l.RequestID, &l.AccessTokenName, &l.ModelAlias, &l.TargetModel, &l.PoolID, &l.AccountID, &l.StatusCode, &l.LatencyMs, &l.InputTokens, &l.OutputTokens, &stream, &l.RequestIP, &success, &l.ErrorMessage, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		l.Stream = stream != 0
		l.Success = success != 0
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
