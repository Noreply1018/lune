package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type NotificationSubscription struct {
	Event       string `json:"event"`
	MinSeverity string `json:"min_severity,omitempty"`
}

type NotificationChannel struct {
	ID            int64                      `json:"id"`
	Name          string                     `json:"name"`
	Type          string                     `json:"type"`
	Enabled       bool                       `json:"enabled"`
	Config        json.RawMessage            `json:"config"`
	Subscriptions []NotificationSubscription `json:"subscriptions"`
	TitleTemplate string                     `json:"title_template"`
	BodyTemplate  string                     `json:"body_template"`
	CreatedAt     string                     `json:"created_at"`
	UpdatedAt     string                     `json:"updated_at"`
	LastDelivery  *NotificationDeliveryMeta  `json:"last_delivery,omitempty"`
}

type NotificationDeliveryMeta struct {
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	UpstreamCode *string `json:"upstream_code,omitempty"`
}

type NotificationDelivery struct {
	ID              int64  `json:"id"`
	ChannelID       int64  `json:"channel_id"`
	ChannelName     string `json:"channel_name"`
	ChannelType     string `json:"channel_type"`
	Event           string `json:"event"`
	Severity        string `json:"severity"`
	Title           string `json:"title"`
	PayloadSummary  string `json:"payload_summary"`
	Status          string `json:"status"`
	UpstreamCode    string `json:"upstream_code"`
	UpstreamMessage string `json:"upstream_message"`
	LatencyMS       int64  `json:"latency_ms"`
	Attempt         int    `json:"attempt"`
	DedupKey        string `json:"dedup_key"`
	TriggeredBy     string `json:"triggered_by"`
	CreatedAt       string `json:"created_at"`
}

type NotificationOutbox struct {
	ID            int64
	ChannelID     int64
	Event         string
	Severity      string
	Payload       string
	DedupKey      string
	Status        string
	Attempt       int
	NextAttemptAt string
	LastError     string
	CreatedAt     string
	UpdatedAt     string
}

