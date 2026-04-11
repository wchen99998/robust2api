package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/wchen99998/robust2api/internal/config"
	"github.com/wchen99998/robust2api/internal/pkg/timezone"
	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/attribute"
)

// BillingDB wraps a dedicated *sql.DB connection pool for billing write operations.
// It is a distinct type so Wire can distinguish it from the main *sql.DB pool.
type BillingDB struct {
	DB *sql.DB
}

// Close releases the underlying connection pool.
func (b *BillingDB) Close() error {
	if b == nil || b.DB == nil {
		return nil
	}
	return b.DB.Close()
}

// ProvideBillingDB creates a dedicated DB connection pool for billing writes.
// It connects to the same database as the main pool but with its own pool settings,
// so billing write transactions cannot starve hot-path reads for connections.
func ProvideBillingDB(cfg *config.Config) (*BillingDB, error) {
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, err
	}

	dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

	db, err := otelsql.Open("postgres", dsn,
		otelsql.WithAttributes(
			semconv.DBSystemPostgreSQL,
			attribute.String("db.pool.name", "billing"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("opening billing db pool: %w", err)
	}

	applyBillingDBPoolSettings(db, cfg)

	if _, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithAttributes(
			attribute.String("db.pool.name", "billing"),
		),
	); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("registering billing db stats metrics: %w", err)
	}

	return &BillingDB{DB: db}, nil
}

// applyBillingDBPoolSettings configures the billing pool.
// When billing-specific settings are 0, sensible defaults are derived from the main pool.
func applyBillingDBPoolSettings(db *sql.DB, cfg *config.Config) {
	bp := cfg.Billing.DBPool

	maxOpen := bp.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = cfg.Database.MaxOpenConns / 4
		if maxOpen < 8 {
			maxOpen = 8
		}
		if maxOpen > 64 {
			maxOpen = 64
		}
	}

	maxIdle := bp.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = maxOpen / 2
		if maxIdle < 4 {
			maxIdle = 4
		}
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}

	lifetime := time.Duration(bp.ConnMaxLifetimeMinutes) * time.Minute
	if bp.ConnMaxLifetimeMinutes <= 0 {
		lifetime = time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute
	}

	idleTime := time.Duration(bp.ConnMaxIdleTimeMinutes) * time.Minute
	if bp.ConnMaxIdleTimeMinutes <= 0 {
		idleTime = time.Duration(cfg.Database.ConnMaxIdleTimeMinutes) * time.Minute
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(lifetime)
	db.SetConnMaxIdleTime(idleTime)
}
