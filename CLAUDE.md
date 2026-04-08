# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Sub2API is an AI API Gateway Platform for subscription quota distribution. It consolidates multiple upstream AI service accounts (OpenAI, Claude/Anthropic, Gemini, Sora, etc.) behind a unified gateway with billing, authentication, rate limiting, and load balancing.

**Tech stack:** Go backend (Gin + Ent ORM) + Vue 3 frontend (TypeScript + Vite + pnpm) + PostgreSQL + Redis

## Common Commands

### Backend (run from `backend/`)

```bash
go run ./cmd/gateway/                    # Run gateway/inference server
go run ./cmd/control/                    # Run control/admin server
go run ./cmd/worker/                    # Run background worker
make build                              # Build binaries to bin/gateway, bin/control, bin/worker
make generate                           # Regenerate Ent ORM + Wire DI code
go test -tags=unit ./...                # Unit tests
go test -tags=integration ./...         # Integration tests (needs DB/Redis)
make test-e2e-local                     # E2E tests locally
golangci-lint run ./...                 # Lint (v2, config in .golangci.yml)
```

### Frontend (run from `frontend/`)

```bash
pnpm install       # Install deps (MUST use pnpm, not npm)
pnpm dev           # Dev server
pnpm build         # Production build
```

### Root level

```bash
make build          # Build backend + frontend
make test           # Run all backend tests + frontend lint/typecheck
```

## Architecture

### Layered Backend (`backend/`)

```
handler/ (HTTP handlers, Gin routes)
    ↓
service/ (business logic)
    ↓
repository/ (data access, caching)
    ↓
ent/ (generated ORM code) + Redis
    ↓
PostgreSQL
```

**Enforced by linter:** Services must NOT import repository, Redis, or GORM directly. Handlers must NOT import repository, Redis, or GORM. See `backend/.golangci.yml` depguard rules.

### Dependency Injection

Uses **Google Wire** for compile-time DI. The wire graphs live in `backend/cmd/gateway/wire.go`, `backend/cmd/control/wire.go`, and `backend/cmd/worker/wire.go`. After changing provider sets, run `make generate` from `backend/`.

### Entry Points

`backend/cmd/gateway/main.go` and `backend/cmd/control/main.go` — initialize config (Viper), DB (Ent), Redis, wire services, and start Gin HTTP servers for their respective roles. `backend/cmd/worker/main.go` — runs background workers (usage recording, billing).

### Key Backend Packages

- `cmd/gateway/` — gateway entry, Wire DI setup, version embedding
- `cmd/control/` — control entry, Wire DI setup, version embedding
- `cmd/worker/` — background worker entry, Wire DI setup
- `ent/schema/` — database schema definitions (source of truth for DB models)
- `internal/handler/` — HTTP handlers, grouped by domain; DTOs in `handler/dto/`
- `internal/service/` — business logic; `GatewayService` is the core API proxy
- `internal/repository/` — data access with Ent queries + Redis caching
- `internal/server/` — Gin router setup, middleware registration, route definitions
- `internal/server/middleware/` — auth (JWT, API key), CORS, rate limiting, security headers
- `internal/config/` — Viper-based config loading from YAML + env vars (no prefix; dots become underscores, e.g. `otel.enabled` → `OTEL_ENABLED`)
- `internal/pkg/` — shared utilities (logger, HTTP client, OAuth, provider-specific API adapters)
- `internal/model/` — custom types (error passthrough rules, TLS fingerprint profiles)

### Frontend (`frontend/src/`)

Vue 3 + Pinia stores + Vue Router + i18n (en/zh/ja) + TailwindCSS.

### Gateway Request Flow

Request → API Key Auth → Rate Limit → Account Selection (sticky session + load balancing) → Request Forwarding (with failover/retry) → Response Transform → Async Usage Recording (Redis queue) → Billing Calculation

## Critical Workflows

### Modifying Ent Schemas

After editing files in `backend/ent/schema/`, you must regenerate:
```bash
cd backend && go generate ./ent
```
The generated files in `ent/` must be committed.

### Modifying Interfaces

When adding methods to a Go interface, all test stubs/mocks implementing that interface must be updated or compilation fails.

### Frontend Dependencies

Always use `pnpm` (never `npm`). The `pnpm-lock.yaml` must be committed. CI uses `--frozen-lockfile`.

## Configuration

