# Bootstrap Binary Separation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract all bootstrap mutation (migrations, secret persistence, admin seed, simple-mode seed) into a dedicated `sub2api-bootstrap` binary and Kubernetes Job, making the serving binary read-only at startup.

**Architecture:** New `internal/bootstrap` package owns the bootstrap runner. New `cmd/bootstrap/main.go` is the entrypoint. The serving binary's `InitEnt()` is stripped of mutation. Helm chart gains a pre-install/pre-upgrade Job and loses the PVC.

**Tech Stack:** Go 1.26, Ent ORM, PostgreSQL, Wire DI, Helm 3

---

### Task 1: Create the internal bootstrap package — config validation

Create the core bootstrap package with config loading and input validation.

**Files:**
- Create: `backend/internal/bootstrap/config.go`
- Create: `backend/internal/bootstrap/config_test.go`

- [ ] **Step 1: Write the failing test for bootstrap config validation**

```go
// backend/internal/bootstrap/config_test.go
//go:build unit

package bootstrap

import (
	"testing"
)

func TestValidateBootstrapEnv_AllRequired(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:     "localhost",
		DatabasePort:     "5432",
		DatabaseUser:     "sub2api",
		DatabasePassword: "secret",
		DatabaseDBName:   "sub2api",
		DatabaseSSLMode:  "disable",
		JWTSecret:        "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456",
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateBootstrapEnv_MissingJWTSecret(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:     "localhost",
		DatabasePort:     "5432",
		DatabaseUser:     "sub2api",
		DatabasePassword: "secret",
		DatabaseDBName:   "sub2api",
		DatabaseSSLMode:  "disable",
		TOTPEncryptionKey: "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456",
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
}

func TestValidateBootstrapEnv_MissingTOTPKey(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:     "localhost",
		DatabasePort:     "5432",
		DatabaseUser:     "sub2api",
		DatabasePassword: "secret",
		DatabaseDBName:   "sub2api",
		DatabaseSSLMode:  "disable",
		JWTSecret:        "abcdefghijklmnopqrstuvwxyz123456",
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error for missing TOTP_ENCRYPTION_KEY")
	}
}

func TestValidateBootstrapEnv_AdminEmailWithoutPassword(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456",
		AdminEmail:        "admin@example.com",
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error when ADMIN_EMAIL set without ADMIN_PASSWORD")
	}
}

func TestValidateBootstrapEnv_AdminEmailAndPassword(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456",
		AdminEmail:        "admin@example.com",
		AdminPassword:     "securepassword123",
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestLoadBootstrapEnvFromEnv(t *testing.T) {
	t.Setenv("DATABASE_HOST", "db.example.com")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "sub2api")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_DBNAME", "sub2api")
	t.Setenv("DATABASE_SSLMODE", "require")
	t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("TOTP_ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("ADMIN_EMAIL", "admin@test.com")
	t.Setenv("ADMIN_PASSWORD", "pass123")
	t.Setenv("RUN_MODE", "simple")

	env := LoadBootstrapEnv()
	if env.DatabaseHost != "db.example.com" {
		t.Errorf("expected db.example.com, got %s", env.DatabaseHost)
	}
	if env.AdminEmail != "admin@test.com" {
		t.Errorf("expected admin@test.com, got %s", env.AdminEmail)
	}
	if env.RunMode != "simple" {
		t.Errorf("expected simple, got %s", env.RunMode)
	}
}

func TestLoadBootstrapEnv_Defaults(t *testing.T) {
	// Clear all relevant env vars
	for _, key := range []string{"DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD",
		"DATABASE_DBNAME", "DATABASE_SSLMODE", "JWT_SECRET", "TOTP_ENCRYPTION_KEY",
		"ADMIN_EMAIL", "ADMIN_PASSWORD", "RUN_MODE"} {
		t.Setenv(key, "")
	}

	env := LoadBootstrapEnv()
	if env.DatabasePort != "5432" {
		t.Errorf("expected default port 5432, got %s", env.DatabasePort)
	}
	if env.DatabaseSSLMode != "disable" {
		t.Errorf("expected default sslmode disable, got %s", env.DatabaseSSLMode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test -tags=unit ./internal/bootstrap/ -v`
