# Kubernetes Deployment Guide

Sub2API runs on DigitalOcean Kubernetes (DOKS) with a clean ownership split:

- **Terraform** manages cloud resources: DOKS cluster, optional managed PostgreSQL, optional R2 storage buckets, and Cloudflare API token bootstrap secrets.
- **Flux CD** manages everything inside the cluster: ingress-nginx, cert-manager, ExternalDNS, the Sub2API application, and the monitoring stack (LGTM) — all via GitOps.

All in-cluster changes are made by editing YAML files in `clusters/production/`, committing, and pushing. Flux reconciles automatically within 1 minute.

```
Cloudflare (DNS managed by ExternalDNS, proxied)
    |
DO Load Balancer (TLS passthrough)
    |
ingress-nginx (TLS via cert-manager DNS-01 / Let's Encrypt)
    |
Sub2API pods (namespace: sub2api)
    +-- Redis (in-cluster Bitnami, standalone)
    +-- PostgreSQL (in-cluster Bitnami or external DO Managed)

Monitoring (namespace: monitoring)
    +-- Prometheus + Alertmanager (metrics)
    +-- Grafana (dashboards)
    +-- Tempo (traces -> R2)
    +-- Loki (logs -> R2)
    +-- Alloy (gRPC OTLP receiver)

Flux CD (namespace: flux-system)
    +-- Watches: clusters/production/ in this Git repo
    +-- Reconciles: infrastructure -> cert-manager-issuers -> monitoring -> apps -> grafana-apps
```

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.7
- [doctl](https://docs.digitalocean.com/reference/doctl/how-to/install/) (DigitalOcean CLI)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/) >= 3
- [Flux CLI](https://fluxcd.io/flux/installation/#install-the-flux-cli) (`brew install fluxcd/tap/flux`)
- A DigitalOcean API token
- A Cloudflare API token with DNS edit permissions
- A GitHub personal access token with `repo` scope (for Flux bootstrap)

## Deployment Ownership

### Terraform manages (`infra/production/`)

| Resource | Purpose |
|----------|---------|
| `module.doks` | DOKS cluster + autoscaling node pool |
| `kubernetes_namespace.cert_manager` | cert-manager namespace |
| `kubernetes_namespace.external_dns` | external-dns namespace |
| `kubernetes_secret.cloudflare_cert_manager` | Cloudflare API token for cert-manager DNS-01 |
| `kubernetes_secret.cloudflare_external_dns` | Cloudflare API token for ExternalDNS |
| `module.storage[0]` (optional) | R2 buckets for Tempo and Loki |
| `module.database[0]` (optional) | DO Managed PostgreSQL |

### Flux manages (`clusters/production/`)

| Kustomization | Path | Resources |
|--------------|------|-----------|
| `infrastructure` | `clusters/production/infrastructure/` | ingress-nginx, cert-manager, external-dns, namespaces, Helm sources |
| `cert-manager-issuers` | `clusters/production/infrastructure/issuers/` | ClusterIssuer (depends on cert-manager CRDs) |
| `monitoring` | `clusters/production/monitoring/` | Monitoring HelmRelease (Prometheus, Grafana, Tempo, Loki, Alloy) |
| `apps` | `clusters/production/apps/` | Sub2API HelmRelease (gateway, control, worker, bootstrap, Grafana datasource, in-cluster DB reader-role provisioning) |
| `grafana-apps` | `clusters/production/grafana-apps/` | Sub2API Grafana dashboards (post-app provisioning) |

Dependency chain: `infrastructure` -> `cert-manager-issuers` -> `monitoring` -> `apps` -> `grafana-apps`.

## 1. Provision Infrastructure

```bash
cd infra/production
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars`:

```hcl
do_token             = "dop_v1_..."
region               = "sgp1"
cluster_name         = "sub2api"
k8s_version          = "1.34"
node_size            = "s-2vcpu-4gb"
min_nodes            = 2
max_nodes            = 3
cloudflare_api_token = "..."

# Optional: R2 storage for Tempo/Loki
# enable_observability_storage = true
# cloudflare_account_id        = "..."
```

```bash
terraform init
terraform apply
eval "$(terraform output -raw kubeconfig_command)"
```

## 2. Create Secrets

Flux HelmReleases reference secrets via `valuesFrom`. Create them before bootstrapping Flux.

### Image Pull Secret (for private GHCR)

```bash
kubectl create secret docker-registry ghcr-pull \
  -n sub2api \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-pat-with-read-packages>
```

### Sub2API Secrets

```bash
kubectl create namespace sub2api --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: sub2api-secrets
  namespace: sub2api
type: Opaque
stringData:
  secrets.jwtSecret: "<random-32-char>"
  secrets.totpEncryptionKey: "<64-hex-char, generate with: openssl rand -hex 32>"
  secrets.adminPassword: "<admin-password>"
  postgresql.auth.postgresPassword: "<postgres-admin-password or empty if postgresql.enabled=false>"
  postgresql.auth.password: "<db-password>"
  redis.auth.password: "<redis-password>"
  externalDatabase.password: ""
  externalRedis.password: ""
  grafanaProvisioning.reader.username: "grafana_reader"
  grafanaProvisioning.reader.password: "<grafana-read-only-db-password>"
EOF
```

If using external PostgreSQL/Redis, set `postgresql.enabled=false` / `redis.enabled=false` in `clusters/production/apps/sub2api.yaml` and put the real passwords in `externalDatabase.password` / `externalRedis.password`. Keep `postgresql.auth.postgresPassword` empty in that mode.

If you provision DigitalOcean Managed PostgreSQL via Terraform, copy the `grafana_reader_user` / `grafana_reader_password` outputs into `grafanaProvisioning.reader.username` / `grafanaProvisioning.reader.password`.

### Monitoring Secrets

```bash
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: monitoring-secrets
  namespace: monitoring
type: Opaque
stringData:
  kube-prometheus-stack.grafana.adminPassword: "<grafana-password>"
  tempo.tempo.storage.trace.s3.access_key: "<r2-access-key>"
  tempo.tempo.storage.trace.s3.secret_key: "<r2-secret-key>"
  loki.loki.storage.s3.accessKeyId: "<r2-access-key>"
  loki.loki.storage.s3.secretAccessKey: "<r2-secret-key>"
EOF
```

## 3. Configure Flux Manifests

Edit `clusters/production/` files with your production values before bootstrapping:

| File | What to set |
|------|-------------|
| `infrastructure/issuers/cluster-issuer.yaml` | `email` (Let's Encrypt) |
| `infrastructure/external-dns.yaml` | `domainFilters`, `txtOwnerId`; uncomment `extraArgs: [--cloudflare-proxied]` if using CF proxy |
| `monitoring/monitoring.yaml` | `public.baseDomain`, `grafanaIngress.enabled`, optional `grafanaIngress.host`, R2 endpoints/buckets |
| `monitoring.yaml` | Set `spec.suspend: false` to enable monitoring |
| `apps/sub2api.yaml` | Image tags, `public.baseDomain`, public URL scheme/host overrides, ingress TLS/override settings, resource limits, `observability` settings, `grafanaProvisioning` enablement |
| `grafana-apps/grafana-apps.yaml` | Dashboard release values if you need to override the monitoring namespace |

By default, public ingress hosts follow the shared `service-namespace.domain`
convention. For example:

- `gateway-sub2api.<baseDomain>` for the API gateway
- `app-sub2api.<baseDomain>` for the control/frontend app
- `grafana-monitoring.<baseDomain>` for Grafana

Set explicit `host` or URL override fields only when you intentionally want a
shared or vanity endpoint instead of the convention-derived default.

Public URL schemes are configured separately from ingress TLS so deployments
that terminate HTTPS at Cloudflare, a load balancer, or another proxy can still
publish `https://` URLs correctly. If Grafana uses an explicit
`monitoring.grafanaIngress.host`, mirror that hostname in
`apps/sub2api.yaml` under `public.grafana.host` unless you set a full
`config.grafanaUrl` override there instead.

## 4. Bootstrap Flux

One-time setup. Installs Flux controllers and connects this repo.

```bash
export GITHUB_TOKEN=<your-github-pat>

flux bootstrap github \
  --owner=<github-org-or-user> \
  --repository=robust2api \
  --branch=main \
  --path=clusters/production \
  --personal
```

Flux will install its controllers, create a GitRepository source, and start reconciling in dependency order.

## 5. Verify

```bash
# Flux status
flux get kustomizations
flux get helmreleases -A
flux get sources git

# All HelmReleases should show Ready: True
# Example output:
# NAME          REVISION  READY  MESSAGE
# cert-manager  v1.17.1   True   Helm upgrade succeeded...
# external-dns  1.16.1    True   Helm upgrade succeeded...
# ingress-nginx 4.12.1    True   Helm upgrade succeeded...
# monitoring    0.1.0+... True   Helm upgrade succeeded...
# sub2api       0.2.0+... True   Helm upgrade succeeded...
# grafana-apps  0.1.0+... True   Helm upgrade succeeded...

# Pods
kubectl get pods -n sub2api
kubectl get pods -n monitoring
kubectl get pods -n ingress-nginx
kubectl get pods -n cert-manager

# Certificates
kubectl get certificates -A

# Ingress
kubectl get ingress -A
```

## Day-2 Operations

### Deploy a New Version

1. Tag and push to trigger CI image builds:

```bash
git tag v0.4.0
git push origin v0.4.0
```

2. Update image tags in `clusters/production/apps/sub2api.yaml`:

```yaml
image:
  gateway:
    tag: "0.4.0"
  control:
    tag: "0.4.0"
  frontend:
    tag: "0.4.0"
  worker:
    tag: "0.4.0"
  bootstrap:
    tag: "0.4.0"
```

3. Commit and push — Flux deploys automatically:

```bash
git add clusters/production/apps/sub2api.yaml
git commit -m "deploy: v0.4.0"
git push
```

### Roll Back

```bash
git revert HEAD
git push
```

### Upgrade Infrastructure Components

Edit the chart version in the HelmRelease file:

```bash
# Example: upgrade ingress-nginx
# Edit clusters/production/infrastructure/ingress-nginx.yaml
# Change: version: "4.12.1" -> version: "4.13.0"
git commit -am "infra: upgrade ingress-nginx to 4.13.0"
git push
```

### Force Immediate Reconciliation

```bash
flux reconcile source git flux-system          # Fetch latest git
flux reconcile kustomization apps              # Reconcile apps layer
flux reconcile helmrelease sub2api -n sub2api  # Reconcile specific release
flux reconcile helmrelease monitoring -n monitoring
```

### Suspend / Resume Reconciliation

```bash
flux suspend kustomization monitoring    # Pause monitoring
flux resume kustomization monitoring     # Resume monitoring
```

## Monitoring

### Dashboards

Four Sub2API dashboards are provisioned automatically via the `grafana-apps` HelmRelease after both the monitoring and app layers are ready:

| Dashboard | Datasource | Description |
|-----------|-----------|-------------|
| **Admin Overview** | PostgreSQL | User/key/account counts, request/token/cost stats, trends |
| **Admin Usage** | PostgreSQL | Hourly requests/tokens, daily cost, model spend, group/user breakdown |
| **Runtime** | Prometheus | RPS, error rate, p95 latency, p95 TTFT, token throughput, upstream errors, failovers, rate limit rejections, queue depth, active accounts |
| **Resources** | Prometheus | Goroutines, memory usage (stack/other), GC pause duration |

The Runtime dashboard metrics (`sub2api_*`) only populate when real authenticated API traffic flows through the gateway. Until then, panels show "No data" — this is expected.

### Observability Pipeline

```
Sub2API pods --OTLP/gRPC--> Alloy (port 4317) ---> Tempo (traces -> R2)
                                                 |-> Loki (logs -> R2)
Sub2API pods --/metrics--> Prometheus (scrape via ServiceMonitor)
```

Configuration in `clusters/production/apps/sub2api.yaml`:

```yaml
observability:
  enabled: true
  otel:
    serviceName: sub2api
    endpoint: "monitoring-alloy.monitoring.svc:4317"
    traceSampleRate: "0.1"
    metricsPort: 9090
  serviceMonitor:
    enabled: true
    interval: 15s
```

### Grafana Datasources

Provisioned automatically:
- `monitoring` provisions Prometheus, Loki, Tempo, and Alertmanager.
- `sub2api` provisions the Sub2API PostgreSQL datasource into the monitoring namespace.
- In in-cluster PostgreSQL mode, `sub2api` also reconciles the `grafana_reader` role after install/upgrade.

## Troubleshooting

### HelmRelease Not Ready

```bash
kubectl describe helmrelease <name> -n <namespace>
helm history <name> -n <namespace>
flux reconcile helmrelease <name> -n <namespace>
```

### Source Not Syncing

```bash
flux get sources git -A
flux reconcile source git flux-system
```

### Pods Pending (Insufficient Resources)

The cluster uses `s-2vcpu-4gb` nodes. During rolling updates, both old and new pods must coexist. If pods are Pending:

```bash
kubectl describe pod <pod> -n <namespace>  # Check Events for scheduling failures
kubectl get nodes -o wide                  # Check node count
```

Options:
- Increase `max_nodes` in `terraform.tfvars` and `terraform apply`
- Reduce resource requests in the HelmRelease values
- The sub2api chart uses `preferredDuringSchedulingIgnoredDuringExecution` anti-affinity, so pods can land on the PostgreSQL node if needed

### ImagePullBackOff

- Verify `ghcr-pull` secret: `kubectl get secret ghcr-pull -n sub2api`
- Check token has `read:packages` scope
- Verify image exists: `docker manifest inspect ghcr.io/wchen99998/robust2api/gateway:<tag>`

### Certificate Not Issuing

```bash
kubectl describe certificate <name> -n <namespace>
kubectl describe challenge -A
kubectl logs -n cert-manager deploy/cert-manager
```

Common cause: Cloudflare API token missing DNS edit permission.

### Bootstrap Job CrashLoop

The bootstrap job runs DB migrations and may CrashLoop briefly while PostgreSQL starts. Check:

```bash
kubectl logs -n sub2api -l app.kubernetes.io/component=bootstrap --tail=50
```

### Flux Controller Logs

```bash
flux logs --level=error
flux logs --kind=HelmRelease --name=sub2api --namespace=sub2api
```
