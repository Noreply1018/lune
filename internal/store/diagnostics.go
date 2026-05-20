package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func AccountKeyHash(accountKey string) string {
	if strings.TrimSpace(accountKey) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(accountKey))
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

func (s *Store) EnsureAccountDiagnostic(accountID int64) error {
	account, err := s.GetAccount(accountID)
	if err != nil {
		return err
	}
	if account == nil {
		return nil
	}
	_, err = s.db.Exec(
		`INSERT OR IGNORE INTO account_diagnostics (
			account_id, account_key_hash, provider, stable_diagnostic_status, last_probe_status,
			scheduler_status, safe_summary
		) VALUES (?, ?, ?, 'unknown', 'not_run', ?, 'no diagnostic has run yet')`,
		account.ID, AccountKeyHash(account.CpaAccountKey), firstNonEmpty(account.CpaProvider, account.Provider), initialSchedulerStatusForAccount(account),
	)
	return err
}

func ensureAccountDiagnosticTx(tx *sql.Tx, account *Account) error {
	if account == nil {
		return nil
	}
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO account_diagnostics (
			account_id, account_key_hash, provider, stable_diagnostic_status, last_probe_status,
			scheduler_status, safe_summary
		) VALUES (?, ?, ?, 'unknown', 'not_run', ?, 'no diagnostic has run yet')`,
		account.ID, AccountKeyHash(account.CpaAccountKey), firstNonEmpty(account.CpaProvider, account.Provider), initialSchedulerStatusForAccount(account),
	)
	return err
}

func (s *Store) GetAccountDiagnostic(accountID int64) (*AccountDiagnostic, error) {
	row := s.db.QueryRow(
		`SELECT id, account_id, account_key_hash, provider, operation_id, started_at, finished_at,
			stable_diagnostic_status, previous_stable_diagnostic_status, last_probe_status, scheduler_status,
			scheduler_override, safe_summary, created_at, updated_at
		 FROM account_diagnostics WHERE account_id = ?`,
		accountID,
	)
	diag, err := scanAccountDiagnosticRow(row)
	if err == sql.ErrNoRows {
		if err := s.EnsureAccountDiagnostic(accountID); err != nil {
			return nil, err
		}
		row = s.db.QueryRow(
			`SELECT id, account_id, account_key_hash, provider, operation_id, started_at, finished_at,
				stable_diagnostic_status, previous_stable_diagnostic_status, last_probe_status, scheduler_status,
				scheduler_override, safe_summary, created_at, updated_at
			 FROM account_diagnostics WHERE account_id = ?`,
			accountID,
		)
		diag, err = scanAccountDiagnosticRow(row)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	evidence, err := s.ListAccountDiagnosticEvidence(diag.ID)
	if err != nil {
		return nil, err
	}
	diag.Evidence = evidence
	return diag, nil
}

func (s *Store) ListAccountDiagnostics() (map[int64]*AccountDiagnostic, error) {
	rows, err := s.db.Query(
		`SELECT id, account_id, account_key_hash, provider, operation_id, started_at, finished_at,
			stable_diagnostic_status, previous_stable_diagnostic_status, last_probe_status, scheduler_status,
			scheduler_override, safe_summary, created_at, updated_at
		 FROM account_diagnostics ORDER BY account_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]*AccountDiagnostic)
	for rows.Next() {
		diag, err := scanAccountDiagnosticRows(rows)
		if err != nil {
			return nil, err
		}
		out[diag.AccountID] = &diag
	}
	return out, rows.Err()
}

func (s *Store) ListAccountDiagnosticEvidence(diagnosticID int64) ([]AccountDiagnosticEvidence, error) {
	rows, err := s.db.Query(
		`SELECT id, diagnostic_id, probe_type, stage, http_status, upstream_error_code,
			normalized_error_code, safe_message, observed_at, request_log_id
		 FROM account_diagnostic_evidence
		 WHERE diagnostic_id = ?
		 ORDER BY datetime(observed_at) ASC, id ASC`,
		diagnosticID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evidence []AccountDiagnosticEvidence
	for rows.Next() {
		var item AccountDiagnosticEvidence
		var httpStatus sql.NullInt64
		var requestLogID sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.DiagnosticID, &item.ProbeType, &item.Stage, &httpStatus, &item.UpstreamErrorCode,
			&item.NormalizedErrorCode, &item.SafeMessage, &item.ObservedAt, &requestLogID,
		); err != nil {
			return nil, err
		}
		if httpStatus.Valid {
			v := int(httpStatus.Int64)
			item.HTTPStatus = &v
		}
		if requestLogID.Valid {
			v := requestLogID.Int64
			item.RequestLogID = &v
		}
		evidence = append(evidence, item)
	}
	return evidence, rows.Err()
}

