# Split Docker Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the single Docker image into separate server and bootstrap images, remove all docker-compose/bare-metal deployment artifacts, and update GoReleaser + Helm accordingly.

**Architecture:** Two independent Dockerfiles per build path (local multi-stage and GoReleaser CI), each producing a minimal image for its binary. Helm chart references each image separately. All docker-compose and bare-metal deployment files are deleted.

**Tech Stack:** Docker multi-stage builds, GoReleaser v2, Helm 3, Alpine Linux

---

## File Map

### Create

| File | Purpose |
|------|---------|
| `Dockerfile` | Server local multi-stage build (replaces existing) |
| `Dockerfile.bootstrap` | Bootstrap local multi-stage build |
| `Dockerfile.goreleaser` | Server CI image (replaces existing) |
| `Dockerfile.goreleaser.bootstrap` | Bootstrap CI image |

### Modify

| File | Change |
|------|--------|
| `.goreleaser.yaml` | 4 docker entries, 8 manifests, remove entrypoint extra_file, remove install.sh from footer |
| `.goreleaser.simple.yaml` | 2 docker entries, 4 manifests, remove entrypoint extra_file |
| `deploy/helm/sub2api/values.yaml` | Split `image:` into `image.server` + `image.bootstrap` |
| `deploy/helm/sub2api/templates/deployment.yaml` | Reference `image.server.*` |
| `deploy/helm/sub2api/templates/bootstrap-job.yaml` | Reference `image.bootstrap.*` |
| `deploy/helm/sub2api/tests/render-test.sh` | Update to verify separate image refs |
| `README.md` | Remove docker-compose sections, update project structure |
| `README_CN.md` | Same as README.md |

### Delete

| File | Reason |
|------|--------|
| `deploy/docker-compose.yml` | Docker Compose removed |
| `deploy/docker-compose.local.yml` | Docker Compose removed |
| `deploy/docker-compose.standalone.yml` | Docker Compose removed |
| `deploy/docker-compose.dev.yml` | Docker Compose removed |
| `deploy/docker-entrypoint.sh` | No longer needed (k8s securityContext handles user) |
| `deploy/docker-deploy.sh` | Docker deployment removed |
| `deploy/build_image.sh` | Docker Compose local build helper removed |
| `deploy/.env.example` | Docker Compose config removed |
| `deploy/DOCKER.md` | Covered by DEPLOY.md |
| `deploy/README.md` | Covered by DEPLOY.md |
| `deploy/install.sh` | Bare-metal installer removed |
| `deploy/install-datamanagementd.sh` | Bare-metal removed |
| `deploy/DATAMANAGEMENTD_CN.md` | Bare-metal removed |
| `deploy/sub2api.service` | Systemd unit removed |
| `deploy/sub2api-datamanagementd.service` | Systemd unit removed |
| `deploy/Makefile` | Deploy Makefile removed |
| `deploy/Dockerfile` | Old copy, replaced by root Dockerfile |
| `backend/Dockerfile` | Stale dev Dockerfile |

---

### Task 1: Delete obsolete files

**Files:**
- Delete: All 18 files listed in the Delete table above

- [ ] **Step 1: Delete docker-compose files**

```bash
cd /Users/chenwuhao/Dev/sub2api
git rm deploy/docker-compose.yml \
       deploy/docker-compose.local.yml \
       deploy/docker-compose.standalone.yml \
       deploy/docker-compose.dev.yml
```

- [ ] **Step 2: Delete deployment scripts and entrypoint**

```bash
git rm deploy/docker-entrypoint.sh \
       deploy/docker-deploy.sh \
       deploy/build_image.sh \
       deploy/.env.example \
       deploy/install.sh \
       deploy/install-datamanagementd.sh
```

- [ ] **Step 3: Delete docs, systemd units, Makefile**

```bash
git rm deploy/DOCKER.md \
       deploy/README.md \
       deploy/DATAMANAGEMENTD_CN.md \
       deploy/sub2api.service \
       deploy/sub2api-datamanagementd.service \
       deploy/Makefile
```

- [ ] **Step 4: Delete stale Dockerfiles**

