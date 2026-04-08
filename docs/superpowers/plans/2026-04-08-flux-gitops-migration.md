# Flux GitOps Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace manual Helm deployments and Terraform-managed in-cluster resources with Flux CD GitOps, leaving Terraform for cloud resources only.

**Architecture:** Three-layer Flux Kustomization hierarchy (infrastructure -> monitoring -> apps) in `clusters/production/`. Third-party charts use upstream HelmRepositories; in-repo charts use the self-referencing GitRepository created by `flux bootstrap`. Terraform scope shrinks to DOKS cluster, managed DB, R2 storage, and Cloudflare API token secrets.

**Tech Stack:** Flux CD v2, Kustomize, Helm, Terraform

---

## File Map

### New files (Flux manifests)

| File | Responsibility |
|------|---------------|
| `clusters/production/infrastructure.yaml` | Flux Kustomization for infra layer |
| `clusters/production/monitoring.yaml` | Flux Kustomization for monitoring layer |
| `clusters/production/apps.yaml` | Flux Kustomization for apps layer |
| `clusters/production/infrastructure/sources/helmrepo-ingress-nginx.yaml` | HelmRepository: ingress-nginx upstream |
| `clusters/production/infrastructure/sources/helmrepo-jetstack.yaml` | HelmRepository: cert-manager upstream |
| `clusters/production/infrastructure/sources/helmrepo-external-dns.yaml` | HelmRepository: external-dns upstream |
| `clusters/production/infrastructure/ingress-nginx.yaml` | HelmRelease: ingress-nginx |
| `clusters/production/infrastructure/cert-manager.yaml` | HelmRelease: cert-manager |
| `clusters/production/infrastructure/cluster-issuer.yaml` | Raw ClusterIssuer manifest |
| `clusters/production/infrastructure/external-dns.yaml` | HelmRelease: external-dns |
| `clusters/production/infrastructure/namespaces.yaml` | Namespace: sub2api |
| `clusters/production/monitoring/monitoring.yaml` | HelmRelease: monitoring stack |
| `clusters/production/apps/sub2api.yaml` | HelmRelease: sub2api application |

### Modified files (Terraform reduction)

| File | Change |
|------|--------|
| `infra/production/main.tf` | Remove `module.kubernetes`, `module.monitoring`, related checks/locals; extract Cloudflare secrets into inline resources; remove Helm provider |
| `infra/production/variables.tf` | Remove monitoring variables (`enable_monitoring`, `grafana_*`, `r2_access_key`, `r2_secret_key`); remove kubernetes bootstrap variables (`letsencrypt_email`, `cloudflare_zone_id`, `domain_suffix`, `cloudflare_proxied`); keep cloud-only variables |
| `infra/production/outputs.tf` | Remove `load_balancer_ip`, all monitoring/grafana outputs; keep cluster and database outputs |
| `infra/production/versions.tf` | Remove `helm` provider requirement |
| `DEPLOY.md` | Rewrite for Flux-based workflow |

### Deleted files/directories (Terraform modules no longer needed)

| Path | Reason |
|------|--------|
| `infra/modules/kubernetes/` | Entire module moved to Flux infrastructure layer |
| `infra/modules/monitoring/` | Entire module moved to Flux monitoring layer |

---

## Task 1: Create infrastructure layer — HelmRepository sources

**Files:**
- Create: `clusters/production/infrastructure/sources/helmrepo-ingress-nginx.yaml`
- Create: `clusters/production/infrastructure/sources/helmrepo-jetstack.yaml`
- Create: `clusters/production/infrastructure/sources/helmrepo-external-dns.yaml`

- [ ] **Step 1: Create the sources directory**

```bash
mkdir -p clusters/production/infrastructure/sources
```

- [ ] **Step 2: Create ingress-nginx HelmRepository**

Create `clusters/production/infrastructure/sources/helmrepo-ingress-nginx.yaml`:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: ingress-nginx
  namespace: flux-system
spec:
  interval: 24h
  url: https://kubernetes.github.io/ingress-nginx
