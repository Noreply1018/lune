package store

import (
	"database/sql"
	"strings"
)

const maxOperationMessageBytes = 2048
const maxOperationItemsPerRecord = 1000

func (s *Store) RecordOperation(op *Operation) error {
	if op == nil || strings.TrimSpace(op.OperationID) == "" || strings.TrimSpace(op.OperationType) == "" {
		return sql.ErrNoRows
	}
	status := strings.TrimSpace(op.Status)
	if status == "" {
		status = "succeeded"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO operations (
			operation_id, operation_type, source, target_type, target_id, target_summary,
			status, error_code, safe_error_message, correlation_id, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), datetime('now')), COALESCE(NULLIF(?, ''), datetime('now')))
		ON CONFLICT(operation_id) DO UPDATE SET
			operation_type=excluded.operation_type,
			source=excluded.source,
			target_type=excluded.target_type,
			target_id=excluded.target_id,
			target_summary=excluded.target_summary,
			status=excluded.status,
			error_code=excluded.error_code,
			safe_error_message=excluded.safe_error_message,
			correlation_id=excluded.correlation_id,
			started_at=excluded.started_at,
			finished_at=excluded.finished_at`,
		op.OperationID, op.OperationType, op.Source, op.TargetType, op.TargetID, op.TargetSummary,
		status, op.ErrorCode, sanitizeOperationMessage(op.SafeErrorMessage), op.CorrelationID, op.StartedAt, op.FinishedAt,
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM operation_items WHERE operation_id = ?`, op.OperationID); err != nil {
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
		var accountID any
		if item.AccountID > 0 {
			accountID = item.AccountID
		}
		var poolMemberID any
		if item.PoolMemberID > 0 {
			poolMemberID = item.PoolMemberID
		}
		if _, err := tx.Exec(
			`INSERT INTO operation_items (
				operation_id, item_index, client_file_name, account_key_hash, action, status,
				runtime_sync, error_code, safe_error_message, account_id, pool_member_id, stage
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			op.OperationID, itemIndex, item.ClientFileName, item.AccountKeyHash, item.Action, item.Status,
			item.RuntimeSync, item.ErrorCode, sanitizeOperationMessage(item.SafeErrorMessage), accountID, poolMemberID, item.Stage,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRecentOperations(limit int) ([]Operation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, operation_id, operation_type, source, target_type, target_id, target_summary,
			status, error_code, safe_error_message, correlation_id, started_at, finished_at, created_at
		 FROM operations
		 ORDER BY datetime(created_at) DESC, id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []Operation
	for rows.Next() {
		op, err := scanOperationRows(rows)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *Store) GetOperation(operationID string) (*Operation, error) {
	row := s.db.QueryRow(
		`SELECT id, operation_id, operation_type, source, target_type, target_id, target_summary,
			status, error_code, safe_error_message, correlation_id, started_at, finished_at, created_at
		 FROM operations
		 WHERE operation_id = ?`,
		operationID,
	)
	op, err := scanOperationRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := s.ListOperationItems(operationID)
	if err != nil {
		return nil, err
	}
	op.Items = items
	return op, nil
}

func (s *Store) ListOperationItems(operationID string) ([]OperationItem, error) {
	rows, err := s.db.Query(
		`SELECT id, operation_id, item_index, client_file_name, account_key_hash, action, status,
			runtime_sync, error_code, safe_error_message, account_id, pool_member_id, stage, created_at
		 FROM operation_items
		 WHERE operation_id = ?
		 ORDER BY item_index, id`,
		operationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OperationItem
	for rows.Next() {
		item, err := scanOperationItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanOperationRow(row rowScanner) (*Operation, error) {
	var op Operation
	if err := row.Scan(
		&op.ID, &op.OperationID, &op.OperationType, &op.Source, &op.TargetType, &op.TargetID, &op.TargetSummary,
		&op.Status, &op.ErrorCode, &op.SafeErrorMessage, &op.CorrelationID, &op.StartedAt, &op.FinishedAt, &op.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &op, nil
}

func scanOperationRows(rows *sql.Rows) (Operation, error) {
	op, err := scanOperationRow(rows)
	if err != nil {
		return Operation{}, err
	}
	return *op, nil
}

func scanOperationItemRows(rows *sql.Rows) (OperationItem, error) {
	var item OperationItem
	var accountID, poolMemberID sql.NullInt64
	if err := rows.Scan(
		&item.ID, &item.OperationID, &item.ItemIndex, &item.ClientFileName, &item.AccountKeyHash, &item.Action, &item.Status,
		&item.RuntimeSync, &item.ErrorCode, &item.SafeErrorMessage, &accountID, &poolMemberID, &item.Stage, &item.CreatedAt,
	); err != nil {
		return item, err
	}
	if accountID.Valid {
		item.AccountID = accountID.Int64
	}
	if poolMemberID.Valid {
		item.PoolMemberID = poolMemberID.Int64
	}
	return item, nil
}

func sanitizeOperationMessage(message string) string {
	return truncateUTF8Bytes(sanitizeRequestLogError(message), maxOperationMessageBytes)
}
