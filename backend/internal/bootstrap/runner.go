package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
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
	stored, err := repository.CreateSecuritySecretIfAbsent(ctx, client, "jwt_secret", secret)
	if err != nil {
		return fmt.Errorf("persist jwt secret: %w", err)
	}
	if stored != secret {
		return fmt.Errorf("jwt secret mismatch: persisted value already exists and differs from configured JWT_SECRET; refusing to continue to preserve cross-instance consistency")
	}
	return nil
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

	result, err := db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role, balance, concurrency, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (email) DO NOTHING`,
		admin.Email, admin.PasswordHash, admin.Role, admin.Balance,
		admin.Concurrency, admin.Status, admin.CreatedAt, admin.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert admin user: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read admin insert result: %w", err)
	}
	if rowsAffected == 0 {
		log.Printf("[bootstrap] skipping admin creation: admin user already exists: %s", env.AdminEmail)
		return nil
	}
	log.Printf("[bootstrap] admin user created: %s", env.AdminEmail)
	return nil
}

func adminConcurrency(env BootstrapEnv) int {
	if env.IsSimpleMode() {
		return repository.SimpleModeTargetAdminConcurrency
	}
	return repository.SimpleModeLegacyAdminConcurrency
}