Expected: FAIL — package does not exist yet

- [ ] **Step 3: Implement the bootstrap config**

```go
// backend/internal/bootstrap/config.go
package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

// BootstrapEnv holds all environment-based configuration for the bootstrap job.
type BootstrapEnv struct {
	DatabaseHost     string
	DatabasePort     string
	DatabaseUser     string
	DatabasePassword string
	DatabaseDBName   string
	DatabaseSSLMode  string
	JWTSecret        string
	TOTPEncryptionKey string
	AdminEmail       string
	AdminPassword    string
	RunMode          string
	Timezone         string
}

// LoadBootstrapEnv reads bootstrap configuration from environment variables.
func LoadBootstrapEnv() BootstrapEnv {
	env := BootstrapEnv{
		DatabaseHost:      getenvOrDefault("DATABASE_HOST", ""),
		DatabasePort:      getenvOrDefault("DATABASE_PORT", "5432"),
		DatabaseUser:      getenvOrDefault("DATABASE_USER", ""),
		DatabasePassword:  os.Getenv("DATABASE_PASSWORD"),
		DatabaseDBName:    getenvOrDefault("DATABASE_DBNAME", ""),
		DatabaseSSLMode:   getenvOrDefault("DATABASE_SSLMODE", "disable"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		TOTPEncryptionKey: os.Getenv("TOTP_ENCRYPTION_KEY"),
		AdminEmail:        strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		RunMode:           strings.ToLower(strings.TrimSpace(os.Getenv("RUN_MODE"))),
		Timezone:          getenvOrDefault("TZ", "Asia/Shanghai"),
	}
	return env
}

// Validate checks that all required bootstrap inputs are present.
func (e *BootstrapEnv) Validate() error {
	if e.DatabaseHost == "" {
		return fmt.Errorf("DATABASE_HOST is required")
	}
	if e.DatabaseUser == "" {
		return fmt.Errorf("DATABASE_USER is required")
	}
	if e.DatabaseDBName == "" {
		return fmt.Errorf("DATABASE_DBNAME is required")
	}
	if strings.TrimSpace(e.JWTSecret) == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len([]byte(strings.TrimSpace(e.JWTSecret))) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}
	if strings.TrimSpace(e.TOTPEncryptionKey) == "" {
		return fmt.Errorf("TOTP_ENCRYPTION_KEY is required")
	}
	if e.AdminEmail != "" && strings.TrimSpace(e.AdminPassword) == "" {
		return fmt.Errorf("ADMIN_PASSWORD is required when ADMIN_EMAIL is set")
	}
	return nil
}

// DSN returns the PostgreSQL connection string.
func (e *BootstrapEnv) DSN() string {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		e.DatabaseHost, e.DatabasePort, e.DatabaseUser,
		e.DatabasePassword, e.DatabaseDBName, e.DatabaseSSLMode,
	)
	if e.Timezone != "" {
		dsn += fmt.Sprintf(" TimeZone=%s", e.Timezone)
	}
	return dsn
}

// IsSimpleMode returns true if RUN_MODE is "simple".
func (e *BootstrapEnv) IsSimpleMode() bool {
	return e.RunMode == "simple"
}

// WantsAdminSeed returns true if admin creation was requested.
func (e *BootstrapEnv) WantsAdminSeed() bool {
	return e.AdminEmail != ""
}

func getenvOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test -tags=unit ./internal/bootstrap/ -v`
Expected: PASS — all 7 tests pass

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/bootstrap/config.go internal/bootstrap/config_test.go
git commit -m "feat(bootstrap): add bootstrap env config loading and validation"
```

---

### Task 2: Export simple-mode seed functions from the repository package

The bootstrap runner (Task 3) needs to call `ensureSimpleModeDefaultGroups` and `ensureSimpleModeAdminConcurrency`, but they're currently unexported. Export them first so Task 3 compiles.

**Files:**
- Modify: `backend/internal/repository/simple_mode_default_groups.go`
- Modify: `backend/internal/repository/simple_mode_admin_concurrency.go`
- Modify: `backend/internal/repository/ent.go`

- [ ] **Step 1: Run existing tests to confirm they pass before changes**

Run: `cd backend && go test -tags=unit ./internal/repository/ -v -run TestSimple -count=1`
Expected: PASS (or no matching tests — these functions are tested indirectly)

- [ ] **Step 2: Export ensureSimpleModeDefaultGroups**

In `backend/internal/repository/simple_mode_default_groups.go`, rename:

```go
// Old:
func ensureSimpleModeDefaultGroups(ctx context.Context, client *dbent.Client) error {
// New:
func EnsureSimpleModeDefaultGroups(ctx context.Context, client *dbent.Client) error {
```

- [ ] **Step 3: Export ensureSimpleModeAdminConcurrency**

In `backend/internal/repository/simple_mode_admin_concurrency.go`, rename:

```go
// Old:
func ensureSimpleModeAdminConcurrency(ctx context.Context, client *dbent.Client) error {
// New:
func EnsureSimpleModeAdminConcurrency(ctx context.Context, client *dbent.Client) error {
```

- [ ] **Step 4: Update the call site in ent.go**

In `backend/internal/repository/ent.go`, update lines 88 and 92:

```go
// Old:
		if err := ensureSimpleModeDefaultGroups(seedCtx, client); err != nil {
// New:
		if err := EnsureSimpleModeDefaultGroups(seedCtx, client); err != nil {
```

```go
// Old:
		if err := ensureSimpleModeAdminConcurrency(seedCtx, client); err != nil {
// New:
		if err := EnsureSimpleModeAdminConcurrency(seedCtx, client); err != nil {
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: SUCCESS

- [ ] **Step 6: Commit**

```bash
cd backend && git add internal/repository/simple_mode_default_groups.go internal/repository/simple_mode_admin_concurrency.go internal/repository/ent.go
git commit -m "refactor(repository): export simple-mode seed functions for bootstrap reuse"
```

---

### Task 3: Create the bootstrap runner — migrations and secret persistence

The runner orchestrates the full bootstrap flow: connect to DB, run migrations, persist JWT secret, run seeds. Depends on Task 2 (exported seed functions).

**Files:**
- Create: `backend/internal/bootstrap/runner.go`
- Create: `backend/internal/bootstrap/runner_test.go`

- [ ] **Step 1: Write failing tests for the runner**

```go
// backend/internal/bootstrap/runner_test.go
//go:build unit

package bootstrap

import (
	"context"
	"errors"
	"testing"
)

func TestRunBootstrap_ValidatesEnvFirst(t *testing.T) {
	env := BootstrapEnv{} // missing required fields
	err := Run(context.Background(), env)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunBootstrap_FailsOnDBConnectionError(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "invalid-host-that-does-not-exist",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: "abcdefghijklmnopqrstuvwxyz123456abcdefghijklmnopqrstuvwxyz123456",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()
	err := Run(ctx, env)
	if err == nil {
		t.Fatal("expected DB connection error")
	}
}

func TestRunSteps_Order(t *testing.T) {
	// Test that runSteps calls steps in the correct order and returns first error.
	order := []string{}
	steps := []step{
		{name: "step1", fn: func() error { order = append(order, "step1"); return nil }},
		{name: "step2", fn: func() error { order = append(order, "step2"); return nil }},
		{name: "step3", fn: func() error { order = append(order, "step3"); return nil }},
	}
	if err := runSteps(steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 || order[0] != "step1" || order[1] != "step2" || order[2] != "step3" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestRunSteps_StopsOnError(t *testing.T) {
	order := []string{}
	testErr := errors.New("step2 failed")
	steps := []step{
		{name: "step1", fn: func() error { order = append(order, "step1"); return nil }},
		{name: "step2", fn: func() error { order = append(order, "step2"); return testErr }},
		{name: "step3", fn: func() error { order = append(order, "step3"); return nil }},
	}
	err := runSteps(steps)
	if !errors.Is(err, testErr) {
		t.Fatalf("expected step2 error, got: %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("step3 should not have run, order: %v", order)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test -tags=unit ./internal/bootstrap/ -v -run TestRun`
Expected: FAIL — `Run`, `step`, `runSteps` not defined

- [ ] **Step 3: Implement the bootstrap runner**

```go
// backend/internal/bootstrap/runner.go
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
// If a secret already exists in the DB, it is used for consistency (logged as warning on mismatch).
func persistJWTSecret(ctx context.Context, client *ent.Client, secret string) error {
	secret = strings.TrimSpace(secret)

	// Try to insert; ON CONFLICT DO NOTHING handles the race.
	if err := client.SecuritySecret.Create().
		SetKey("jwt_secret").
		SetValue(secret).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		// DoNothing returns sql.ErrNoRows when the row already exists; that's fine.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test -tags=unit ./internal/bootstrap/ -v`
Expected: PASS — all unit tests pass (the DB-connection test will fail fast on unreachable host)

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/bootstrap/runner.go internal/bootstrap/runner_test.go
git commit -m "feat(bootstrap): add bootstrap runner with migrations, secrets, and admin seed"
```

---

### Task 4: Create the bootstrap binary entrypoint

**Files:**
- Create: `backend/cmd/bootstrap/main.go`
- Modify: `backend/Makefile`

- [ ] **Step 1: Create the bootstrap main.go**

```go
// backend/cmd/bootstrap/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Wei-Shaw/sub2api/internal/bootstrap"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
)

func main() {
	log.Println("[bootstrap] starting sub2api-bootstrap")

	env := bootstrap.LoadBootstrapEnv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Allow graceful cancellation on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[bootstrap] received signal %s, cancelling...", sig)
		cancel()
	}()

	if err := bootstrap.Run(ctx, env); err != nil {
		log.Fatalf("[bootstrap] FAILED: %v", err)
	}

	log.Println("[bootstrap] completed successfully")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd backend && go build -o bin/bootstrap ./cmd/bootstrap`
Expected: SUCCESS — binary created at `bin/bootstrap`

- [ ] **Step 3: Add bootstrap build target to Makefile**

In `backend/Makefile`, add after the existing `build` target (line 7):

```makefile
build-bootstrap:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/bootstrap ./cmd/bootstrap
```

Update the `.PHONY` line to include `build-bootstrap`.

- [ ] **Step 4: Verify the Makefile target works**

Run: `cd backend && make build-bootstrap`
Expected: SUCCESS — `bin/bootstrap` created

- [ ] **Step 5: Commit**

```bash
cd backend && git add cmd/bootstrap/main.go Makefile
git commit -m "feat(bootstrap): add sub2api-bootstrap binary entrypoint and Makefile target"
```

---

### Task 5: Add bootstrap binary to Docker build

Both binaries are built from the same image. The Dockerfile builds the bootstrap binary alongside the server.

**Files:**
- Modify: `Dockerfile`

- [ ] **Step 1: Add bootstrap binary build step**

In `Dockerfile`, after line 73 (`./cmd/server`), add the bootstrap build:

```dockerfile
# Build bootstrap binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /app/sub2api-bootstrap \
    ./cmd/bootstrap
```

- [ ] **Step 2: Copy bootstrap binary to final image**

In `Dockerfile`, after line 118 (the COPY for the server binary), add:

```dockerfile
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api-bootstrap /app/sub2api-bootstrap
```

- [ ] **Step 3: Verify Docker build succeeds**

Run: `docker build -t sub2api:bootstrap-test --target backend-builder .`
Expected: SUCCESS (builds both binaries)

- [ ] **Step 4: Commit**

```bash
git add Dockerfile
git commit -m "feat(docker): build sub2api-bootstrap binary alongside server"
```

---

### Task 6: Strip bootstrap logic from the serving binary

Remove migration, secret persistence, and simple-mode seed from `InitEnt()`. Remove setup wizard branching from `main.go`.

**Files:**
- Modify: `backend/internal/repository/ent.go`
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/wire.go`

- [ ] **Step 1: Strip InitEnt() down to read-only startup**

Replace `backend/internal/repository/ent.go` content (lines 38-99) with:

```go
func InitEnt(cfg *config.Config) (*ent.Client, *sql.DB, error) {
	if err := timezone.Init(cfg.Timezone); err != nil {
		return nil, nil, err
	}

	dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

	drv, err := entsql.Open(dialect.Postgres, dsn)
	if err != nil {
		return nil, nil, err
	}
	applyDBPoolSettings(drv.DB(), cfg)

	client := ent.NewClient(ent.Driver(drv))
	return client, drv.DB(), nil
}
```

This removes:
- Migration execution (lines 60-65)
- `ensureBootstrapSecrets()` call (lines 70-74)
- Post-secret `cfg.Validate()` (lines 77-80)
- Simple-mode seed calls (lines 85-96)
- The `migrations` import

- [ ] **Step 2: Remove setup branching from main.go**

Replace `backend/cmd/server/main.go` function `main()` (lines 57-97) with:

```go
func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	runMainServer()
}
```

Remove the `runSetupServer()` function (lines 99-129) entirely.

Remove unused imports: `"github.com/Wei-Shaw/sub2api/internal/setup"`, `"github.com/Wei-Shaw/sub2api/internal/web"`, `"github.com/Wei-Shaw/sub2api/internal/server/middleware"` (only used in runSetupServer), `"github.com/gin-gonic/gin"`, `"golang.org/x/net/http2"`, `"golang.org/x/net/http2/h2c"`.

Note: Keep `"github.com/Wei-Shaw/sub2api/internal/server/middleware"` ONLY if it's used elsewhere in this file. Check `wire_gen.go` — middleware is wired via DI, not imported directly in main.go. So remove the import from main.go.

- [ ] **Step 3: Make config loading strict — remove allowMissingJWTSecret**

In `backend/internal/config/config.go`:

Replace `LoadForBootstrap()` (lines 890-895) with:

```go
// Load reads and validates the application configuration.
func Load() (*Config, error) {
	return load()
}
```

Replace the `load` function signature (line 897) from:
```go
func load(allowMissingJWTSecret bool) (*Config, error) {
```
to:
```go
func load() (*Config, error) {
```

Remove the `allowMissingJWTSecret` conditional blocks (lines 993-1004):
```go
	// DELETE these lines:
	originalJWTSecret := cfg.JWT.Secret
	if allowMissingJWTSecret && originalJWTSecret == "" {
		cfg.JWT.Secret = strings.Repeat("0", 32)
	}
	// ... (validation runs) ...
	if allowMissingJWTSecret && originalJWTSecret == "" {
		cfg.JWT.Secret = ""
	}
```

Instead, `cfg.Validate()` will simply fail if JWT_SECRET is missing — which is the desired behavior.

- [ ] **Step 4: Update config wire provider**

In `backend/internal/config/wire.go`, change:

```go
func ProvideConfig() (*Config, error) {
	return Load()
}
```

- [ ] **Step 5: Update main.go to use Load() instead of LoadForBootstrap()**

In `backend/cmd/server/main.go`, change line 132:

```go
// Old:
	cfg, err := config.LoadForBootstrap()
// New:
	cfg, err := config.Load()
```

- [ ] **Step 6: Regenerate Wire code**

Run: `cd backend && go generate ./cmd/server`
Expected: SUCCESS — `wire_gen.go` regenerated

- [ ] **Step 7: Verify compilation**

Run: `cd backend && go build ./...`
Expected: SUCCESS

- [ ] **Step 8: Commit**

```bash
cd backend && git add internal/repository/ent.go cmd/server/main.go internal/config/config.go internal/config/wire.go cmd/server/wire_gen.go
git commit -m "refactor: strip bootstrap mutation from serving binary startup

InitEnt() no longer runs migrations, persists secrets, or seeds data.
main.go no longer checks NeedsSetup() or runs the setup wizard.
Config loading is now strict — JWT_SECRET must be provided."
```

---

### Task 7: Update existing tests for the new behavior

Fix tests that depend on the old `LoadForBootstrap()` name or the `allowMissingJWTSecret` behavior.

**Files:**
- Modify: `backend/internal/config/config_test.go`

- [ ] **Step 1: Check which tests reference LoadForBootstrap**

Run: `cd backend && grep -rn "LoadForBootstrap" --include="*.go"`
Expected: Find references in `config_test.go` and possibly other test files.

- [ ] **Step 2: Update test references**

Rename all `LoadForBootstrap()` calls to `Load()` in test files.

For `TestLoadForBootstrapAllowsMissingJWTSecret` in `config_test.go`: This test validates that loading succeeds with missing JWT secret. Since `Load()` no longer allows this, convert it to test the opposite — that missing JWT secret fails:

```go
func TestLoadRejectsEmptyJWTSecret(t *testing.T) {
	// Clear JWT_SECRET env
	t.Setenv("JWT_SECRET", "")
	// Provide other required config to isolate the JWT test
	t.Setenv("DATABASE_HOST", "localhost")
	t.Setenv("DATABASE_USER", "sub2api")
	t.Setenv("DATABASE_DBNAME", "sub2api")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty JWT_SECRET")
	}
}
```

- [ ] **Step 3: Run all config tests**

Run: `cd backend && go test -tags=unit ./internal/config/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Run full unit test suite**

Run: `cd backend && go test -tags=unit ./... -count=1`
Expected: PASS (fix any additional compilation errors from renamed functions)

- [ ] **Step 5: Commit**

```bash
cd backend && git add -A
git commit -m "test: update config tests for strict Load() behavior"
```

---

### Task 8: Add bootstrap Job to Helm chart

**Files:**
- Create: `deploy/helm/sub2api/templates/bootstrap-job.yaml`
- Modify: `deploy/helm/sub2api/templates/deployment.yaml`
- Modify: `deploy/helm/sub2api/templates/configmap.yaml`
- Delete: `deploy/helm/sub2api/templates/pvc.yaml`
- Modify: `deploy/helm/sub2api/values.yaml`
- Modify: `deploy/helm/sub2api/values-production.yaml`

- [ ] **Step 1: Create the bootstrap Job template**

```yaml
# deploy/helm/sub2api/templates/bootstrap-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "sub2api.fullname" . }}-bootstrap
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: bootstrap
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "0"
    "helm.sh/hook-delete-policy": before-hook-creation
spec:
  backoffLimit: {{ .Values.bootstrap.backoffLimit }}
  ttlSecondsAfterFinished: {{ .Values.bootstrap.ttlSecondsAfterFinished }}
  template:
    metadata:
      labels:
        {{- include "sub2api.selectorLabels" . | nindent 8 }}
        app.kubernetes.io/component: bootstrap
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "sub2api.serviceAccountName" . }}
      securityContext:
        fsGroup: 1000
      restartPolicy: OnFailure
      containers:
        - name: bootstrap
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          command: ["/app/sub2api-bootstrap"]
          envFrom:
            - configMapRef:
                name: {{ include "sub2api.fullname" . }}
            - secretRef:
                name: {{ include "sub2api.secretName" . }}
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

- [ ] **Step 2: Remove PVC template**

Delete the file `deploy/helm/sub2api/templates/pvc.yaml`.

- [ ] **Step 3: Remove PVC references from deployment.yaml**

Remove these sections from `deploy/helm/sub2api/templates/deployment.yaml`:

Remove the strategy conditional (lines 9-12):
```yaml
  {{- if and .Values.persistence.enabled (eq (int .Values.replicaCount) 1) }}
  strategy:
    type: Recreate
  {{- end }}
```

Remove the volumeMounts block (lines 73-77):
```yaml
          {{- if .Values.persistence.enabled }}
          volumeMounts:
            - name: data
              mountPath: /app/data
          {{- end }}
```

Remove the volumes block (lines 78-83):
```yaml
      {{- if .Values.persistence.enabled }}
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: {{ include "sub2api.fullname" . }}
      {{- end }}
```

- [ ] **Step 4: Remove AUTO_SETUP from configmap.yaml**

In `deploy/helm/sub2api/templates/configmap.yaml`, remove line 8:
```yaml
  AUTO_SETUP: "true"
```

- [ ] **Step 5: Update values.yaml**

Remove the entire `persistence` section (lines 82-90):
```yaml
# =============================================================================
# Persistence (/app/data volume)
# =============================================================================
persistence:
  enabled: true
  size: 1Gi
  storageClass: ""
  accessModes:
    - ReadWriteOnce
```

Add bootstrap configuration after `extraSecretEnv`:
```yaml
# =============================================================================
# Bootstrap Job
# =============================================================================
bootstrap:
  backoffLimit: 5
  ttlSecondsAfterFinished: 300
```

- [ ] **Step 6: Update values-production.yaml**

No persistence references exist in `values-production.yaml` currently (confirmed from reading — it only overrides `replicaCount`, `config`, `ingress`, `resources`, `postgresql`, `redis`, `externalDatabase`, `externalRedis`). No changes needed.

- [ ] **Step 7: Verify Helm template renders**

Run: `helm template test deploy/helm/sub2api/ 2>&1 | head -200`
Expected: Output includes the bootstrap Job with hook annotations; no PVC; no AUTO_SETUP in ConfigMap.

- [ ] **Step 8: Commit**

```bash
git add deploy/helm/sub2api/templates/bootstrap-job.yaml deploy/helm/sub2api/templates/deployment.yaml deploy/helm/sub2api/templates/configmap.yaml deploy/helm/sub2api/values.yaml deploy/helm/sub2api/values-production.yaml
git rm deploy/helm/sub2api/templates/pvc.yaml
git commit -m "feat(helm): add bootstrap Job, remove PVC and AUTO_SETUP

Bootstrap Job runs as pre-install/pre-upgrade hook using the same
image with sub2api-bootstrap command. PVC removed — pricing cache
is ephemeral, logs go to stdout via LGTM stack."
```

---

### Task 9: Add Helm template render tests

Verify the chart renders correctly with the new structure.

**Files:**
- Create: `deploy/helm/sub2api/tests/bootstrap_test.yaml` (or shell-based test)

- [ ] **Step 1: Write render verification script**

```bash
# deploy/helm/sub2api/tests/render-test.sh
#!/bin/bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== Helm template render tests ==="

RENDERED=$(helm template test "$CHART_DIR" --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long --set secrets.adminPassword=testpass 2>&1)

# Test 1: Bootstrap Job exists
echo -n "Bootstrap Job exists... "
echo "$RENDERED" | grep -q 'kind: Job' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 2: Bootstrap Job has hook annotations
echo -n "Bootstrap Job has pre-install hook... "
echo "$RENDERED" | grep -q 'helm.sh/hook.*pre-install' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 3: Bootstrap Job uses bootstrap command
echo -n "Bootstrap Job uses sub2api-bootstrap command... "
echo "$RENDERED" | grep -q 'sub2api-bootstrap' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 4: No PVC
echo -n "No PersistentVolumeClaim... "
echo "$RENDERED" | grep -q 'kind: PersistentVolumeClaim' && { echo "FAIL — PVC still exists"; exit 1; } || echo "PASS"

# Test 5: No AUTO_SETUP in ConfigMap
echo -n "No AUTO_SETUP in ConfigMap... "
echo "$RENDERED" | grep -q 'AUTO_SETUP' && { echo "FAIL — AUTO_SETUP still present"; exit 1; } || echo "PASS"

# Test 6: No volumeMounts in Deployment
echo -n "No /app/data volumeMount in Deployment... "
echo "$RENDERED" | grep -q 'mountPath: /app/data' && { echo "FAIL — volume mount still present"; exit 1; } || echo "PASS"

# Test 7: Deployment still exists
echo -n "Server Deployment exists... "
echo "$RENDERED" | grep -q 'kind: Deployment' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 8: existingSecret mode works
RENDERED_EXT=$(helm template test "$CHART_DIR" --set existingSecret=my-secret --set secrets.jwtSecret=test-secret-that-is-at-least-32-bytes --set secrets.totpEncryptionKey=test-totp-key-that-is-at-least-32-bytes-long 2>&1)
echo -n "existingSecret mode renders... "
echo "$RENDERED_EXT" | grep -q 'my-secret' && echo "PASS" || { echo "FAIL"; exit 1; }

echo "=== All render tests passed ==="
```

- [ ] **Step 2: Run the render tests**

Run: `bash deploy/helm/sub2api/tests/render-test.sh`
Expected: All 8 tests PASS

- [ ] **Step 3: Commit**

```bash
chmod +x deploy/helm/sub2api/tests/render-test.sh
git add deploy/helm/sub2api/tests/render-test.sh
git commit -m "test(helm): add template render tests for bootstrap Job and PVC removal"
```

---

### Task 10: Add regression tests for serving binary

Verify the serving binary no longer performs bootstrap mutation.

**Files:**
- Create: `backend/internal/repository/ent_no_bootstrap_test.go`

- [ ] **Step 1: Write regression tests**

```go
// backend/internal/repository/ent_no_bootstrap_test.go
//go:build unit

package repository

import (
	"strings"
	"testing"

	"go/ast"
	"go/parser"
	"go/token"
)

// TestInitEnt_DoesNotCallMigrations verifies that InitEnt no longer references
// migration functions, ensuring bootstrap separation.
func TestInitEnt_DoesNotCallBootstrapFunctions(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "ent.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse ent.go: %v", err)
	}

	banned := []string{
		"applyMigrationsFS",
		"ApplyMigrations",
		"ensureBootstrapSecrets",
		"EnsureSimpleModeDefaultGroups",
		"EnsureSimpleModeAdminConcurrency",
		"ensureSimpleModeDefaultGroups",
		"ensureSimpleModeAdminConcurrency",
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			var name string
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			for _, b := range banned {
				if name == b {
					t.Errorf("InitEnt must not call %s — bootstrap mutation belongs in the bootstrap binary", b)
				}
			}
		}
		return true
	})
}

// TestInitEnt_DoesNotImportMigrations verifies the migrations package is no longer imported.
func TestInitEnt_DoesNotImportMigrations(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "ent.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse ent.go: %v", err)
	}

	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasSuffix(path, "/migrations") {
			t.Errorf("ent.go must not import the migrations package — bootstrap owns migration execution")
		}
	}
}
```

- [ ] **Step 2: Run the regression tests**

Run: `cd backend && go test -tags=unit ./internal/repository/ -v -run TestInitEnt_DoesNot`
Expected: PASS — `ent.go` no longer references banned functions or imports.

- [ ] **Step 3: Commit**

```bash
cd backend && git add internal/repository/ent_no_bootstrap_test.go
git commit -m "test(regression): verify serving binary no longer performs bootstrap mutation"
```

---

### Task 11: Lint and full test pass

Run the full CI checks to make sure everything is clean.

**Files:** None (verification only)

- [ ] **Step 1: Run unit tests**

Run: `cd backend && go test -tags=unit ./... -count=1`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `cd backend && golangci-lint run ./...`
Expected: PASS — no lint errors

- [ ] **Step 3: Verify both binaries compile**

Run: `cd backend && go build ./cmd/server && go build ./cmd/bootstrap`
Expected: SUCCESS

- [ ] **Step 4: Run Helm render tests**

Run: `bash deploy/helm/sub2api/tests/render-test.sh`
Expected: PASS

- [ ] **Step 5: Final commit if any fixups needed**

```bash
git add -A
git commit -m "fix: address lint and test issues from bootstrap separation"
```

(Skip if nothing to fix.)
