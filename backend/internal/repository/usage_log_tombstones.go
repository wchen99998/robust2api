package repository

import (
	"context"
	"fmt"
	"strings"
)

const (
	usageLogDeleteReasonCleanupTask = "cleanup_task"
	usageLogDeleteReasonRetention   = "retention_cleanup"
	usageLogDeleteReasonManual      = "manual_delete"
	// Legacy rows can have NULL/empty request_id, but tombstones still need a stable PK.
	usageLogTombstoneSyntheticRequestIDPrefix = "usage-log-id:"
)

func usageLogTombstoneSyntheticRequestID(id int64) string {
	return fmt.Sprintf("%s%d", usageLogTombstoneSyntheticRequestIDPrefix, id)
}

func usageLogTombstoneRequestIDExpr(requestIDExpr, idExpr string) string {
	return fmt.Sprintf("COALESCE(NULLIF(%s, ''), '%s' || %s::text)", requestIDExpr, usageLogTombstoneSyntheticRequestIDPrefix, idExpr)
}

func deleteUsageLogsWithTombstones(ctx context.Context, q sqlQueryer, whereClause string, args []any, limit int, deleteReason string, sourceTaskID *int64) (int64, error) {
	whereClause = strings.TrimSpace(whereClause)
	if whereClause == "" {
		return 0, fmt.Errorf("usage log delete where clause is required")
	}
	if limit <= 0 {
		return 0, fmt.Errorf("usage log delete limit must be positive")
	}

	argLimitPos := len(args) + 1
	argReasonPos := len(args) + 2
	argSourceTaskPos := len(args) + 3
	deleteArgs := append(append(append([]any{}, args...), limit), deleteReason, sourceTaskID)
	tombstoneRequestIDExpr := usageLogTombstoneRequestIDExpr("request_id", "id")

	query := fmt.Sprintf(`
		WITH target AS (
			SELECT
				id,
				%s AS tombstone_request_id,
				api_key_id,
				user_id,
				account_id,
				created_at
			FROM usage_logs
			WHERE %s
			ORDER BY created_at ASC, id ASC
			LIMIT $%d
		), tombstoned AS (
			INSERT INTO usage_log_tombstones (
				request_id,
				api_key_id,
				usage_log_id,
				user_id,
				account_id,
				original_created_at,
				delete_reason,
				source_task_id
			)
			SELECT
				tombstone_request_id,
				api_key_id,
				id,
				user_id,
				account_id,
				created_at,
				$%d,
				$%d
			FROM target
			ON CONFLICT (request_id, api_key_id) DO NOTHING
		), deleted AS (
			DELETE FROM usage_logs
			WHERE id IN (SELECT id FROM target)
			RETURNING id
		)
		SELECT COUNT(*) FROM deleted
	`, tombstoneRequestIDExpr, whereClause, argLimitPos, argReasonPos, argSourceTaskPos)

	var deleted int64
	if err := scanSingleRow(ctx, q, query, deleteArgs, &deleted); err != nil {
		return 0, err
	}
	return deleted, nil
}

func deleteUsageLogByIDWithTombstone(ctx context.Context, q sqlQueryer, id int64, deleteReason string) (bool, error) {
	deleted, err := deleteUsageLogsWithTombstones(ctx, q, "id = $1", []any{id}, 1, deleteReason, nil)
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}
