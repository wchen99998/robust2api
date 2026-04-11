package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const resetSessionDriverName = "robust2api-test-reset-session"

var (
	registerResetSessionDriverOnce sync.Once
	resetSessionCallCount          atomic.Int64
)

func TestOpenInstrumentedDB_OmitsConnResetSessionSpans(t *testing.T) {
	registerResetSessionDriverOnce.Do(func() {
		sql.Register(resetSessionDriverName, resetSessionDriver{})
	})
	resetSessionCallCount.Store(0)

	exporter := tracetest.NewInMemoryExporter()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(traceProvider)
	defer func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
		otel.SetTracerProvider(previousProvider)
	}()

	db, err := openInstrumentedDB(resetSessionDriverName, "ignored")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	var value int64
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT 1").Scan(&value))
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT 1").Scan(&value))
	require.Equal(t, int64(1), value)
	require.Greater(t, resetSessionCallCount.Load(), int64(0), "test driver must exercise database/sql ResetSession")

	spanNames := make([]string, 0, len(exporter.GetSpans()))
	for _, span := range exporter.GetSpans() {
		spanNames = append(spanNames, span.Name)
		require.NotEqual(t, "sql.conn.reset_session", span.Name)
	}
	require.True(t, containsSpanWithSubstring(spanNames, "query"), "expected query spans, got %v", spanNames)
}

type resetSessionDriver struct{}

func (resetSessionDriver) Open(name string) (driver.Conn, error) {
	return &resetSessionConn{}, nil
}

type resetSessionConn struct{}

func (c *resetSessionConn) Prepare(query string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (c *resetSessionConn) Close() error {
	return nil
}

func (c *resetSessionConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (c *resetSessionConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &resetSessionRows{}, nil
}

func (c *resetSessionConn) ResetSession(context.Context) error {
	resetSessionCallCount.Add(1)
	return nil
}

func (c *resetSessionConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}

type resetSessionRows struct {
	returned bool
}

func (r *resetSessionRows) Columns() []string {
	return []string{"value"}
}

func (r *resetSessionRows) Close() error {
	return nil
}

func (r *resetSessionRows) Next(dest []driver.Value) error {
	if r.returned {
		return io.EOF
	}
	dest[0] = int64(1)
	r.returned = true
	return nil
}

func containsSpanWithSubstring(spanNames []string, substring string) bool {
	for _, name := range spanNames {
		if strings.Contains(name, substring) {
			return true
		}
	}
	return false
}
