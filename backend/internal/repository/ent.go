// Package repository 提供应用程序的基础设施层组件。
// 包括数据库连接初始化、ORM 客户端管理、Redis 连接、数据库迁移等核心功能。
package repository

import (
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/XSAM/otelsql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // PostgreSQL 驱动，通过副作用导入注册驱动
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitEnt initializes a read-only Ent ORM client (no migrations, no seeds).
//
// The caller must close the returned ent.Client when done.
func InitEnt(cfg *config.Config) (*ent.Client, *sql.DB, error) {
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, nil, err
	}

	dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

	// Wrap the SQL driver with OpenTelemetry instrumentation.
	// This produces child spans for every DB query (db.query, db.exec).
	db, err := openInstrumentedDB("postgres", dsn,
		otelsql.WithAttributes(attribute.String("db.pool.name", "main")),
	)
	if err != nil {
		return nil, nil, err
	}
	applyDBPoolSettings(db, cfg)
	if _, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithAttributes(
			attribute.String("db.pool.name", "main"),
		),
	); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("registering db stats metrics: %w", err)
	}
	drv := entsql.OpenDB(dialect.Postgres, db)

	client := ent.NewClient(ent.Driver(drv))
	return client, drv.DB(), nil
}

func openInstrumentedDB(driverName, dsn string, opts ...otelsql.Option) (*sql.DB, error) {
	baseOptions := []otelsql.Option{
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
		otelsql.WithSpanOptions(otelsql.SpanOptions{OmitConnResetSession: true}),
	}
	baseOptions = append(baseOptions, opts...)
	return otelsql.Open(driverName, dsn, baseOptions...)
}
