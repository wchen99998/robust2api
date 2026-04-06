# Phase 1: Separate Bootstrap into a Dedicated Binary and Kubernetes Job

## Summary

Introduce a new bootstrap executable (`sub2api-bootstrap`) responsible for all schema and bootstrap mutation. The current serving binary remains a single runtime (no api/worker split in this phase). The bootstrap job becomes mandatory in the supported Kubernetes deployment path. All bootstrap behavior is removed from the serving binary: no setup wizard, no `AUTO_SETUP`, no schema mutation, no secret generation, no config-file installation flow.

## Key Changes

### Bootstrap Binary

Add a new command entrypoint at `backend/cmd/bootstrap/` that performs one idempotent run and exits non-zero on failure.

Extract reusable bootstrap logic from the current setup and repository startup paths into an internal bootstrap package so both migration and admin-seed logic live outside the serving binary.

**Bootstrap job responsibilities:**

- Load bootstrap configuration from env only.
- Validate required bootstrap inputs before touching the database.
- Acquire the existing migration advisory lock (`pg_try_advisory_lock(694208311321144027)`) and apply SQL migrations via the existing `applyMigrationsFS()` runner.
- Perform bootstrap-secret handling currently done in `ensureBootstrapSecrets()` (persists JWT secret to `security_secrets` table), but only inside the bootstrap binary.
- Perform simple-mode seed logic currently done in `InitEnt()` (`ensureSimpleModeDefaultGroups()` and `upgradeSimpleModeAdminConcurrency()`).
- Optionally create the first admin if and only if bootstrap admin env vars are present and the database is empty of users/admins.

**Bootstrap job requires PostgreSQL connectivity only.** Redis is not a bootstrap dependency in this phase.

### Serving Binary

Refactor `cmd/server/main.go` so normal startup always goes directly to runtime boot. Remove `NeedsSetup()` check (line 80), `AUTO_SETUP` env check (lines 82-87), `--setup` flag (line 62), and `runSetupServer()` branching (lines 89-91) from supported startup.

Refactor `internal/repository/ent.go` so `InitEnt()` no longer:
- Applies migrations (lines 57-65)
- Calls `ensureBootstrapSecrets()` (lines 70-74)
- Seeds simple-mode data (lines 82-96)

Serving startup becomes read-only with respect to bootstrap state:
- It validates config and secrets (strict mode — JWT_SECRET must be present, no `allowMissingJWTSecret` grace period).
- It may verify bootstrap prerequisites (e.g., schema version check) if needed.
- It must not create or mutate bootstrap data.

Leave setup package code and frontend setup assets in the repo for now to reduce churn, but stop wiring them into runtime startup. Deletion is a later phase.

### Config Loading

Both binaries use strict config loading — `JWT_SECRET` and `TOTP_ENCRYPTION_KEY` must be provided via env. Remove the `allowMissingJWTSecret` parameter and placeholder logic in `config.LoadForBootstrap()`.

**TOTP asymmetry note:** Unlike JWT (which is persisted to the `security_secrets` DB table for multi-instance consistency), TOTP encryption key is a pure config value — never DB-persisted. Both binaries read it from env. The bootstrap binary validates its presence but does not persist it.

### Bootstrap Contract

**Required env vars:**
- Database connection vars (`DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_USER`, `DATABASE_PASSWORD`, `DATABASE_DBNAME`, `DATABASE_SSLMODE`)
- `JWT_SECRET`
- `TOTP_ENCRYPTION_KEY`

**Optional admin seed:**
- `ADMIN_EMAIL` — if unset, skip admin creation entirely
- `ADMIN_PASSWORD` — must be set when `ADMIN_EMAIL` is set; do not generate passwords inside the job

**Mode control:**
- `RUN_MODE` — continues to control simple-mode seed behavior

**Admin seed rules:**
- If `ADMIN_EMAIL` is unset, skip admin creation.
- If `ADMIN_EMAIL` is set but `ADMIN_PASSWORD` is missing, fail fast with a clear error.
- If users or admins already exist in the database, skip admin creation without error.

**Secret rules:**
- `JWT_SECRET` and `TOTP_ENCRYPTION_KEY` are mandatory in the supported Kubernetes path.
- The bootstrap binary owns DB synchronization of JWT secret (persisting to `security_secrets` table).
- The serving binary must never auto-generate either secret.

