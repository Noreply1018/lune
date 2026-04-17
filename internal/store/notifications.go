package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// SingletonChannelID is the synthetic channel id retained for historical
// outbox/deliveries rows after the notification schema collapsed to a single
// WeChat-Work-Bot config.
const SingletonChannelID int64 = 1

// SingletonChannelName / SingletonChannelType are written verbatim into
// notification_deliveries so the Activity page continues to show a consistent
// label without needing a cross-table lookup.
const (
	SingletonChannelName = "wechat_work_bot"
	SingletonChannelType = "wechat_work_bot"
)

// NotificationSettings is the single row in notification_settings that drives
// the WeChat Work Bot integration.
type NotificationSettings struct {
	Enabled           bool     `json:"enabled"`
	WebhookURL        string   `json:"webhook_url"`
	Format            string   `json:"format"`
	MentionMobileList []string `json:"mention_mobile_list"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// NotificationSubscription captures whether a built-in event is delivered and
// the per-event title/body templates the admin edited.
type NotificationSubscription struct {
	Event         string `json:"event"`
	Subscribed    bool   `json:"subscribed"`
	TitleTemplate string `json:"title_template"`
	BodyTemplate  string `json:"body_template"`
	UpdatedAt     string `json:"updated_at"`
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

type NotificationDeliveryFilter struct {
	ChannelID   int64
	Event       string
	Status      string
	TriggeredBy string
	Limit       int
	Before      string
	BeforeID    int64
}

// GetNotificationSettings returns the singleton settings row. The migration
// seeds a default row at id=1, so this should always succeed once the store
// is open.
func (s *Store) GetNotificationSettings() (NotificationSettings, error) {
	var (
		settings    NotificationSettings
		enabled     int
		mentionRaw  string
	)
	err := s.db.QueryRow(
		`SELECT enabled, webhook_url, format, mention_mobile_list, created_at, updated_at
		 FROM notification_settings
		 WHERE id = ?`,
		SingletonChannelID,
	).Scan(
		&enabled,
		&settings.WebhookURL,
		&settings.Format,
		&mentionRaw,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)
	if err != nil {
		return NotificationSettings{}, err
	}
	settings.Enabled = enabled != 0
	settings.MentionMobileList = parseMentionMobileList(mentionRaw)
	if settings.Format == "" {
		settings.Format = "markdown"
	}
	return settings, nil
}

// UpdateNotificationSettings upserts the singleton settings row with the
// supplied values. Callers are expected to validate the fields (URL format,
// mobile list entries, etc.) before invoking.
func (s *Store) UpdateNotificationSettings(settings NotificationSettings) error {
	mentions := settings.MentionMobileList
	if mentions == nil {
		mentions = []string{}
	}
	mentionJSON, err := json.Marshal(mentions)
	if err != nil {
		return fmt.Errorf("marshal mention list: %w", err)
	}
	format := settings.Format
	if format == "" {
		format = "markdown"
	}
	_, err = s.db.Exec(
		`INSERT INTO notification_settings (id, enabled, webhook_url, format, mention_mobile_list, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		 ON CONFLICT(id) DO UPDATE SET
		    enabled             = excluded.enabled,
		    webhook_url         = excluded.webhook_url,
		    format              = excluded.format,
		    mention_mobile_list = excluded.mention_mobile_list,
		    updated_at          = datetime('now')`,
		SingletonChannelID,
		boolToInt(settings.Enabled),
		strings.TrimSpace(settings.WebhookURL),
		format,
		string(mentionJSON),
	)
	return err
}

// ListNotificationSubscriptions returns the fixed set of event subscriptions
// (seeded at migration). The order matches the built-in event list so the UI
// can rely on a stable ordering.
func (s *Store) ListNotificationSubscriptions() ([]NotificationSubscription, error) {
	rows, err := s.db.Query(
		`SELECT event, subscribed, title_template, body_template, updated_at
		 FROM notification_subscriptions
		 ORDER BY event ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []NotificationSubscription
	for rows.Next() {
		var (
			sub        NotificationSubscription
			subscribed int
		)
		if err := rows.Scan(&sub.Event, &subscribed, &sub.TitleTemplate, &sub.BodyTemplate, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		sub.Subscribed = subscribed != 0
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if subs == nil {
		subs = []NotificationSubscription{}
	}
	return subs, nil
}

// GetNotificationSubscription returns a single subscription by event name.
// Returns (nil, nil) when the event is not present.
func (s *Store) GetNotificationSubscription(event string) (*NotificationSubscription, error) {
	var (
		sub        NotificationSubscription
		subscribed int
	)
	err := s.db.QueryRow(
		`SELECT event, subscribed, title_template, body_template, updated_at
		 FROM notification_subscriptions
		 WHERE event = ?`,
		event,
	).Scan(&sub.Event, &subscribed, &sub.TitleTemplate, &sub.BodyTemplate, &sub.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sub.Subscribed = subscribed != 0
	return &sub, nil
}

// UpdateNotificationSubscription rewrites the subscription row for the given
// event. Callers must validate that title/body are non-empty after trimming
// before invoking.
func (s *Store) UpdateNotificationSubscription(event string, subscribed bool, title, body string) error {
	res, err := s.db.Exec(
		`UPDATE notification_subscriptions
		 SET subscribed     = ?,
		     title_template = ?,
		     body_template  = ?,
		     updated_at     = datetime('now')
		 WHERE event = ?`,
		boolToInt(subscribed),
		title,
		body,
		event,
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

func (s *Store) InsertNotificationOutbox(item *NotificationOutbox) (int64, error) {
	channelID := item.ChannelID
	if channelID == 0 {
		channelID = SingletonChannelID
	}
	res, err := s.db.Exec(
		`INSERT INTO notification_outbox (channel_id, event, severity, payload, dedup_key, status, attempt, next_attempt_at, last_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		channelID,
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

// HasPendingNotificationOutbox checks if an active (pending/retrying) entry
// with the given dedup key already exists. channelID is accepted for API
// continuity but is effectively always SingletonChannelID under the new
// schema.
func (s *Store) HasPendingNotificationOutbox(channelID int64, dedupKey string) (bool, error) {
	if channelID == 0 {
		channelID = SingletonChannelID
	}
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
		defaultChannelID(item.ChannelID),
		defaultString(item.ChannelName, SingletonChannelName),
		defaultString(item.ChannelType, SingletonChannelType),
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
		defaultChannelID(delivery.ChannelID),
		defaultString(delivery.ChannelName, SingletonChannelName),
		defaultString(delivery.ChannelType, SingletonChannelType),
		delivery.Event, delivery.Severity,
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
		defaultChannelID(delivery.ChannelID),
		defaultString(delivery.ChannelName, SingletonChannelName),
		defaultString(delivery.ChannelType, SingletonChannelType),
		delivery.Event, delivery.Severity,
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
		defaultChannelID(delivery.ChannelID),
		defaultString(delivery.ChannelName, SingletonChannelName),
		defaultString(delivery.ChannelType, SingletonChannelType),
		delivery.Event, delivery.Severity,
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
	if channelID == 0 {
		channelID = SingletonChannelID
	}
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

// ListRecentNotificationDeliveryMeta returns the N most-recent deliveries for
// the singleton channel. The API is kept for admin handlers that surface a
// quick "last delivery" chip on the settings page.
func (s *Store) ListRecentNotificationDeliveryMeta(limit int) ([]NotificationDeliveryMeta, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.Query(
		`SELECT status, created_at, upstream_code
		 FROM notification_deliveries
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]NotificationDeliveryMeta, 0, limit)
	for rows.Next() {
		var (
			item         NotificationDeliveryMeta
			upstreamCode sql.NullString
		)
		if err := rows.Scan(&item.Status, &item.CreatedAt, &upstreamCode); err != nil {
			return nil, err
		}
		if upstreamCode.Valid && strings.TrimSpace(upstreamCode.String) != "" {
			item.UpstreamCode = &upstreamCode.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
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

func truncateDeliverySummary(payload string, limit int) string {
	if limit <= 0 || len(payload) <= limit {
		return payload
	}
	// Walk back from the byte budget to the nearest rune boundary so we
	// never write a partial UTF-8 sequence into the deliveries table.
	cut := limit
	for cut > 0 && !utf8.RuneStart(payload[cut]) {
		cut--
	}
	return payload[:cut]
}

// TruncateDeliverySummary exposes the internal summary truncator so notify
// package callers can produce identical summaries.
func TruncateDeliverySummary(payload string, limit int) string {
	return truncateDeliverySummary(payload, limit)
}

func parseMentionMobileList(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func defaultChannelID(id int64) int64 {
	if id == 0 {
		return SingletonChannelID
	}
	return id
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
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
