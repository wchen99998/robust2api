# Terraform DOKS Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provision a DigitalOcean Kubernetes cluster with autoscaling, optional managed PostgreSQL, Cloudflare DNS, and in-cluster bootstrap (ingress-nginx, cert-manager) using Terraform modules.

**Architecture:** Four Terraform modules (`doks`, `kubernetes`, `database`, `dns`) composed by a production environment root. Each module is self-contained with its own variables, outputs, and resources. The production root wires them together and exposes a single set of inputs/outputs.

**Tech Stack:** Terraform (~1.7+), DigitalOcean provider, Cloudflare provider, Kubernetes provider, Helm provider.

**Spec:** `infra/specs/2026-04-05-terraform-doks-infra-design.md`

---

### Task 1: Project scaffolding and gitignore

**Files:**
- Modify: `.gitignore`
- Create: `infra/README.md`

- [ ] **Step 1: Add Terraform patterns to .gitignore**

Add these lines to the end of `.gitignore`:

```gitignore
# ===================
# Terraform
# ===================
**/.terraform/
*.tfstate
*.tfstate.*
*.tfvars
!*.tfvars.example
crash.log
override.tf
override.tf.json
*_override.tf
*_override.tf.json
```

- [ ] **Step 2: Create infra README**

Create `infra/README.md`:

```markdown
# Sub2API Infrastructure

Terraform modules for provisioning DigitalOcean Kubernetes infrastructure.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.7
- [doctl](https://docs.digitalocean.com/reference/doctl/how-to/install/) (DigitalOcean CLI)
- A DigitalOcean API token
- A Cloudflare API token with DNS edit permissions for your zone

## Quick Start

```bash
cd production/
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your tokens and settings
terraform init
terraform plan
terraform apply
```

## After Apply

Configure kubectl:

```bash
doctl kubernetes cluster kubeconfig save sub2api
```

Deploy Sub2API via Helm:

```bash
helm install sub2api ../../deploy/helm/sub2api \
  -n sub2api \
  -f ../../deploy/helm/sub2api/values-production.yaml \
  --set secrets.jwtSecret=<value> \
  --set secrets.totpEncryptionKey=<value> \
  --set secrets.adminPassword=<value>
```

## Modules

| Module | Description |
|--------|-------------|
| `modules/doks` | DOKS cluster with autoscaling node pool |
| `modules/kubernetes` | In-cluster bootstrap: ingress-nginx, cert-manager, namespace |
| `modules/database` | Optional DO Managed PostgreSQL |
| `modules/dns` | Cloudflare DNS A record |

## Environments

| Directory | Description |
|-----------|-------------|
| `production/` | Production environment root |
```

- [ ] **Step 3: Commit**

```bash
git add .gitignore infra/README.md
git commit -m "infra: scaffold Terraform project structure and gitignore"
```

---

### Task 2: DOKS module

**Files:**
- Create: `infra/modules/doks/main.tf`
- Create: `infra/modules/doks/variables.tf`
- Create: `infra/modules/doks/outputs.tf`

- [ ] **Step 1: Create variables.tf**

Create `infra/modules/doks/variables.tf`:

```hcl
variable "cluster_name" {
  description = "Name of the DOKS cluster"
  type        = string
  default     = "sub2api"
}

variable "region" {
  description = "DigitalOcean region"
  type        = string
  default     = "sgp1"
}

variable "k8s_version" {
  description = "Kubernetes version prefix (latest patch auto-selected)"
  type        = string
  default     = "1.31"
}

variable "node_size" {
  description = "Droplet size for worker nodes"
  type        = string
  default     = "s-2vcpu-4gb"
}

variable "min_nodes" {
  description = "Minimum number of nodes in the autoscaling pool"
  type        = number
  default     = 1
}

variable "max_nodes" {
  description = "Maximum number of nodes in the autoscaling pool"
  type        = number
  default     = 3
}

variable "auto_upgrade" {
  description = "Enable automatic patch version upgrades"
  type        = bool
  default     = true
}

variable "surge_upgrade" {
  description = "Enable surge upgrades for zero-downtime node upgrades"
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = list(string)
  default     = ["sub2api", "terraform"]
}
```

