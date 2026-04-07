# Phase 2: Split Runtime into `api` and `worker`

## Summary

Split the current monolithic server binary (`cmd/server/`) into two separate binaries — `cmd/api/` and `cmd/worker/` — with separate Docker images, separate Helm deployments, and compile-time enforced role boundaries via distinct Wire injectors. This enables horizontal scaling of the API tier independently from the singleton worker.

Clean break: `cmd/server/`, its Dockerfile, and the single-deployment Helm template are deleted and replaced.

## Service Classification

### API role (`cmd/api/`)

**HTTP stack (API-exclusive):**
- `handler.ProviderSet` — all HTTP handlers
- `middleware.ProviderSet` — JWT, API key auth, CORS, rate limiting
- `server.ProviderSet` — Gin router, HTTP server
- Frontend asset embedding via `//go:embed`

**Request-path async continuations (stay in API because they process API-generated work):**
- `BillingCacheService` — 10 cache-write workers draining request-path enqueues
- `UsageRecordWorkerPool` — 128–512 workers processing post-request usage records
- `EmailQueueService` — 3 workers sending API-triggered emails

**Cache invalidation (must run on every API instance):**
- `APIKeyAuthCacheInvalidator` — Redis Pub/Sub subscriber for L1 cache consistency across replicas

**Read-side dependencies (constructed, no `.Start()`):**
- `PricingService` — `.Initialize()` loads data into memory, no update scheduler
- `SchedulerSnapshotService` — read-only access to Redis snapshots, no background rebuild
- `ConcurrencyService` — slot acquire/release only, no cleanup loop

### Worker role (`cmd/worker/`)

**Maintenance loops (singleton, no HTTP serving):**
- `TokenRefreshService` — background OAuth token refresh (5min interval)
- `DashboardAggregationService` — stats aggregation
- `UsageCleanupService` — old usage record cleanup
- `AccountExpiryService` — expired account handling
- `SubscriptionExpiryService` — expired subscription handling
- `IdempotencyCleanupService` — stale idempotency key cleanup
- `BackupService` — scheduled backups
- `ScheduledTestRunnerService` — scheduled test execution

**Data refresh loops (keep caches fresh for API reads):**
- `SchedulerSnapshotService` — 3 goroutines: initial rebuild, outbox polling (5s), full rebuild (1h)
- `PricingService` — periodic pricing sync from LiteLLM (10min)
- `ConcurrencyService` — stale slot cleanup worker
- `UserMessageQueueService` — cleanup worker

## Wire Provider Set Restructuring

### New provider set structure in `service/wire.go`

```
service.SharedProviderSet     — constructors only, no Start() calls
service.APIProviderSet        — SharedProviderSet + API-specific providers with Start()
service.WorkerProviderSet     — SharedProviderSet + worker-specific providers with Start()
```

**`SharedProviderSet`** contains all pure constructors — services like `NewGatewayService`, `NewAccountService`, `NewSubscriptionService`, etc. that don't start goroutines. This is the bulk of the ~83 providers.

**`APIProviderSet`** adds:
- `ProvideAPIBillingCacheService` — constructs + starts 10 cache-write workers
- `ProvideAPIUsageRecordWorkerPool` — constructs + starts worker pool
- `ProvideAPIEmailQueueService` — constructs + starts 3 email workers
- `ProvideAPIKeyAuthCacheInvalidator` — starts Pub/Sub subscriber
- `ProvidePricingServiceReadOnly` — calls `.Initialize()` only, no update scheduler
- `ProvideSchedulerSnapshotReadOnly` — constructs without calling `.Start()`

