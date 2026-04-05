# Sub2API Kubernetes Helm Chart Design

**Date:** 2026-04-05
**Status:** Draft
**Goal:** Deploy Sub2API to Kubernetes via a Helm chart, targeting staging/testing first with a clear path to production.

## Context

Sub2API currently deploys via Docker Compose (3 services: Go app, PostgreSQL, Redis). There are no Kubernetes manifests or Helm charts. The project needs a robust K8s deployment for staging validation, with production-readiness as a follow-up.

**Local test environment:** OrbStack Kubernetes (v1.33.5, single node, `local-path` StorageClass, no ingress controller pre-installed).

## Approach

Monorepo Helm chart with Bitnami PostgreSQL and Redis as optional subcharts. One `helm install` deploys the full stack for testing; subcharts are disabled when using external managed services in production.

## Chart Structure

```
deploy/helm/sub2api/
├── Chart.yaml              # Metadata + subchart dependencies
├── values.yaml             # Defaults (local/staging)
├── values-production.yaml  # Production overrides
├── templates/
│   ├── _helpers.tpl        # Template helpers (names, labels, selectors)
│   ├── deployment.yaml     # App deployment
│   ├── service.yaml        # ClusterIP service
│   ├── ingress.yaml        # Ingress resource (optional TLS)
│   ├── configmap.yaml      # Non-sensitive configuration
│   ├── secret.yaml         # Sensitive values
│   ├── serviceaccount.yaml # Optional service account
│   └── NOTES.txt           # Post-install instructions
├── charts/                 # Subchart tarballs (gitignored)
└── Chart.lock              # Locked dependency versions
```

## Dependencies

Defined in `Chart.yaml`:

| Subchart | Repository | Default | Purpose |
|----------|-----------|---------|---------|
| `bitnami/postgresql` | `oci://registry-1.docker.io/bitnamicharts` | Enabled | In-cluster PostgreSQL |
| `bitnami/redis` | `oci://registry-1.docker.io/bitnamicharts` | Enabled | In-cluster Redis |

Both toggleable via `postgresql.enabled` and `redis.enabled`.

## Image

Uses existing `ghcr.io/wchen99998/sub2api:latest`. Overridable:

```yaml
image:
  repository: ghcr.io/wchen99998/sub2api
  tag: latest
  pullPolicy: IfNotPresent
```

## Configuration & Secrets

All configuration uses environment variables, matching the app's existing Viper-based config.

### ConfigMap (non-sensitive)

- `SERVER_HOST`, `SERVER_PORT`, `SERVER_MODE`, `RUN_MODE`
- Database connection: host, port, dbname, sslmode, pool settings
- Redis connection: host, port, db, pool settings
- `TZ`, security URL allowlist settings, `UPDATE_PROXY_URL`
- `AUTO_SETUP=true`

### Secret (sensitive)

- `DATABASE_PASSWORD`, `REDIS_PASSWORD`
- `JWT_SECRET`, `TOTP_ENCRYPTION_KEY`
- `ADMIN_EMAIL`, `ADMIN_PASSWORD`
- OAuth secrets (Gemini client secrets)

### Best Practices

- Secrets referenced via `secretKeyRef` in pod spec, never in ConfigMap
- Support `existingSecret` — users can pre-create a K8s Secret (e.g., via external-secrets or sealed-secrets) and reference it by name instead of having the chart create one
- Secret values in `values.yaml` default to empty; production values passed via `--set` or a separate values file (never committed)
- ConfigMap and Secret checksums annotated on the Deployment so pods restart on config changes

## Deployment

- **Replicas:** 1 (default, overridable via `replicaCount`)
- **Container port:** 8080
- **Probes:**
  - Startup: HTTP GET `/health`, `failureThreshold: 30`, `periodSeconds: 2` (up to 60s startup)
  - Liveness: HTTP GET `/health`, `periodSeconds: 30`, `timeoutSeconds: 10`
  - Readiness: HTTP GET `/health`, `periodSeconds: 10`, `timeoutSeconds: 5`
- **Resources (defaults):**
  - Requests: 100m CPU, 256Mi memory
  - Limits: 500m CPU, 512Mi memory
- **Security context:**
  - Pod: `fsGroup: 1000`
  - Container: `runAsNonRoot: true`, `runAsUser: 1000`, `readOnlyRootFilesystem: false` (app writes to `/app/data`)
- **Termination:** `terminationGracePeriodSeconds: 30` (app handles SIGTERM)
- **Labels:** Standard `app.kubernetes.io/*` labels (name, instance, version, managed-by)
- **Volume:** PVC for `/app/data` using default StorageClass. Controlled via:
  ```yaml
  persistence:
    enabled: true
    size: 1Gi
    storageClass: ""  # uses cluster default
    accessModes: [ReadWriteOnce]
  ```

## Service

- Type: `ClusterIP` (default)
- Port: 80 -> targetPort 8080
- Switchable to `NodePort` or `LoadBalancer` via `service.type`

## Ingress

### Local/Staging (default)

```yaml
ingress:
  enabled: true
  className: ""          # cluster default
  host: sub2api.local
  annotations: {}
  tls:
    enabled: false
    secretName: ""
```

- No TLS, HTTP only
- Host-based routing to app service

### Production (values-production.yaml)

```yaml
ingress:
  host: sub2api.example.com
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  tls:
    enabled: true
```

- cert-manager handles TLS certificate provisioning
- TLS secretName auto-generated from host if not specified

### Ingress Controller Note

OrbStack does not include an ingress controller. NOTES.txt will remind users to install one (e.g., `helm install ingress-nginx ingress-nginx/ingress-nginx`) or use `kubectl port-forward` for quick testing.

## Subchart Configuration & External Service Toggle

### PostgreSQL

**In-cluster (default):**

```yaml
postgresql:
  enabled: true
  auth:
    username: sub2api
    password: ""        # set via --set or external secret
    database: sub2api
  primary:
    persistence:
      size: 1Gi
```

**External (production):**

```yaml
postgresql:
  enabled: false

externalDatabase:
  host: ""
  port: 5432
  user: sub2api
  password: ""
  database: sub2api
  sslmode: require
```

### Redis

**In-cluster (default):**

```yaml
redis:
  enabled: true
  auth:
    enabled: true
    password: ""        # set via --set or external secret
  architecture: standalone
  master:
    persistence:
      size: 1Gi
```

**External (production):**

```yaml
redis:
  enabled: false

externalRedis:
  host: ""
  port: 6379
  password: ""
  enableTLS: false
```

### Template Wiring

ConfigMap and Secret templates use conditional logic:
- `if .Values.postgresql.enabled` -> point at subchart service (`{{ .Release.Name }}-postgresql`)
- else -> use `externalDatabase.*` values
- Same pattern for Redis

## Testing Plan

1. `helm dependency build` to fetch subcharts
2. `helm install sub2api ./deploy/helm/sub2api --set postgresql.auth.password=test --set redis.auth.password=test` on local OrbStack cluster
3. Verify all 3 pods running and healthy
4. `kubectl port-forward svc/sub2api 8080:80` and hit `http://localhost:8080/health`
5. Verify app can connect to in-cluster PostgreSQL and Redis
6. Test `helm upgrade` with a config change and verify pod restart

## Deferred (Future Iterations)

- Horizontal Pod Autoscaling (HPA)
- Pod Disruption Budgets (PDB)
- PostgreSQL backup CronJob
- Prometheus/Grafana monitoring
- Resource tuning based on real usage data
- NetworkPolicy for pod-to-pod isolation
