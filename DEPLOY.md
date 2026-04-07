# Kubernetes Deployment Guide

Deploy Sub2API on DigitalOcean Kubernetes (DOKS) with a clean ownership split:

- Terraform manages cluster-level infrastructure: DOKS, ingress-nginx, cert-manager, ExternalDNS, optional managed PostgreSQL, optional R2 buckets, and optional monitoring.
- Helm manages the `sub2api` application release in the `sub2api` namespace.

Do not manage the `sub2api` application Helm release through Terraform. Keep app rollouts, rollbacks, and image updates in Helm so application deployment remains independent from infrastructure reconciliation.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.7
- [doctl](https://docs.digitalocean.com/reference/doctl/how-to/install/) (DigitalOcean CLI)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/) >= 3
- A DigitalOcean API token ([create one](https://cloud.digitalocean.com/account/api/tokens))
- A Cloudflare API token with DNS edit permissions ([create one](https://dash.cloudflare.com/profile/api-tokens))
- Your Cloudflare zone ID (found on your domain's overview page)

## Deployment Ownership

Use Terraform for:
- DOKS cluster lifecycle
- ingress-nginx, cert-manager, ExternalDNS
- Cloudflare-backed DNS/certificate bootstrap
- Optional external DO managed PostgreSQL
- Optional R2 buckets for Tempo and Loki
- Optional monitoring stack in `monitoring`

Use Helm for:
- Installing `sub2api`
- Bundled PostgreSQL in the `sub2api` namespace
- Bundled Redis in the `sub2api` namespace
- Upgrading `sub2api` image versions
- Switching the app between bundled and external PostgreSQL/Redis
- App-level rollback and runtime tuning

## 1. Provision Infrastructure

```bash
cd infra/production

# Create your config from the example
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars` with your values:

```hcl
# DigitalOcean
do_token     = "dop_v1_..."
region       = "sgp1"
cluster_name = "sub2api"
k8s_version  = "1.34"

# Kubernetes bootstrap
letsencrypt_email = "admin@yourdomain.com"

# Cloudflare DNS
cloudflare_api_token = "..."
cloudflare_zone_id   = "..."
# DNS convention: <service>-<namespace>.<suffix>
# e.g. with suffix "do-prod.yourdomain.com", the default Ingress host
# becomes "api-sub2api.do-prod.yourdomain.com"
domain_suffix        = "do-prod.yourdomain.com"
cloudflare_proxied   = true

# Optional: managed PostgreSQL (default false, uses in-cluster Bitnami PG)
# enable_managed_database = true
```

> **Important:** Check available Kubernetes versions before deploying:
> ```bash
> curl -s -X GET "https://api.digitalocean.com/v2/kubernetes/options" \
>   -H "Authorization: Bearer $DO_TOKEN" | python3 -c \
>   "import sys,json; d=json.load(sys.stdin); [print(v['slug']) for v in d['options']['versions']]"
> ```

### Staged Apply (Required)

Terraform must be applied in stages because `kubernetes_manifest` (ClusterIssuer) requires the CRDs from cert-manager, which requires the cluster to exist first.

```bash
terraform init

# Stage 1: Create the DOKS cluster
terraform apply -target=module.doks

# Stage 2: Install ingress-nginx, cert-manager, ExternalDNS, and app namespace
terraform apply \
  -target=module.kubernetes.kubernetes_namespace.app \
  -target=module.kubernetes.helm_release.ingress_nginx \
  -target=module.kubernetes.helm_release.cert_manager \
  -target=module.kubernetes.helm_release.external_dns

# Stage 3: Create ClusterIssuer and remaining resources
terraform apply
```

This creates:
- DOKS cluster with autoscaling (1-3 nodes)
- ingress-nginx controller + DO load balancer
- cert-manager with Let's Encrypt ClusterIssuer (DNS-01 via Cloudflare)
- ExternalDNS (auto-creates Cloudflare DNS records from Ingress resources)
- `sub2api` namespace

## 2. Configure kubectl

```bash
doctl auth init
doctl kubernetes cluster kubeconfig save sub2api
kubectl get nodes     # verify connectivity
```

## 3. Deploy Sub2API with Helm

`sub2api` should be deployed directly with Helm, not through Terraform.

### Build Helm dependencies

```bash
helm dependency build deploy/helm/sub2api
```

### Create image pull secret (private GHCR)

The container image is in a private GHCR registry. Create a pull secret using a GitHub PAT with `read:packages` scope:

```bash
kubectl -n sub2api create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io \
  --docker-username=wchen99998 \
  --docker-password=<GITHUB_PAT_WITH_READ_PACKAGES>
```

### Generate secrets

```bash
JWT_SECRET=$(openssl rand -hex 32)
TOTP_KEY=$(openssl rand -hex 32)
ADMIN_PASS=$(openssl rand -base64 16)
PG_PASS=$(openssl rand -base64 16)
REDIS_PASS=$(openssl rand -base64 16)

echo "JWT_SECRET:    $JWT_SECRET"
echo "ADMIN_PASS:    $ADMIN_PASS"
```

For the default deployment path, keep PostgreSQL and Redis in-cluster and install the app with Helm:

```bash
helm upgrade --install sub2api deploy/helm/sub2api \
  -n sub2api \
  --create-namespace \
  --set image.api.tag=0.1.8 \
  --set image.bootstrap.tag=0.1.8 \
  --set replicaCount=1 \
  --set ingress.host=api-sub2api.do-prod.yourdomain.com \
  --set ingress.tls.enabled=true \
  --set ingress.annotations.nginx\\.ingress\\.kubernetes\\.io/ssl-redirect=true \
  --set secrets.jwtSecret="$JWT_SECRET" \
  --set secrets.totpEncryptionKey="$TOTP_KEY" \
  --set secrets.adminEmail="admin@sub2api.local" \
  --set secrets.adminPassword="$ADMIN_PASS" \
  --set postgresql.auth.password="$PG_PASS" \
  --set redis.auth.password="$REDIS_PASS" \
  --set 'imagePullSecrets[0].name=ghcr-pull' \
  --set observability.enabled=false \
  --set observability.serviceMonitor.enabled=false
```

This path uses:
- The chart-managed PostgreSQL subchart for the application database
- The chart-managed Redis subchart for caching / coordination
- The default ingress naming pattern `api-sub2api.<domain_suffix>`
- Helm as the source of truth for future app upgrades

Because the bundled PostgreSQL and Redis instances are created by the `sub2api` chart in the `sub2api` namespace, treat them as application components, not Terraform-managed infrastructure.

> **Important:** `TOTP_KEY` must be a 64-character hex key. `openssl rand -hex 32` is correct. Do not use a 32-character random string.

> **Important:** `deploy/helm/sub2api/values-production.yaml` is for external database / external Redis style deployments. Do not include it for the default bundled PostgreSQL + Redis installation above.

> **Cloudflare SSL:** Set your Cloudflare SSL/TLS mode to **"Full (Strict)"** in the dashboard (SSL/TLS -> Overview). This ensures end-to-end encryption: client -> Cloudflare -> HTTPS -> nginx (Let's Encrypt cert) -> app. Using "Flexible" mode will cause a 308 redirect loop because nginx forces HTTPS.

> **Note on Bitnami image tags:** The Bitnami PostgreSQL and Redis subcharts pin specific image tags that may be removed from Docker Hub over time. If pods show `ImagePullBackOff`, override with available tags:
> ```bash
> helm upgrade sub2api deploy/helm/sub2api -n sub2api --reuse-values \
>   --set postgresql.image.tag=latest \
>   --set redis.image.tag=latest
> ```

### Verify

```bash
kubectl -n sub2api get pods        # all pods should be Running
kubectl -n sub2api get ingress     # should show your host + LB IP
kubectl -n sub2api get certificate # TLS cert should show READY=True
```

Your app should be accessible at `https://api-sub2api.do-prod.yourdomain.com`.

### Upgrade Sub2API

Use Helm for all app upgrades:

```bash
helm upgrade sub2api deploy/helm/sub2api \
  -n sub2api \
  --reuse-values \
  --set image.api.tag=<new-version> \
  --set image.bootstrap.tag=<new-version>
```

### Roll back Sub2API

```bash
helm history sub2api -n sub2api
helm rollback sub2api <revision> -n sub2api
```

## DNS Pattern and ExternalDNS

DNS records are managed automatically by ExternalDNS running in the cluster. When an Ingress resource is created, ExternalDNS reads its hostname and creates the corresponding Cloudflare DNS record pointing to the load balancer IP.

### Naming convention

Hostnames follow the pattern `<service>-<namespace>.<domain_suffix>`. For example, with `domain_suffix = "do-prod.yourdomain.com"`:

| Service | Namespace | Hostname |
|---------|-----------|----------|
| api | sub2api | `api-sub2api.do-prod.yourdomain.com` |

### Cloudflare proxy

By default, ExternalDNS creates records with Cloudflare proxy enabled or disabled based on the `cloudflare_proxied` Terraform variable. You can override this per-Ingress using the Helm chart's `cloudflareProxied` value:

```bash
# Disable Cloudflare proxy for a specific deployment
helm install ... --set ingress.cloudflareProxied="false"
```

### Custom domains (extraHosts)

To serve the application on additional hostnames (e.g. a vanity domain), use the `extraHosts` value:

```bash
helm install ... \
  --set ingress.host=api-sub2api.do-prod.yourdomain.com \
  --set 'ingress.extraHosts[0].host=api.mycustomdomain.com'
```

ExternalDNS will create records for all hosts listed in the Ingress. For custom domains outside the `domain_suffix`, ensure their DNS is configured separately to point to the load balancer.

## 4. Using Managed PostgreSQL (Optional)

If you want Terraform to provision DigitalOcean Managed PostgreSQL, let Terraform create the external database and keep Helm responsible for switching the app release to it.

```bash
cd infra/production

# Enable in terraform.tfvars
# enable_managed_database = true

terraform apply
```

Then update the Helm release to use the external database. Start from `values-production.yaml`, which is designed for external services:

```bash
DB_HOST=$(terraform output -raw database_host)
DB_PORT=$(terraform output -raw database_port)
DB_USER=$(terraform output -raw database_user)
DB_PASS=$(terraform output -raw database_password)

helm upgrade sub2api deploy/helm/sub2api \
  -n sub2api \
  -f deploy/helm/sub2api/values-production.yaml \
  --reset-values \
  --set postgresql.enabled=false \
  --set externalDatabase.host="$DB_HOST" \
  --set externalDatabase.port="$DB_PORT" \
  --set externalDatabase.user="$DB_USER" \
  --set externalDatabase.password="$DB_PASS" \
  --set externalDatabase.database=sub2api \
  --set externalDatabase.sslmode=require \
  --set secrets.jwtSecret="$JWT_SECRET" \
  --set secrets.totpEncryptionKey="$TOTP_KEY" \
  --set secrets.adminPassword="$ADMIN_PASS"
```

If you also move Redis out of cluster, set:

```bash
  --set redis.enabled=false \
  --set externalRedis.host="<redis-host>" \
  --set externalRedis.port=6379 \
  --set externalRedis.password="<redis-password>" \
  --set externalRedis.enableTLS=true
```

## 5. Deploy Monitoring Stack (Optional)

The monitoring stack is cluster-level infrastructure. Manage it with Terraform, not a separate manual `helm install`.

### Prerequisites

1. Your Cloudflare API token must have **R2 Storage Edit** permission (in addition to DNS Edit)
2. Add to `infra/production/terraform.tfvars`:
   ```hcl
   enable_observability_storage = true
   cloudflare_account_id        = "your_cloudflare_account_id"
   ```

Set the following in `terraform.tfvars` before applying:

```hcl
enable_observability_storage = true
cloudflare_account_id        = "your_cloudflare_account_id"
enable_monitoring            = true
r2_access_key                = "your_r2_access_key"
r2_secret_key                = "your_r2_secret_key"
```

Then apply:

```bash
cd infra/production
terraform apply
```

This creates:
- Two R2 buckets (`<cluster_name>-tempo` and `<cluster_name>-loki`)
- The `monitoring` Helm release
- Grafana ingress at `grafana.<domain_suffix>`

### Create an R2 API token

Create an R2-scoped API token from the [Cloudflare dashboard](https://dash.cloudflare.com) -> R2 -> Manage R2 API Tokens. This gives you S3-compatible credentials (access key ID + secret) that Tempo and Loki use.

> **Note:** The default Loki cache memory (9.8 GB) is too large for small clusters. The monitoring chart is configured to reduce it to 512 MB / 256 MB. Adjust this if your cluster capacity changes.
>
> **Note:** `loki.loki.auth_enabled=false` disables Loki's multi-tenant auth. Without this, Alloy log pushes fail with 401 "no org id".
>
> **Note:** The monitoring Terraform module configures the Alloy gRPC OTLP receiver (port 4317) so Sub2API can send traces and metrics cross-namespace.

### Enable OTel in Sub2API

Once the monitoring stack is running, enable OTel in the app with Helm:

```bash
helm upgrade sub2api deploy/helm/sub2api \
  -n sub2api --reuse-values \
  --set observability.enabled=true \
  --set observability.otel.serviceName=sub2api \
  --set observability.otel.endpoint="monitoring-alloy.monitoring.svc:4317" \
  --set observability.otel.traceSampleRate="0.1" \
  --set observability.otel.metricsPort=9090 \
  --set observability.serviceMonitor.enabled=true \
  --set observability.serviceMonitor.interval=15s
```

> **Note:** When using `--reuse-values`, all `observability.otel.*` sub-keys must be explicitly set since they don't exist in the prior release values.

### Accessing the Monitoring UIs

Grafana is accessible externally if the ingress above is enabled (e.g. `https://grafana.<domain_suffix>`). Other services are ClusterIP-only — use `kubectl port-forward`:

| Service | Access | Credentials |
|---------|--------|-------------|
| **Grafana** | `https://grafana.<domain_suffix>` or `kubectl -n monitoring port-forward svc/monitoring-grafana 3000:80` → http://localhost:3000 | `admin` / `terraform -chdir=infra/production output -raw grafana_admin_password` |
| **Prometheus** | `kubectl -n monitoring port-forward svc/monitoring-kube-prometheus-prometheus 9090:9090` → http://localhost:9090 | None |
| **Tempo** | `kubectl -n monitoring port-forward svc/monitoring-tempo 3200:3200` → http://localhost:3200 | None |
| **Alertmanager** | `kubectl -n monitoring port-forward svc/monitoring-kube-prometheus-alertmanager 9093:9093` → http://localhost:9093 | None |

#### Grafana: Dashboards and Explore

Grafana is the main UI. Pre-configured datasources are available in the Explore tab:

- **Prometheus** — query metrics (e.g. `rate(http_server_request_duration_seconds_count[5m])`)
- **Tempo** — search traces by service name, duration, or trace ID
- **Loki** — query logs (e.g. `{namespace="sub2api"}` or `{app="sub2api"} |= "error"`)

Pre-built dashboards (loaded via sidecar):
- **Sub2API Overview** — RED metrics, request rates, error rates, latencies
- **Sub2API Resources** — Go runtime metrics (goroutines, memory, GC)

#### Tempo: Direct Trace Lookup

```bash
# Search traces by service
curl -s http://localhost:3200/api/search?tags=service.name%3Dsub2api&limit=10

# Look up a specific trace by ID (from X-Trace-Id response header)
curl -s http://localhost:3200/api/traces/<trace-id>
```

#### Loki: Direct Log Queries

```bash
# Recent sub2api logs
curl -sG http://localhost:3200/loki/api/v1/query_range \
  --data-urlencode 'query={namespace="sub2api"}' \
  --data-urlencode 'limit=50'

# Logs correlated to a specific trace
curl -sG http://localhost:3200/loki/api/v1/query_range \
  --data-urlencode 'query={namespace="sub2api"} | json | trace_id="<trace-id>"'
```

> **Tip:** In Grafana Explore, clicking a trace ID in Tempo automatically links to the correlated logs in Loki (and vice versa) via the pre-configured datasource correlations.

### Verify

```bash
kubectl -n monitoring get pods        # all pods should be Running
kubectl -n sub2api logs deployment/sub2api | grep "Metrics server"  # metrics server started
```

## Common Operations

### Scale the cluster

Edit `terraform.tfvars`:

```hcl
min_nodes = 2
max_nodes = 5
```

```bash
cd infra/production && terraform apply
```

### View logs

```bash
kubectl -n sub2api logs -f deployment/sub2api
```

### Check Terraform outputs

```bash
cd infra/production
terraform output                    # all outputs
terraform output load_balancer_ip   # specific output
terraform output kubeconfig_command # kubectl setup command
```

### Tear down

```bash
# Remove monitoring stack (if deployed)
cd infra/production
terraform destroy -target=module.monitoring
terraform destroy -target=module.storage

# Remove app
helm uninstall sub2api -n sub2api

# Destroy infrastructure
terraform destroy
```

## Architecture Overview

```
Cloudflare (DNS managed by ExternalDNS, CDN/WAF if proxied)
    |
DO Load Balancer (TLS passthrough)
    |
ingress-nginx (TLS via cert-manager DNS-01 / Let's Encrypt)
    |
Sub2API pods (namespace: sub2api)
    |
    +-- Redis (in-cluster, Bitnami subchart, standalone)
    +-- PostgreSQL (in-cluster Bitnami subchart, or DO Managed)

Monitoring stack (namespace: monitoring, optional)
    |
    +-- Prometheus (metrics) ← scrapes Sub2API /metrics
    +-- Grafana (dashboards) ← queries Prometheus, Tempo, Loki
    +-- Tempo (traces) → Cloudflare R2
    +-- Loki (logs) → Cloudflare R2
    +-- Alloy (collector) ← receives OTLP from Sub2API
```

## Terraform Modules

| Module | What it provisions |
|--------|--------------------|
| `infra/modules/doks` | DOKS cluster with autoscaling node pool |
| `infra/modules/kubernetes` | ingress-nginx, cert-manager (DNS-01), ExternalDNS, ClusterIssuer, app namespace |
| `infra/modules/database` | Optional DO Managed PostgreSQL with VPC firewall |
| `infra/modules/storage` | Optional Cloudflare R2 buckets for Tempo and Loki |
| `infra/modules/monitoring` | Optional monitoring Helm release managed by Terraform |

## Troubleshooting

### Pods stuck in ImagePullBackOff

Check which image is failing:

```bash
kubectl -n sub2api describe pod <pod-name> | tail -10
```

Common causes:
- **Private GHCR image:** Create an image pull secret and set `imagePullSecrets`
- **Bitnami tag removed:** Override with `--set postgresql.image.tag=latest` or `--set redis.image.tag=latest`

### App pod CrashLoopBackOff

Usually means PostgreSQL or Redis aren't ready yet. Delete the app pod to trigger a restart once dependencies are running:

```bash
kubectl -n sub2api delete pod -l app.kubernetes.io/name=sub2api
```

### Stale ReplicaSets after upgrade

If old ReplicaSets keep spawning pods with wrong images:

```bash
kubectl -n sub2api get rs
kubectl -n sub2api scale rs <old-rs-name> --replicas=0
```

### Pods stuck in Pending

Check node capacity -- the autoscaler may need time to add nodes:

```bash
kubectl get nodes
kubectl describe pod <pod-name> -n sub2api
```

### Certificate not issuing

Check cert-manager logs and the certificate resource:

```bash
kubectl -n cert-manager logs deployment/cert-manager
kubectl -n sub2api describe certificate
kubectl -n sub2api describe certificaterequest
```

For DNS-01 challenges, also verify the Cloudflare API token has DNS edit permissions and the zone ID is correct.

### Load balancer IP not assigned

Check the ingress-nginx service:

```bash
kubectl -n ingress-nginx get svc ingress-nginx-controller
```

DO load balancers can take 2-3 minutes to provision.

### DNS record not appearing

If ExternalDNS is not creating the expected Cloudflare DNS record, check its logs:

```bash
kubectl -n external-dns logs deployment/external-dns
```

Common causes:
- **Domain filter mismatch:** The Ingress hostname must be under the configured `domain_suffix`. ExternalDNS only manages records matching its `domainFilters`.
- **Cloudflare token permissions:** The API token needs `Zone:DNS:Edit` and `Zone:Zone:Read` permissions.
- **Ingress not ready:** ExternalDNS reads hostnames from Ingress resources. Verify the Ingress exists and has a host set: `kubectl -n sub2api get ingress -o wide`

### ExternalDNS general troubleshooting

```bash
# Check ExternalDNS pod status
kubectl -n external-dns get pods

# View recent logs
kubectl -n external-dns logs deployment/external-dns --tail=50

# Verify the Ingress annotations and hosts
kubectl -n sub2api get ingress -o yaml
```

### 308 redirect loop with Cloudflare

If the site returns `308 Permanent Redirect` in a loop, Cloudflare's SSL mode is likely "Flexible" (connects to origin over HTTP) while nginx forces HTTPS. Fix by setting Cloudflare SSL to **"Full (Strict)"** in the dashboard -> SSL/TLS -> Overview. This is the recommended mode since cert-manager provides a valid Let's Encrypt certificate on the origin.

### Terraform staged apply errors

If `terraform apply` fails with "no matches for kind ClusterIssuer" or "cannot create REST client", you need to apply in stages (see Section 1). This happens because:
- Stage 1 creates the cluster (needed for kubernetes/helm providers)
- Stage 2 installs cert-manager and ExternalDNS (needed for ClusterIssuer CRD)
- Stage 3 creates the ClusterIssuer and remaining resources