```bash
git rm deploy/Dockerfile \
       backend/Dockerfile
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: remove docker-compose, bare-metal deployment, and stale Dockerfiles"
```

---

### Task 2: Write server Dockerfile (local multi-stage)

**Files:**
- Rewrite: `Dockerfile`

- [ ] **Step 1: Write the server Dockerfile**

Replace the entire contents of `Dockerfile` with:

```dockerfile
# =============================================================================
# Sub2API Server — Multi-Stage Dockerfile
# =============================================================================
# Builds the server binary with embedded frontend assets.
# Produces a minimal image with pg_dump/psql for backup support.
# =============================================================================

ARG NODE_IMAGE=node:24-alpine
ARG GOLANG_IMAGE=golang:1.26.1-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG POSTGRES_IMAGE=postgres:18-alpine
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

# -----------------------------------------------------------------------------
# Stage 1: Frontend Builder
# -----------------------------------------------------------------------------
FROM ${NODE_IMAGE} AS frontend-builder

WORKDIR /app/frontend

RUN corepack enable && corepack prepare pnpm@latest --activate

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm run build

# -----------------------------------------------------------------------------
# Stage 2: Backend Builder
# -----------------------------------------------------------------------------
FROM ${GOLANG_IMAGE} AS backend-builder

ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/backend

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

COPY --from=frontend-builder /app/backend/internal/web/dist ./internal/web/dist

RUN VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(tr -d '\r\n' < ./cmd/server/VERSION)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api \
    ./cmd/server

# -----------------------------------------------------------------------------
# Stage 3: PostgreSQL Client
# -----------------------------------------------------------------------------
FROM ${POSTGRES_IMAGE} AS pg-client

# -----------------------------------------------------------------------------
# Stage 4: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${ALPINE_IMAGE}

LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API Server - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    libpq \
    zstd-libs \
    lz4-libs \
    krb5-libs \
    libldap \
    libedit \
    && rm -rf /var/cache/apk/*

COPY --from=pg-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=pg-client /usr/local/lib/libpq.so.5* /usr/local/lib/

RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

WORKDIR /app

COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api /app/sub2api
COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources

USER sub2api

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/sub2api"]
```

- [ ] **Step 2: Verify it builds**

```bash
docker build -t sub2api-server:test .
```

Expected: builds successfully, final image contains `/app/sub2api` binary, no entrypoint script.

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "feat(docker): rewrite server Dockerfile without entrypoint script"
```

---

### Task 3: Write bootstrap Dockerfile (local multi-stage)

**Files:**
- Create: `Dockerfile.bootstrap`

- [ ] **Step 1: Write the bootstrap Dockerfile**

Create `Dockerfile.bootstrap`:

```dockerfile
# =============================================================================
# Sub2API Bootstrap — Multi-Stage Dockerfile
# =============================================================================
# Builds the bootstrap binary for DB migration and seeding.
# Produces a minimal image with only the binary and TLS/timezone support.
# =============================================================================

ARG GOLANG_IMAGE=golang:1.26.1-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

# -----------------------------------------------------------------------------
# Stage 1: Backend Builder
# -----------------------------------------------------------------------------
FROM ${GOLANG_IMAGE} AS backend-builder

ARG GOPROXY
ARG GOSUMDB

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/backend

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /app/sub2api-bootstrap \
    ./cmd/bootstrap

# -----------------------------------------------------------------------------
# Stage 2: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${ALPINE_IMAGE}

LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API Bootstrap - DB Migration & Seeding"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

WORKDIR /app

COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api-bootstrap /app/sub2api-bootstrap

USER sub2api

