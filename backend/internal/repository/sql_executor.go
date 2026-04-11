package repository

import (
	"context"
	"database/sql"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

type contextSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type queryRowExecutor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type rowScanner interface {
	Scan(dest ...any) error
}

func sqlExecutorFromContext(ctx context.Context, fallback sqlExecutor) (sqlExecutor, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		driver, ok := tx.Client().Driver().(contextSQLExecutor)
		if !ok {
			return nil, fmt.Errorf("transaction driver does not expose sql executor")
		}
		return driver, nil
	}
	if fallback == nil {
		return nil, fmt.Errorf("nil sql executor")
	}
	return fallback, nil
}

func queryRowContext(ctx context.Context, exec sqlExecutor, query string, args ...any) rowScanner {
	if qre, ok := exec.(queryRowExecutor); ok {
		return qre.QueryRowContext(ctx, query, args...)
	}
	return &rowsRow{
		ctx:   ctx,
		exec:  exec,
		query: query,
		args:  args,
	}
}

type rowsRow struct {
	ctx   context.Context
	exec  sqlExecutor
	query string
	args  []any
}

func (r *rowsRow) Scan(dest ...any) error {
	rows, err := r.exec.QueryContext(r.ctx, r.query, r.args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}