func attachAccountDiagnostic(account *Account, diag *AccountDiagnostic) {
	if account == nil || diag == nil {
		return
	}
	account.Diagnostic = diag
	account.DiagnosticStatus = diag.StableDiagnosticStatus
	account.SchedulerStatus = diag.SchedulerStatus
	account.LastDiagnosedAt = diag.FinishedAt
	account.DiagnosticSummary = diag.SafeSummary
}

func AttachAccountDiagnostic(account *Account, diag *AccountDiagnostic) {
	attachAccountDiagnostic(account, diag)
}

type AccountDiagnosticUpdate struct {
	OperationID                    string
	StableDiagnosticStatus         string
	PreviousStableDiagnosticStatus string
	LastProbeStatus                string
	SchedulerStatus                string
	SchedulerOverride              string
	SafeSummary                    string
	PreserveStableStatus           bool
}

type AccountDiagnosticEvidenceInput struct {
	ProbeType           string
	Stage               string
	HTTPStatus          *int
	UpstreamErrorCode   string
	NormalizedErrorCode string
	SafeMessage         string
	RequestLogID        *int64
	ObservedAt          string
}

func (s *Store) UpdateAccountDiagnostic(accountID int64, update AccountDiagnosticUpdate) error {
	update.StableDiagnosticStatus = strings.ToLower(strings.TrimSpace(update.StableDiagnosticStatus))
	update.LastProbeStatus = strings.TrimSpace(update.LastProbeStatus)
	update.SchedulerStatus = strings.ToLower(strings.TrimSpace(update.SchedulerStatus))
	update.SchedulerOverride = strings.TrimSpace(update.SchedulerOverride)
	update.SafeSummary = strings.TrimSpace(update.SafeSummary)
	if err := ValidateDiagnosticStatus(update.StableDiagnosticStatus); err != nil {
		return err
	}
	if update.LastProbeStatus == "" {
		update.LastProbeStatus = "not_run"
	}
	if update.SchedulerOverride == "" {
		update.SchedulerStatus = SchedulerStatusForStableDiagnostic(update.StableDiagnosticStatus)
	} else if err := ValidateSchedulerStatus(update.SchedulerStatus); err != nil {
		return err
	}
	if update.SafeSummary == "" {
		update.SafeSummary = "diagnostic status updated"
	}
	if err := s.EnsureAccountDiagnostic(accountID); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE account_diagnostics
		 SET operation_id=?,
		     started_at=COALESCE(NULLIF(started_at, ''), datetime('now')),
		     finished_at=datetime('now'),
		     stable_diagnostic_status=?,
		     previous_stable_diagnostic_status=?,
		     last_probe_status=?,
		     scheduler_status=?,
		     scheduler_override=?,
		     safe_summary=?,
		     updated_at=datetime('now')
		 WHERE account_id=?`,
		update.OperationID, update.StableDiagnosticStatus, update.PreviousStableDiagnosticStatus,
		update.LastProbeStatus, update.SchedulerStatus, update.SchedulerOverride, update.SafeSummary, accountID,
	)
	return err
}

func (s *Store) UpdateAccountDiagnosticWithEvidence(accountID int64, update AccountDiagnosticUpdate, evidence AccountDiagnosticEvidenceInput) error {
	update.StableDiagnosticStatus = strings.ToLower(strings.TrimSpace(update.StableDiagnosticStatus))
	update.LastProbeStatus = strings.TrimSpace(update.LastProbeStatus)
	update.SchedulerStatus = strings.ToLower(strings.TrimSpace(update.SchedulerStatus))
	update.SchedulerOverride = strings.TrimSpace(update.SchedulerOverride)
	update.SafeSummary = strings.TrimSpace(update.SafeSummary)
	if err := ValidateDiagnosticStatus(update.StableDiagnosticStatus); err != nil {
		return err
	}
	if update.LastProbeStatus == "" {
		update.LastProbeStatus = "not_run"
	}
	if update.SchedulerOverride == "" {
		update.SchedulerStatus = SchedulerStatusForStableDiagnostic(update.StableDiagnosticStatus)
	} else if err := ValidateSchedulerStatus(update.SchedulerStatus); err != nil {
		return err
	}
	if update.SafeSummary == "" {
		update.SafeSummary = "diagnostic status updated"
	}
	if strings.TrimSpace(evidence.ProbeType) == "" {
		evidence.ProbeType = "routing_observation"
	}
	if strings.TrimSpace(evidence.ObservedAt) == "" {
		evidence.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.EnsureAccountDiagnostic(accountID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lastErr = s.updateAccountDiagnosticWithEvidenceTx(accountID, update, evidence)
		if lastErr == nil {
			return nil
		}
		if !isSQLiteBusyError(lastErr) {
			return lastErr
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return lastErr
}

func (s *Store) updateAccountDiagnosticWithEvidenceTx(accountID int64, update AccountDiagnosticUpdate, evidence AccountDiagnosticEvidenceInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var diagnosticID int64
	var previousStable, currentSchedulerStatus, currentSchedulerOverride string
	if err := tx.QueryRow(
		`SELECT id, stable_diagnostic_status, scheduler_status, scheduler_override FROM account_diagnostics WHERE account_id = ?`,
		accountID,
	).Scan(&diagnosticID, &previousStable, &currentSchedulerStatus, &currentSchedulerOverride); err != nil {
		return err
	}
	if update.PreserveStableStatus {
		update.StableDiagnosticStatus = previousStable
		update.SchedulerStatus = currentSchedulerStatus
		update.SchedulerOverride = currentSchedulerOverride
		if strings.TrimSpace(update.SchedulerStatus) == "" {
			update.SchedulerStatus = SchedulerStatusForStableDiagnostic(previousStable)
		}
	}
	if strings.TrimSpace(update.PreviousStableDiagnosticStatus) == "" {
		update.PreviousStableDiagnosticStatus = previousStable
	}
	_, err = tx.Exec(
		`UPDATE account_diagnostics
		 SET operation_id=?,
		     started_at=COALESCE(NULLIF(started_at, ''), datetime('now')),
		     finished_at=datetime('now'),
		     stable_diagnostic_status=?,
		     previous_stable_diagnostic_status=?,
		     last_probe_status=?,
		     scheduler_status=?,
		     scheduler_override=?,
		     safe_summary=?,
		     updated_at=datetime('now')
		 WHERE account_id=?`,
		update.OperationID, update.StableDiagnosticStatus, update.PreviousStableDiagnosticStatus,
		update.LastProbeStatus, update.SchedulerStatus, update.SchedulerOverride, update.SafeSummary, accountID,
	)
	if err != nil {
		return err
	}
	var httpStatus any
	if evidence.HTTPStatus != nil {
		httpStatus = *evidence.HTTPStatus
	}
	var requestLogID any
	if evidence.RequestLogID != nil {
		requestLogID = *evidence.RequestLogID
	}
	_, err = tx.Exec(
		`INSERT INTO account_diagnostic_evidence (
			diagnostic_id, probe_type, stage, http_status, upstream_error_code,
			normalized_error_code, safe_message, observed_at, request_log_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		diagnosticID, strings.TrimSpace(evidence.ProbeType), strings.TrimSpace(evidence.Stage), httpStatus,
		strings.TrimSpace(evidence.UpstreamErrorCode), strings.TrimSpace(evidence.NormalizedErrorCode),
		strings.TrimSpace(evidence.SafeMessage), strings.TrimSpace(evidence.ObservedAt), requestLogID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	code := sqliteErrorCode(err)
	return code == "sqlite_busy" || code == "sqlite_locked"
}

func SchedulerStatusForStableDiagnostic(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "usable":
		return "eligible"
	case "quota_probe_auth_failed_but_usable":
		return "eligible_with_warning"
	case "banned", "quota_exhausted", "auth_invalid":
		return "ineligible"
	default:
		return "eligible_with_warning"
	}
}