```

- [ ] **Step 3: Create jetstack HelmRepository**

Create `clusters/production/infrastructure/sources/helmrepo-jetstack.yaml`:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: jetstack
  namespace: flux-system
spec:
  interval: 24h
  url: https://charts.jetstack.io
```

- [ ] **Step 4: Create external-dns HelmRepository**

Create `clusters/production/infrastructure/sources/helmrepo-external-dns.yaml`:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: external-dns
  namespace: flux-system
spec:
  interval: 24h
  url: https://kubernetes-sigs.github.io/external-dns
```

- [ ] **Step 5: Commit**

```bash
git add clusters/production/infrastructure/sources/
git commit -m "feat(flux): add HelmRepository sources for third-party charts"
```

---

## Task 2: Create infrastructure layer — ingress-nginx HelmRelease

**Files:**
- Create: `clusters/production/infrastructure/ingress-nginx.yaml`

- [ ] **Step 1: Create ingress-nginx HelmRelease**

Create `clusters/production/infrastructure/ingress-nginx.yaml`:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: ingress-nginx
  namespace: ingress-nginx
spec:
  interval: 10m
  chart:
    spec:
      chart: ingress-nginx
      version: "4.12.1"
      sourceRef:
        kind: HelmRepository
        name: ingress-nginx
        namespace: flux-system
  install:
    createNamespace: true
    remediation:
      retries: 3
  upgrade:
    remediation:
      retries: 3
      remediateLastFailure: true
  timeout: 10m
  values:
    controller:
      service:
        externalTrafficPolicy: Local
        annotations:
          service.beta.kubernetes.io/do-loadbalancer-name: sub2api-lb
          service.beta.kubernetes.io/do-loadbalancer-tls-passthrough: "true"
      config:
        enable-underscores-in-headers: "true"
```

- [ ] **Step 2: Commit**

```bash
git add clusters/production/infrastructure/ingress-nginx.yaml
git commit -m "feat(flux): add ingress-nginx HelmRelease"
```

---

## Task 3: Create infrastructure layer — cert-manager HelmRelease and ClusterIssuer

**Files:**
- Create: `clusters/production/infrastructure/cert-manager.yaml`
- Create: `clusters/production/infrastructure/cluster-issuer.yaml`

- [ ] **Step 1: Create cert-manager HelmRelease**

Create `clusters/production/infrastructure/cert-manager.yaml`:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cert-manager
  namespace: cert-manager
spec:
  interval: 10m
  chart:
    spec:
      chart: cert-manager
      version: "1.17.1"
      sourceRef:
        kind: HelmRepository
        name: jetstack
        namespace: flux-system
  install:
    createNamespace: true
    remediation:
      retries: 3
  upgrade:
    remediation:
      retries: 3
      remediateLastFailure: true
  timeout: 10m
  values:
    crds:
      enabled: true
```

- [ ] **Step 2: Create ClusterIssuer manifest**

Create `clusters/production/infrastructure/cluster-issuer.yaml`:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    # CHANGEME: Set your Let's Encrypt notification email
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - dns01:
          cloudflare:
            apiTokenSecretRef:
              name: cloudflare-api-token
              key: api-token
```

Note: The `email` field must be updated to your actual Let's Encrypt email before deploying. The `cloudflare-api-token` Secret in the `cert-manager` namespace is created by Terraform.

- [ ] **Step 3: Commit**

```bash
git add clusters/production/infrastructure/cert-manager.yaml clusters/production/infrastructure/cluster-issuer.yaml
git commit -m "feat(flux): add cert-manager HelmRelease and ClusterIssuer"
```

---

## Task 4: Create infrastructure layer — external-dns HelmRelease and namespaces

**Files:**
- Create: `clusters/production/infrastructure/external-dns.yaml`
- Create: `clusters/production/infrastructure/namespaces.yaml`

- [ ] **Step 1: Create external-dns HelmRelease**

Create `clusters/production/infrastructure/external-dns.yaml`:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: external-dns
  namespace: external-dns