ENTRYPOINT ["/app/sub2api-bootstrap"]
```

- [ ] **Step 2: Verify it builds**

```bash
docker build -t sub2api-bootstrap:test -f Dockerfile.bootstrap .
```

Expected: builds successfully, image is much smaller than the server image (no pg_dump, no frontend, no libpq).

- [ ] **Step 3: Commit**

```bash
git add Dockerfile.bootstrap
git commit -m "feat(docker): add bootstrap Dockerfile for minimal migration image"
```

---

### Task 4: Write GoReleaser Dockerfiles

**Files:**
- Rewrite: `Dockerfile.goreleaser`
- Create: `Dockerfile.goreleaser.bootstrap`

- [ ] **Step 1: Rewrite Dockerfile.goreleaser for server only**

Replace the entire contents of `Dockerfile.goreleaser`:

```dockerfile
# =============================================================================
# Sub2API Server — GoReleaser Dockerfile
# =============================================================================
# Packages the pre-built server binary. No compilation needed.
# =============================================================================

ARG ALPINE_IMAGE=alpine:3.21
ARG POSTGRES_IMAGE=postgres:18-alpine

FROM ${POSTGRES_IMAGE} AS pg-client

FROM ${ALPINE_IMAGE}

LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API Server - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    libpq \
    zstd-libs \
    lz4-libs \
    krb5-libs \
    libldap \
    libedit \
    && rm -rf /var/cache/apk/*

COPY --from=pg-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=pg-client /usr/local/lib/libpq.so.5* /usr/local/lib/

RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

WORKDIR /app

COPY sub2api /app/sub2api
COPY resources /app/resources

RUN chown -R sub2api:sub2api /app

USER sub2api

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/sub2api"]
```

- [ ] **Step 2: Create Dockerfile.goreleaser.bootstrap**

```dockerfile
# =============================================================================
# Sub2API Bootstrap — GoReleaser Dockerfile
# =============================================================================
# Packages the pre-built bootstrap binary. No compilation needed.
# =============================================================================

ARG ALPINE_IMAGE=alpine:3.21

FROM ${ALPINE_IMAGE}

LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API Bootstrap - DB Migration & Seeding"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

WORKDIR /app

COPY sub2api-bootstrap /app/sub2api-bootstrap

RUN chown -R sub2api:sub2api /app

USER sub2api

ENTRYPOINT ["/app/sub2api-bootstrap"]
```

- [ ] **Step 3: Commit**

```bash
git add Dockerfile.goreleaser Dockerfile.goreleaser.bootstrap
git commit -m "feat(docker): split GoReleaser Dockerfiles for server and bootstrap"
```

---

### Task 5: Update `.goreleaser.yaml`

**Files:**
- Modify: `.goreleaser.yaml`

- [ ] **Step 1: Update dockers section**

Replace the existing `dockers:` section (the two entries for amd64/arm64) with four entries. Each server entry references only build ID `sub2api` and uses `Dockerfile.goreleaser` with `backend/resources` as extra_files (no `deploy/docker-entrypoint.sh`). Each bootstrap entry references only build ID `sub2api-bootstrap` and uses `Dockerfile.goreleaser.bootstrap` with no extra_files.

The image templates change from:
```
ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}:{{ .Version }}-amd64
```
to:
```
ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-amd64
ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64
```

Full replacement for `dockers:` section:

```yaml
dockers:
  - id: server-amd64
    ids: [sub2api]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-amd64"
    dockerfile: Dockerfile.goreleaser
    extra_files:
      - backend/resources
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: server-arm64
    ids: [sub2api]
    goos: linux
    goarch: arm64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-arm64"
    dockerfile: Dockerfile.goreleaser
    extra_files:
      - backend/resources
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: bootstrap-amd64
    ids: [sub2api-bootstrap]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
    dockerfile: Dockerfile.goreleaser.bootstrap
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: bootstrap-arm64
    ids: [sub2api-bootstrap]
    goos: linux
    goarch: arm64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
    dockerfile: Dockerfile.goreleaser.bootstrap
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"
```

- [ ] **Step 2: Update docker_manifests section**

Replace the existing `docker_manifests:` section (4 manifests) with 8 manifests — 4 for server, 4 for bootstrap:

```yaml
docker_manifests:
  # Server manifests
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:latest"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Major }}.{{ .Minor }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Major }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}-arm64"
  # Bootstrap manifests
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:latest"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Major }}.{{ .Minor }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Major }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
```

- [ ] **Step 3: Update archives files list**

In the `archives:` section, the `files:` list includes `deploy/*` which now mostly contains deleted files. Update it to only include the files that still exist:

```yaml
archives:
  - id: default
    formats:
      - tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    files:
      - LICENSE*
      - README*
```

- [ ] **Step 4: Update release footer**

In the `release:` section's `footer:`, remove the "One-line install" block:

```
    **One-line install (Linux):**
    ```bash
    curl -sSL https://raw.githubusercontent.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}/main/deploy/install.sh | sudo bash
    ```
```

And update the Docker pull example to use the `/server` path:

```
    **Docker:**
    ```bash
    docker pull ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}
    ```
```

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml
git commit -m "feat(release): split GoReleaser into separate server and bootstrap images"
```

---

### Task 6: Update `.goreleaser.simple.yaml`

**Files:**
- Modify: `.goreleaser.simple.yaml`

- [ ] **Step 1: Update dockers section**

Replace the single `dockers:` entry with two entries (server + bootstrap, amd64 only):

```yaml
dockers:
  - id: server-amd64
    ids: [sub2api]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:latest"
    dockerfile: Dockerfile.goreleaser
    extra_files:
      - backend/resources
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: bootstrap-amd64
    ids: [sub2api-bootstrap]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:latest"
    dockerfile: Dockerfile.goreleaser.bootstrap
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"
```

- [ ] **Step 2: Update release footer**

Update the Docker pull command in the footer to use `/server` path:

```
    **Docker (x86_64 only):**
    ```bash
    docker pull ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/server:{{ .Version }}
    ```
```

- [ ] **Step 3: Commit**

```bash
git add .goreleaser.simple.yaml
git commit -m "feat(release): split simple GoReleaser config for separate images"
```

---

### Task 7: Update Helm chart

**Files:**
- Modify: `deploy/helm/sub2api/values.yaml`
- Modify: `deploy/helm/sub2api/templates/deployment.yaml`
- Modify: `deploy/helm/sub2api/templates/bootstrap-job.yaml`

- [ ] **Step 1: Update values.yaml**

Replace the `image:` block (lines 4-10 of `values.yaml`):

```yaml
image:
  # -- Container image repository
  repository: ghcr.io/wchen99998/robust2api
  # -- Image tag (defaults to Chart appVersion)
  tag: ""
  # -- Image pull policy
  pullPolicy: IfNotPresent
```

with:

```yaml
image:
  server:
    # -- Server container image repository
    repository: ghcr.io/wchen99998/sub2api/server
    # -- Image tag (defaults to Chart appVersion)
    tag: ""
    # -- Image pull policy
    pullPolicy: IfNotPresent
  bootstrap:
    # -- Bootstrap container image repository
    repository: ghcr.io/wchen99998/sub2api/bootstrap
    # -- Image tag (defaults to Chart appVersion)
    tag: ""
    # -- Image pull policy
    pullPolicy: IfNotPresent
```

- [ ] **Step 2: Update deployment.yaml**

On line 30, change:

```yaml
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
```

to:

```yaml
          image: "{{ .Values.image.server.repository }}:{{ .Values.image.server.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.server.pullPolicy }}
```

- [ ] **Step 3: Update bootstrap-job.yaml**

On line 27, change:

```yaml
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
```

to:

```yaml
          image: "{{ .Values.image.bootstrap.repository }}:{{ .Values.image.bootstrap.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.bootstrap.pullPolicy }}
```

- [ ] **Step 4: Run Helm render test**

```bash
cd /Users/chenwuhao/Dev/sub2api/deploy/helm/sub2api/tests
bash render-test.sh
```

Expected: All tests pass. The bootstrap job image now points to the bootstrap repository.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/sub2api/values.yaml \
       deploy/helm/sub2api/templates/deployment.yaml \
       deploy/helm/sub2api/templates/bootstrap-job.yaml
git commit -m "feat(helm): split image config into server and bootstrap"
```

---

### Task 8: Update Helm render test

**Files:**
- Modify: `deploy/helm/sub2api/tests/render-test.sh`

- [ ] **Step 1: Add image verification tests**

Add two new tests after the existing test 8 ("Server Deployment exists"), before test 9 ("existingSecret mode works"). Insert these after line 44 (`echo "$RENDERED" | grep -q 'kind: Deployment' && echo "PASS" || { echo "FAIL"; exit 1; }`):

```bash
# Test 9: Deployment uses server image
echo -n "Deployment uses server image... "
echo "$RENDERED" | grep -q 'image:.*sub2api/server:' && echo "PASS" || { echo "FAIL"; exit 1; }

# Test 10: Bootstrap Job uses bootstrap image
echo -n "Bootstrap Job uses bootstrap image... "
echo "$BOOTSTRAP_JOB" | grep -q 'image:.*sub2api/bootstrap:' && echo "PASS" || { echo "FAIL"; exit 1; }
```

Renumber the existing test 9 ("existingSecret mode works") to test 11.

- [ ] **Step 2: Run the render test**

```bash
cd /Users/chenwuhao/Dev/sub2api/deploy/helm/sub2api/tests
bash render-test.sh
```

Expected: All 11 tests pass.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/sub2api/tests/render-test.sh
git commit -m "test(helm): add render tests for split server/bootstrap images"
```

---

### Task 9: Update README files

**Files:**
- Modify: `README.md`
- Modify: `README_CN.md`

- [ ] **Step 1: Update README.md**

Remove the following sections that reference docker-compose and bare-metal deployment:
- The "Quick Start (Linux)" section with the `curl | bash` installer
- The "Uninstall" section with `install.sh uninstall`
- The "Docker Compose Variants" table
- The "Manual Docker Setup" section
- The "Find Admin Password", "Update", "Server Migration", "Common Commands" sections that reference docker-compose
- Update the project structure tree to remove deleted files (`docker-compose.yml`, `.env.example`, `install.sh`)

Keep the Helm/k8s deployment references intact. Keep the Docker pull command but update it to the `/server` image path.

- [ ] **Step 2: Update README_CN.md**

Apply the same removals as README.md (the Chinese version mirrors the English structure).

- [ ] **Step 3: Commit**

```bash
git add README.md README_CN.md
git commit -m "docs: remove docker-compose and bare-metal references from READMEs"
```

---

### Task 10: Final verification

- [ ] **Step 1: Verify no dangling references to deleted files**

```bash
cd /Users/chenwuhao/Dev/sub2api
grep -r "docker-compose\|docker-entrypoint\|build_image\.sh\|\.env\.example\|install\.sh\|sub2api\.service\|deploy/Makefile\|deploy/Dockerfile\|deploy/README\|deploy/DOCKER\|AUTO_SETUP" \
  --include="*.go" --include="*.yaml" --include="*.yml" --include="*.sh" --include="*.md" \
  --include="*.tf" --include="*.tpl" . | grep -v node_modules | grep -v '.git/'
```

Expected: No results (or only irrelevant matches like code comments about setup logic).

- [ ] **Step 2: Verify deleted files are gone**

```bash
ls deploy/docker-compose* deploy/docker-entrypoint.sh deploy/build_image.sh \
   deploy/.env.example deploy/install.sh deploy/DOCKER.md deploy/README.md \
   deploy/Makefile deploy/Dockerfile backend/Dockerfile 2>&1
```

Expected: All "No such file or directory" errors.

- [ ] **Step 3: Verify Helm render test passes**

```bash
cd /Users/chenwuhao/Dev/sub2api/deploy/helm/sub2api/tests
bash render-test.sh
```

Expected: All tests pass.

- [ ] **Step 4: Verify both Dockerfiles build**

```bash
cd /Users/chenwuhao/Dev/sub2api
docker build -t sub2api-server:verify .
docker build -t sub2api-bootstrap:verify -f Dockerfile.bootstrap .
```

Expected: Both build successfully.

- [ ] **Step 5: Verify GoReleaser config is valid**

```bash
cd /Users/chenwuhao/Dev/sub2api
goreleaser check
goreleaser check --config=.goreleaser.simple.yaml
```

Expected: Both pass validation (if goreleaser is installed locally; skip if not).
