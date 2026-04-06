// Package repository 提供应用程序的基础设施层组件。
// 包括数据库连接初始化、ORM 客户端管理、Redis 连接、数据库迁移等核心功能。
package repository

import (
	"database/sql"
	"fmt"

	"github.com/XSAM/otelsql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // PostgreSQL 驱动，通过副作用导入注册驱动
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
	db, err := otelsql.Open("postgres", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, nil, err
	}
	applyDBPoolSettings(db, cfg)
	if _, err := otelsql.RegisterDBStatsMetrics(db); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("registering db stats metrics: %w", err)
	}
	drv := entsql.OpenDB(dialect.Postgres, db)

	client := ent.NewClient(ent.Driver(drv))
	return client, drv.DB(), nil
}