- [ ] **Step 2: Create main.tf**

Create `infra/modules/doks/main.tf`:

```hcl
data "digitalocean_kubernetes_versions" "this" {
  version_prefix = "${var.k8s_version}."
}

resource "digitalocean_kubernetes_cluster" "this" {
  name          = var.cluster_name
  region        = var.region
  version       = data.digitalocean_kubernetes_versions.this.latest_version
  auto_upgrade  = var.auto_upgrade
  surge_upgrade = var.surge_upgrade
  tags          = var.tags

  node_pool {
    name       = "${var.cluster_name}-default"
    size       = var.node_size
    auto_scale = true
    min_nodes  = var.min_nodes
    max_nodes  = var.max_nodes
    tags       = var.tags
  }
}
```

- [ ] **Step 3: Create outputs.tf**

Create `infra/modules/doks/outputs.tf`:

```hcl
output "cluster_id" {
  description = "ID of the DOKS cluster"
  value       = digitalocean_kubernetes_cluster.this.id
}

output "cluster_urn" {
  description = "URN of the DOKS cluster"
  value       = digitalocean_kubernetes_cluster.this.urn
}

output "endpoint" {
  description = "Kubernetes API endpoint"
  value       = digitalocean_kubernetes_cluster.this.endpoint
}

output "kubeconfig" {
  description = "Raw kubeconfig for the cluster"
  value       = digitalocean_kubernetes_cluster.this.kube_config[0]
  sensitive   = true
}

output "cluster_ca_certificate" {
  description = "CA certificate for the cluster"
  value       = base64decode(digitalocean_kubernetes_cluster.this.kube_config[0].cluster_ca_certificate)
  sensitive   = true
}

output "token" {
  description = "Authentication token for the cluster"
  value       = digitalocean_kubernetes_cluster.this.kube_config[0].token
  sensitive   = true
}

output "name" {
  description = "Name of the cluster"
  value       = digitalocean_kubernetes_cluster.this.name
}
```

- [ ] **Step 4: Commit**

```bash
git add infra/modules/doks/
git commit -m "infra: add DOKS cluster module with autoscaling"
```

---

### Task 3: Kubernetes bootstrap module

**Files:**
- Create: `infra/modules/kubernetes/main.tf`
- Create: `infra/modules/kubernetes/variables.tf`
- Create: `infra/modules/kubernetes/outputs.tf`

- [ ] **Step 1: Create variables.tf**

Create `infra/modules/kubernetes/variables.tf`:

```hcl
variable "ingress_nginx_version" {
  description = "ingress-nginx Helm chart version"
  type        = string
  default     = "4.12.1"
}

variable "cert_manager_version" {
  description = "cert-manager Helm chart version"
  type        = string
  default     = "1.17.1"
}

variable "letsencrypt_email" {
  description = "Email for Let's Encrypt certificate notifications"
  type        = string
}

variable "app_namespace" {
  description = "Namespace to create for the application"
  type        = string
  default     = "sub2api"
}
```

- [ ] **Step 2: Create main.tf**

Create `infra/modules/kubernetes/main.tf`:

```hcl
resource "kubernetes_namespace" "app" {
  metadata {
    name = var.app_namespace
  }
}

# --- ingress-nginx ---

resource "helm_release" "ingress_nginx" {
  name             = "ingress-nginx"
  repository       = "https://kubernetes.github.io/ingress-nginx"
  chart            = "ingress-nginx"
  version          = var.ingress_nginx_version
  namespace        = "ingress-nginx"
  create_namespace = true
  wait             = true
  timeout          = 600

  set {
    name  = "controller.service.externalTrafficPolicy"
    value = "Local"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/do-loadbalancer-name"
    value = "sub2api-lb"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/do-loadbalancer-protocol"
    value = "http"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/do-loadbalancer-tls-ports"
    value = "443"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/do-loadbalancer-certificate-id"
    value = ""
  }
}

# --- cert-manager ---

resource "helm_release" "cert_manager" {
  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  version          = var.cert_manager_version
  namespace        = "cert-manager"
  create_namespace = true
  wait             = true
  timeout          = 600

  set {
    name  = "crds.enabled"
    value = "true"
  }
}

resource "kubernetes_manifest" "letsencrypt_issuer" {
  manifest = {
    apiVersion = "cert-manager.io/v1"
    kind       = "ClusterIssuer"
    metadata = {
      name = "letsencrypt-prod"
    }
    spec = {
      acme = {
        server = "https://acme-v02.api.letsencrypt.org/directory"
        email  = var.letsencrypt_email
        privateKeySecretRef = {
          name = "letsencrypt-prod"
        }
        solvers = [{
          http01 = {
            ingress = {
              class = "nginx"
            }
          }
        }]
      }
    }
  }

  depends_on = [helm_release.cert_manager]
}

# --- Load balancer IP lookup ---

data "kubernetes_service" "ingress_nginx" {
  metadata {
    name      = "ingress-nginx-controller"
    namespace = "ingress-nginx"
  }

  depends_on = [helm_release.ingress_nginx]
}
```

- [ ] **Step 3: Create outputs.tf**

Create `infra/modules/kubernetes/outputs.tf`:

```hcl
output "load_balancer_ip" {
  description = "External IP of the ingress-nginx load balancer"
  value       = data.kubernetes_service.ingress_nginx.status[0].load_balancer[0].ingress[0].ip
}

output "app_namespace" {
  description = "Name of the application namespace"
  value       = kubernetes_namespace.app.metadata[0].name
}
```

- [ ] **Step 4: Commit**

```bash
git add infra/modules/kubernetes/
git commit -m "infra: add kubernetes bootstrap module (ingress-nginx, cert-manager)"
```

---

### Task 4: Database module (optional)

**Files:**
- Create: `infra/modules/database/main.tf`
- Create: `infra/modules/database/variables.tf`
- Create: `infra/modules/database/outputs.tf`

- [ ] **Step 1: Create variables.tf**

Create `infra/modules/database/variables.tf`:

```hcl
variable "cluster_name" {
  description = "Name prefix for the database cluster"
  type        = string
  default     = "sub2api"
}

variable "region" {
  description = "DigitalOcean region"
  type        = string
}

variable "db_size" {
  description = "Database droplet size"
  type        = string
  default     = "db-s-1vcpu-1gb"
}

variable "db_engine_version" {
  description = "PostgreSQL major version"
  type        = string
  default     = "16"
}

variable "db_name" {
  description = "Name of the database to create"
  type        = string
  default     = "sub2api"
}

variable "db_user" {
  description = "Name of the database user to create"
  type        = string
  default     = "sub2api"
}

variable "doks_cluster_id" {
  description = "DOKS cluster ID for firewall rules (restrict DB access to cluster)"
  type        = string
}

variable "tags" {
  description = "Tags to apply to database resources"
  type        = list(string)
  default     = ["sub2api", "terraform"]
}
```

- [ ] **Step 2: Create main.tf**

Create `infra/modules/database/main.tf`:

```hcl
resource "digitalocean_database_cluster" "postgres" {
  name       = "${var.cluster_name}-pg"
  engine     = "pg"
  version    = var.db_engine_version
  size       = var.db_size
  region     = var.region
  node_count = 1
  tags       = var.tags
}

resource "digitalocean_database_db" "app" {
  cluster_id = digitalocean_database_cluster.postgres.id
  name       = var.db_name
}

resource "digitalocean_database_user" "app" {
  cluster_id = digitalocean_database_cluster.postgres.id
  name       = var.db_user
}

resource "digitalocean_database_firewall" "app" {
  cluster_id = digitalocean_database_cluster.postgres.id

  rule {
    type  = "k8s"
    value = var.doks_cluster_id
  }
}
```

- [ ] **Step 3: Create outputs.tf**

Create `infra/modules/database/outputs.tf`:

```hcl
output "host" {
  description = "Database host"
  value       = digitalocean_database_cluster.postgres.host
}

output "port" {
  description = "Database port"
  value       = digitalocean_database_cluster.postgres.port
}

output "user" {
  description = "Database user"
  value       = digitalocean_database_user.app.name
}

output "password" {
  description = "Database password"
  value       = digitalocean_database_user.app.password
  sensitive   = true
}

output "database" {
  description = "Database name"
  value       = digitalocean_database_db.app.name
}

output "sslmode" {
  description = "SSL mode for database connections"
  value       = "require"
}

output "uri" {
  description = "Full connection URI"
  value       = digitalocean_database_cluster.postgres.uri
  sensitive   = true
}
```

- [ ] **Step 4: Commit**

```bash
git add infra/modules/database/
git commit -m "infra: add optional managed PostgreSQL module"
```

---

### Task 5: DNS module (Cloudflare)

**Files:**
- Create: `infra/modules/dns/main.tf`
- Create: `infra/modules/dns/variables.tf`
- Create: `infra/modules/dns/outputs.tf`

- [ ] **Step 1: Create variables.tf**

Create `infra/modules/dns/variables.tf`:

```hcl
variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID"
  type        = string
}

variable "record_name" {
  description = "DNS record name (e.g. 'api' or 'sub2api')"
  type        = string
}

variable "record_value" {
  description = "IP address to point the record to (load balancer IP)"
  type        = string
}

variable "proxied" {
  description = "Enable Cloudflare proxy (CDN/WAF)"
  type        = bool
  default     = true
}
```

- [ ] **Step 2: Create main.tf**

Create `infra/modules/dns/main.tf`:

```hcl
resource "cloudflare_record" "app" {
  zone_id = var.cloudflare_zone_id
  name    = var.record_name
  content = var.record_value
  type    = "A"
  proxied = var.proxied
  ttl     = var.proxied ? 1 : 300 # Auto TTL when proxied
}
```

- [ ] **Step 3: Create outputs.tf**

Create `infra/modules/dns/outputs.tf`:

```hcl
output "fqdn" {
  description = "Fully qualified domain name of the record"
  value       = cloudflare_record.app.hostname
}

output "record_id" {
  description = "Cloudflare DNS record ID"
  value       = cloudflare_record.app.id
}
```

- [ ] **Step 4: Commit**

```bash
git add infra/modules/dns/
git commit -m "infra: add Cloudflare DNS module"
```

---

### Task 6: Production environment root

**Files:**
- Create: `infra/production/versions.tf`
- Create: `infra/production/variables.tf`
- Create: `infra/production/main.tf`
- Create: `infra/production/outputs.tf`
- Create: `infra/production/terraform.tfvars.example`

- [ ] **Step 1: Create versions.tf**

Create `infra/production/versions.tf`:

```hcl
terraform {
  required_version = ">= 1.7"

  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.43"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.35"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.17"
    }
  }
}
```

- [ ] **Step 2: Create variables.tf**

Create `infra/production/variables.tf`:

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
  default     = "1.31"
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

# --- Kubernetes bootstrap ---

variable "letsencrypt_email" {
  description = "Email for Let's Encrypt certificate notifications"
  type        = string
}

# --- DNS ---

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for the domain"
  type        = string
}

variable "domain_name" {
  description = "DNS record name (e.g. 'api' for api.yourdomain.com)"
  type        = string
}

