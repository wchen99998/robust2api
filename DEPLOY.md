# Kubernetes Deployment Guide

Deploy Sub2API on DigitalOcean Kubernetes (DOKS) with a clean ownership split:

- **Terraform** manages cloud resources: DOKS cluster, optional managed PostgreSQL, optional R2 storage buckets, and Cloudflare API token bootstrap secrets.
- **Flux CD** manages everything inside the cluster: ingress-nginx, cert-manager, ExternalDNS, monitoring stack, and the Sub2API application — all via GitOps.

All in-cluster changes are made by editing YAML files in `clusters/production/`, committing, and pushing. Flux reconciles automatically.

```
Cloudflare (DNS managed by ExternalDNS, CDN/WAF if proxied)
    |
DO Load Balancer (TLS passthrough)
    |
ingress-nginx (TLS via cert-manager DNS-01 / Let's Encrypt)
    |
Sub2API pods (namespace: sub2api)
    +-- Redis (in-cluster Bitnami, standalone)
    +-- PostgreSQL (in-cluster Bitnami or external DO Managed)

Monitoring (namespace: monitoring, optional)
    +-- Prometheus (metrics)
    +-- Grafana (dashboards)
    +-- Tempo (traces -> R2)
    +-- Loki (logs -> R2)
    +-- Alloy (gRPC OTLP receiver)

Flux CD (namespace: flux-system)
    +-- Watches: clusters/production/ in this Git repo
    +-- Reconciles: infrastructure -> monitoring -> apps
```

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.7
- [doctl](https://docs.digitalocean.com/reference/doctl/how-to/install/) (DigitalOcean CLI)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/) >= 3
- [Flux CLI](https://fluxcd.io/flux/installation/#install-the-flux-cli) (`brew install fluxcd/tap/flux`)
- A DigitalOcean API token ([create one](https://cloud.digitalocean.com/account/api/tokens))
- A Cloudflare API token with DNS edit permissions ([create one](https://dash.cloudflare.com/profile/api-tokens))
- A GitHub personal access token with `repo` scope (for Flux bootstrap)

## Deployment Ownership

Use Terraform for:
- DOKS cluster lifecycle
- Optional external DO managed PostgreSQL
- Optional R2 buckets for Tempo and Loki
- Cloudflare API token secrets (bootstrap for cert-manager and ExternalDNS)

Use Flux (GitOps) for:
- ingress-nginx, cert-manager, ExternalDNS
- Monitoring stack (Prometheus, Grafana, Tempo, Loki, Alloy)
- Sub2API application (gateway, control, worker)
- All in-cluster configuration changes

## 1. Provision Infrastructure

```bash
cd infra/production
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars` with your values:

```hcl
# DigitalOcean
do_token     = "dop_v1_..."
region       = "sgp1"
cluster_name = "sub2api"
k8s_version  = "1.34"

# Cloudflare (for API token secrets only)
cloudflare_api_token = "..."

# Optional: managed PostgreSQL (default false, uses in-cluster Bitnami PG)
# enable_managed_database = true

# Optional: R2 storage for Tempo/Loki
# enable_observability_storage = true
# cloudflare_account_id        = "..."
```

Apply:

```bash
terraform init
terraform apply
```

Configure kubectl:

```bash
eval "$(terraform output -raw kubeconfig_command)"
```

## 2. Bootstrap Flux

One-time setup. This installs Flux controllers and creates a GitRepository pointing at this repo.

```bash
export GITHUB_TOKEN=<your-github-pat>

flux bootstrap github \
  --owner=<github-org-or-user> \
  --repository=robust2api \
  --branch=main \
  --path=clusters/production \
  --personal
```

Flux will:
1. Install its controllers in the `flux-system` namespace
2. Create a GitRepository source pointing at this repo
3. Start from the checked-in root `clusters/production/kustomization.yaml`
4. Apply infrastructure -> monitoring -> apps in dependency order

## 3. Create Secrets

Flux HelmReleases reference secrets via `valuesFrom`. These must be created manually before the first deploy, and the secret keys must match the explicit `valuesKey` mappings in the HelmRelease manifests.

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
  postgresql.auth.password: "<db-password-or-empty-if-external>"
  redis.auth.password: "<redis-password-or-empty-if-external>"
  externalDatabase.password: ""
  externalRedis.password: ""
EOF
```

If using external PostgreSQL and/or Redis instead of the bundled charts, set `postgresql.enabled=false` and/or `redis.enabled=false` in `clusters/production/apps/sub2api.yaml` and move the real passwords into `externalDatabase.password` / `externalRedis.password`. Leave the unused password keys present with empty string values so Flux can still resolve every mapped secret key.

### Monitoring Secrets (if using monitoring stack)

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
  grafanaPostgresDatasource.user: "<grafana-db-user>"
  grafanaPostgresDatasource.password: "<grafana-db-password>"
EOF
```

## 4. Configure Flux Manifests

Update `CHANGEME` values in the Flux YAML files before deploying:

| File | Values to set |
|------|--------------|
| `clusters/production/infrastructure/issuers/cluster-issuer.yaml` | `email` (Let's Encrypt notification email) |
| `clusters/production/infrastructure/external-dns.yaml` | `txtOwnerId`, `domainFilters` |
| `clusters/production/monitoring/monitoring.yaml` | `grafanaIngress.host`, R2 endpoints, bucket names, DB host |
| `clusters/production/apps/sub2api.yaml` | Image tags, ingress hosts, `gatewayUrl`, `grafanaUrl` |

## 5. Initial Deploy

Commit the configured manifests and push:

```bash
git add clusters/production/
git commit -m "deploy: configure Flux manifests for production"
git push
```

Flux syncs automatically within 1 minute.

## 6. Verify

```bash
# Check Kustomization status (dependency chain)
flux get kustomizations

# Check all HelmReleases
flux get helmreleases -A

# Check pods
kubectl get pods -n sub2api
kubectl get pods -n monitoring
kubectl get pods -n ingress-nginx
kubectl get pods -n cert-manager
kubectl get pods -n external-dns

# Check certificates
kubectl get certificates -A

# Check ingress
kubectl get ingress -A
```

All HelmReleases should show `Ready: True`. If not, see Troubleshooting below.

## Deploying a New Version

1. Tag and push to trigger image builds:

```bash
git tag v0.3.0
git push origin v0.3.0
```

2. Update image tags in `clusters/production/apps/sub2api.yaml`:

```yaml
image:
  gateway:
    tag: "0.3.0"
  control:
    tag: "0.3.0"
  frontend:
    tag: "0.3.0"
  worker:
    tag: "0.3.0"
  bootstrap:
    tag: "0.3.0"
```

3. Commit and push:

```bash
git add clusters/production/apps/sub2api.yaml
git commit -m "deploy: v0.3.0"
git push
```

4. Flux syncs automatically within 1 minute.

## Rolling Back

Revert the image tag commit:

```bash
git revert HEAD
git push
```

Flux will roll back to the previous version automatically.

## Upgrading Infrastructure Components

Edit the chart version in the relevant HelmRelease file, commit, and push:

```bash
# Example: upgrade ingress-nginx
# Edit clusters/production/infrastructure/ingress-nginx.yaml
# Change: version: "4.12.1" -> version: "4.13.0"
git add clusters/production/infrastructure/ingress-nginx.yaml
git commit -m "infra: upgrade ingress-nginx to 4.13.0"
git push
```

## Migrating from Terraform-Managed In-Cluster Resources

If you previously used Terraform to manage ingress-nginx, cert-manager, ExternalDNS, and/or the monitoring stack, remove them from Terraform state after Flux has adopted them:

```bash
cd infra/production

# Remove kubernetes module resources
terraform state rm 'module.kubernetes.helm_release.ingress_nginx'
terraform state rm 'module.kubernetes.helm_release.cert_manager'
terraform state rm 'module.kubernetes.helm_release.external_dns'
terraform state rm 'module.kubernetes.kubernetes_manifest.letsencrypt_issuer'
terraform state rm 'module.kubernetes.kubernetes_namespace.app'
terraform state rm 'module.kubernetes.kubernetes_namespace.external_dns'
terraform state rm 'module.kubernetes.kubernetes_secret.cloudflare_cert_manager'
terraform state rm 'module.kubernetes.kubernetes_secret.cloudflare_external_dns'
terraform state rm 'module.kubernetes.data.kubernetes_service.ingress_nginx'

# Remove monitoring module resources (if enabled)
terraform state rm 'module.monitoring[0].helm_release.monitoring'
terraform state rm 'module.monitoring[0].null_resource.helm_deps'

# Verify clean state
terraform plan
```

## Monitoring & Status

```bash
# Overall Flux status
flux get kustomizations
flux get helmreleases -A
flux get sources git

# Reconcile immediately (don't wait for interval)
flux reconcile kustomization apps
flux reconcile helmrelease sub2api -n sub2api

# View Flux controller logs
flux logs --level=error
```

## Troubleshooting

### HelmRelease stuck in "not ready"

```bash
# Check the HelmRelease status and last error
kubectl describe helmrelease <name> -n <namespace>

# Check Helm release history
helm history <name> -n <namespace>

# Force reconciliation
flux reconcile helmrelease <name> -n <namespace>
```

### Source not syncing

```bash
# Check GitRepository status
flux get sources git -A

# Force source refresh
flux reconcile source git flux-system
```

### Suspended reconciliation

```bash
# Check if reconciliation is suspended
flux get kustomizations
flux get helmreleases -A

# Resume if suspended
flux resume kustomization <name>
flux resume helmrelease <name> -n <namespace>
```

### ImagePullBackOff

- Verify `ghcr-pull` secret exists: `kubectl get secret ghcr-pull -n sub2api`
- Check token permissions: must have `read:packages` scope
- Verify image tag exists: `docker manifest inspect ghcr.io/wchen99998/robust2api/gateway:<tag>`

### Certificate not issuing

```bash
kubectl describe certificate <name> -n <namespace>
kubectl describe certificaterequest -n <namespace>
kubectl describe challenge -A
kubectl logs -n cert-manager deploy/cert-manager
```

Common cause: Cloudflare API token missing DNS edit permission, or `cloudflare-api-token` secret not created by Terraform.

### Pod CrashLoopBackOff

- Bootstrap job may CrashLoop briefly while PostgreSQL starts — this is expected
- Check logs: `kubectl logs <pod> -n sub2api`
- If database connection fails, verify DB credentials in `sub2api-secrets`