spec:
  interval: 10m
  chart:
    spec:
      chart: external-dns
      version: "1.16.1"
      sourceRef:
        kind: HelmRepository
        name: external-dns
        namespace: flux-system
  install:
    createNamespace: true
    remediation:
      retries: 3
  upgrade:
    remediation:
      retries: 3
      remediateLastFailure: true
  timeout: 5m
  values:
    provider:
      name: cloudflare
    env:
      - name: CF_API_TOKEN
        valueFrom:
          secretKeyRef:
            name: cloudflare-api-token
            key: api-token
    policy: sync
    # CHANGEME: Set your cluster name for TXT record ownership
    txtOwnerId: sub2api
    # CHANGEME: Set your domain suffix
    domainFilters:
      - do-prod.yourdomain.com
    sources:
      - ingress
    # Uncomment to enable Cloudflare proxy on DNS records
    # extraArgs:
    #   - --cloudflare-proxied
```

Note: `txtOwnerId` and `domainFilters` must be updated to your actual values before deploying.

- [ ] **Step 2: Create namespaces manifest**

Create `clusters/production/infrastructure/namespaces.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: sub2api
```

- [ ] **Step 3: Commit**

```bash
git add clusters/production/infrastructure/external-dns.yaml clusters/production/infrastructure/namespaces.yaml
git commit -m "feat(flux): add external-dns HelmRelease and namespaces"
```

---

## Task 5: Create Flux Kustomizations (all three layers)

The Flux `Kustomization` CRDs live at `clusters/production/` (the root path Flux watches). They point to subdirectories containing the actual manifests. This avoids naming conflicts with Kustomize's own `kustomization.yaml` files.

**Files:**
- Create: `clusters/production/infrastructure.yaml`
- Create: `clusters/production/monitoring.yaml`
- Create: `clusters/production/apps.yaml`

- [ ] **Step 1: Create infrastructure Kustomization**

Create `clusters/production/infrastructure.yaml`:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: infrastructure
  namespace: flux-system
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-system
  path: ./clusters/production/infrastructure
  prune: true
  wait: true
  timeout: 15m
```

- [ ] **Step 2: Create monitoring Kustomization**

Create `clusters/production/monitoring.yaml`:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: monitoring
  namespace: flux-system
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-system
  path: ./clusters/production/monitoring
  prune: true
  wait: true
  timeout: 15m
  dependsOn:
    - name: infrastructure
```

- [ ] **Step 3: Create apps Kustomization**

Create `clusters/production/apps.yaml`:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-system
  path: ./clusters/production/apps
  prune: true
  wait: true
  timeout: 15m
  dependsOn:
    - name: monitoring
```

The `wait: true` on infrastructure ensures all resources (including CRDs from cert-manager) are ready before monitoring proceeds. The dependency chain is: infrastructure -> monitoring -> apps.

- [ ] **Step 4: Commit**

```bash
git add clusters/production/infrastructure.yaml clusters/production/monitoring.yaml clusters/production/apps.yaml
git commit -m "feat(flux): add Kustomizations for infrastructure, monitoring, and apps layers"
```

---

## Task 6: Create monitoring layer — HelmRelease

**Files:**
- Create: `clusters/production/monitoring/monitoring.yaml`

- [ ] **Step 1: Create monitoring directory**

```bash
mkdir -p clusters/production/monitoring
```

- [ ] **Step 2: Create monitoring HelmRelease**

Create `clusters/production/monitoring/monitoring.yaml`:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: monitoring
  namespace: monitoring
spec:
  interval: 10m
  chart:
    spec:
      chart: deploy/helm/monitoring
      sourceRef:
        kind: GitRepository
        name: flux-system
        namespace: flux-system
  install:
    createNamespace: true
    remediation:
      retries: 3
  upgrade:
    remediation:
      retries: 3
      remediateLastFailure: true
  timeout: 15m
  valuesFrom:
    - kind: Secret
      name: monitoring-secrets
      optional: false
  values:
    # CHANGEME: Set your Grafana hostname
    grafanaIngress:
      host: grafana.do-prod.yourdomain.com

    # CHANGEME: Set your R2 endpoint and bucket names
    tempo:
      tempo:
        storage:
          trace:
            s3:
              bucket: sub2api-tempo
              endpoint: your-account-id.r2.cloudflarestorage.com

    loki:
      loki:
        storage:
          s3:
            endpoint: your-account-id.r2.cloudflarestorage.com
          bucketNames:
            chunks: sub2api-loki
            ruler: sub2api-loki
            admin: sub2api-loki

    grafanaPostgresDatasource:
      enabled: true
      # CHANGEME: Set your DB host (non-sensitive, sensitive fields come from monitoring-secrets)
      host: your-db-host.db.ondigitalocean.com
      port: 25060
      database: sub2api
      sslmode: require
