# Terraform `helm_release` Integration for Sub2API + Monitoring

**Date:** 2026-04-06
**Status:** Approved

## Goal

Replace manual `helm install/upgrade` commands with Terraform-managed `helm_release` resources, making the entire infrastructure + application deployment a single deterministic `terraform apply`. All credentials become Terraform outputs.

## Current State

- `infra/modules/kubernetes/` already uses `helm_release` for platform components (ingress-nginx, cert-manager, external-dns)
- Application (sub2api) and monitoring charts are deployed manually via `helm install` with `--set` flags
- Database credentials are Terraform outputs but must be manually copy-pasted into `helm install` commands
- App secrets (JWT, TOTP, admin password) are manually generated outside Terraform

## Design

### Module Structure

```
infra/modules/
├── doks/           # (existing) DOKS cluster + node pool
├── kubernetes/     # (existing) ingress-nginx, cert-manager, external-dns, namespaces
├── database/       # (existing) DO Managed PostgreSQL
├── storage/        # (existing) R2 buckets for Tempo/Loki
├── sub2api/        # (NEW) sub2api helm_release
└── monitoring/     # (NEW) monitoring helm_release
```

### Credential Flow

```
terraform.tfvars (user-provided secrets):
    ├── do_token                ──→ doks, database modules
    ├── cloudflare_api_token    ──→ kubernetes module (external-dns, cert-manager)
    ├── app_image_tag           ──→ sub2api module
    ├── admin_email             ──→ sub2api module (default: admin@sub2api.local)
    ├── r2_access_key           ──→ monitoring module (only when enable_monitoring=true)
    └── r2_secret_key           ──→ monitoring module (only when enable_monitoring=true)

Terraform-generated (random_password, created in production/main.tf):
    ├── jwt_secret              ──→ sub2api module
    ├── totp_encryption_key     ──→ sub2api module
    ├── admin_password          ──→ sub2api module
    └── grafana_admin_password  ──→ monitoring module

Module outputs wired automatically:
    database module   ──→ host, port, user, password, database, sslmode ──→ sub2api module
    storage module    ──→ tempo_bucket, loki_bucket, s3_endpoint ─────────→ monitoring module
    kubernetes module ──→ app_namespace, domain_suffix ───────────────────→ both modules
```

### New Module: `infra/modules/sub2api/`

**Files:** `main.tf`, `variables.tf`

**Resources:**
- `helm_release.sub2api` — deploys `deploy/helm/sub2api` chart

**Key configuration:**
- Chart source: local path `${path.module}/../../../deploy/helm/sub2api`
- Base values: loads `values-production.yaml` via the `values` attribute
- Namespace: passed from kubernetes module output (`sub2api`)
- `create_namespace = false` (namespace already created by kubernetes module)
- `wait = true`, reasonable timeout

**Values wired via `set` / `set_sensitive`:**
- `image.tag` — from `var.app_image_tag`
- `ingress.host` — constructed as `sub2api-<namespace>.<domain_suffix>` (matching ExternalDNS convention)
- `ingress.tls.secretName` — `sub2api-<namespace>-tls`
- `postgresql.enabled` — set to `false` when managed DB is provided, `true` otherwise (in-cluster subchart)
- `redis.enabled = true` (in-cluster Redis via Bitnami subchart — no managed Redis module exists yet)
- `externalDatabase.host`, `.port`, `.user`, `.password`, `.database`, `.sslmode` — from database module outputs (only when managed DB enabled)
- `secrets.jwtSecret` — from random_password
- `secrets.totpEncryptionKey` — from random_password
- `secrets.adminEmail` — from variable
- `secrets.adminPassword` — from random_password

**Note on Redis:** No managed Redis module exists yet. The sub2api module keeps `redis.enabled = true` (in-cluster Bitnami subchart). This can be extended later with a managed Redis module + external Redis wiring, mirroring the database pattern.

**Dependencies:** `depends_on = [module.kubernetes]`

### New Module: `infra/modules/monitoring/`

**Files:** `main.tf`, `variables.tf`

**Resources:**
- `helm_release.monitoring` — deploys `deploy/helm/monitoring` chart

**Enabled conditionally:** via `count = var.enable_monitoring ? 1 : 0` at the module call site in `production/main.tf`

