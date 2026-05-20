package store

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
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

	credentialRows, err := s.db.Query(
		`SELECT id, label, cpa_credential_reason, cpa_credential_last_error
		 FROM accounts
		 WHERE source_kind = 'cpa'
		   AND cpa_credential_status = 'needs_login'
		   AND lower(cpa_quota_status) <> 'blocked'
		   AND enabled = 1
		 ORDER BY cpa_credential_checked_at DESC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer credentialRows.Close()

	for credentialRows.Next() {
		var (
			accountID int64
			label     string
			reason    string
			lastError string
		)
		if err := credentialRows.Scan(&accountID, &label, &reason, &lastError); err != nil {
			return nil, err
		}
		detail := credentialDetail(reason, lastError)
		accountIDCopy := accountID
		notifications = append(notifications, SystemNotification{
			Type:      "cpa_credential_error",
			Severity:  "critical",
			Title:     "CPA credential requires login",
			Message:   fmt.Sprintf("Account %q CPA credential requires login: %s", label, detail),
			AccountID: &accountIDCopy,
			Label:     label,
			LastError: detail,
		})
	}
	if err := credentialRows.Err(); err != nil {
		return nil, err
	}

	quotaRows, err := s.db.Query(
		`SELECT id, label, cpa_quota_last_error
		 FROM accounts
		 WHERE source_kind = 'cpa'
		   AND lower(cpa_provider) = 'codex'
		   AND lower(cpa_quota_status) = 'blocked'
		   AND enabled = 1
		 ORDER BY cpa_quota_checked_at DESC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer quotaRows.Close()

	for quotaRows.Next() {
		var accountID int64
		var label, lastError string
		if err := quotaRows.Scan(&accountID, &label, &lastError); err != nil {
			return nil, err
		}
		if lastError == "" {
			lastError = "quota blocked"
		}
		accountIDCopy := accountID
		notifications = append(notifications, SystemNotification{
			Type:      "cpa_quota_blocked",
			Severity:  "warning",
			Title:     "CPA quota blocked",
			Message:   fmt.Sprintf("Account %q CPA quota is blocked: %s", label, lastError),
			AccountID: &accountIDCopy,
			Label:     label,
			LastError: lastError,
		})
	}
	if err := quotaRows.Err(); err != nil {
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

func credentialDetail(reason, lastError string) string {
	if lastError != "" {
		return lastError
	}
	if reason != "" {
		return reason
	}
	return "needs login"
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
	if s.dbPath != "" {
		if info, err := os.Stat(s.dbPath + "-wal"); err == nil {
			summary.DatabaseWalSizeBytes = info.Size()
		}
		if info, err := os.Stat(s.dbPath + "-shm"); err == nil {
			summary.DatabaseShmSizeBytes = info.Size()
		}
	}

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
	summary.LastPruneDeletedOperations = parseInt64(settings["last_prune_deleted_operations"])
	summary.LastPruneDeletedOperationItems = parseInt64(settings["last_prune_deleted_operation_items"])

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
	if err := s.db.QueryRow(
		`WITH keep AS (
			SELECT operation_id
			FROM operations
			ORDER BY datetime(created_at) DESC, id DESC
			LIMIT 200
		)
		 SELECT COUNT(*)
		 FROM operations o
		 LEFT JOIN keep k ON k.operation_id = o.operation_id
		 WHERE k.operation_id IS NULL
		   AND o.created_at < ?`,
		deliveryCutoff,
	).Scan(&preview.OperationsToDelete); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(
		`WITH keep AS (
			SELECT operation_id
			FROM operations
			ORDER BY datetime(created_at) DESC, id DESC
			LIMIT 200
		)
		 SELECT COUNT(*)
		 FROM operation_items oi
		 JOIN operations o ON o.operation_id = oi.operation_id
		 LEFT JOIN keep k ON k.operation_id = oi.operation_id
		 WHERE k.operation_id IS NULL
		   AND o.created_at < ?`,
		deliveryCutoff,
	).Scan(&preview.OperationItemsToDelete); err != nil {
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

func (s *Store) PruneDataRetention(retentionDays int, source string) (*DataRetentionPruneResult, error) {
	result := &DataRetentionPruneResult{RetentionDays: retentionDays}
	if retentionDays <= 0 {
		return result, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02 15:04:05")
	safetyDays := retentionDays
	if safetyDays < 7 {
		safetyDays = 7
	}
	outboxCutoff := time.Now().UTC().AddDate(0, 0, -safetyDays).Format("2006-01-02 15:04:05")

	res, err := tx.Exec(`DELETE FROM request_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	result.DeletedLogs, err = res.RowsAffected()
	if err != nil {
		return nil, err
	}

	res, err = tx.Exec(`DELETE FROM notification_deliveries WHERE created_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	result.DeletedDeliveries, err = res.RowsAffected()
	if err != nil {
		return nil, err
	}

	res, err = tx.Exec(`DELETE FROM notification_outbox WHERE status = 'dropped' AND created_at < ?`, outboxCutoff)
	if err != nil {
		return nil, err
	}
	result.DeletedOutbox, err = res.RowsAffected()
	if err != nil {
		return nil, err
	}

	if err := tx.QueryRow(
		`WITH keep AS (
			SELECT operation_id
			FROM operations
			ORDER BY datetime(created_at) DESC, id DESC
			LIMIT 200
		)
		 SELECT COUNT(*)
		 FROM operation_items oi
		 JOIN operations o ON o.operation_id = oi.operation_id
		 LEFT JOIN keep k ON k.operation_id = oi.operation_id
		 WHERE k.operation_id IS NULL
		   AND o.created_at < ?`,
		cutoff,
	).Scan(&result.DeletedOperationItems); err != nil {
		return nil, err
	}
	res, err = tx.Exec(
		`DELETE FROM operations
		 WHERE operation_id IN (
			SELECT o.operation_id
			FROM operations o
			LEFT JOIN (
				SELECT operation_id
				FROM operations
				ORDER BY datetime(created_at) DESC, id DESC
				LIMIT 200
			) keep ON keep.operation_id = o.operation_id
			WHERE keep.operation_id IS NULL
			  AND o.created_at < ?
		 )`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	result.DeletedOperations, err = res.RowsAffected()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := recordPruneRunTx(tx, now, result); err != nil {
		return nil, err
	}
	if shouldRecordPruneOperation(result, source) {
		if err := recordDataRetentionPruneOperationTx(tx, result, source, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// RecordPruneRun persists the outcome of the most recent prune (auto or
// manual) into system_config. The fields are exposed back via
// GetDataRetentionSummary so the UI can show users "auto-prune is alive,
// last run X minutes ago, cleared Y rows" without keeping its own state.
func (s *Store) RecordPruneRun(result *DataRetentionPruneResult) error {
	if result == nil {
		result = &DataRetentionPruneResult{}
	}
	return s.UpdateSettings(pruneRunSettings(time.Now().UTC().Format(time.RFC3339), result))
}

func pruneRunSettings(now string, result *DataRetentionPruneResult) map[string]string {
	if result == nil {
		result = &DataRetentionPruneResult{}
	}
	return map[string]string{
		"last_prune_at":                      now,
		"last_prune_deleted_logs":            strconv.FormatInt(result.DeletedLogs, 10),
		"last_prune_deleted_deliveries":      strconv.FormatInt(result.DeletedDeliveries, 10),
		"last_prune_deleted_outbox":          strconv.FormatInt(result.DeletedOutbox, 10),
		"last_prune_deleted_operations":      strconv.FormatInt(result.DeletedOperations, 10),
		"last_prune_deleted_operation_items": strconv.FormatInt(result.DeletedOperationItems, 10),
	}
}

func recordPruneRunTx(tx *sql.Tx, now string, result *DataRetentionPruneResult) error {
	stmt, err := tx.Prepare(`INSERT INTO system_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range pruneRunSettings(now, result) {
		if _, err := stmt.Exec(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recordDataRetentionPruneOperation(result *DataRetentionPruneResult, source string) error {
	if result == nil {
		return nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "system"
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	return s.RecordOperation(&Operation{
		OperationID:   "op_retention_prune_" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		OperationType: "data_retention_prune",
		Source:        source,
		TargetType:    "data_retention",
		TargetID:      strconv.Itoa(result.RetentionDays),
		TargetSummary: fmt.Sprintf("retention_days=%d deleted_logs=%d deleted_operations=%d deleted_operation_items=%d deleted_deliveries=%d deleted_outbox=%d",
			result.RetentionDays, result.DeletedLogs, result.DeletedOperations, result.DeletedOperationItems, result.DeletedDeliveries, result.DeletedOutbox),
		Status:     "succeeded",
		StartedAt:  now,
		FinishedAt: now,
		Items: []OperationItem{
			{ItemIndex: 1, Action: "prune", Status: "succeeded", Stage: "request_logs", SafeErrorMessage: strconv.FormatInt(result.DeletedLogs, 10)},
			{ItemIndex: 2, Action: "prune", Status: "succeeded", Stage: "operations", SafeErrorMessage: strconv.FormatInt(result.DeletedOperations, 10)},
			{ItemIndex: 3, Action: "prune", Status: "succeeded", Stage: "operation_items", SafeErrorMessage: strconv.FormatInt(result.DeletedOperationItems, 10)},
			{ItemIndex: 4, Action: "prune", Status: "succeeded", Stage: "notification_deliveries", SafeErrorMessage: strconv.FormatInt(result.DeletedDeliveries, 10)},
			{ItemIndex: 5, Action: "prune", Status: "succeeded", Stage: "notification_outbox", SafeErrorMessage: strconv.FormatInt(result.DeletedOutbox, 10)},
		},
	})
}

func shouldRecordPruneOperation(result *DataRetentionPruneResult, source string) bool {
	if result == nil {
		return false
	}
	if strings.TrimSpace(source) != "health_checker" {
		return true
	}
	return result.DeletedLogs > 0 || result.DeletedDeliveries > 0 || result.DeletedOutbox > 0 ||
		result.DeletedOperations > 0 || result.DeletedOperationItems > 0
}

func recordDataRetentionPruneOperationTx(tx *sql.Tx, result *DataRetentionPruneResult, source string, nowRFC3339 string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "system"
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if parsed, err := time.Parse(time.RFC3339, nowRFC3339); err == nil {
		now = parsed.UTC().Format("2006-01-02 15:04:05")
	}
	op := Operation{
		OperationID:   "op_retention_prune_" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		OperationType: "data_retention_prune",
		Source:        source,
		TargetType:    "data_retention",
		TargetID:      strconv.Itoa(result.RetentionDays),
		TargetSummary: fmt.Sprintf("retention_days=%d deleted_logs=%d deleted_operations=%d deleted_operation_items=%d deleted_deliveries=%d deleted_outbox=%d",
			result.RetentionDays, result.DeletedLogs, result.DeletedOperations, result.DeletedOperationItems, result.DeletedDeliveries, result.DeletedOutbox),
		Status:     "succeeded",
		StartedAt:  now,
		FinishedAt: now,
		Items: []OperationItem{
			{ItemIndex: 1, Action: "prune", Status: "succeeded", Stage: "request_logs", SafeErrorMessage: strconv.FormatInt(result.DeletedLogs, 10)},
			{ItemIndex: 2, Action: "prune", Status: "succeeded", Stage: "operations", SafeErrorMessage: strconv.FormatInt(result.DeletedOperations, 10)},
			{ItemIndex: 3, Action: "prune", Status: "succeeded", Stage: "operation_items", SafeErrorMessage: strconv.FormatInt(result.DeletedOperationItems, 10)},
			{ItemIndex: 4, Action: "prune", Status: "succeeded", Stage: "notification_deliveries", SafeErrorMessage: strconv.FormatInt(result.DeletedDeliveries, 10)},
			{ItemIndex: 5, Action: "prune", Status: "succeeded", Stage: "notification_outbox", SafeErrorMessage: strconv.FormatInt(result.DeletedOutbox, 10)},
		},
	}
	if _, err := tx.Exec(
		`INSERT INTO operations (
			operation_id, operation_type, source, target_type, target_id, target_summary,
			status, error_code, safe_error_message, correlation_id, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.OperationID, op.OperationType, op.Source, op.TargetType, op.TargetID, op.TargetSummary,
		op.Status, op.ErrorCode, sanitizeOperationMessage(op.SafeErrorMessage), op.CorrelationID, op.StartedAt, op.FinishedAt,
	); err != nil {
		return err
	}
	items := op.Items
	if len(items) > maxOperationItemsPerRecord {
		items = items[:maxOperationItemsPerRecord]
	}
	for i, item := range items {
		itemIndex := item.ItemIndex
		if itemIndex == 0 {
			itemIndex = i + 1
		}
		if _, err := tx.Exec(
			`INSERT INTO operation_items (
				operation_id, item_index, client_file_name, account_key_hash, action, status,
				runtime_sync, error_code, safe_error_message, account_id, pool_member_id, stage
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			op.OperationID, itemIndex, item.ClientFileName, item.AccountKeyHash, item.Action, item.Status,
			item.RuntimeSync, item.ErrorCode, sanitizeOperationMessage(item.SafeErrorMessage), nil, nil, item.Stage,
		); err != nil {
			return err
		}
	}
	return nil
}
