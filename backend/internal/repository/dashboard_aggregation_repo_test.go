package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDashboardAggregationRepositoryCleanupUsageLogsNonPartitioned(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT EXISTS\\(").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("WITH target AS").
		WithArgs(cutoff.UTC(), usageLogsCleanupBatchSize, usageLogDeleteReasonRetention, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	require.NoError(t, repo.CleanupUsageLogs(context.Background(), cutoff))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositoryCleanupUsageLogsPartitioned(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	cutoff := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT EXISTS\\(").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT c\\.relname").
		WillReturnRows(sqlmock.NewRows([]string{"relname"}).
			AddRow("usage_logs_202401").
			AddRow("usage_logs_202402").
			AddRow("usage_logs_202403"))
	mock.ExpectExec("INSERT INTO usage_log_tombstones").
		WithArgs(usageLogDeleteReasonRetention).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DROP TABLE IF EXISTS "usage_logs_202401"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO usage_log_tombstones").
		WithArgs(usageLogDeleteReasonRetention).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DROP TABLE IF EXISTS "usage_logs_202402"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("WITH target AS").
		WithArgs(cutoff.UTC(), usageLogsCleanupBatchSize, usageLogDeleteReasonRetention, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	require.NoError(t, repo.CleanupUsageLogs(context.Background(), cutoff))
	require.NoError(t, mock.ExpectationsWereMet())
}