variable "cloudflare_proxied" {
  description = "Enable Cloudflare proxy (CDN/WAF)"
  type        = bool
  default     = true
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
```

- [ ] **Step 3: Create main.tf**

Create `infra/production/main.tf`:

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
  token                  = module.doks.token
  cluster_ca_certificate = module.doks.cluster_ca_certificate
}

provider "helm" {
  kubernetes {
    host                   = module.doks.endpoint
    token                  = module.doks.token
    cluster_ca_certificate = module.doks.cluster_ca_certificate
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

module "kubernetes" {
  source = "../modules/kubernetes"

  letsencrypt_email = var.letsencrypt_email

  depends_on = [module.doks]
}

module "dns" {
  source = "../modules/dns"

  cloudflare_zone_id = var.cloudflare_zone_id
  record_name        = var.domain_name
  record_value       = module.kubernetes.load_balancer_ip
  proxied            = var.cloudflare_proxied
}

module "database" {
  source = "../modules/database"
  count  = var.enable_managed_database ? 1 : 0

  cluster_name    = var.cluster_name
  region          = var.region
  db_size         = var.db_size
  doks_cluster_id = module.doks.cluster_id
}
```

- [ ] **Step 4: Create outputs.tf**

Create `infra/production/outputs.tf`:

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

# --- Networking ---

output "load_balancer_ip" {
  description = "Ingress load balancer IP"
  value       = module.kubernetes.load_balancer_ip
}

output "app_fqdn" {
  description = "Application FQDN"
  value       = module.dns.fqdn
}

# --- Database (conditional) ---

output "database_host" {
  description = "Managed PostgreSQL host (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].host : ""
}

output "database_port" {
  description = "Managed PostgreSQL port (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].port : ""
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
```

- [ ] **Step 5: Create terraform.tfvars.example**

Create `infra/production/terraform.tfvars.example`:

```hcl
# DigitalOcean
do_token     = "dop_v1_your_token_here"
region       = "sgp1"
cluster_name = "sub2api"
k8s_version  = "1.31"
node_size    = "s-2vcpu-4gb"
min_nodes    = 1
max_nodes    = 3

# Kubernetes bootstrap
letsencrypt_email = "admin@yourdomain.com"

# Cloudflare DNS
cloudflare_api_token = "your_cloudflare_api_token_here"
cloudflare_zone_id   = "your_zone_id_here"
domain_name          = "api"
cloudflare_proxied   = true

# Managed PostgreSQL (optional, default false)
enable_managed_database = false
# db_size               = "db-s-1vcpu-1gb"
```

- [ ] **Step 6: Commit**

```bash
git add infra/production/
git commit -m "infra: add production environment root composing all modules"
```

---

### Task 7: Validate and format

- [ ] **Step 1: Run terraform fmt**

```bash
cd infra/production && terraform fmt -recursive ..
```

Expected: files are formatted (or no changes needed).

- [ ] **Step 2: Run terraform init**

```bash
cd infra/production && terraform init
```

Expected: providers downloaded, `.terraform.lock.hcl` created. Do NOT commit `.terraform/` directory.

- [ ] **Step 3: Run terraform validate**

```bash
cd infra/production && terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 4: Fix any validation errors**

If `terraform validate` reports errors, fix them in the relevant module files and re-run validate until it passes.

- [ ] **Step 5: Commit lock file**

```bash
git add infra/production/.terraform.lock.hcl
git commit -m "infra: add terraform lock file"
```

---

### Task 8: Final review

- [ ] **Step 1: Verify file structure**

```bash
find infra -type f | sort
```

Expected output:

```
infra/README.md
infra/modules/database/main.tf
infra/modules/database/outputs.tf
infra/modules/database/variables.tf
infra/modules/doks/main.tf
infra/modules/doks/outputs.tf
infra/modules/doks/variables.tf
infra/modules/dns/main.tf
infra/modules/dns/outputs.tf
infra/modules/dns/variables.tf
infra/modules/kubernetes/main.tf
infra/modules/kubernetes/outputs.tf
infra/modules/kubernetes/variables.tf
infra/production/.terraform.lock.hcl
infra/production/main.tf
infra/production/outputs.tf
infra/production/terraform.tfvars.example
infra/production/variables.tf
infra/production/versions.tf
infra/specs/2026-04-05-terraform-doks-infra-design.md
infra/specs/2026-04-05-terraform-doks-infra-plan.md
```

- [ ] **Step 2: Verify terraform plan runs (dry run)**

This requires actual credentials. If you have them:

```bash
cd infra/production
cp terraform.tfvars.example terraform.tfvars
# Fill in real values
terraform plan
```

If you don't have credentials yet, this step is deferred to first real deployment.
