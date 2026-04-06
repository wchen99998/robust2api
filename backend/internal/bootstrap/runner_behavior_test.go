//go:build unit

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newBootstrapTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)

	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestPersistJWTSecretRejectsMismatch(t *testing.T) {
	client := newBootstrapTestClient(t)
	_, err := client.SecuritySecret.Create().
		SetKey("jwt_secret").
		SetValue("existing-jwt-secret-32bytes-long!!!!").
		Save(context.Background())
	require.NoError(t, err)

	err = persistJWTSecret(context.Background(), client, "another-configured-jwt-secret-32!!!!")
	require.Error(t, err)
	require.Contains(t, err.Error(), "jwt secret mismatch")
}

func TestPersistJWTSecretAcceptsMatchingStoredValue(t *testing.T) {
	client := newBootstrapTestClient(t)
	_, err := client.SecuritySecret.Create().
		SetKey("jwt_secret").
		SetValue("existing-jwt-secret-32bytes-long!!!!").
		Save(context.Background())
	require.NoError(t, err)

	err = persistJWTSecret(context.Background(), client, "existing-jwt-secret-32bytes-long!!!!")
	require.NoError(t, err)
}

func TestSeedAdmin_IdempotentOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT COUNT\\(1\\) FROM users").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO users").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = seedAdmin(context.Background(), db, BootstrapEnv{
		AdminEmail:    "admin@example.com",
		AdminPassword: "securepassword123",
		RunMode:       "standard",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