```

Note: The `monitoring-secrets` Secret must be pre-created in the `monitoring` namespace with these keys:

```yaml
# kubectl create secret generic monitoring-secrets -n monitoring --from-literal=...
# Required keys (as Helm value paths):
#   kube-prometheus-stack.grafana.adminPassword: <grafana-admin-password>
#   tempo.tempo.storage.trace.s3.access_key: <r2-access-key>
#   tempo.tempo.storage.trace.s3.secret_key: <r2-secret-key>
#   loki.loki.storage.s3.accessKeyId: <r2-access-key>
#   loki.loki.storage.s3.secretAccessKey: <r2-secret-key>
#   grafanaPostgresDatasource.user: <grafana-db-user>
#   grafanaPostgresDatasource.password: <grafana-db-password>
```

- [ ] **Step 4: Commit**

```bash
git add clusters/production/monitoring/
git commit -m "feat(flux): add monitoring layer with LGTM stack HelmRelease"
```

---

## Task 7: Create apps layer — sub2api HelmRelease

**Files:**
- Create: `clusters/production/apps/sub2api.yaml`

- [ ] **Step 1: Create apps directory**

```bash
mkdir -p clusters/production/apps
```

- [ ] **Step 2: Create sub2api HelmRelease**

Create `clusters/production/apps/sub2api.yaml`:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: sub2api
  namespace: sub2api
spec:
  interval: 10m
  chart:
    spec:
      chart: deploy/helm/sub2api
      sourceRef:
        kind: GitRepository
        name: flux-system
        namespace: flux-system
  install:
    createNamespace: true
    remediation:
      retries: 3
  upgrade:
    remediation:
      retries: 3
      remediateLastFailure: true
  timeout: 10m
  valuesFrom:
    - kind: Secret
      name: sub2api-secrets
      optional: false
  values:
    # --- Image tags (update these to deploy new versions) ---
    image:
      gateway:
        tag: "0.1.0"
      control:
        tag: "0.1.0"
      frontend:
        tag: "0.1.0"
      worker:
        tag: "0.1.0"
      bootstrap:
        tag: "0.1.0"

    imagePullSecrets:
      - name: ghcr-pull

    # --- Ingress ---
    ingress:
      enabled: true
      className: nginx
      cloudflareProxied: "true"
      maxBodySize: "50m"
      annotations:
        cert-manager.io/cluster-issuer: letsencrypt-prod
      # CHANGEME: Set your gateway and control hostnames
      gateway:
        enabled: true
        host: gateway-sub2api.do-prod.yourdomain.com
        tls:
          enabled: true
      control:
        enabled: true
        host: app-sub2api.do-prod.yourdomain.com
        tls:
          enabled: true

    # --- Config ---
    config:
      serverPort: "8080"
      serverMode: release
      runMode: standard
      timezone: Asia/Shanghai
      # CHANGEME: Set your public gateway URL
      gatewayUrl: "https://gateway-sub2api.do-prod.yourdomain.com"
      # CHANGEME: Set your Grafana URL (if monitoring enabled)
      grafanaUrl: "https://grafana.do-prod.yourdomain.com"

    # --- Observability ---
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

    # --- Database (use external if managed DB enabled) ---
    # Uncomment and configure for external DB:
    # postgresql:
    #   enabled: false
    # externalDatabase:
    #   host: your-db-host.db.ondigitalocean.com
    #   port: 25060
    #   user: sub2api
    #   database: sub2api
    #   sslmode: require
```

Note: The `sub2api-secrets` Secret must be pre-created in the `sub2api` namespace with these keys:

```yaml
# kubectl create secret generic sub2api-secrets -n sub2api --from-literal=...
# Required keys (as Helm value paths):
#   secrets.jwtSecret: <jwt-secret>
#   secrets.totpEncryptionKey: <totp-key>
#   secrets.adminPassword: <admin-password>
#   postgresql.auth.password: <db-password>       (if using bundled PostgreSQL)
#   redis.auth.password: <redis-password>          (if using bundled Redis)
#   externalDatabase.password: <db-password>       (if using external PostgreSQL)
#   externalRedis.password: <redis-password>       (if using external Redis)
```

- [ ] **Step 4: Commit**

```bash
git add clusters/production/apps/
git commit -m "feat(flux): add apps layer with sub2api HelmRelease"
```

---

## Task 8: Reduce Terraform scope — update main.tf

**Files:**
- Modify: `infra/production/main.tf`

- [ ] **Step 1: Read current main.tf**

Read `infra/production/main.tf` to confirm current state.

- [ ] **Step 2: Rewrite main.tf**

Replace the entire file with:

```hcl
# --- Providers ---

provider "digitalocean" {
  token = var.do_token
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

provider "kubernetes" {
  host                   = module.doks.endpoint
  cluster_ca_certificate = module.doks.cluster_ca_certificate

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "doctl"
    args = [
      "kubernetes",
      "cluster",
      "kubeconfig",
      "exec-credential",
      "--version=v1beta1",
      "--context=do",
      module.doks.cluster_id,
    ]
  }
}

# --- Modules ---

module "doks" {
  source = "../modules/doks"

  cluster_name = var.cluster_name
  region       = var.region
  k8s_version  = var.k8s_version
  node_size    = var.node_size
  min_nodes    = var.min_nodes
  max_nodes    = var.max_nodes
}

module "database" {
  source = "../modules/database"
  count  = var.enable_managed_database ? 1 : 0

  cluster_name        = var.cluster_name
  region              = var.region
  db_size             = var.db_size
  grafana_reader_user = var.managed_grafana_reader_user
  doks_cluster_id     = module.doks.cluster_id
}

module "storage" {
  source = "../modules/storage"
  count  = var.enable_observability_storage ? 1 : 0

  cloudflare_account_id = var.cloudflare_account_id
  cluster_name          = var.cluster_name
}

# --- Cloudflare API token secrets (for cert-manager and ExternalDNS) ---
# These must exist before Flux can reconcile cert-manager and external-dns.

resource "kubernetes_namespace" "cert_manager" {
  metadata {
    name = "cert-manager"
  }

  lifecycle {
    ignore_changes = [metadata[0].labels, metadata[0].annotations]
  }
}

resource "kubernetes_secret" "cloudflare_cert_manager" {
  metadata {
    name      = "cloudflare-api-token"
    namespace = "cert-manager"
  }

  data = {
    api-token = var.cloudflare_api_token
  }

  depends_on = [kubernetes_namespace.cert_manager]
}

resource "kubernetes_namespace" "external_dns" {
  metadata {
    name = "external-dns"
  }

  lifecycle {
    ignore_changes = [metadata[0].labels, metadata[0].annotations]
  }
}

resource "kubernetes_secret" "cloudflare_external_dns" {
  metadata {
    name      = "cloudflare-api-token"
    namespace = "external-dns"
  }

  data = {
    api-token = var.cloudflare_api_token
  }

  depends_on = [kubernetes_namespace.external_dns]
}
```

Key changes:
- Removed `helm` provider block
- Removed `module.kubernetes` and `module.monitoring`
- Removed all monitoring-related checks, locals, and `random_password`
- Extracted Cloudflare API token secrets from `module.kubernetes` into inline resources
- Added `lifecycle.ignore_changes` on namespaces so Terraform doesn't fight Flux over labels/annotations
- Kept `kubernetes` provider (needed for secrets and namespaces)

- [ ] **Step 3: Commit**

```bash
git add infra/production/main.tf
git commit -m "refactor(infra): remove kubernetes and monitoring modules from Terraform

Flux now manages all in-cluster resources. Terraform retains DOKS,
managed DB, R2 storage, and Cloudflare API token bootstrap secrets."
```

