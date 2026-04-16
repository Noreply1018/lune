package store

import (
	"database/sql"
	"fmt"
	"time"

	"lune/internal/syscfg"
)

func (s *Store) ListSystemNotifications() ([]SystemNotification, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	notifications := make([]SystemNotification, 0, 8)

	if syscfg.ParseBool(settings["notification_expiring_enabled"], syscfg.DefaultNotificationExpiringEnabled) {
		expiringDays := syscfg.ParsePositiveInt(settings["notification_expiring_days"], syscfg.DefaultNotificationExpiringDays)
		rows, err := s.db.Query(
			`SELECT id, label, cpa_expired_at
			 FROM accounts
			 WHERE source_kind = 'cpa'
			   AND cpa_expired_at != ''
			 ORDER BY cpa_expired_at ASC, id ASC`,
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
				ExpiresAt: expiresAt,
			})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	if syscfg.ParseBool(settings["notification_error_enabled"], syscfg.DefaultNotificationErrorEnabled) {
		accountRows, err := s.db.Query(
			`SELECT id, label, last_error
			 FROM accounts
			 WHERE status = 'error'
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
			})
		}
		if err := accountRows.Err(); err != nil {
			return nil, err
		}

		serviceRows, err := s.db.Query(
			`SELECT id, label, last_error
			 FROM cpa_services
			 WHERE status = 'error'
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
				Title:     "CPA service unhealthy",
				Message:   fmt.Sprintf("CPA service %q is unhealthy: %s", label, lastError),
				ServiceID: &serviceIDCopy,
			})
		}
		if err := serviceRows.Err(); err != nil {
			return nil, err
		}
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
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_deliveries`).Scan(&summary.TotalNotificationDeliveries); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&summary.TotalNotificationOutbox); err != nil {
		return nil, err
	}
	return &summary, nil
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