**`WorkerProviderSet`** adds:
- `ProvideTokenRefreshService` — with `.Start()`
- `ProvideDashboardAggregationService` — with `.Start()`
- `ProvideSchedulerSnapshotService` — with `.Start()` (all 3 background goroutines)
- `ProvidePricingServiceFull` — `.Initialize()` + update scheduler
- `ProvideUsageCleanupService` — with `.Start()`
- `ProvideAccountExpiryService` — with `.Start()`
- `ProvideSubscriptionExpiryService` — with `.Start()`
- `ProvideIdempotencyCleanupService` — with `.Start()`
- `ProvideBackupService` — with `.Start()`
- `ProvideScheduledTestRunnerService` — with `.Start()`
- `ProvideConcurrencyCleanupWorker` — with `.StartSlotCleanupWorker()`
- `ProvideUserMessageQueueCleanup` — with `.StartCleanupWorker()`

### Wire injectors

```go
// cmd/api/wire.go
func initializeAPIApplication(buildInfo handler.BuildInfo) (*APIApplication, error) {
    wire.Build(
        config.ProviderSet,
        appelotel.ProviderSet,
        repository.ProviderSet,
        service.APIProviderSet,
        middleware.ProviderSet,
        handler.ProviderSet,
        server.ProviderSet,
        provideAPICleanup,
        wire.Struct(new(APIApplication), "*"),
    )
    return nil, nil
}

// cmd/worker/wire.go
func initializeWorkerApplication(buildInfo handler.BuildInfo) (*WorkerApplication, error) {
    wire.Build(
        config.ProviderSet,
        appelotel.ProviderSet,
        repository.ProviderSet,
        service.WorkerProviderSet,
        provideWorkerCleanup,
        wire.Struct(new(WorkerApplication), "*"),
    )
    return nil, nil
}
```

Worker has no `handler`, `middleware`, or `server` provider sets.

Note: `SharedProviderSet` includes constructors for services like `GatewayService` that the worker doesn't actively use. Some worker services depend on shared services transitively (e.g., `TokenRefreshService` needs account repositories). Constructing unused services is negligible overhead and avoids maintaining a minimal shared subset.

## Entry Points and Lifecycle

### `cmd/api/main.go`

1. Init logger, load config
2. `initializeAPIApplication(buildInfo)` — Wire builds the API graph
3. Start HTTP server on configured port
4. Start metrics server (if OTel enabled)
5. Register `/livez`, `/readyz`, `/startupz` probes
6. Wait for SIGINT/SIGTERM
7. Graceful shutdown (5s grace period):
   - Stop accepting new HTTP connections
   - Drain in-flight requests
   - Stop UsageRecordWorkerPool (flush pending)
   - Stop BillingCacheService (flush pending writes)
   - Stop EmailQueueService
   - Stop Pub/Sub subscriber
   - Close Redis, Ent

### `cmd/worker/main.go`

1. Init logger, load config
2. `initializeWorkerApplication(buildInfo)` — Wire builds the worker graph
3. Start internal health HTTP server on `worker.health_port` (default 8081)
   - Serves `/livez`, `/readyz`, `/startupz` only
   - Minimal `net/http` server, no Gin
4. Wait for SIGINT/SIGTERM
5. Graceful shutdown (30s grace period):
   - Signal all background loops to stop (context cancellation)
   - Wait for in-flight work (snapshot rebuild, token refresh cycle, backup)
   - Stop all services in parallel with 10s per-service timeout
   - Stop health server
   - Close Redis, Ent

### Health probes

| Endpoint | API behavior | Worker behavior |
|----------|-------------|-----------------|
| `/livez` | Always 200 | Always 200 |
| `/readyz` | 200 if PostgreSQL + Redis reachable, 503 otherwise | Same |
| `/startupz` | 200 after router is ready | 200 after all `.Start()` calls return |
| `/health` | Alias for `/readyz` (transitional, remove later) | Not served |

## Build and Release

### Directory layout

```
cmd/
  api/
    main.go
    wire.go
    wire_gen.go
  worker/
    main.go
    wire.go
    wire_gen.go
  bootstrap/          — unchanged
  server/             — deleted
```

### Docker

```
Dockerfile.api        — replaces Dockerfile (includes frontend asset build)
Dockerfile.worker     — new, minimal Alpine (similar to Dockerfile.bootstrap)
Dockerfile.bootstrap  — unchanged
Dockerfile            — deleted
```

