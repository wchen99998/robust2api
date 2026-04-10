# robust2api

`robust2api` is an AI API gateway for routing requests across multiple upstream AI accounts behind a single authenticated control plane. It handles authentication, API key issuance, billing, load balancing, rate limits, and request forwarding for providers such as OpenAI, Anthropic, Gemini, and related integrations.

This repository still uses legacy `sub2api` names in some paths, binaries, Helm chart directories, and configuration examples. Those identifiers are kept where they reflect the current codebase layout.

## Core Capabilities

- Multi-account upstream management across OAuth and API key based providers
- Platform-issued API keys for end users and internal consumers
- Token-level usage metering with billing support
- Sticky-session aware account selection and failover
- Per-user and per-account concurrency controls
- Request and token rate limiting
- Admin dashboard for operations and account management
- Optional observability stack with OpenTelemetry, Prometheus, Grafana, Tempo, and Loki

## Stack

| Component | Technology |
|-----------|------------|
| Backend | Go, Gin, Ent |
| Frontend | Vue 3, TypeScript, Vite, TailwindCSS |
| Database | PostgreSQL |
| Cache / Queue | Redis |
| Infra | Terraform, Helm, Flux CD |

## Architecture

The backend follows a layered structure:

```text
handler -> service -> repository -> ent/redis -> PostgreSQL
```

Main services:

- `gateway`: inference proxy and upstream request router
- `control`: admin/auth API and frontend host
- `worker`: background jobs for usage recording and billing
- `bootstrap`: migrations and optional initial admin seeding

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js 18+
- pnpm
- PostgreSQL 15+
- Redis 7+

### Build From Source

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api

cd frontend
pnpm install
pnpm build

cd ../backend
go build -o sub2api-gateway ./cmd/gateway
go build -o sub2api-control ./cmd/control
go build -o sub2api-bootstrap ./cmd/bootstrap
go build -o sub2api-worker ./cmd/worker
```

Create the runtime config file at `/etc/sub2api/config.yaml`, then export bootstrap secrets before the first run:

```bash
export DATABASE_HOST=localhost
export DATABASE_PORT=5432
export DATABASE_USER=postgres
export DATABASE_PASSWORD=your_password
export DATABASE_DBNAME=sub2api
export DATABASE_SSLMODE=disable
export JWT_SECRET="$(openssl rand -hex 32)"
export TOTP_ENCRYPTION_KEY="$(openssl rand -hex 32)"
export ADMIN_EMAIL=admin@example.com
export ADMIN_PASSWORD=change-me
```

Run bootstrap once, then start the services:

```bash
./sub2api-bootstrap
./sub2api-gateway
./sub2api-control
./sub2api-worker
```

## Configuration Notes

- Runtime YAML config is loaded from `/etc/sub2api/config.yaml`
- Bootstrap reads required secrets from environment variables
- `JWT_SECRET` must be at least 32 bytes
- `TOTP_ENCRYPTION_KEY` must be 64 hex characters
- `RUN_MODE=simple` enables the simplified operating mode

Example config skeleton:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "your_password"
  dbname: "sub2api"

redis:
  host: "localhost"
  port: 6379
  password: ""
```

## Development

Run the backend services from `backend/`:

```bash
go run ./cmd/gateway/
go run ./cmd/control/
go run ./cmd/worker/
```

Run the frontend from `frontend/`:

```bash
pnpm install
pnpm dev
```

When editing `backend/ent/schema`, regenerate generated code:

```bash
cd backend
go generate ./ent
go generate ./cmd/gateway
go generate ./cmd/control
go generate ./cmd/worker
```

## Deployment

- Kubernetes deployment and GitOps operations are documented in [DEPLOY.md](DEPLOY.md)
- Terraform infrastructure lives under `infra/production/`
- Helm manifests live under `deploy/helm/sub2api/`
- Production cluster manifests live under `clusters/production/`

If you use Nginx in front of the gateway and need Codex CLI compatibility, add this to the `http` block:

```nginx
underscores_in_headers on;
```

## Project Layout

```text
sub2api/
├── backend/
│   ├── cmd/
│   ├── ent/
│   ├── internal/
│   └── resources/
├── frontend/
├── deploy/
│   ├── Caddyfile
│   └── helm/sub2api/
├── clusters/production/
├── infra/
└── docs/
```

## Health Checks

```bash
curl -I http://localhost:8080/livez
curl -I http://localhost:8080/readyz
curl -I http://localhost:8081/livez
curl -I http://localhost:8081/readyz
```

## License

MIT
