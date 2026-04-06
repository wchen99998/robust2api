package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

type step struct {
	name string
	fn   func() error
}

func runSteps(steps []step) error {
	for _, s := range steps {
		log.Printf("[bootstrap] running: %s", s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		log.Printf("[bootstrap] done: %s", s.name)
	}
	return nil
}

// Run executes the full bootstrap flow: validate → connect → migrate → secrets → seed → admin.
func Run(ctx context.Context, env BootstrapEnv) error {
	if err := env.Validate(); err != nil {
		return fmt.Errorf("validate bootstrap env: %w", err)
	}

	if err := timezone.Init(env.Timezone); err != nil {
		return fmt.Errorf("init timezone: %w", err)
	}

	// Open raw *sql.DB for migrations (no Ent client needed yet).
	db, err := sql.Open("postgres", env.DSN())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Verify connectivity with a short timeout.
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	// Run migrations.
	migCtx, migCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer migCancel()
	if err := repository.ApplyMigrations(migCtx, db); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	// Open Ent client for post-migration steps.
	drv, err := entsql.Open(dialect.Postgres, env.DSN())
	if err != nil {
		return fmt.Errorf("open ent driver: %w", err)
	}
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	steps := []step{
		{name: "persist JWT secret", fn: func() error {
			return persistJWTSecret(ctx, client, env.JWTSecret)
		}},
	}

	if env.IsSimpleMode() {
		steps = append(steps,
			step{name: "seed simple-mode default groups", fn: func() error {
				return repository.EnsureSimpleModeDefaultGroups(ctx, client)
			}},
			step{name: "upgrade simple-mode admin concurrency", fn: func() error {
				return repository.EnsureSimpleModeAdminConcurrency(ctx, client)
			}},
		)
	}

	if env.WantsAdminSeed() {
		steps = append(steps, step{name: "seed admin user", fn: func() error {
			return seedAdmin(ctx, db, env)
		}})
	}

	return runSteps(steps)
}

// persistJWTSecret stores the JWT secret in the security_secrets table.
func persistJWTSecret(ctx context.Context, client *ent.Client, secret string) error {
	secret = strings.TrimSpace(secret)

	// Try to insert; ON CONFLICT DO NOTHING handles the race.
	if err := client.SecuritySecret.Create().
		SetKey("jwt_secret").
		SetValue(secret).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		if !isNoRowsErr(err) {
			return fmt.Errorf("persist jwt secret: %w", err)
		}
	}

	// Read back the persisted value (may differ from input if already existed).
	stored, err := client.SecuritySecret.Query().
		Where(securitysecret.KeyEQ("jwt_secret")).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("read persisted jwt secret: %w", err)
	}
	if strings.TrimSpace(stored.Value) != secret {
		log.Println("[bootstrap] WARNING: JWT_SECRET differs from previously persisted value; the persisted value takes precedence for cross-instance consistency")
	}
	return nil
}

func isNoRowsErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}

// seedAdmin creates an admin user if the database has no users.
func seedAdmin(ctx context.Context, db *sql.DB, env BootstrapEnv) error {
	var totalUsers int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users").Scan(&totalUsers); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if totalUsers > 0 {
		log.Println("[bootstrap] skipping admin creation: users already exist")
		return nil
	}

	admin := &service.User{
		Email:       env.AdminEmail,
		Role:        service.RoleAdmin,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: adminConcurrency(env),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := admin.SetPassword(env.AdminPassword); err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role, balance, concurrency, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		admin.Email, admin.PasswordHash, admin.Role, admin.Balance,
		admin.Concurrency, admin.Status, admin.CreatedAt, admin.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert admin user: %w", err)
	}
	log.Printf("[bootstrap] admin user created: %s", env.AdminEmail)
	return nil
}

func adminConcurrency(env BootstrapEnv) int {
	if env.IsSimpleMode() {
		return 30
	}
	return 5
}