### GoReleaser (`.goreleaser.yaml`)

Replace the `sub2api` server build with:
- `sub2api-api` from `cmd/api/`
- `sub2api-worker` from `cmd/worker/`

Keep `sub2api-bootstrap` unchanged. 3 binaries, 3 images, 6 Docker manifests (amd64+arm64 each).

### Makefile

- `make build` → builds `bin/api` and `bin/worker`
- `make build-api` / `make build-worker` for individual builds
- `make generate` → runs Wire for both `cmd/api/` and `cmd/worker/`

## Kubernetes Packaging

### Helm templates

```
templates/
  api-deployment.yaml        — replaces deployment.yaml
  api-service.yaml           — replaces service.yaml
  api-ingress.yaml           — replaces ingress.yaml
  api-hpa.yaml               — new, API-only autoscaling
  api-pdb.yaml               — new, API-only disruption budget
  worker-deployment.yaml     — new, no Service/Ingress
  bootstrap-job.yaml         — unchanged
```

### `values.yaml` structure

```yaml
api:
  image:
    repository: ghcr.io/wchen99998/sub2api/api
    tag: ""
  replicaCount: 1
  resources:
    requests: { cpu: 250m, memory: 512Mi }
    limits: { cpu: "2", memory: 2Gi }
  autoscaling:
    enabled: false
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 70
  pdb:
    enabled: false
    minAvailable: 1
  terminationGracePeriodSeconds: 5
  probes:
    startup: { path: /startupz, failureThreshold: 30, periodSeconds: 2 }
    liveness: { path: /livez, periodSeconds: 30 }
    readiness: { path: /readyz, periodSeconds: 10 }

worker:
  image:
    repository: ghcr.io/wchen99998/sub2api/worker
    tag: ""
  replicaCount: 1
  resources:
    requests: { cpu: 100m, memory: 256Mi }
    limits: { cpu: "1", memory: 1Gi }
  terminationGracePeriodSeconds: 30
  probes:
    startup: { path: /startupz, port: health, failureThreshold: 30, periodSeconds: 2 }
    liveness: { path: /livez, port: health, periodSeconds: 30 }
    readiness: { path: /readyz, port: health, periodSeconds: 10 }

bootstrap:
  # unchanged
```

Worker has no Service or Ingress. Health port exposed only to kubelet probes via `containerPort`.

## Test Plan

### Unit tests

- **Role startup isolation**: API binary starts HTTP server, does NOT start any worker loops. Worker binary starts all background loops, does NOT bind main HTTP port.
- **Provider construction purity**: `SharedProviderSet` constructors have zero side effects — no goroutines, no `.Start()` calls. Use `goleak` to verify no goroutine leaks from construction alone.
- **Probe tests**: `/readyz` returns 503 when DB or Redis unreachable, 200 when healthy. `/livez` always 200. `/startupz` returns 503 before init, 200 after.

### Integration tests

- **Bootstrap -> API**: Run bootstrap, start API, verify requests served and probes pass.
- **Bootstrap -> Worker**: Run bootstrap, start worker, verify background loops run (scheduler snapshot rebuilds into Redis).
- **Bootstrap -> API x2 + Worker x1**: Two API instances + one worker, verify cache invalidation propagates and sticky sessions work.

### Helm render tests

- Default render produces: bootstrap Job, api Deployment + Service + Ingress, worker Deployment (no Service/Ingress).
- API gets HPA + PDB when enabled.
- Worker defaults to `replicaCount: 1`.
- Correct image repositories, probe paths, ports, and termination grace periods.

### Multi-replica API regression

- API-key cache invalidation across instances via Pub/Sub.
- Sticky session routing with `session_id` across replicas.
- Request-path OAuth refresh doesn't race across instances.

## Assumptions

- `cmd/server/` is deleted in this branch (clean break).
- Worker is singleton in this phase (`replicaCount: 1`).
- API is the first horizontally scalable role.
- One codebase, three images (api, worker, bootstrap).
- Worker health model is a private HTTP listener — no public Service or Ingress.