---

## Task 9: Reduce Terraform scope — update variables.tf

**Files:**
- Modify: `infra/production/variables.tf`

- [ ] **Step 1: Read current variables.tf**

Read `infra/production/variables.tf` to confirm current state.

- [ ] **Step 2: Rewrite variables.tf**

Replace the entire file with:

```hcl
# --- Provider credentials ---

variable "do_token" {
  description = "DigitalOcean API token"
  type        = string
  sensitive   = true
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token with DNS edit permissions"
  type        = string
  sensitive   = true
}

# --- DOKS cluster ---

variable "region" {
  description = "DigitalOcean region"
  type        = string
  default     = "sgp1"
}

variable "cluster_name" {
  description = "DOKS cluster name"
  type        = string
  default     = "sub2api"
}

variable "k8s_version" {
  description = "Kubernetes version prefix"
  type        = string
  default     = "1.34"
}

variable "node_size" {
  description = "Droplet size for worker nodes"
  type        = string
  default     = "s-2vcpu-4gb"
}

variable "min_nodes" {
  description = "Autoscaler minimum nodes"
  type        = number
  default     = 1
}

variable "max_nodes" {
  description = "Autoscaler maximum nodes"
  type        = number
  default     = 3
}

# --- Database (optional) ---

variable "enable_managed_database" {
  description = "Create a DO Managed PostgreSQL instance"
  type        = bool
  default     = false
}

variable "db_size" {
  description = "Database droplet size (only used when enable_managed_database=true)"
  type        = string
  default     = "db-s-1vcpu-1gb"
}

variable "managed_grafana_reader_user" {
  description = "Database username to create for Grafana read-only access when managed DB is enabled"
  type        = string
  default     = "grafana_reader"
}

# --- Observability storage (optional) ---

variable "enable_observability_storage" {
  description = "Create Cloudflare R2 buckets for Tempo and Loki"
  type        = bool
  default     = false
}

variable "cloudflare_account_id" {
  description = "Cloudflare account ID (required when enable_observability_storage=true)"
  type        = string
  default     = ""
}
```

Removed variables:
- `letsencrypt_email` — now in `cluster-issuer.yaml`
- `cloudflare_zone_id` — now in `external-dns.yaml` values
- `domain_suffix` — now in Flux HelmRelease values
- `cloudflare_proxied` — now in `external-dns.yaml` values
- `enable_monitoring` — monitoring managed by Flux
- `grafana_hostname_prefix` — now in `monitoring.yaml` values
- `existing_grafana_admin_password` — now in `monitoring-secrets` Secret
- `r2_access_key`, `r2_secret_key` — now in `monitoring-secrets` Secret
- `grafana_db_*` — now in `monitoring-secrets` Secret

- [ ] **Step 3: Commit**

```bash
git add infra/production/variables.tf
git commit -m "refactor(infra): remove monitoring and kubernetes variables from Terraform"
```

---

## Task 10: Reduce Terraform scope — update outputs.tf and versions.tf

**Files:**
- Modify: `infra/production/outputs.tf`
- Modify: `infra/production/versions.tf`

- [ ] **Step 1: Read current outputs.tf**

Read `infra/production/outputs.tf` to confirm current state.

- [ ] **Step 2: Rewrite outputs.tf**

Replace the entire file with:

```hcl
# --- Cluster ---

output "cluster_endpoint" {
  description = "Kubernetes API endpoint"
  value       = module.doks.endpoint
}

output "cluster_name" {
  description = "DOKS cluster name"
  value       = module.doks.name
}

output "kubeconfig_command" {
  description = "Command to configure kubectl"
  value       = "doctl kubernetes cluster kubeconfig save ${module.doks.name}"
}

# --- Database (conditional) ---

output "database_host" {
  description = "Managed PostgreSQL host (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].host : ""
}

output "database_port" {
  description = "Managed PostgreSQL port (empty if disabled)"
  value       = var.enable_managed_database ? tostring(module.database[0].port) : ""
}

output "database_user" {
  description = "Managed PostgreSQL user (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].user : ""
}

output "database_password" {
  description = "Managed PostgreSQL password (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].password : ""
  sensitive   = true
}

output "database_name" {
  description = "Managed PostgreSQL database name (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].database : ""
}

output "grafana_reader_user" {
  description = "Grafana read-only DB user from managed PostgreSQL (empty if managed DB disabled)"
  value       = var.enable_managed_database ? module.database[0].grafana_reader_user : ""
}

output "grafana_reader_password" {
  description = "Grafana read-only DB password from managed PostgreSQL (empty if managed DB disabled)"
  value       = var.enable_managed_database ? module.database[0].grafana_reader_password : ""
  sensitive   = true
}

# --- Observability storage (conditional) ---

output "r2_tempo_bucket" {
  description = "Tempo R2 bucket name (empty if disabled)"
  value       = var.enable_observability_storage ? module.storage[0].tempo_bucket : ""
}

output "r2_loki_bucket" {
  description = "Loki R2 bucket name (empty if disabled)"
  value       = var.enable_observability_storage ? module.storage[0].loki_bucket : ""
}

output "r2_s3_endpoint" {
  description = "R2 S3-compatible endpoint (empty if disabled)"
  value       = var.enable_observability_storage ? module.storage[0].s3_endpoint : ""
}
```

Removed outputs: `load_balancer_ip`, `domain_suffix`, all `grafana_*` outputs.

- [ ] **Step 3: Read current versions.tf**

Read `infra/production/versions.tf` to confirm current state.

- [ ] **Step 4: Remove Helm provider from versions.tf**

Remove the `helm` provider block from the `required_providers` section and the `helm` provider configuration. Keep `digitalocean`, `cloudflare`, and `kubernetes` providers.

- [ ] **Step 5: Commit**

```bash
git add infra/production/outputs.tf infra/production/versions.tf
git commit -m "refactor(infra): remove monitoring outputs and Helm provider"
```

---

## Task 11: Delete unused Terraform modules

**Files:**
- Delete: `infra/modules/kubernetes/` (entire directory)
- Delete: `infra/modules/monitoring/` (entire directory)

- [ ] **Step 1: Delete the kubernetes module**

```bash
rm -rf infra/modules/kubernetes
```

- [ ] **Step 2: Delete the monitoring module**

```bash
rm -rf infra/modules/monitoring
```

- [ ] **Step 3: Verify no other references exist**

```bash
grep -r "modules/kubernetes" infra/
grep -r "modules/monitoring" infra/
```

Expected: no matches (references were already removed in Task 8).

- [ ] **Step 4: Commit**

```bash
git add -A infra/modules/kubernetes infra/modules/monitoring
git commit -m "refactor(infra): delete kubernetes and monitoring Terraform modules

These are now managed by Flux in clusters/production/infrastructure/
and clusters/production/monitoring/."
```

---

## Task 12: Update DEPLOY.md

**Files:**
- Modify: `DEPLOY.md`

- [ ] **Step 1: Read current DEPLOY.md**

Read the full `DEPLOY.md` to understand the current structure.

- [ ] **Step 2: Rewrite DEPLOY.md**

Rewrite the deployment guide to cover:

1. **Ownership model** — Terraform for cloud resources, Flux for in-cluster
2. **Prerequisites** — add `flux` CLI to the list
3. **Provision Infrastructure** — Terraform section (simplified, no more staged apply for cert-manager/ingress)
4. **Bootstrap Flux** — one-time `flux bootstrap github` instructions
5. **Create Secrets** — manual kubectl commands for `monitoring-secrets` and `sub2api-secrets`
6. **Configure Flux Manifests** — update CHANGEME values in Flux YAML files
7. **Deploy** — push to Git, Flux syncs automatically
8. **Application Updates** — edit image tags in `sub2api.yaml`, commit, push
9. **Infrastructure Updates** — bump chart versions in HelmRelease files
10. **Rollback** — `git revert` workflow
11. **Terraform State Migration** — `terraform state rm` commands for existing deployments
12. **Monitoring Status** — `flux get` commands
13. **Troubleshooting** — Flux-specific issues (suspended reconciliation, failed HelmRelease, source errors)

