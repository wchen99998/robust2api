# Split Docker Images & Remove Docker Compose

## Goals

1. **Separate images**: Server and bootstrap each get their own Docker image, sized to their actual needs
2. **Remove Docker Compose**: All docker-compose files and related bare-metal deployment artifacts are deleted; Helm/k8s is the only supported deployment method
3. **Clean monorepo**: Each `cmd/` entry point maps to its own image; no dead entrypoint scripts or unused runtime dependencies

## Image Definitions

### Server (`ghcr.io/.../sub2api/server`)

Contents: `sub2api` binary, `resources/` dir, pg_dump/psql client (for backup service), ca-certs, tzdata, libpq deps.

- `ENTRYPOINT ["/app/sub2api"]`
- Runs as UID 1000 (non-root user baked in, enforced by k8s securityContext)
- Healthcheck on `/health`, exposes 8080
- No entrypoint shell script, no su-exec

### Bootstrap (`ghcr.io/.../sub2api/bootstrap`)

Contents: `sub2api-bootstrap` binary, ca-certs, tzdata.

- `ENTRYPOINT ["/app/sub2api-bootstrap"]`
- Runs as UID 1000
- No healthcheck, no exposed ports, no volumes

Both images share the same version tags and are released together.

## Dockerfiles (Approach B: Fully Separate)

### `Dockerfile` (server, local multi-stage build)

- Stage 1 `frontend-builder`: pnpm install + build
- Stage 2 `backend-builder`: go mod download, copy source + frontend dist, build `cmd/server` with `-tags embed`
- Stage 3 `pg-client`: extract pg_dump/psql from `postgres:18-alpine`
- Stage 4 final: alpine + ca-certs + tzdata + libpq deps + pg_dump/psql + binary + resources + non-root user

### `Dockerfile.bootstrap` (bootstrap, local multi-stage build)

- Stage 1 `backend-builder`: go mod download, copy source, build `cmd/bootstrap` (no frontend, no embed tag)
- Stage 2 final: alpine + ca-certs + tzdata + binary + non-root user

### `Dockerfile.goreleaser` (server, CI)

- Stage 1 `pg-client`: extract pg_dump/psql
- Stage 2 final: alpine + runtime deps + pre-built `sub2api` binary + `resources/`

### `Dockerfile.goreleaser.bootstrap` (bootstrap, CI)

- Final alpine + ca-certs + tzdata + pre-built `sub2api-bootstrap` binary + non-root user (~15 lines)
- No `resources/`, no pg_dump, no libpq deps

### Deleted

- `backend/Dockerfile` (stale)
- `deploy/Dockerfile` (old copy)

## GoReleaser Changes

### `.goreleaser.yaml`

`dockers:` section expands from 2 entries to 4:
- `server-amd64`: build ID `sub2api`, `Dockerfile.goreleaser`, extra_files `backend/resources`
- `server-arm64`: same, `--platform=linux/arm64`
- `bootstrap-amd64`: build ID `sub2api-bootstrap`, `Dockerfile.goreleaser.bootstrap`, no extra_files
- `bootstrap-arm64`: same, `--platform=linux/arm64`

`docker_manifests:` expands from 4 to 8:
- `sub2api/server:{{ .Version }}`, `:latest`, `:{{ .Major }}.{{ .Minor }}`, `:{{ .Major }}`
- `sub2api/bootstrap:{{ .Version }}`, `:latest`, `:{{ .Major }}.{{ .Minor }}`, `:{{ .Major }}`

Remove `deploy/docker-entrypoint.sh` from extra_files (deleted).

Release footer: remove the `deploy/install.sh` curl one-liner.

### `.goreleaser.simple.yaml`

Same pattern, amd64-only: 2 docker entries (server + bootstrap), 2 manifest sets.

## Helm Chart Changes

### `values.yaml`

Replace single `image:` block with:

```yaml
image:
  server:
    repository: ghcr.io/wchen99998/sub2api/server
    tag: ""
    pullPolicy: IfNotPresent
  bootstrap:
    repository: ghcr.io/wchen99998/sub2api/bootstrap
    tag: ""
    pullPolicy: IfNotPresent
```

### `deployment.yaml`

Reference `image.server.repository` / `image.server.tag`.

### `bootstrap-job.yaml`

Reference `image.bootstrap.repository` / `image.bootstrap.tag`.

### `values-production.yaml`

No changes needed (doesn't override image settings).

### `_helpers.tpl`

No changes needed (no image-related helpers).

## Files to Delete

### Docker Compose ecosystem

- `deploy/docker-compose.yml`
- `deploy/docker-compose.local.yml`
- `deploy/docker-compose.standalone.yml`
- `deploy/docker-compose.dev.yml`
- `deploy/docker-entrypoint.sh`
- `deploy/docker-deploy.sh`
- `deploy/build_image.sh`
- `deploy/.env.example`

### Documentation (covered by existing DEPLOY.md)

- `deploy/DOCKER.md`
- `deploy/README.md`

### Bare-metal / systemd deployment

- `deploy/install.sh`
- `deploy/install-datamanagementd.sh`
- `deploy/DATAMANAGEMENTD_CN.md`
- `deploy/sub2api.service`
- `deploy/sub2api-datamanagementd.service`
- `deploy/Makefile`

### Stale Dockerfiles

- `deploy/Dockerfile`
- `backend/Dockerfile`

## Files Kept in `deploy/`

- `deploy/Caddyfile`
- `deploy/config.example.yaml`
- `deploy/helm/` (updated per above)
- `deploy/.gitignore`
