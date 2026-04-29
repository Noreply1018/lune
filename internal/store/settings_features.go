package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"lune/internal/syscfg"
)

func (s *Store) ListSystemNotifications() ([]SystemNotification, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	notifications := make([]SystemNotification, 0, 8)

	expiringDays := syscfg.ParsePositiveInt(settings["notification_expiring_days"], syscfg.DefaultNotificationExpiringDays)
	rows, err := s.db.Query(
		`SELECT id, label,
		        CASE
		          WHEN lower(cpa_provider) = 'codex' THEN cpa_subscription_expires_at
		          ELSE cpa_expired_at
		        END AS expires_at
		 FROM accounts
		 WHERE source_kind = 'cpa'
		   AND (
		     (lower(cpa_provider) = 'codex' AND cpa_subscription_expires_at != '')
		     OR (lower(cpa_provider) != 'codex' AND cpa_expired_at != '')
		   )
		   AND enabled = 1
		 ORDER BY expires_at ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, expiringDays)
	for rows.Next() {
		var (
			accountID int64
			label     string
			expiresAt string
		)
		if err := rows.Scan(&accountID, &label, &expiresAt); err != nil {
			return nil, err
		}
		expiry, err := parseCpaExpiry(expiresAt)
		if err != nil || expiry.After(cutoff) {
			continue
		}
		severity := "warning"
		title := "CPA account expiring soon"
		if !expiry.After(now) {
			severity = "critical"
			title = "CPA account expired"
		}
		accountIDCopy := accountID
		notifications = append(notifications, SystemNotification{
			Type:      "account_expiring",
			Severity:  severity,
			Title:     title,
			Message:   fmt.Sprintf("Account %q expires at %s", label, expiresAt),
			AccountID: &accountIDCopy,
			Label:     label,
			ExpiresAt: expiresAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	accountRows, err := s.db.Query(
		`SELECT id, label, last_error
		 FROM accounts
		 WHERE status = 'error'
		   AND enabled = 1
		 ORDER BY last_checked_at DESC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer accountRows.Close()

	for accountRows.Next() {
		var (
			accountID int64
			label     string
			lastError string
		)
		if err := accountRows.Scan(&accountID, &label, &lastError); err != nil {
			return nil, err
		}
		if lastError == "" {
			lastError = "unknown error"
		}
		accountIDCopy := accountID
		notifications = append(notifications, SystemNotification{
			Type:      "account_error",
			Severity:  "critical",
			Title:     "Account health check failed",
			Message:   fmt.Sprintf("Account %q is in error state: %s", label, lastError),
			AccountID: &accountIDCopy,
			Label:     label,
			LastError: lastError,
		})
	}
	if err := accountRows.Err(); err != nil {
		return nil, err
	}

	serviceRows, err := s.db.Query(
		`SELECT id, label, last_error
		 FROM cpa_services
		 WHERE status = 'error'
		   AND enabled = 1
		 ORDER BY last_checked_at DESC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer serviceRows.Close()

	for serviceRows.Next() {
		var (
			serviceID int64
			label     string
			lastError string
		)
		if err := serviceRows.Scan(&serviceID, &label, &lastError); err != nil {
			return nil, err
		}
		if lastError == "" {
			lastError = "unknown error"
		}
		serviceIDCopy := serviceID
		notifications = append(notifications, SystemNotification{
			Type:      "cpa_service_error",
			Severity:  "critical",
			Title:     "CPA runtime unhealthy",
			Message:   fmt.Sprintf("CPA runtime %q is unhealthy: %s", label, lastError),
			ServiceID: &serviceIDCopy,
			Label:     label,
			LastError: lastError,
		})
	}
	if err := serviceRows.Err(); err != nil {
		return nil, err
	}

	return notifications, nil
}

func parseCpaExpiry(raw string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.UTC(), nil
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return ts.UTC(), nil
}

func (s *Store) GetDataRetentionSummary(retentionDays int) (*DataRetentionSummary, error) {
	var summary DataRetentionSummary
	summary.RetentionDays = retentionDays

	var oldest, newest sql.NullString
	if err := s.db.QueryRow(
		`SELECT COUNT(*), MIN(created_at), MAX(created_at) FROM request_logs`,
	).Scan(&summary.TotalLogs, &oldest, &newest); err != nil {
		return nil, err
	}
	if oldest.Valid {
		summary.OldestLogAt = &oldest.String
	}
	if newest.Valid {
		summary.NewestLogAt = &newest.String
	}

	var logsSize sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT COALESCE(SUM(
			LENGTH(COALESCE(request_id, '')) +
			LENGTH(COALESCE(access_token_name, '')) +
			LENGTH(COALESCE(model_requested, '')) +
			LENGTH(COALESCE(model_actual, '')) +
			LENGTH(COALESCE(request_ip, '')) +
			LENGTH(COALESCE(error_message, '')) +
			LENGTH(COALESCE(source_kind, '')) +
			80
		), 0) FROM request_logs`,
	).Scan(&logsSize); err != nil {
		return nil, err
	}
	if logsSize.Valid {
		summary.LogsSizeBytes = logsSize.Int64
	}

	var delivOldest, delivNewest sql.NullString
	if err := s.db.QueryRow(
		`SELECT COUNT(*), MIN(created_at), MAX(created_at) FROM notification_deliveries`,
	).Scan(&summary.TotalNotificationDeliveries, &delivOldest, &delivNewest); err != nil {
		return nil, err
	}
	if delivOldest.Valid {
		summary.NotificationDeliveriesOldestAt = &delivOldest.String
	}
	if delivNewest.Valid {
		summary.NotificationDeliveriesNewestAt = &delivNewest.String
	}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&summary.TotalNotificationOutbox); err != nil {
		return nil, err
	}
	outboxRows, err := s.db.Query(`SELECT status, COUNT(*) FROM notification_outbox GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer outboxRows.Close()
	for outboxRows.Next() {
		var status string
		var count int64
		if err := outboxRows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch status {
		case "pending", "retrying":
			summary.OutboxPendingCount += count
		case "dropped":
			summary.OutboxDroppedCount += count
		}
	}
	if err := outboxRows.Err(); err != nil {
		return nil, err
	}

	// Database file size via PRAGMA; two round trips because SQLite's
	// QueryRow on PRAGMA is finicky and returning a single int is simplest.
	var pageCount, pageSize int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		return nil, err
	}
	summary.DatabaseSizeBytes = pageCount * pageSize

	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	if v := settings["last_prune_at"]; v != "" {
		val := v
		summary.LastPruneAt = &val
	}
	summary.LastPruneDeletedLogs = parseInt64(settings["last_prune_deleted_logs"])
	summary.LastPruneDeletedDeliveries = parseInt64(settings["last_prune_deleted_deliveries"])
	summary.LastPruneDeletedOutbox = parseInt64(settings["last_prune_deleted_outbox"])

	return &summary, nil
}

// GetDataRetentionPreview reports the number of rows (and approximate bytes
// for request_logs) that a prune at the current retention window would
// delete. It never mutates data. When retentionDays <= 0 the preview
// returns zeros because auto-prune is disabled in that mode.
func (s *Store) GetDataRetentionPreview(retentionDays int) (*DataRetentionPreview, error) {
	preview := &DataRetentionPreview{RetentionDays: retentionDays}
	if retentionDays <= 0 {
		return preview, nil
	}

	safetyDays := retentionDays
	if safetyDays < 7 {
		safetyDays = 7
	}
	preview.OutboxSafetyDays = safetyDays

	deliveryCutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02 15:04:05")
	outboxCutoff := time.Now().UTC().AddDate(0, 0, -safetyDays).Format("2006-01-02 15:04:05")

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM request_logs WHERE created_at < ?`,
		deliveryCutoff,
	).Scan(&preview.LogsToDelete); err != nil {
		return nil, err
	}

	var size sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT COALESCE(SUM(
			LENGTH(COALESCE(request_id, '')) +
			LENGTH(COALESCE(access_token_name, '')) +
			LENGTH(COALESCE(model_requested, '')) +
			LENGTH(COALESCE(model_actual, '')) +
			LENGTH(COALESCE(request_ip, '')) +
			LENGTH(COALESCE(error_message, '')) +
			LENGTH(COALESCE(source_kind, '')) +
			80
		), 0) FROM request_logs WHERE created_at < ?`,
		deliveryCutoff,
	).Scan(&size); err != nil {
		return nil, err
	}
	if size.Valid {
		preview.LogsToDeleteSizeBytes = size.Int64
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM notification_deliveries WHERE created_at < ?`,
		deliveryCutoff,
	).Scan(&preview.DeliveriesToDelete); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM notification_outbox WHERE status = 'dropped' AND created_at < ?`,
		outboxCutoff,
	).Scan(&preview.OutboxToDelete); err != nil {
		return nil, err
	}

	return preview, nil
}

func parseInt64(raw string) int64 {
	if raw == "" {
		return 0
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n
	}
	return 0
}

func (s *Store) PruneRequestLogs(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02 15:04:05")
	res, err := s.db.Exec(`DELETE FROM request_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecordPruneRun persists the outcome of the most recent prune (auto or
// manual) into system_config. The fields are exposed back via
// GetDataRetentionSummary so the UI can show users "auto-prune is alive,
// last run X minutes ago, cleared Y rows" without keeping its own state.
func (s *Store) RecordPruneRun(deletedLogs, deletedDeliveries, deletedOutbox int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.UpdateSettings(map[string]string{
		"last_prune_at":                 now,
		"last_prune_deleted_logs":       strconv.FormatInt(deletedLogs, 10),
		"last_prune_deleted_deliveries": strconv.FormatInt(deletedDeliveries, 10),
		"last_prune_deleted_outbox":     strconv.FormatInt(deletedOutbox, 10),
	})
}