Key sections to include:

```markdown
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
```

```markdown
## Bootstrap Flux

One-time setup. Install the Flux CLI, then bootstrap:

\```bash
# Install Flux CLI
brew install fluxcd/tap/flux

# Bootstrap Flux into the cluster
# This creates the flux-system namespace, installs controllers,
# and creates a GitRepository pointing at this repo.
flux bootstrap github \
  --owner=<github-org-or-user> \
  --repository=sub2api \
  --branch=main \
  --path=clusters/production \
  --personal
\```

Flux will:
1. Install its controllers in `flux-system` namespace
2. Create a GitRepository source pointing at this repo
3. Start reconciling Kustomizations found in `clusters/production/`
4. Apply infrastructure -> monitoring -> apps in dependency order
```

```markdown
## Deploying a New Version

1. Tag and push to trigger image builds:
   \```bash
   git tag v0.3.0
   git push origin v0.3.0
   \```

2. Update image tags in `clusters/production/apps/sub2api.yaml`:
   \```yaml
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
   \```

3. Commit and push:
   \```bash
   git add clusters/production/apps/sub2api.yaml
   git commit -m "deploy: v0.3.0"
   git push
   \```

4. Flux syncs automatically within 1 minute.
```

```markdown
## Migrating from Terraform-Managed In-Cluster Resources

If you previously used Terraform to manage ingress-nginx, cert-manager,
ExternalDNS, and/or the monitoring stack, remove them from Terraform state
after Flux has adopted them:

\```bash
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
\```
```

- [ ] **Step 3: Commit**

```bash
git add DEPLOY.md
git commit -m "docs: rewrite DEPLOY.md for Flux GitOps workflow"
```

---

## Task 13: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Read current CLAUDE.md**

Read `CLAUDE.md` to confirm the deployment-related sections.

- [ ] **Step 2: Update deployment sections**

Update the following sections in `CLAUDE.md`:

- **Architecture** section: Update the deployment ownership description
- **Infrastructure (`infra/`)** section: Remove references to `modules/kubernetes/` and `modules/monitoring/`; add `clusters/production/` directory layout description
- **Deployment (`deploy/helm/sub2api/`)** section: Update the "Quick Deploy/Upgrade" to reference the Flux git-commit workflow instead of `helm upgrade`
- **Key Deployment Notes**: Update to reflect Flux ownership

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for Flux GitOps architecture"
```

---

## Task 14: Validate Terraform changes

**Files:** None (validation only)

- [ ] **Step 1: Run terraform fmt**

```bash
cd infra/production && terraform fmt -recursive ..
```

Expected: no files changed (or minor formatting fixes).

- [ ] **Step 2: Run terraform validate**

```bash
cd infra/production && terraform validate
```

Expected: `Success! The configuration is valid.`

If validation fails due to missing variable references or provider issues, fix the errors and re-run.

- [ ] **Step 3: Commit any formatting fixes**

```bash
git add infra/
git commit -m "style(infra): terraform fmt"
```

(Skip if no changes.)

---

## Task 15: Validate Flux manifests

**Files:** None (validation only)

- [ ] **Step 1: Install Flux CLI if not present**

```bash
brew install fluxcd/tap/flux 2>/dev/null || flux --version
```

- [ ] **Step 2: Validate Kustomization files**

```bash
cd clusters/production
kustomize build infrastructure/ > /dev/null && echo "infrastructure: OK"
kustomize build monitoring/ > /dev/null && echo "monitoring: OK"
kustomize build apps/ > /dev/null && echo "apps: OK"
```

Expected: all three print "OK".

Note: `kustomize build` validates the YAML structure. The Flux CRD-specific validation happens at apply time, but structural issues are caught here.

- [ ] **Step 3: Validate YAML syntax**

```bash
find clusters/production -name '*.yaml' -exec sh -c 'echo "Checking $1" && python3 -c "import yaml; yaml.safe_load(open(\"$1\"))"' _ {} \;
```

Expected: no errors.

- [ ] **Step 4: Commit if any fixes were needed**

(Skip if no changes.)
