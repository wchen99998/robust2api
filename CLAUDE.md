# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Sub2API is an AI API Gateway Platform for subscription quota distribution. It consolidates multiple upstream AI service accounts (OpenAI, Claude/Anthropic, Gemini, Sora, etc.) behind a unified gateway with billing, authentication, rate limiting, and load balancing.

**Tech stack:** Go backend (Gin + Ent ORM) + Vue 3 frontend (TypeScript + Vite + pnpm) + PostgreSQL + Redis

## Common Commands

### Backend (run from `backend/`)

```bash
go run ./cmd/server/                    # Run dev server
make build                              # Build binary to bin/server
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

Uses **Google Wire** for compile-time DI. The wire graph is in `backend/cmd/server/wire.go`. After changing provider sets, run `make generate` from `backend/`.

### Entry Point

`backend/cmd/server/main.go` — initializes config (Viper), DB (Ent), Redis, wires services, starts Gin HTTP server with graceful shutdown.

### Key Backend Packages

- `cmd/server/` — main entry, Wire DI setup, version embedding
- `ent/schema/` — database schema definitions (source of truth for DB models)
- `internal/handler/` — HTTP handlers, grouped by domain; DTOs in `handler/dto/`
- `internal/service/` — business logic; `GatewayService` is the core API proxy
- `internal/repository/` — data access with Ent queries + Redis caching
- `internal/server/` — Gin router setup, middleware registration, route definitions
- `internal/server/middleware/` — auth (JWT, API key), CORS, rate limiting, security headers
- `internal/config/` — Viper-based config loading from YAML + env vars (prefix `SUB2API_`)
- `internal/pkg/` — shared utilities (logger, HTTP client, OAuth, provider-specific API adapters)
- `internal/model/` — custom types (error passthrough rules, TLS fingerprint profiles)
- `internal/web/` — frontend asset embedding via `//go:embed` (build tag `embed`)

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

- Config file: YAML loaded by Viper (see `deploy/config.example.yaml`)
- Environment variable override: prefix `SUB2API_` (e.g., `SUB2API_SERVER_PORT=8080`)
- Run modes: `standard` (full SaaS with billing) or `simple` (internal use)

## CI Requirements

- Go version pinned in CI (check `.github/workflows/backend-ci.yml`)
- All unit + integration tests must pass
- `golangci-lint run ./...` must pass (v2 config)
- `pnpm-lock.yaml` must be in sync with `package.json`
- Security scanning: govulncheck, gosec, pnpm audit

## Infrastructure (`infra/`)

Terraform modules for provisioning DigitalOcean Kubernetes (DOKS) infrastructure with Cloudflare DNS.

### Directory Layout

```
infra/
├── modules/
│   ├── doks/           # DOKS cluster + autoscaling node pool
│   ├── kubernetes/     # In-cluster bootstrap (ingress-nginx, cert-manager)
│   ├── database/       # Optional DO Managed PostgreSQL
│   └── dns/            # Cloudflare A record
├── production/         # Production environment root
│   ├── main.tf         # Composes all modules
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
terraform output            # Show outputs (LB IP, cluster endpoint, etc.)
```

### Important Notes

- `terraform.tfvars` contains secrets and is **gitignored** — copy from `terraform.tfvars.example`
- After `terraform apply`, configure kubectl: `doctl kubernetes cluster kubeconfig save sub2api`
- Sub2API itself is deployed via Helm (`deploy/helm/sub2api/`), not Terraform
- See `DEPLOY.md` for the full deployment guide

## PR Checklist

- `go test -tags=unit ./...` passes
- `go test -tags=integration ./...` passes
- `golangci-lint run ./...` clean
- `pnpm-lock.yaml` updated if `package.json` changed
- Ent generated code committed if schema changed
- Test stubs updated if interfaces changed
- Terraform: `terraform fmt` and `terraform validate` pass if `infra/` changed