func ValidateSchedulerStatus(status string) error {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "eligible", "eligible_with_warning", "ineligible":
		return nil
	default:
		return fmt.Errorf("invalid scheduler status: %s", status)
	}
}

func scanAccountDiagnosticRows(rows *sql.Rows) (AccountDiagnostic, error) {
	diag, err := scanAccountDiagnosticRow(rows)
	if err != nil {
		return AccountDiagnostic{}, err
	}
	return *diag, nil
}

func scanAccountDiagnosticRow(row rowScanner) (*AccountDiagnostic, error) {
	var diag AccountDiagnostic
	if err := row.Scan(
		&diag.ID, &diag.AccountID, &diag.AccountKeyHash, &diag.Provider, &diag.OperationID, &diag.StartedAt, &diag.FinishedAt,
		&diag.StableDiagnosticStatus, &diag.PreviousStableDiagnosticStatus, &diag.LastProbeStatus, &diag.SchedulerStatus,
		&diag.SchedulerOverride, &diag.SafeSummary, &diag.CreatedAt, &diag.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &diag, nil
}

func initialSchedulerStatusForAccount(account *Account) string {
	if account == nil {
		return "eligible_with_warning"
	}
	if !account.Enabled || strings.EqualFold(account.Status, "disabled") {
		return "ineligible"
	}
	if strings.EqualFold(account.Status, "error") {
		return "eligible_with_warning"
	}
	return "eligible_with_warning"
}

func ValidateDiagnosticStatus(status string) error {
	switch status {
	case "usable", "banned", "quota_exhausted", "quota_probe_auth_failed_but_usable", "auth_invalid", "unknown":
		return nil
	default:
		return fmt.Errorf("invalid stable diagnostic status: %s", status)
	}
}