func (s *Store) GetNotificationOutbox(id int64) (*NotificationOutbox, error) {
	row := s.db.QueryRow(
		`SELECT id, channel_id, event, severity, payload, dedup_key, status, attempt, next_attempt_at, last_error, created_at, updated_at
		 FROM notification_outbox
		 WHERE id = ?`,
		id,
	)
	var item NotificationOutbox
	if err := row.Scan(
		&item.ID, &item.ChannelID, &item.Event, &item.Severity, &item.Payload, &item.DedupKey,
		&item.Status, &item.Attempt, &item.NextAttemptAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

type NotificationDeliveryFilter struct {
	ChannelID   int64
	Event       string
	Status      string
	TriggeredBy string
	Limit       int
	Before      string
	BeforeID    int64
}

func (s *Store) ListNotificationChannels() ([]NotificationChannel, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.name, c.type, c.enabled, c.config, c.subscriptions, c.title_template, c.body_template, c.created_at, c.updated_at,
			d.status, d.created_at, d.upstream_code
		FROM notification_channels c
		LEFT JOIN notification_deliveries d ON d.id = (
			SELECT id FROM notification_deliveries nd
			WHERE nd.channel_id = c.id
			ORDER BY nd.created_at DESC, nd.id DESC
			LIMIT 1
		)
		ORDER BY c.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []NotificationChannel
	for rows.Next() {
		ch, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, *ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []NotificationChannel{}
	}
	return channels, nil
}

func (s *Store) ListEnabledNotificationChannels() ([]NotificationChannel, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, enabled, config, subscriptions, title_template, body_template, created_at, updated_at,
			NULL, NULL, NULL
		FROM notification_channels
		WHERE enabled = 1
		ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []NotificationChannel
	for rows.Next() {
		ch, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, *ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []NotificationChannel{}
	}
	return channels, nil
}

func (s *Store) GetNotificationChannel(id int64) (*NotificationChannel, error) {
	row := s.db.QueryRow(`
		SELECT c.id, c.name, c.type, c.enabled, c.config, c.subscriptions, c.title_template, c.body_template, c.created_at, c.updated_at,
			d.status, d.created_at, d.upstream_code
		FROM notification_channels c
		LEFT JOIN notification_deliveries d ON d.id = (
			SELECT id FROM notification_deliveries nd
			WHERE nd.channel_id = c.id
			ORDER BY nd.created_at DESC, nd.id DESC
			LIMIT 1
		)
		WHERE c.id = ?`, id)

	ch, err := scanNotificationChannel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (s *Store) CreateNotificationChannel(ch *NotificationChannel) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO notification_channels (name, type, enabled, config, subscriptions, title_template, body_template)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(ch.Name),
		ch.Type,
		boolToInt(ch.Enabled),
		rawJSONOrDefault(ch.Config, `{}`),
		marshalJSONOrDefault(ch.Subscriptions, `[]`),
		ch.TitleTemplate,
		ch.BodyTemplate,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateNotificationChannel(id int64, ch *NotificationChannel) error {
	_, err := s.db.Exec(
		`UPDATE notification_channels
		 SET name = ?, type = ?, enabled = ?, config = ?, subscriptions = ?, title_template = ?, body_template = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		strings.TrimSpace(ch.Name),
		ch.Type,
		boolToInt(ch.Enabled),
		rawJSONOrDefault(ch.Config, `{}`),
		marshalJSONOrDefault(ch.Subscriptions, `[]`),
		ch.TitleTemplate,
		ch.BodyTemplate,
		id,
	)
	return err
}

func (s *Store) DeleteNotificationChannel(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(tx)

	if _, err := tx.Exec(`DELETE FROM notification_outbox WHERE channel_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) SetNotificationChannelEnabled(id int64, enabled bool) error {
	res, err := s.db.Exec(
		`UPDATE notification_channels SET enabled = ?, updated_at = datetime('now') WHERE id = ?`,
		boolToInt(enabled),
		id,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) InsertNotificationOutbox(item *NotificationOutbox) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO notification_outbox (channel_id, event, severity, payload, dedup_key, status, attempt, next_attempt_at, last_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ChannelID,
		item.Event,
		item.Severity,
		item.Payload,
		item.DedupKey,
		item.Status,
		item.Attempt,
		nullOrString(item.NextAttemptAt, time.Now().UTC().Format("2006-01-02 15:04:05")),
		item.LastError,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) HasPendingNotificationOutbox(channelID int64, dedupKey string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM notification_outbox WHERE channel_id = ? AND dedup_key = ? AND status IN ('pending', 'retrying')`,
		channelID, dedupKey,
	).Scan(&count)
	return count > 0, err
}

func (s *Store) ListDueNotificationOutbox(limit int) ([]NotificationOutbox, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, channel_id, event, severity, payload, dedup_key, status, attempt, next_attempt_at, last_error, created_at, updated_at
		 FROM notification_outbox
		 WHERE status IN ('pending', 'retrying') AND next_attempt_at <= datetime('now')
		 ORDER BY next_attempt_at ASC, id ASC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NotificationOutbox
	for rows.Next() {
		var item NotificationOutbox
		if err := rows.Scan(
			&item.ID, &item.ChannelID, &item.Event, &item.Severity, &item.Payload, &item.DedupKey,
			&item.Status, &item.Attempt, &item.NextAttemptAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []NotificationOutbox{}
	}
	return items, nil
}

func (s *Store) ListNotificationChannelsByIDs(ids []int64) (map[int64]NotificationChannel, error) {
	if len(ids) == 0 {
		return map[int64]NotificationChannel{}, nil
	}
	seen := make(map[int64]struct{}, len(ids))
	args := make([]any, 0, len(ids))
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		args = append(args, id)
		placeholders = append(placeholders, "?")
	}
	if len(args) == 0 {
		return map[int64]NotificationChannel{}, nil
	}
	rows, err := s.db.Query(
		`SELECT id, name, type, enabled, config, subscriptions, title_template, body_template, created_at, updated_at,
			NULL, NULL, NULL
		FROM notification_channels
		WHERE id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]NotificationChannel, len(args))
	for rows.Next() {
		ch, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		out[ch.ID] = *ch
	}
	return out, rows.Err()
}

func (s *Store) UpdateNotificationOutboxRetry(id int64, attempt int, nextAttemptAt, lastError string) error {
	_, err := s.db.Exec(
		`UPDATE notification_outbox
		 SET status = 'retrying', attempt = ?, next_attempt_at = ?, last_error = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		attempt, nextAttemptAt, lastError, id,
	)
	return err
}

func (s *Store) MarkNotificationOutboxDropped(id int64, attempt int, lastError string) error {
	_, err := s.db.Exec(
		`UPDATE notification_outbox
		 SET status = 'dropped', attempt = ?, last_error = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		attempt, lastError, id,
	)
	return err
}

func (s *Store) DeleteNotificationOutbox(id int64) error {
	res, err := s.db.Exec(`DELETE FROM notification_outbox WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateNotificationDelivery(item *NotificationDelivery) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO notification_deliveries
		 (channel_id, channel_name, channel_type, event, severity, title, payload_summary, status, upstream_code, upstream_message, latency_ms, attempt, dedup_key, triggered_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ChannelID,
		item.ChannelName,
		item.ChannelType,
		item.Event,
		item.Severity,
		item.Title,
		item.PayloadSummary,
		item.Status,
		item.UpstreamCode,
		item.UpstreamMessage,
		item.LatencyMS,
		item.Attempt,
		item.DedupKey,
		item.TriggeredBy,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) RecordNotificationAttemptRetry(outboxID int64, delivery *NotificationDelivery, attempt int, nextAttemptAt, lastError string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(tx)

	if _, err := tx.Exec(
		`INSERT INTO notification_deliveries
		 (channel_id, channel_name, channel_type, event, severity, title, payload_summary, status, upstream_code, upstream_message, latency_ms, attempt, dedup_key, triggered_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ChannelID, delivery.ChannelName, delivery.ChannelType, delivery.Event, delivery.Severity,
		delivery.Title, delivery.PayloadSummary, delivery.Status, delivery.UpstreamCode, delivery.UpstreamMessage,
		delivery.LatencyMS, delivery.Attempt, delivery.DedupKey, delivery.TriggeredBy,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE notification_outbox
		 SET status = 'retrying', attempt = ?, next_attempt_at = ?, last_error = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		attempt, nextAttemptAt, lastError, outboxID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordNotificationAttemptDropped(outboxID int64, delivery *NotificationDelivery, attempt int, lastError string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(tx)

	if _, err := tx.Exec(
		`INSERT INTO notification_deliveries
		 (channel_id, channel_name, channel_type, event, severity, title, payload_summary, status, upstream_code, upstream_message, latency_ms, attempt, dedup_key, triggered_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ChannelID, delivery.ChannelName, delivery.ChannelType, delivery.Event, delivery.Severity,
		delivery.Title, delivery.PayloadSummary, delivery.Status, delivery.UpstreamCode, delivery.UpstreamMessage,
		delivery.LatencyMS, delivery.Attempt, delivery.DedupKey, delivery.TriggeredBy,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE notification_outbox
		 SET status = 'dropped', attempt = ?, last_error = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		attempt, lastError, outboxID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordNotificationAttemptSuccess(outboxID int64, delivery *NotificationDelivery) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(tx)

	if _, err := tx.Exec(
		`INSERT INTO notification_deliveries
		 (channel_id, channel_name, channel_type, event, severity, title, payload_summary, status, upstream_code, upstream_message, latency_ms, attempt, dedup_key, triggered_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ChannelID, delivery.ChannelName, delivery.ChannelType, delivery.Event, delivery.Severity,
		delivery.Title, delivery.PayloadSummary, delivery.Status, delivery.UpstreamCode, delivery.UpstreamMessage,
		delivery.LatencyMS, delivery.Attempt, delivery.DedupKey, delivery.TriggeredBy,
	); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM notification_outbox WHERE id = ?`, outboxID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) HasRecentNotificationDelivery(channelID int64, dedupKey string, since time.Time) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*)
		 FROM notification_deliveries
		 WHERE channel_id = ? AND dedup_key = ? AND status = 'success' AND created_at >= ?`,
		channelID,
		dedupKey,
		since.UTC().Format("2006-01-02 15:04:05"),
	).Scan(&count)
	return count > 0, err
}

func (s *Store) ListNotificationDeliveries(filter NotificationDeliveryFilter) ([]NotificationDelivery, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := `
		SELECT id, channel_id, channel_name, channel_type, event, severity, title, payload_summary, status, upstream_code, upstream_message, latency_ms, attempt, dedup_key, triggered_by, created_at
		FROM notification_deliveries
		WHERE 1 = 1`
	args := make([]any, 0, 4)
	if filter.ChannelID > 0 {
		query += ` AND channel_id = ?`
		args = append(args, filter.ChannelID)
	}
	if filter.Event != "" {
		query += ` AND event = ?`
		args = append(args, filter.Event)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.TriggeredBy != "" {
		query += ` AND triggered_by = ?`
		args = append(args, filter.TriggeredBy)
	}
	if filter.Before != "" {
		if err := ValidateNotificationDeliveryCursor(filter.Before); err != nil {
			return nil, err
		}
		if filter.BeforeID > 0 {
			query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
			args = append(args, filter.Before, filter.Before, filter.BeforeID)
		} else {
			query += ` AND created_at < ?`
			args = append(args, filter.Before)
		}
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []NotificationDelivery
	for rows.Next() {
		var item NotificationDelivery
		if err := rows.Scan(
			&item.ID, &item.ChannelID, &item.ChannelName, &item.ChannelType, &item.Event, &item.Severity,
			&item.Title, &item.PayloadSummary, &item.Status, &item.UpstreamCode, &item.UpstreamMessage,
			&item.LatencyMS, &item.Attempt, &item.DedupKey, &item.TriggeredBy, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if deliveries == nil {
		deliveries = []NotificationDelivery{}
	}
	return deliveries, nil
}

func (s *Store) PruneNotificationHistory(retentionDays int) (int64, int64, error) {
	if retentionDays <= 0 {
		return 0, 0, nil
	}
	safetyDays := retentionDays
	if safetyDays < 7 {
		safetyDays = 7
	}
	deliveryCutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02 15:04:05")
	outboxCutoff := time.Now().UTC().AddDate(0, 0, -safetyDays).Format("2006-01-02 15:04:05")

	res, err := s.db.Exec(`DELETE FROM notification_deliveries WHERE created_at < ?`, deliveryCutoff)
	if err != nil {
		return 0, 0, err
	}
	deletedDeliveries, _ := res.RowsAffected()

	res, err = s.db.Exec(`DELETE FROM notification_outbox WHERE status = 'dropped' AND created_at < ?`, outboxCutoff)
	if err != nil {
		return deletedDeliveries, 0, err
	}
	deletedOutbox, _ := res.RowsAffected()
	return deletedDeliveries, deletedOutbox, nil
}

func scanNotificationChannel(scanner interface {
	Scan(dest ...any) error
}) (*NotificationChannel, error) {
	var (
		ch               NotificationChannel
		enabled          int
		configRaw        string
		subscriptionsRaw string
		lastStatus       sql.NullString
		lastCreatedAt    sql.NullString
		lastUpstreamCode sql.NullString
	)

	if err := scanner.Scan(
		&ch.ID,
		&ch.Name,
		&ch.Type,
		&enabled,
		&configRaw,
		&subscriptionsRaw,
		&ch.TitleTemplate,
		&ch.BodyTemplate,
		&ch.CreatedAt,
		&ch.UpdatedAt,
		&lastStatus,
		&lastCreatedAt,
		&lastUpstreamCode,
	); err != nil {
		return nil, err
	}
	ch.Enabled = enabled != 0
	ch.Config = json.RawMessage(configRaw)
	if err := json.Unmarshal([]byte(subscriptionsRaw), &ch.Subscriptions); err != nil {
		return nil, fmt.Errorf("decode subscriptions: %w", err)
	}
	if lastStatus.Valid && lastCreatedAt.Valid {
		meta := &NotificationDeliveryMeta{
			Status:    lastStatus.String,
			CreatedAt: lastCreatedAt.String,
		}
		if lastUpstreamCode.Valid && strings.TrimSpace(lastUpstreamCode.String) != "" {
			meta.UpstreamCode = &lastUpstreamCode.String
		}
		ch.LastDelivery = meta
	}
	return &ch, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func rawJSONOrDefault(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	return string(raw)
}

func marshalJSONOrDefault(v any, fallback string) string {
	body, err := json.Marshal(v)
	if err != nil || len(body) == 0 || string(body) == "null" {
		return fallback
	}
	return string(body)
}

func ValidateNotificationDeliveryCursor(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return nil
	}
	return fmt.Errorf("invalid before cursor")
}

func rollbackNotificationTx(tx *sql.Tx) {
	_ = tx.Rollback()
}

func nullOrString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