- Config file: YAML loaded by Viper from `/etc/sub2api/config.yaml`
- Environment variable override: no prefix, dots replaced by underscores (e.g., `SERVER_PORT=8080`, `OTEL_ENABLED=true`)
- Run modes: `standard` (full SaaS with billing) or `simple` (internal use)
- Bootstrap: `backend/cmd/bootstrap` runs migrations and optional initial admin seeding from environment variables

## CI Requirements

- Go version pinned in CI (check `.github/workflows/backend-ci.yml`)
- All unit + integration tests must pass
- `golangci-lint run ./...` must pass (v2 config)
- `pnpm-lock.yaml` must be in sync with `package.json`
- Security scanning: govulncheck, gosec, pnpm audit

## Infrastructure (`infra/`)

Terraform modules for provisioning cloud resources only. All in-cluster resources are managed by Flux CD (see `clusters/production/`).

### Directory Layout

```
infra/
├── modules/
│   ├── doks/           # DOKS cluster + autoscaling node pool
│   ├── database/       # Optional DO Managed PostgreSQL
│   └── storage/        # Optional Cloudflare R2 buckets
├── production/         # Production environment root
│   ├── main.tf         # Composes modules + Cloudflare API token secrets
│   ├── variables.tf
│   ├── outputs.tf
│   ├── versions.tf
│   └── terraform.tfvars.example
└── README.md
```

### Common Terraform Commands (run from `infra/production/`)

```bash
terraform init              # Initialize providers
terraform fmt -recursive .. # Format all .tf files
terraform validate          # Validate configuration
terraform plan              # Preview changes
terraform apply             # Apply changes
terraform output            # Show outputs (cluster endpoint, DB credentials, etc.)
```

### Important Notes

- `terraform.tfvars` contains secrets and is **gitignored** — copy from `terraform.tfvars.example`
- After `terraform apply`, configure kubectl: `doctl kubernetes cluster kubeconfig save sub2api`
- Terraform only manages cloud resources and Cloudflare API token bootstrap secrets
- All in-cluster resources are managed by Flux — see `DEPLOY.md` for the full guide

## GitOps (`clusters/production/`)

Flux CD manages all in-cluster resources via GitOps. Three layers with dependency ordering:

```
clusters/production/
├── flux-system/           # Auto-generated by flux bootstrap
├── kustomization.yaml     # Root Kustomize entrypoint for Flux bootstrap
├── infrastructure.yaml    # Kustomization: infra layer
├── cert-manager-issuers.yaml  # Kustomization: post-cert-manager issuers
├── monitoring.yaml        # Kustomization: optional monitoring (suspended by default)
├── apps.yaml              # Kustomization: apps (depends on cert-manager issuers)
├── infrastructure/        # ingress-nginx, cert-manager, external-dns, namespaces
├── monitoring/            # monitoring namespace + LGTM stack
└── apps/                  # Sub2API application
```

To deploy or change anything in-cluster: edit the relevant YAML file, commit, push. Flux syncs within 1 minute.

## Deployment (`deploy/helm/sub2api/`)

The application is deployed as three separate services via Flux GitOps:

- **Gateway** (`cmd/gateway`) — AI inference proxy, serves `/v1/*` API endpoints
- **Control** (`cmd/control`) — admin/auth backend, serves `/api/*` endpoints; includes a frontend sidecar (Nginx-served Vue SPA)
- **Worker** (`cmd/worker`) — background jobs (usage recording, billing)

### Quick Deploy/Upgrade

Edit image tags in `clusters/production/apps/sub2api.yaml`, commit, and push:

```bash
# Update image tags in clusters/production/apps/sub2api.yaml
git add clusters/production/apps/sub2api.yaml
git commit -m "deploy: v0.3.0"
git push
# Flux syncs automatically within 1 minute
```

### Key Deployment Notes

- All deployments go through Git — no manual `helm upgrade` commands
- PostgreSQL/Redis StatefulSet `persistence.size` is immutable after first install
- Bootstrap job runs DB migrations; it retries on failure (may CrashLoop briefly while PG starts)
- The control pod has 2 containers: `control` (Go backend) + `frontend` (Nginx SPA)
- See `DEPLOY.md` for the full deployment guide including secrets, monitoring, and troubleshooting

## PR Checklist

- `go test -tags=unit ./...` passes
- `go test -tags=integration ./...` passes
- `golangci-lint run ./...` clean
- `pnpm-lock.yaml` updated if `package.json` changed
- Ent generated code committed if schema changed
- Test stubs updated if interfaces changed
- Terraform: `terraform fmt` and `terraform validate` pass if `infra/` changed