**Key configuration:**
- Chart source: local path `${path.module}/../../../deploy/helm/monitoring`
- Namespace: `monitoring` (created by the chart's own `namespace.yaml` template)
- `create_namespace = true`
- `wait = true`, longer timeout (monitoring stack is large)

**Values wired via `set` / `set_sensitive`:**
- `kube-prometheus-stack.grafana.adminPassword` — from random_password
- `grafanaIngress.host` — constructed from convention (e.g., `grafana-monitoring.<domain_suffix>`)
- `tempo.tempo.storage.trace.s3.bucket` — from storage module output
- `tempo.tempo.storage.trace.s3.endpoint` — from storage module output
- `tempo.tempo.storage.trace.s3.access_key` — from variable
- `tempo.tempo.storage.trace.s3.secret_key` — from variable
- `loki.loki.storage.s3.endpoint` — from storage module output
- `loki.loki.storage.s3.accessKeyId` — from variable
- `loki.loki.storage.s3.secretAccessKey` — from variable
- `loki.loki.storage.bucketNames.chunks/ruler/admin` — from storage module output

**Dependencies:** `depends_on = [module.kubernetes]`

### Changes to `production/main.tf`

**New resources:**
```hcl
resource "random_password" "jwt_secret" {
  length  = 32
  special = true
}

resource "random_password" "totp_encryption_key" {
  length  = 32
  special = true
}

resource "random_password" "admin_password" {
  length  = 16
  special = true
}

resource "random_password" "grafana_admin_password" {
  length  = 16
  special = true
}
```

**New module blocks:**
```hcl
module "sub2api" {
  source = "../modules/sub2api"

  chart_path      = "${path.module}/../../deploy/helm/sub2api"
  namespace       = module.kubernetes.app_namespace
  domain_suffix   = var.domain_suffix
  app_image_tag   = var.app_image_tag

  # Database (from managed DB module)
  database_host     = module.database[0].host
  database_port     = module.database[0].port
  database_user     = module.database[0].user
  database_password = module.database[0].password
  database_name     = module.database[0].database

  # Secrets (auto-generated)
  jwt_secret          = random_password.jwt_secret.result
  totp_encryption_key = random_password.totp_encryption_key.result
  admin_email         = var.admin_email
  admin_password      = random_password.admin_password.result

  depends_on = [module.kubernetes]
}

module "monitoring" {
  source = "../modules/monitoring"
  count  = var.enable_monitoring ? 1 : 0

  chart_path    = "${path.module}/../../deploy/helm/monitoring"
  domain_suffix = var.domain_suffix

  grafana_admin_password = random_password.grafana_admin_password.result

  # R2 storage (from storage module)
  r2_endpoint    = module.storage[0].s3_endpoint
  r2_access_key  = var.r2_access_key
  r2_secret_key  = var.r2_secret_key
  tempo_bucket   = module.storage[0].tempo_bucket
  loki_bucket    = module.storage[0].loki_bucket

  depends_on = [module.kubernetes]
}
```

### Changes to `production/variables.tf`

New variables:
- `app_image_tag` — required string, no default
- `admin_email` — string, default `"admin@sub2api.local"`
- `enable_monitoring` — bool, default `false`
- `r2_access_key` — sensitive string, default `""`
- `r2_secret_key` — sensitive string, default `""`

### Changes to `production/outputs.tf`

New outputs (all sensitive where applicable):
- `app_url` — `"https://sub2api-<namespace>.<domain_suffix>"`
- `admin_password` — sensitive
- `jwt_secret` — sensitive
- `totp_encryption_key` — sensitive
- `grafana_url` — conditional on `enable_monitoring`
- `grafana_admin_password` — sensitive, conditional on `enable_monitoring`

### Changes to `production/versions.tf`

Add `random` and `null` providers:
```hcl
random = {
  source  = "hashicorp/random"
  version = "~> 3.6"
}
null = {
  source  = "hashicorp/null"
  version = "~> 3.2"
}
```

### Deployment Workflow (after implementation)

**Fresh deployment:**
```bash
cd infra/production/
# Edit terraform.tfvars: set app_image_tag, provider tokens, optionally R2 keys
terraform init
terraform apply
# Retrieve generated credentials:
terraform output -raw admin_password
terraform output -raw jwt_secret
```

**App version update:**
```bash
# Update app_image_tag in terraform.tfvars
terraform apply   # only the helm_release changes
```

**Enable monitoring:**
```bash
# Set enable_monitoring = true, enable_observability_storage = true
# Add r2_access_key and r2_secret_key
terraform apply
terraform output -raw grafana_admin_password
```

## What Does NOT Change

- Helm chart source code (`deploy/helm/sub2api/`, `deploy/helm/monitoring/`) — no modifications needed
- `values.yaml` and `values-production.yaml` — still used as-is
- Existing modules (doks, kubernetes, database, storage) — no changes
- Docker Compose deployment path — still works independently
- CI/CD release workflow — still builds and pushes images to GHCR

## Edge Cases

**Managed database disabled:** If `enable_managed_database = false`, the sub2api module cannot wire DB credentials from the database module. The sub2api module accepts optional database variables (all defaulting to `""`). When no external DB host is provided, `postgresql.enabled` stays `true` (in-cluster subchart). The `production/main.tf` module block uses conditional expressions: `database_host = var.enable_managed_database ? module.database[0].host : ""`.

**Monitoring requires storage:** When `enable_monitoring = true`, `enable_observability_storage` must also be `true` (R2 buckets are required for Tempo and Loki). Add a `validation` block or `precondition` in the monitoring module call to enforce this. Alternatively, the monitoring module could make R2 storage optional and fall back to local storage, but this is not recommended for production.

**First-time apply ordering:** Terraform handles this via `depends_on`. The kubernetes module (namespaces, ingress, cert-manager) must complete before helm_release resources attempt deployment. The database module has no dependency on kubernetes and can provision in parallel with it.

**Helm chart dependency resolution:** Both modules use a `null_resource` with a `local-exec` provisioner to run `helm dependency build` on the local chart directory before the `helm_release` resource. This ensures subchart tarballs (`charts/*.tgz`) are always present, even from a clean git clone (the monitoring chart has no committed tarballs, and `.gitignore` excludes `**/charts/*.tgz`).

```hcl
resource "null_resource" "helm_deps" {
  triggers = {
    chart_lock = filemd5("${var.chart_path}/Chart.lock")
  }

  provisioner "local-exec" {
    command = "helm dependency build ${var.chart_path}"
  }
}

resource "helm_release" "sub2api" {
  # ...
  depends_on = [null_resource.helm_deps]
}
```

The `triggers` block uses `Chart.lock` checksum so the provisioner only re-runs when dependency versions actually change, not on every apply. Requires `helm` CLI on the machine running Terraform (already a practical requirement for the Helm provider).

**Note:** The monitoring chart is currently missing `Chart.lock`. It must be generated (`helm dependency update deploy/helm/monitoring/`) and committed before this workflow works. This is a one-time fix.