### Kubernetes Packaging

**Add a bootstrap Job** to the Helm chart using the same container image but a different command (`sub2api-bootstrap`).

Recommended job behavior:
- Helm `pre-install` and `pre-upgrade` hook
- `restartPolicy: OnFailure`
- Finite `backoffLimit` (e.g., 5)
- Short `ttlSecondsAfterFinished` (e.g., 300)
- Shares the same ConfigMap and Secret (or `existingSecret`) as the Deployment via `envFrom`

**Keep the existing server Deployment** shape. The Deployment receives explicit env/Secret inputs and assumes schema/bootstrap work is already complete before it starts.

**Remove the PVC:**
- Remove the `pvc.yaml` template
- Remove the `/app/data` volume and volumeMount from the Deployment
- Remove `persistence.*` from `values.yaml` and `values-production.yaml`

Rationale: The PVC only stored setup artifacts (`config.yaml`, `.installed`), a re-downloadable pricing cache (`model_pricing.json`, `.sha256`), and a log file redundant with the LGTM observability stack. Confirmed on live cluster — no other files present. The pricing service will use ephemeral container storage (default `./data` resolves fine without a PVC).

**Remove `AUTO_SETUP=true`** from the ConfigMap template.

**Update `values-production.yaml`** alongside `values.yaml` to remove persistence references and any setup-related config.

## Public Interfaces

- New executable: `sub2api-bootstrap`
- New chart workload: `bootstrap` Job (Helm hook)
- Removed from supported serving startup:
  - `--setup` CLI flag
  - `AUTO_SETUP` env var
  - Startup-time setup wizard mode
  - `NeedsSetup()` check
- Removed from chart:
  - PVC template and persistence values
  - `AUTO_SETUP` ConfigMap entry
- Phase-1 runtime config source:
  - Serving binary: env-driven runtime config, no config-file generation
  - Bootstrap binary: env-driven bootstrap config, no `config.yaml`, no `.installed`
- Temporary repo compatibility:
  - Setup package code may remain on disk but is no longer part of the supported server boot path

## Dead Code Notes

The following become dead code in this phase and are left for later cleanup:
- `internal/setup/` package (setup wizard, CLI setup, auto-setup, `GetDataDir()`, `NeedsSetup()`, `Install()`)
- `DATA_DIR` env var resolution in setup and config packages
- `.installed` file logic
- `initializeDatabase()` in setup package (separate migration caller, distinct from `InitEnt()`)
- Frontend setup wizard assets (if embedded)

## Test Plan

### Unit tests for the bootstrap runner
- Migrations run once and are idempotent
- Admin creation only happens on empty DB
- Admin creation is skipped when users/admins exist
- Missing `JWT_SECRET` or `TOTP_ENCRYPTION_KEY` fails fast
- Missing `ADMIN_PASSWORD` fails only when `ADMIN_EMAIL` is set

### Integration tests
- `bootstrap job -> server startup` on a fresh Postgres database
- `bootstrap job` rerun on an already-bootstrapped database (idempotent)
- Server startup failure when bootstrap prerequisites are absent (e.g., no schema, missing secrets)

### Regression tests for the serving binary
- Does not run migrations
- Does not write bootstrap secrets
- Does not enter setup mode
- Fails fast when `JWT_SECRET` or `TOTP_ENCRYPTION_KEY` is missing

### Helm render tests
- Bootstrap Job exists with correct hook annotations and command
- Server Deployment has no PVC volume mount
- No `AUTO_SETUP` in ConfigMap
- `existingSecret` mode still works for both Job and Deployment

## Assumptions and Defaults

- This phase only separates bootstrap mutation from runtime serving; api/worker separation comes later.
- PostgreSQL is the only bootstrap dependency; Redis readiness stays a runtime concern.
- Kubernetes job-based bootstrap is mandatory for the supported deployment path.
- Deterministic bootstrap: no generated admin password, no generated runtime secrets, no config-file installation flow.
- Reuse existing env names (`ADMIN_EMAIL`, `ADMIN_PASSWORD`, `RUN_MODE`, `JWT_SECRET`, `TOTP_ENCRYPTION_KEY`) rather than introducing bootstrap-specific names.
- Pricing data cache is ephemeral — re-downloaded on pod start, no persistence needed.
- File-based logging is superseded by the LGTM observability stack (stdout -> Alloy -> Loki).
